package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"golang.org/x/term"
	"io/ioutil"
	"log"
	"os"
	"os/signal"
	"path"
	"regexp"
	"syscall"

	"go.i3wm.org/i3/v4"
)

var (
	I3TMUX_BIN = ""
)

var (
	terminalBinFlag  = flag.String("terminal", "", "the binary path of the terminal to use")
	terminalNameFlag = flag.String("nameFlag", "", "the flag used by the terminal of choice"+
		"to define the window instance name")
	hostFlag     = flag.String("host", "", "remote host where tmux server runs")
	sessionFlag  = flag.String("session", "", "session to attach shell to")
	createCmd    = flag.String("create", "", "create new group")
	addCmd       = flag.Bool("add", false, "add window to the current group")
	listCmd      = flag.Bool("list", false, "list sessions groups")
	resumeCmd    = flag.String("resume", "", "resume group")
	detachCmd    = flag.Bool("detach", false, "detach current group")
	killCmd      = flag.Bool("kill", false, "kill current session locally and remotely")
	shellCmd     = flag.Bool("shell", false, "spawn shell for session")
	serverCmd    = flag.Bool("server", false, "run i3tmux server")
	sessionFmtRe = regexp.MustCompile(`^[a-zA-Z]*(\d+)$`)

	pref Pref
)

type SessionsPerGroup map[string]Sessions
type Sessions map[string]bool

func createAction(group, host string) error {
	client, err := newClient()
	if err != nil {
		return err
	}
	defer client.Close()
	// Create client

	res, err := client.RequestResponse(&RequestCreate{RequestBase{host}, group})
	if err != nil {
		return err
	}
	errCode, errMsg := res.Error()
	if errCode != ErrOk {
		switch errCode {
		case GroupAlreadyExistsError:
			return fmt.Errorf("already exists")
		default:
			return fmt.Errorf("%s", errMsg)
		}
	}
	// Receive response

	log.Println("Created new sessions group")
	return res.Do(client, host)
}

func addAction() error {
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

	client, err := newClient()
	if err != nil {
		return err
	}
	defer client.Close()
	// Create client

	res, err := client.RequestResponse(&RequestAdd{RequestBase{host}, group})
	if err != nil {
		return err
	}
	errCode, errMsg := res.Error()
	if errCode != ErrOk {
		return fmt.Errorf("%s", errMsg)
	}
	// Receive response

	return res.Do(client, host)
}

func listAction(host string) error {
	fmt.Println("Retrieving available sessions groups ...")
	client, err := newClient()
	if err != nil {
		return err
	}
	defer client.Close()
	// Create client

	res, err := client.RequestResponse(&RequestList{RequestBase{host}})
	if err != nil {
		return err
	}
	errCode, errMsg := res.Error()
	if errCode != ErrOk {
		switch errCode {
		case TmuxNoSessionsError:
			fmt.Println("No session found")
			return nil
		default:
			return fmt.Errorf("%s", errMsg)
		}
	}
	// Receive response

	return res.Do(client, host)
}

