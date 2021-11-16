package main

import (
	"encoding/gob"
	"fmt"
	"log"
	"net"
	"os"
	"strings"
	"sync/atomic"
	"time"

	"golang.org/x/crypto/ssh"
)

var (
	terminalModes = ssh.TerminalModes{
		ssh.ECHO:          1,     // disable echoing
		ssh.TTY_OP_ISPEED: 14400, // input speed = 14.4kbaud
		ssh.TTY_OP_OSPEED: 14400, // output speed = 14.4kbaud
	}
)

type Request interface {
	Do(*SSHClient, *ServerClient) Response
	GetHost() string
}

// var _ Request = (*RequestBase)(nil)
type RequestBase struct {
	Host string
}

func (r *RequestBase) GetHost() string {
	return r.Host
}

var _ Request = (*RequestList)(nil)

type RequestList struct {
	RequestBase
}

func (r *RequestList) Do(sshClient *SSHClient, client *ServerClient) Response {
	sessionsPerGroup, errCode, errMsg := fetchSessionsPerGroup(sshClient)
	if errCode != ErrOk {
		return newErrorResponse(errCode, errMsg)
	}
	return &ResponseList{Sessions: sessionsPerGroup}
}

var _ Request = (*RequestCreate)(nil)

type RequestCreate struct {
	RequestBase
	Group string
}

func (r *RequestCreate) Do(sshClient *SSHClient, client *ServerClient) Response {
	if strings.Contains(r.Group, GROUP_SESS_DELIM) {
		// errMsg := fmt.Sprintf("group name cannot contain '%s'", GROUP_SESS_DELIM)
		return newErrorResponse(InvalidGroupNameError, "")
	}
	sessionsPerGroup, errCode, errMsg := fetchSessionsPerGroup(sshClient)
	switch errCode {
	case ErrOk:
	case TmuxNoSessionsError:
	default:
		return newErrorResponse(errCode, errMsg)
	}
	if _, ok := sessionsPerGroup[r.Group]; ok {
		return newErrorResponse(GroupAlreadyExistsError, errMsg)
	}
	_, stderr, err := createSession(r.Group, "session0", sshClient)
	if err != nil {
		return newErrorResponse(UnknownError, fmt.Sprintf("%s: %s", err, stderr))
	}
	return &ResponseCreate{SessionGroup: serializeGroupSess(r.Group, "session0")}
}

var _ Request = (*RequestAdd)(nil)

type RequestAdd struct {
	RequestBase
	Group string
}

func (r *RequestAdd) Do(sshClient *SSHClient, client *ServerClient) Response {
	sessionsPerGroup, errCode, errMsg := fetchSessionsPerGroup(sshClient)
	if errCode != ErrOk {
		return newErrorResponse(errCode, errMsg)
	}
	nextSessIdx, err := getNextSessIdx(sessionsPerGroup, r.Group)
	if err != nil {
		return newErrorResponse(UnknownError, err.Error())
	}
	nextSess := fmt.Sprintf("session%d", nextSessIdx)
	log.Println("Adding session to group", r.Group, nextSess)
	_, stderr, err := createSession(r.Group, nextSess, sshClient)
	if err != nil {
		return newErrorResponse(UnknownError, fmt.Sprintf("%s: %s", err, stderr))
	}
	return &ResponseAdd{Group: r.Group, Session: nextSess}
}

var _ Request = (*RequestResume)(nil)

type RequestResume struct {
	RequestBase
	Group string
}

func (r *RequestResume) Do(sshClient *SSHClient, client *ServerClient) Response {
	sessionsPerGroup, errCode, errMsg := fetchSessionsPerGroup(sshClient)
	if errCode != ErrOk {
		return newErrorResponse(errCode, errMsg)
	}
	if sessions, ok := sessionsPerGroup[r.Group]; ok {
		return &ResponseResume{Group: r.Group, Sessions: sessions}
	} else {
		return newErrorResponse(GroupNotFoundError, "")
	}
}

var _ Request = (*RequestKill)(nil)

type RequestKill struct {
	RequestBase
	Group string
	Sess  string
}

func (r *RequestKill) Do(sshClient *SSHClient, client *ServerClient) Response {
	groupSess := serializeGroupSess(r.Group, r.Sess)
	cmd := fmt.Sprintf("tmux kill-session -t %s", groupSess)
	_, stderr, err := sshClient.Run(cmd)
	if err != nil {
		errMsg := fmt.Sprintf("unable to execute remote cmd: %s, %s, %s",
			cmd,
			stderr,
			err)
		return newErrorResponse(UnknownError, errMsg)
	}
	return &ResponseKill{}
}

