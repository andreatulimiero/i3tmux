package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"log/syslog"
	"os"
	"os/exec"
	"os/user"
	"path"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"go.i3wm.org/i3/v4"
)

const (
	GROUP_SESS_DELIM = "_"
	HOST_DELIM       = "@"
)

var (
	terminalBinFlag  = flag.String("terminal", "", "the binary path of the terminal to use")
	terminalNameFlag = flag.String("nameFlag", "", "the flag used by the terminal of choice"+
		"to define the window instance name")
	hostFlag     = flag.String("host", "", "remote host where tmux server runs")
	newMode      = flag.String("new", "", "create new session group")
	addMode      = flag.Bool("add", false, "add window to current session group")
	detachMode   = flag.Bool("detach", false, "detach current session group")
	resumeMode   = flag.String("resume", "", "resume session group")
	listMode     = flag.Bool("list", false, "list sessions groups")
	serverMode   = flag.Bool("server", false, "react to closing session windows")
	killMode     = flag.Bool("kill", false, "kill current session locally and remotely")
	sessionFmtRe = regexp.MustCompile(`^[a-zA-Z]*(\d+)$`)

	TmuxNoSessionsError = errors.New("tmux no sessions")
	UnknownRemoteError  = errors.New("unknown remote error")
)

type SessionsPerGroup map[string]map[string]bool

func init() {
	logwriter, err := syslog.New(syslog.LOG_INFO, "i3tmux")
	if err != nil {
		log.Fatal(err)
	}
	log.SetOutput(logwriter)
	// Set logger to use syslog

	user, err := user.Current()
	if err != nil {
		log.Fatal(err)
	}
	workingDir := path.Join(user.HomeDir, ".local", "share", "i3tmux")
	err = os.Chdir(workingDir)
	if err != nil {
		if os.IsNotExist(err) {
			err = os.MkdirAll(workingDir, 0755)
			if err != nil {
				log.Fatal(err)
			}
			log.Println("Created " + workingDir + " directory")
		} else {
			log.Fatal(err)
		}
	}
	// Chdir to ~/.config/i3tmux/
}

func serializeGroupSess(group string, session string) string {
	return fmt.Sprintf("%s%s%s", group, GROUP_SESS_DELIM, session)
}

func serializeHostGroupSess(host, group, session string) string {
	return fmt.Sprintf("%s@%s", serializeGroupSess(group, session), host)
}

func deserializeHostGroupSessFromString(s string) (string, string, string, error) {
	split := strings.Split(s, HOST_DELIM)
	if len(split) != 2 {
		return "", "", "", fmt.Errorf("name not in GROUP%sSESSION%sHOST format: %s",
			GROUP_SESS_DELIM,
			HOST_DELIM,
			s)
	}
	host := split[1]
	group, sess, err := deserializeGroupSessFromString(split[0])
	if err != nil {
		return "", "", "", err
	}
	return host, group, sess, nil
}

func deserializeGroupSessFromString(s string) (string, string, error) {
	split := strings.Split(s, GROUP_SESS_DELIM)
	if len(split) != 2 {
		return "", "", fmt.Errorf("name not in GROUP%sSESSION format: %s",
			GROUP_SESS_DELIM,
			s)
	}
	return split[0], split[1], nil
}

func deserializeHostGroupSessFromCon(con *i3.Node) (string, string, string, error) {
	return deserializeHostGroupSessFromString(con.WindowProperties.Instance)
}

func deserializeGroupSessFromCon(con *i3.Node) (string, string, error) {
	return deserializeGroupSessFromString(con.WindowProperties.Instance)
}

func fetchSessionsPerGroup(host string) (SessionsPerGroup, error) {
	cmd := exec.Command("ssh", host, `tmux ls -F "#{session_name}"`)
	out, err := cmd.CombinedOutput()
	// For simplicity's sake we assume that if the command succeeds
	// stderr messages do not pollute stdout
	outStr := string(out)
	if err != nil {
		if strings.HasPrefix(outStr, "no server running on ") {
			return nil, TmuxNoSessionsError
		}
		return nil, err
	}
	lines := strings.Split(outStr, "\n")
	sessionsPerGroup := make(map[string]map[string]bool)
	for _, l := range lines {
		group, session, err := deserializeGroupSessFromString(l)
		if err != nil {
			// Skip unrecognized format
			continue
		}
		if _, ok := sessionsPerGroup[group]; !ok {
			sessionsPerGroup[group] = make(map[string]bool)
		}
		sessionsPerGroup[group][session] = true
	}
	return sessionsPerGroup, nil
}

