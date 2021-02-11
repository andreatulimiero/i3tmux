package main

import (
	"encoding/json"
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
	HOST_DELIM = "@"
)

var (
	terminalBin      = "kitty"
	terminalNameFlag = "--name"
	hostFlag         = flag.String("host", "", "remote host where tmux server runs")
	addMode          = flag.Bool("add", false, "add window to current session group")
	detachMode       = flag.Bool("detach", false, "detach current session group")
	resumeGroup      = flag.String("resume", "", "resume session group")
	listMode         = flag.Bool("list", false, "list sessions groups")
	serverMode       = flag.Bool("server", false, "react to closing session windows")
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
  return fmt.Sprintf("%s@%s",serializeGroupSess(group, session), host)
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
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("error listing sessions groups: %s. Maybe no session was created?", err)
	}
	lines := strings.Split(string(out), "\n")
	sessionsPerGroup := make(map[string]map[string]bool)
	for _, l := range lines {
		group, session, err := deserializeGroupSessFromString(l)
		if err != nil {
			continue
		}
		// Skip unrecognized format
		if _, ok := sessionsPerGroup[group]; !ok {
			sessionsPerGroup[group] = make(map[string]bool)
		}
		sessionsPerGroup[group][session] = true
	}
	return sessionsPerGroup, nil
}

var sessionFmtRe = regexp.MustCompile(`^[a-zA-Z]*(\d+)$`)

func getNextSessIdx(sessionsPerGroup SessionsPerGroup, group string) int {
	sessions := sessionsPerGroup[group]
	var idxs []int
	for s, _ := range sessions {
		res := sessionFmtRe.FindStringSubmatch(s)
		// TODO: Should handle if session name is malformed?
		i, err := strconv.Atoi(res[1])
		if err != nil {
			log.Fatal(err)
			// FIXME: Return the error
		}
		idxs = append(idxs, i)
	}
	sort.Ints(idxs)
	for i, idx := range idxs {
		if i < idx {
			return i
		}
	}
	return len(sessions)
}

func launchTermForSession(host string, group string, session string) error {
	sshCmd := fmt.Sprintf("ssh -t %s tmux attach -t %s", host, serializeGroupSess(group, session))
	i3cmd := fmt.Sprintf("exec %s %s '%s' %s",
		terminalBin,
		terminalNameFlag,
    serializeHostGroupSess(host,group,session),
		sshCmd)
	_, err := i3.RunCommand(i3cmd)
	return err
}

func addWindow() error {
	// TODO: Infer host from window
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

	nextSessIdx := getNextSessIdx(sessionsPerGroup, group)
	nextSess := fmt.Sprintf("session%d", nextSessIdx)
	log.Println("Adding session to group", group, nextSess)
	err = exec.Command("ssh",
		host,
		"tmux new -d -s "+serializeGroupSess(group, nextSess)).Run()
	if err != nil {
		return fmt.Errorf("error creating new session %s: %s", nextSess, err)
	}
  // Add new session remotely

	err = launchTermForSession(host, group, nextSess)
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
  // TODO: Handle workspaces containers too
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

func detachSessionGroup() error {
	// TODO: Add killing of terminals running ssh sessions once layout is retrieved
	//       to simulate a proper detach
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
	return nil
}

func resumeSessionGroup(host string) error {
	sessionsPerGroup, err := fetchSessionsPerGroup(host)
	if err != nil {
		return err
	}
	sessions, ok := sessionsPerGroup[*resumeGroup]
	if !ok {
		return fmt.Errorf("group not found")
	}

	resumeLayoutPath := *resumeGroup + ".json"
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
		err := launchTermForSession(host, *resumeGroup, s)
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

func startServer(host string) error {
	// TODO: Add shutdown receiver to spawn a new WindowEventType receiver
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

func main() {
	flag.Parse()

	if *hostFlag == "" {
		log.Fatal(fmt.Errorf("You must specify the target host"))
	}

	modsCount := 0
	if *addMode {
		modsCount++
	}
	if *detachMode {
		modsCount++
	}
	if *resumeGroup != "" {
		modsCount++
	}
	if *listMode {
		modsCount++
	}
	if *serverMode {
		modsCount++
	}
	if modsCount != 1 {
		log.Fatal(fmt.Errorf("You must specify one mode among 'add', 'detach', 'resume' and 'server'"))
	}
	// Ensure only one mode is selected

	if *addMode {
		if err := addWindow(); err != nil {
			log.Fatal(err)
		}
	}
	if *detachMode {
		if err := detachSessionGroup(); err != nil {
			log.Fatal(err)
		}
	}
	if *resumeGroup != "" {
		if err := resumeSessionGroup(*hostFlag); err != nil {
			log.Fatal(err)
		}
	}
	if *listMode {
		if err := listSessionsGroup(*hostFlag); err != nil {
			fmt.Println(err)
			log.Fatal(err)
		}
	}
	if *serverMode {
		if err := startServer(*hostFlag); err != nil {
			log.Fatal(err)
		}
	}
}
