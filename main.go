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
	newMode      = flag.String("new", "", "create new group")
	addMode      = flag.Bool("add", false, "add window to the current group")
	listMode     = flag.Bool("list", false, "list sessions groups")
	resumeMode   = flag.String("resume", "", "resume group")
	detachMode   = flag.Bool("detach", false, "detach current group")
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

func createGroup(groupName, host string) error {
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
	if *killMode {
		modsCount++
	}
	if modsCount != 1 {
		log.Fatal(fmt.Errorf("You must specify one mode among 'new', 'add', 'detach', 'resume' and 'kill'"))
	}
	// Ensure only one mode is selected

	if *newMode != "" {
		if err := createGroup(*newMode, *hostFlag); err != nil {
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
	if *killMode {
		if err := killSession(); err != nil {
			log.Fatal(fmt.Errorf("Error killing session: %w", err))
		}
	}
}