func getNextSessIdx(sessionsPerGroup SessionsPerGroup, group string) (int, error) {
	sessions := sessionsPerGroup[group]
	var idxs []int
	for s, _ := range sessions {
		res := sessionFmtRe.FindStringSubmatch(s)
		if len(res) != 2 {
			return -1, fmt.Errorf("malformed session '%s'", s)
		}
		i, err := strconv.Atoi(res[1])
		if err != nil {
			return -1, err
		}
		idxs = append(idxs, i)
	}
	sort.Ints(idxs)
	for i, idx := range idxs {
		if i < idx {
			return i, nil
		}
	}
	return len(sessions), nil
}

func launchTermForSession(host, group, session, terminalBin, nameFlag string) error {
	sshCmd := fmt.Sprintf("ssh -t %s tmux attach -t %s", host, serializeGroupSess(group, session))
	log.Println(sshCmd)
	i3cmd := fmt.Sprintf("exec %s %s '%s' %s",
		terminalBin,
		nameFlag,
		serializeHostGroupSess(host, group, session),
		sshCmd)
	_, err := i3.RunCommand(i3cmd)
	return err
}

func addWindow(terminalBin, nameFlag string) error {
	// TODO: Add swallow container first to inform user operation is being performed?
	tree, err := i3.GetTree()
	if err != nil {
		return err
	}
	con, err := getFocusedCon(&tree)
	if err != nil {
		return err
	}
	host, group, _, err := deserializeHostGroupSessFromCon(con)
	if err != nil {
		return err
	}
	sessionsPerGroup, err := fetchSessionsPerGroup(host)
	if err != nil {
		return err
	}

	nextSessIdx, err := getNextSessIdx(sessionsPerGroup, group)
	if err != nil {
		return err
	}
	nextSess := fmt.Sprintf("session%d", nextSessIdx)
	log.Println("Adding session to group", group, nextSess)
	err = exec.Command("ssh",
		host,
		"tmux new -d -s "+serializeGroupSess(group, nextSess)).Run()
	if err != nil {
		return UnknownRemoteError
	}
	// Add new session remotely

	err = launchTermForSession(host, group, nextSess, terminalBin, nameFlag)
	if err != nil {
		return err
	}
	return nil
}

func getFocusedWs(tree *i3.Tree) (*i3.Node, error) {
	ws := tree.Root.FindFocused(func(n *i3.Node) bool {
		return n.Type == i3.WorkspaceNode
	})
	if ws == nil {
		return nil, fmt.Errorf("could not locate focused workspace")
	}
	return ws, nil
}

func getFocusedCon(tree *i3.Tree) (*i3.Node, error) {
	var con *i3.Node
	tree.Root.FindFocused(func(n *i3.Node) bool {
		con = n
		return false
	})
	if con == nil {
		return nil, fmt.Errorf("could not locate focused container")
	}
	return con, nil
}

func nodeIsLeaf(n *i3.Node) bool {
	return n.Type == i3.Con && len(n.Nodes) == 0
}

func getTreeOfGroupSess(u *i3.Node) map[string]interface{} {
	if nodeIsLeaf(u) {
		host, group, session, err := deserializeHostGroupSessFromCon(u)
		if err != nil {
			// We care about tmux session leaves only
			return nil
		}
		m := make(map[string]interface{})
		m["type"] = i3.Con
		m["swallows"] = []map[string]string{{"instance": serializeHostGroupSess(host, group, session)}}
		return m
	} else {
		var nodes []map[string]interface{}
		for _, v := range u.Nodes {
			sessionNodes := getTreeOfGroupSess(v)
			if sessionNodes == nil {
				continue
			}
			nodes = append(nodes, sessionNodes)
		}
		switch len(nodes) {
		case 0:
			// No child contains a session, skip this
			return nil
		case 1:
			// Optimize out self and return the only child
			// TODO: Make this an option. If optimization is not done it should
			//       be easier to recreate an entire workspace layout with other
			//       applications (e.g., browser)
			return nodes[0]
		default:
			m := make(map[string]interface{})
			m["layout"] = u.Layout
			m["type"] = i3.Con
			m["percent"] = u.Percent
			m["nodes"] = nodes
			return m
		}
	}
}

