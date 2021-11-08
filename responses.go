package main

import (
	"encoding/gob"
	"fmt"
	"log"
	"net"
	"os"
	"path"

	"go.i3wm.org/i3/v4"
	"golang.org/x/term"
)

const (
	ErrOk                   = iota // means no error
	TmuxNoSessionsError     = iota
	GroupAlreadyExistsError = iota
	GroupNotFoundError      = iota
	InvalidGroupNameError   = iota
	ExitMissingErrorError   = iota
	CannotConnectError      = iota
	UnknownError            = iota
)

type Response interface {
	Error() (int, string)
	Do(client *Client, host, session string) error
}

func newErrorResponse(errorCode int, errorMsg string) *ResponseBase {
	return &ResponseBase{errorCode, errorMsg}
}

var _ Response = (*ResponseBase)(nil)

type ResponseBase struct {
	ErrorCode int
	ErrorMsg  string
}

func (r *ResponseBase) Error() (int, string) {
	return r.ErrorCode, r.ErrorMsg
}

func (r *ResponseBase) Do(client *Client, host, session string) error {
	return nil
}

var _ Response = (*ResponseCreate)(nil)

type ResponseCreate struct {
	ResponseBase
	SessionGroup string
}

func (r *ResponseCreate) Do(client *Client, host, session string) error {
	return nil
}

var _ Response = (*ResponseList)(nil)

type ResponseList struct {
	ResponseBase
	Sessions SessionsPerGroup
}

func (r *ResponseList) Do(client *Client, host, session string) error {
	for g, _ := range r.Sessions {
		fmt.Println("- " + g)
	}
	return nil
}

var _ Response = (*ResponseResume)(nil)

type ResponseResume struct {
	ResponseBase
	Group    string
	Sessions Sessions
}

func (r *ResponseResume) Do(client *Client, host, session string) error {
	resumeLayoutPath := path.Join(DATA_DIR, r.Group+".json")
	_, err := os.Stat(resumeLayoutPath)
	if err != nil {
		if !os.IsNotExist(err) {
			// If error is not expected exit
			return fmt.Errorf("opening saved layout: %s", err)
		}
	} else {
		_, err = i3.RunCommand(fmt.Sprintf("append_layout %s", resumeLayoutPath))
		if err != nil {
			return fmt.Errorf("appending i3 layout: %w", err)
		}
	}
	// Try to load a layout for the target sessions group

	for s := range r.Sessions {
		err := launchTermForSession(r.Group, s, host)
		if err != nil {
			return fmt.Errorf("launching term for %s: %w", s, err)
		}
	}
	return nil
}

var _ Response = (*ResponseAdd)(nil)

type ResponseAdd struct {
	ResponseBase
	Group   string
	Session string
}

func (r *ResponseAdd) Do(client *Client, host, session string) error {
	err := launchTermForSession(r.Group, r.Session, host)
	if err != nil {
		return fmt.Errorf("launching term for %s: %w", r.Session, err)
	}
	return nil
}

var _ Response = (*ResponseKill)(nil)

type ResponseKill struct{ ResponseBase }

var _ Response = (*ResponseShell)(nil)

type ResponseShell struct {
	ResponseBase
	FdSockPath string
}

func (r *ResponseShell) Do(client *Client, host, session string) error {
	fdConn, err := net.Dial("unix", r.FdSockPath)
	if err != nil {
		return fmt.Errorf("Failed to dial: %w", err)
	}
	log.Println("Dialled ", r.FdSockPath)
	defer fdConn.Close()

	oldState, err := term.MakeRaw(int(os.Stdin.Fd()))
	if err != nil {
		return fmt.Errorf("making raw terminal: %w", err)
	}
	fd := int(os.Stdin.Fd())
	defer term.Restore(fd, oldState)
	// Make raw terminal

	err = SendFds(fdConn.(*net.UnixConn), os.Stdin, os.Stdout, os.Stderr)
	if err != nil {
		return fmt.Errorf("sending fds: %w", err)
	}
	res, err := client.Response()
	if err != nil {
		return err
	}
	return res.Do(client, host, session)
}

func init() {
	gob.Register(&ResponseBase{})
	gob.Register(&ResponseCreate{})
	gob.Register(&ResponseAdd{})
	gob.Register(&ResponseList{})
	gob.Register(&ResponseResume{})
	gob.Register(&ResponseKill{})
	gob.Register(&ResponseShell{})
}
