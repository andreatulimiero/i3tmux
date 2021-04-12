package main

import (
	"encoding/gob"
	"fmt"
	"golang.org/x/crypto/ssh"
	"log"
	"net"
	"os"
	"strings"
)

var (
	terminalModes = ssh.TerminalModes{
		ssh.ECHO:          1,     // disable echoing
		ssh.TTY_OP_ISPEED: 14400, // input speed = 14.4kbaud
		ssh.TTY_OP_OSPEED: 14400, // output speed = 14.4kbaud
	}
)

type Request interface {
	Do(*SSHClient, *Client) Response
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

func (r *RequestList) Do(sshClient *SSHClient, client *Client) Response {
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

func (r *RequestCreate) Do(sshClient *SSHClient, client *Client) Response {
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

func (r *RequestAdd) Do(sshClient *SSHClient, client *Client) Response {
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

func (r *RequestResume) Do(sshClient *SSHClient, client *Client) Response {
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

func (r *RequestKill) Do(sshClient *SSHClient, client *Client) Response {
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
	sockCounter = 0
)

func (r *RequestShell) Do(sshClient *SSHClient, client *Client) Response {
	fdSockPath := fmt.Sprintf("/tmp/i3tmux-fd%d.sock", sockCounter) // FIXME: Generate random sock path
	sockCounter += 1
	os.Remove(fdSockPath)
	listener, err := net.Listen("unix", fdSockPath)
	if err != nil {
		log.Fatalf("Unable to listen on socket file %s: %s", fdSockPath, err)
	}
	defer listener.Close()
	// Open socket to received fds

	res := (Response)(&ResponseShell{FdSockPath: fdSockPath})
	err = client.enc.Encode(&res)
	if err != nil {
		return newErrorResponse(UnknownError, err.Error())
	}
	// Communicate fdSockPath is ready

	fdConn, err := listener.Accept()
	if err != nil {
		return newErrorResponse(UnknownError, fmt.Sprintf("error accepting client: %s", err))
	}
	defer fdConn.Close()
	stdin, stdout, stderr, err := RecvFds(fdConn.(*net.UnixConn), 3)
	if err != nil {
		return newErrorResponse(UnknownError, err.Error())
	}
	// Get stdin, stdout and stderr of client

	session, err := sshClient.NewSession()
	if err != nil {
		return newErrorResponse(UnknownError, err.Error())
	}
	defer session.Close()

	// Request pseudo terminal
	if err := session.RequestPty("xterm-256color", r.Height, r.Width, terminalModes); err != nil {
		log.Fatal(err)
	}

	session.Stdin = stdin
	session.Stdout = stdout
	session.Stderr = stderr

	go func() {
		for {
			var winSize WindowSize
			if err := client.dec.Decode(&winSize); err != nil {
				return
			}
			session.WindowChange(winSize.Height, winSize.Width)
		}
	}()
	cmd := fmt.Sprintf("tmux attach-session -d -t %s", r.SessionGroup)
	if err := session.Run(cmd); err != nil {
		log.Println(err)
	}
	return &ResponseBase{}
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