func closeGroupSessWindows(u *i3.Node, group *string) error {
	for _, v := range u.Nodes {
		err := closeGroupSessWindows(v, group)
		if err != nil {
			return err
		}
	}
	_, g, _, err := deserializeHostGroupSessFromCon(u)
	if err != nil || g != *group {
		return nil
		// Just skip container since not targeted
	}
	_, err = i3.RunCommand(fmt.Sprintf("[con_id=%d] kill", u.ID))
	if err != nil {
		return err
	}
	return nil
}

func detachSessionGroup() error {
	tree, err := i3.GetTree()
	if err != nil {
		return err
	}
	con, err := getFocusedCon(&tree)
	if err != nil {
		return err
	}
	host, group, _, err := deserializeHostGroupSessFromCon(con)
	if err != nil {
		return err
	}
	ws, err := getFocusedWs(&tree)
	if err != nil {
		return err
	}
	groupSessLayout := getTreeOfGroupSess(ws)
	j, err := json.Marshal(groupSessLayout)
	if err != nil {
		return err
	}
	err = ioutil.WriteFile(group+".json", j, 0644)
	if err != nil {
		return err
	}
	log.Printf("Saved layout for %s@%s", group, host)
	err = closeGroupSessWindows(ws, &group)
	if err != nil {
		log.Fatal(err)
	}
	return nil
}

func resumeSessionGroup(host, terminalBin, nameFlag string) error {
	sessionsPerGroup, err := fetchSessionsPerGroup(host)
	if err != nil {
		return err
	}
	sessions, ok := sessionsPerGroup[*resumeMode]
	if !ok {
		return fmt.Errorf("group '%s' not found", *resumeMode)
	}

	resumeLayoutPath := *resumeMode + ".json"
	_, err = os.Stat(resumeLayoutPath)
	if err != nil {
		if !os.IsNotExist(err) {
			log.Fatal(err)
		}
		// If error is not expected exit
	} else {
		cwd, err := os.Getwd()
		if err != nil {
			return err
		}
		absResumeLayoutPath := path.Join(cwd, resumeLayoutPath)
		_, err = i3.RunCommand(fmt.Sprintf("append_layout %s", absResumeLayoutPath))
		if err != nil {
			log.Fatal(err)
		}
	}
	// Try to load a layout for the target sessions group

	for s, _ := range sessions {
		err := launchTermForSession(host, *resumeMode, s, terminalBin, nameFlag)
		if err != nil {
			log.Fatal(err)
		}
	}
	return nil
}

func listSessionsGroup(host string) error {
	fmt.Println("Retrieving available sessions groups ...")
	sessionsPerGroup, err := fetchSessionsPerGroup(host)
	if err != nil {
		return err
	}
	if len(sessionsPerGroup) == 0 {
		fmt.Println("No available session")
	} else {
		for g, sessions := range sessionsPerGroup {
			fmt.Println(g + ":")
			for s, _ := range sessions {
				fmt.Printf("- %s\n", s)
			}
		}
	}
	return nil
}

func startServer() error {
	// TODO: Invalidate/update old layout when window is closed
	recv := i3.Subscribe(i3.WindowEventType)
	for recv.Next() {
		ev := recv.Event().(*i3.WindowEvent)
		if ev.Change == "close" {
			host, group, session, err := deserializeHostGroupSessFromCon(&ev.Container)
			if err != nil {
				continue
				// Not a container of interest
			}
			cmd := exec.Command("ssh", host, "tmux kill-session -t "+serializeGroupSess(group, session))
			_, err = cmd.Output()
			if err != nil {
				return fmt.Errorf("error killing session %s: %s", serializeGroupSess(group, session), err)
			}
			log.Println("Closed session", serializeGroupSess(group, session))
		}
	}
	return recv.Close()
}

