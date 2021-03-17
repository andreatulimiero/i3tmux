package main

import (
  "errors"
	"fmt"
	"os"
	"os/exec"
	"path"
	"runtime"
	"testing"
)

func init() {
	_, filename, _, _ := runtime.Caller(0)
	err := os.Chdir(path.Dir(filename))
	if err != nil {
		panic(err)
	}
}

func TestNoGroups(t *testing.T) {
	sessionsPerGroup, err := fetchSessionsPerGroup("i3tmux")
	if !errors.Is(err, TmuxNoSessionsError) {
		t.Error(err)
	}
  if len(sessionsPerGroup) != 0 {
    t.Error("No groups should be returned")
  }
}

type Step struct {
  cmd   *exec.Cmd
  check bool
}

func runSteps(steps []Step) error {
	for _, s := range steps {
		out, err := s.cmd.CombinedOutput()
		if s.check && err != nil {
			return fmt.Errorf("%s: %s, %w", s.cmd, out, err)
		}
	}
	return nil
}

func startSSHServer() error {
	return runSteps([]Step{
		{cmd: exec.Command("podman", "build", "-t", "i3tmux", "-f", "Test.Dockerfile", "."), check: true},
		{cmd: exec.Command("podman", "stop", "i3tmux"), check: false},
		{cmd: exec.Command("podman", "run", "--rm", "-d", "-p", "2222:22", "--name", "i3tmux", "i3tmux"), check: true},
	})
}

func stopSSHServer() error {
	return runSteps([]Step{
		{cmd: exec.Command("podman", "stop", "i3tmux"), check: false},
	})
}

func TestMain(m *testing.M) {
	err := startSSHServer()
	if err != nil {
		panic(fmt.Errorf("error starting ssh server: %w", err))
	}
  errno := m.Run()
	err = stopSSHServer()
	if err != nil {
		panic(fmt.Errorf("error stopping ssh server: %w", err))
	}
  os.Exit(errno)
}
