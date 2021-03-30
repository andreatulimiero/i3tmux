package main

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path"
	"runtime"
	"sync/atomic"
	"testing"
)

const (
	IMAGE_TAG = "i3tmux"
)

var (
	globContName uint32 = 0
	globPortNo   uint32 = 2222
	baseConf            = Conf{
		hostname:     "localhost",
		user:         "root",
		identityFile: "./test_key",
	}
	containerCmd = "podman"
)

func init() {
	_, filename, _, _ := runtime.Caller(0)
	err := os.Chdir(path.Dir(filename))
	if err != nil {
		panic(err)
	}
	err = runSteps([]*exec.Cmd{
		exec.Command("chmod", "0600", "test_key", "test_key.pub"),
		exec.Command(containerCmd, "build", "-t", IMAGE_TAG, "-f", "Test.Dockerfile", "."),
	})
	if err != nil {
		panic(fmt.Errorf("error initializing environment: %w", err))
	}
	if cmd := os.Getenv("I3TMUX_TEST_CONTAINER_CMD"); cmd != "" {
		containerCmd = cmd
	}
}

func runSteps(steps []*exec.Cmd) error {
	for _, cmd := range steps {
		out, err := cmd.CombinedOutput()
		if err != nil {
			return fmt.Errorf("%s: %s, %w", cmd, out, err)
		}
	}
	return nil
}

type SSHServer struct {
	containerName string
}

func newSSHServer() (*SSHServer, uint32) {
	containerName := fmt.Sprintf("%s%d", IMAGE_TAG, atomic.AddUint32(&globContName, 1))
	portNo := atomic.AddUint32(&globPortNo, 1)
	portMapping := fmt.Sprintf("%d:22", portNo)
	err := runSteps([]*exec.Cmd{
		exec.Command(containerCmd, "run", "--rm", "-d", "-p", portMapping, "--name", containerName, IMAGE_TAG),
	})
	if err != nil {
		panic(fmt.Errorf("error starting ssh server: %w", err))
	}
	return &SSHServer{containerName: containerName}, portNo
}

func (s *SSHServer) stop() {
	err := runSteps([]*exec.Cmd{
		exec.Command(containerCmd, "stop", s.containerName),
	})
	if err != nil {
		panic(fmt.Errorf("error stopping ssh server: %w", err))
	}
}

func TestNoGroups(t *testing.T) {
	sshServer, portNo := newSSHServer()
	defer sshServer.stop()
	conf := baseConf
	conf.portNo = int(portNo)

	sessionsPerGroup, err := fetchSessionsPerGroup(&conf)
	if !errors.Is(err, TmuxNoSessionsError) {
		t.Error(err)
	}
	if len(sessionsPerGroup) != 0 {
		t.Error("No group should be returned")
	}
}

func TestCreateGroup(t *testing.T) {
	sshServer, portNo := newSSHServer()
	defer sshServer.stop()
	conf := baseConf
	conf.portNo = int(portNo)

	group := "foo"
	err := createGroup(group, &conf)
	if err != nil {
		t.Error(err)
	}
	sessionsPerGroup, err := fetchSessionsPerGroup(&conf)
	if err != nil {
		t.Error(err)
	}
	if _, ok := sessionsPerGroup[group]; !ok {
		t.Errorf("Group not found: %v\n", sessionsPerGroup)
	}
}

func TestAddSessionsToGroup(t *testing.T) {
	sshServer, portNo := newSSHServer()
	defer sshServer.stop()
	conf := baseConf
	conf.portNo = int(portNo)

	group := "foo"
	err := createGroup("foo", &conf)
	if err != nil {
		t.Error(err)
	}
	for i := 1; i < 10; i++ {
		nextSess, err := addSessionToGroup(group, &conf)
		expectedNextSess := fmt.Sprintf("session%d", i)
		if nextSess != expectedNextSess {
			t.Errorf("Unexpected next session: %s is not %s", nextSess, expectedNextSess)
		}
		sessionsPerGroup, err := fetchSessionsPerGroup(&conf)
		if err != nil {
			t.Error(err)
		}
		if _, ok := sessionsPerGroup["foo"][expectedNextSess]; !ok {
			t.Errorf("Next session not found: %v\n", sessionsPerGroup["foo"])
		}
	}
}

func TestKillSessions(t *testing.T) {
	sshServer, portNo := newSSHServer()
	defer sshServer.stop()
	conf := baseConf
	conf.portNo = int(portNo)

	group := "foo"
	err := createGroup("foo", &conf)
	if err != nil {
		t.Error(err)
	}
	for i := 1; i < 10; i++ {
		nextSess, err := addSessionToGroup(group, &conf)
		expectedNextSess := fmt.Sprintf("session%d", i)
		if nextSess != expectedNextSess {
			t.Errorf("Unexpected next session: %s is not %s", nextSess, expectedNextSess)
		}
		sessionsPerGroup, err := fetchSessionsPerGroup(&conf)
		if err != nil {
			t.Error(err)
		}
		if _, ok := sessionsPerGroup[group][expectedNextSess]; !ok {
			t.Errorf("Next session not found: %v\n", sessionsPerGroup[group])
		}
	}

	for i := range []int{0, 3, 5, 7, 9} {
		targetSess := fmt.Sprintf("session%d", i)
		err = killSession(group, targetSess, &conf)
		if err != nil {
			t.Error(err)
		}
		sessionsPerGroup, err := fetchSessionsPerGroup(&conf)
		if err != nil {
			t.Error(err)
		}
		if _, ok := sessionsPerGroup[group][targetSess]; ok {
			t.Errorf("Session should be gone: %v\n", sessionsPerGroup[group])
		}
	}
}