var _ Request = (*RequestShell)(nil)

type RequestShell struct {
	RequestBase
	SessionGroup  string
	Width, Height int
}

type WindowSize struct {
	Width, Height int
}

var (
	sockCounter = uint32(0)
)

func (r *RequestShell) getClientFds(client *ServerClient) (*os.File, *os.File, *os.File, error) {
	// getClientFsd gets stdin, stdout and stderr from the client
	fdSockPath := fmt.Sprintf("/tmp/i3tmux-fd%d.sock", sockCounter) // FIXME: Implement better random file generation
	atomic.AddUint32(&sockCounter, 1)
	os.Remove(fdSockPath)
	listener, err := net.Listen("unix", fdSockPath)
	if err != nil {
		log.Fatalf("Unable to listen on socket file %s: %s", fdSockPath, err)
	}
	defer listener.Close()
	// Open socket to receive fds

	res := (Response)(&ResponseShell{FdSockPath: fdSockPath})
	err = client.enc.Encode(&res)
	if err != nil {
		return nil, nil, nil, err
	}
	// Communicate fdSockPath is ready

	fdConn, err := listener.Accept()
	if err != nil {
		return nil, nil, nil, err
	}
	defer fdConn.Close()
	stdin, stdout, stderr, err := RecvFds(fdConn.(*net.UnixConn), 3)
	if err != nil {
		return nil, nil, nil, err
	}
	return stdin, stdout, stderr, nil
}

func (r *RequestShell) Do(sshClient *SSHClient, client *ServerClient) Response {
	stdin, stdout, stderr, err := r.getClientFds(client)
	if err != nil {
		return newErrorResponse(UnknownError, err.Error())
	}
	// Get stdin, stdout and stderr of client

	var session *ssh.Session
	var winSize WindowSize
	go func() {
		for {
			if err := client.dec.Decode(&winSize); err != nil {
				log.Println(err)
				return
			}
			session.WindowChange(winSize.Height, winSize.Width)
		}
	}()

	for {
		session, err = sshClient.NewSession()
		if err != nil {
			log.Printf("Unexpected error: %#v", err)
			return newErrorResponse(UnknownError, err.Error())
		}
		if err := session.RequestPty("xterm-256color", r.Height, r.Width, terminalModes); err != nil {
			log.Printf("Unexpected error: %#v", err)
			log.Fatal(err)
		}
		// Request pseudo terminal
		session.Stdin = stdin
		session.Stdout = stdout
		session.Stderr = stderr
		if winSize.Height != 0 && winSize.Width != 0 {
			session.WindowChange(winSize.Height, winSize.Width)
			// Send a window change request everytime we re-establish a session
		}

		cmd := fmt.Sprintf("tmux attach-session -d -t %s", r.SessionGroup)
		if err := session.Run(cmd); err != nil {
			switch err.(type) {
			case *ssh.ExitMissingError:
				removeSSHClient(r.GetHost())
				stdout.Write([]byte("\x1b[2J"))
				// Clear the terminal content
				stdout.Write([]byte("\x1b[1;1H"))
				// Position the cursor at the begiing of the first row
				lostConnectionTime := time.Now().Format(time.UnixDate)
				stdout.Write([]byte("Lost connection: " + lostConnectionTime + "\n"))
				stdout.Write([]byte("\x1b[2;1H"))
				// Position the cursor at the beginning of the second row
				stdout.Write([]byte("Retrying "))
				for {
					stdout.Write([]byte("."))
					sshClient, err = ensureSSHClient(r.GetHost())
					if err == nil {
						break
					} else if _, ok := err.(*net.OpError); ok {
						log.Println("Host still down, retrying in 1s")
						time.Sleep(1 * time.Second)
					} else {
						log.Printf("Unexpected error: %#v", err)
						return &ResponseBase{UnknownError, err.Error()}
					}
				}
			default:
				log.Printf("%#v", err)
				return &ResponseBase{UnknownError, err.Error()}
			}
		} else {
			return &ResponseBase{}
		}
	}
}

func init() {
	gob.Register(&RequestList{})
	gob.Register(&RequestCreate{})
	gob.Register(&RequestAdd{})
	gob.Register(&RequestResume{})
	gob.Register(&RequestKill{})
	gob.Register(&WindowSize{})
	gob.Register(&RequestShell{})
}