func detachAction() error {
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
	err = ioutil.WriteFile(path.Join(DATA_DIR, group+".json"), j, 0644)
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

func resumeAction(group, host string) error {
	fmt.Println("Resuming sessions ...")
	client, err := newClient()
	if err != nil {
		return err
	}
	defer client.Close()
	// Create client

	res, err := client.RequestResponse(&RequestResume{RequestBase{host}, group})
	if err != nil {
		return err
	}
	errCode, errMsg := res.Error()
	if errCode != ErrOk {
		switch errCode {
		case TmuxNoSessionsError:
			log.Println("No sessions found")
			return nil
		default:
			return fmt.Errorf("%s", errMsg)
		}
	}
	// Receive response
	return res.Do(client, host)
}

func shellAction(session, host string) error {
	client, err := newClient()
	if err != nil {
		return fmt.Errorf("creating client: %w", err)
	}
	defer client.Close()
	// Create client

	w, h, err := term.GetSize(int(os.Stdin.Fd()))
	if err != nil {
		return fmt.Errorf("getting size: %w", err)
	}
	winCh := make(chan os.Signal, 1)
	signal.Notify(winCh, syscall.SIGWINCH)
	go func() {
		for {
			<-winCh
			w, h, err := term.GetSize(int(os.Stdin.Fd()))
			if err != nil {
				fmt.Printf("Error getting size: %s\n", err)
			}
			if err := client.enc.Encode(&WindowSize{w, h}); err != nil {
				fmt.Printf("Error encoding size: %s\n", err)
			}
		}
	}()

	res, err := client.RequestResponse(&RequestShell{RequestBase{host}, session, w, h})
	if err != nil {
		return err
	}
	return res.Do(client, host)
}

func killAction() error {
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

	client, err := newClient()
	if err != nil {
		return err
	}
	defer client.Close()
	res, err := client.RequestResponse(&RequestKill{RequestBase{host}, group, session})
	if err != nil {
		return err
	}
	errCode, errMsg := res.Error()
	if errCode != ErrOk {
		switch errCode {
		default:
			return fmt.Errorf("%s", errMsg)
		}
	}
	log.Println("Killed session", serializeGroupSess(group, session))
	log.Println(res)
	return nil
}

func serverAction() error {
	s := newServer()
	return s.Run()
}

func initLogger() error {
	logFile, err := os.OpenFile(LOG_FILE, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return fmt.Errorf("opening file for log: %s", err)
	}
	log.SetOutput(logFile)
	log.SetFlags(log.LstdFlags | log.Lshortfile)
	return nil
}

func setUpBasics() error {
	I3TMUX_BIN = os.Args[0]
	if err := ensureBaseDirs(); err != nil {
		return err
	}
	if err := initLogger(); err != nil {
		return err
	}
	return nil
}

func parseFlags() {
	flag.Parse()
	if *createCmd != "" || *resumeCmd != "" || *listCmd || *shellCmd {
		if *hostFlag == "" {
			fmt.Println("You must specify the target host")
		}
	}

	modsCount := 0
	if *createCmd != "" {
		modsCount++
	}
	if *addCmd {
		modsCount++
	}
	if *detachCmd {
		modsCount++
	}
	if *resumeCmd != "" {
		modsCount++
	}
	if *shellCmd {
		modsCount++
	}
	if *listCmd {
		modsCount++
	}
	if *killCmd {
		modsCount++
	}
	if *serverCmd {
		modsCount++
	}
	if modsCount != 1 {
		fmt.Println("You must specify one mode among 'new', 'add', 'detach', 'resume', 'kill', 'shell' and 'server'")
	}
	// Ensure only one mode is selected
}

func main() {
	if err := setUpBasics(); err != nil {
		log.Fatal("Error performing initial setup: ", err)
	}

	parseFlags()
	pref = getUserPreferences()

	if *createCmd != "" {
		if err := createAction(*createCmd, *hostFlag); err != nil {
			fmt.Printf("Error creating group: %s\n", err)
		}
	}
	if *addCmd {
		if err := addAction(); err != nil {
			log.Fatal("Error adding window: ", err)
		}
	}
	if *detachCmd {
		if err := detachAction(); err != nil {
			log.Fatal(fmt.Sprintf("Error detaching group: %s", err))
		}
	}
	if *resumeCmd != "" {
		if pref.Terminal.Bin == "" || pref.Terminal.NameFlag == "" {
			fmt.Println("You must specify 'terminal.bin' and 'terminal.nameFlag' options")
		}
		if err := resumeAction(*resumeCmd, *hostFlag); err != nil {
			fmt.Printf("Error resuming group: %s\n", err)
		}
	}
	if *shellCmd {
		if *sessionFlag == "" {
			log.Fatal(fmt.Errorf("You must specify the target session"))
		}
		err := shellAction(*sessionFlag, *hostFlag)
		if err != nil {
			log.Fatal(fmt.Sprintf("Error starting shell for %s: %s", *sessionFlag, err))
		}
	}
	if *listCmd {
		err := listAction(*hostFlag)
		if err != nil {
			errMsg := fmt.Sprintf("Error listing group: %s", err)
			log.Fatal(fmt.Errorf(errMsg))
		}
	}
	if *killCmd {
		if err := killAction(); err != nil {
			log.Fatal(fmt.Errorf("Error killing session: %w", err))
		}
	}
	if *serverCmd {
		if err := serverAction(); err != nil {
			log.Fatal("Error spawning server: ", err)
		}
	}
}