func createSessionGroup(groupName, host string) error {
	if strings.Contains(groupName, GROUP_SESS_DELIM) {
		return fmt.Errorf("group name cannot contain '%s'", GROUP_SESS_DELIM)
	}
	sessionsPerGroup, err := fetchSessionsPerGroup(host)
	if err != nil && !errors.Is(err, TmuxNoSessionsError) {
		return fmt.Errorf("error fetching sessions: %s", err)
	}
	if _, ok := sessionsPerGroup[groupName]; ok {
		return fmt.Errorf("already exists")
	}
	groupSessName := fmt.Sprintf("%s%s%s", groupName, GROUP_SESS_DELIM, "session0")
	cmd := exec.Command("ssh", host, fmt.Sprintf("tmux new -d -s %s", groupSessName))
	out, err := cmd.CombinedOutput()
	// For simplicity's sake we assume that if the command succeeds
	// stderr messages do not pollute stdout
	if err != nil {
		return fmt.Errorf("%s, %w", string(out), err)
	}
	return nil
}

func killSession() error {
	tree, err := i3.GetTree()
	if err != nil {
		return err
	}
	con, err := getFocusedCon(&tree)
	if err != nil {
		return err
	}
	host, group, session, err := deserializeHostGroupSessFromCon(con)
	if err != nil {
		return err
	}
	cmd := exec.Command("ssh", host, "tmux kill-session -t "+serializeGroupSess(group, session))
	_, err = cmd.Output()
	if err != nil {
		return fmt.Errorf("error killing session %s: %s", serializeGroupSess(group, session), err)
	}
	log.Println("Closed session", serializeGroupSess(group, session))
	return nil
}

func main() {
	flag.Parse()

	if *hostFlag == "" && (*newMode != "" || *resumeMode != "" || *listMode) {
		log.Fatal(fmt.Errorf("You must specify the target host"))
	}

	modsCount := 0
	if *newMode != "" {
		modsCount++
	}
	if *addMode {
		modsCount++
	}
	if *detachMode {
		modsCount++
	}
	if *resumeMode != "" {
		modsCount++
	}
	if *listMode {
		modsCount++
	}
	if *serverMode {
		modsCount++
	}
	if *killMode {
		modsCount++
	}
	if modsCount != 1 {
		log.Fatal(fmt.Errorf("You must specify one mode among 'new', 'add', 'detach', 'resume', 'server' and 'kill'"))
	}
	// Ensure only one mode is selected

	if *newMode != "" {
		if err := createSessionGroup(*newMode, *hostFlag); err != nil {
			log.Fatal(fmt.Errorf("Error creating new sessions group: %w", err))
		}
	}
	if *addMode {
		if err := addWindow(*terminalBinFlag, *terminalNameFlag); err != nil {
			log.Fatal(fmt.Errorf("Error adding window: %w", err))
		}
	}
	if *detachMode {
		if err := detachSessionGroup(); err != nil {
			log.Fatal(fmt.Errorf("Error detaching group: %w", err))
		}
	}
	if *resumeMode != "" {
		if *terminalBinFlag == "" || *terminalNameFlag == "" {
			log.Fatal(fmt.Errorf("You must specify the 'terminal' and 'nameFlag'"))
		}
		if err := resumeSessionGroup(*hostFlag, *terminalBinFlag, *terminalNameFlag); err != nil {
			log.Fatal(fmt.Errorf("Error resuming group: %w", err))
		}
	}
	if *listMode {
		err := listSessionsGroup(*hostFlag)
		if err != nil {
			errMsg := fmt.Sprintf("Error listing group: %s", err)
			if errors.Is(err, TmuxNoSessionsError) {
				errMsg += "\nHint: maybe you have no sessions yet?"
			}
			log.Fatal(fmt.Errorf(errMsg))
		}
	}
	if *serverMode {
		if err := startServer(); err != nil {
			log.Fatal(err)
		}
	}
	if *killMode {
		if err := killSession(); err != nil {
			log.Fatal(fmt.Errorf("Error killing session: %w", err))
		}
	}
}
