package main

import (
	"fmt"
	"net"
	"os"
	"syscall"
)

func RecvFds(conn *net.UnixConn, num int) (*os.File, *os.File, *os.File, error) {
	oob := make([]byte, syscall.CmsgSpace(num*4))
	_, oobn, _, _, err := conn.ReadMsgUnix(nil, oob)
	if err != nil {
		return nil, nil, nil, err
	}
	scms, err := syscall.ParseSocketControlMessage(oob[:oobn])
	if err != nil {
		return nil, nil, nil, fmt.Errorf("ParseSocketControlMessage failed %w", err)
	}
	// Receive fds

	var fds []int
	for _, scm := range scms {
		scmFds, err := syscall.ParseUnixRights(&scm)
		if err != nil {
			return nil, nil, nil, fmt.Errorf("ParseUnixRights failed %v", err)
		}
		for _, fd := range scmFds {
			fds = append(fds, fd)
		}
	}
	// Exctract fds

	if len(fds) != 3 {
		return nil, nil, nil, fmt.Errorf("expected %d files, received %d", 3, len(fds))
	}
	fileNames := []string{"stdin", "stdout", "stderr"}
	files := make([]*os.File, 3)
	for i, fd := range fds {
		files[i] = os.NewFile(uintptr(fd), fileNames[i])
	}
	return files[0], files[1], files[2], nil
}

func SendFds(conn *net.UnixConn, stdin, stdout, stderr *os.File) error {
	fds := []int{int(stdin.Fd()), int(stdout.Fd()), int(stderr.Fd())}
	rights := syscall.UnixRights(fds...)
	_, oobn, err := conn.WriteMsgUnix(nil, rights, nil)
	if err != nil {
		return err
	}
	if oobn != len(rights) {
		return fmt.Errorf("missing oob bytes: %d < %d", oobn, len(rights))
	}
	return nil
}
