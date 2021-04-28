package main

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"path"
	"runtime"
	"sync/atomic"
	"testing"
)

const (
	TESTS_DIR                  = "tests"
	POD_BASE_NAME              = "i3tmux-pod"
	SERVER_IMAGE_TAG           = "docker.io/andreatulimiero/i3tmux:i3tmux-server"
	SERVER_CONTAINER_BASE_NAME = "i3tmux-server"
	SSH_HOSTNAME               = "i3tmux"
	CLIENT_IMAGE_TAG           = "i3tmux-client"
	CLIENT_CONTAINER_BASE_NAME = "i3tmux-client"
)

var (
	containerCmd = "podman"
)

func init() {
	_, filename, _, _ := runtime.Caller(0)
	err := os.Chdir(path.Dir(filename))
	if err != nil {
		panic(err)
	}
	if cmd := os.Getenv("I3TMUX_TEST_CONTAINER_CMD"); cmd != "" {
		containerCmd = cmd
	}
}

func runSteps(steps []*exec.Cmd) {
	for _, cmd := range steps {
		out, err := cmd.CombinedOutput()
		if err != nil {
			panic(fmt.Errorf("%s: %s, %s", cmd, out, err))
		}
	}
}

var globalEnvNo = uint32(0)

type Environment struct {
	ClientContainerName string
	PodName             string
	Containers          []string
}

func newEnvironment() *Environment {
	envNo := atomic.AddUint32(&globalEnvNo, 1)
	podName := fmt.Sprintf("%s-%d", POD_BASE_NAME, envNo)
	serverContainerName := fmt.Sprintf("%s-%d", SERVER_CONTAINER_BASE_NAME, envNo)
	clientContainerName := fmt.Sprintf("%s-%d", CLIENT_CONTAINER_BASE_NAME, envNo)
	runSteps([]*exec.Cmd{exec.Command(containerCmd,
		"run",
		"--rm",
		"-d",
		"--pod", "new:"+podName,
		"--name", serverContainerName,
		SERVER_IMAGE_TAG),
		exec.Command(containerCmd,
			"run",
			"--rm",
			"-d",
			"--pod", podName,
			"--name", clientContainerName,
			CLIENT_IMAGE_TAG)})
	return &Environment{clientContainerName,
		podName,
		[]string{clientContainerName, serverContainerName}}
}

func (e *Environment) Close() {
	for _, c := range e.Containers {
		runSteps([]*exec.Cmd{exec.Command(containerCmd, "stop", c)})
	}
	runSteps([]*exec.Cmd{exec.Command(containerCmd, "pod", "stop", e.PodName),
		exec.Command(containerCmd, "pod", "rm", e.PodName)})
}

func (e *Environment) Run(command ...string) ([]byte, error) {
	execCmd := []string{"exec", e.ClientContainerName}
	cmd := exec.Command(containerCmd, append(execCmd, command...)...)
	out, err := cmd.CombinedOutput()
	return out, err
}

func (e *Environment) RunSuccess(command ...string) []byte {
	out, err := e.Run(command...)
	if err != nil {
		panic(fmt.Errorf("%s: %s %s", command, out, err))
	}
	return out
}

func (e *Environment) Start(command ...string) error {
	execCmd := []string{"exec", e.ClientContainerName}
	cmd := exec.Command(containerCmd, append(execCmd, command...)...)
	return cmd.Start()
}

func (e *Environment) StartSuccess(command ...string) {
	err := e.Start(command...)
	if err != nil {
		panic(err)
	}
}

func checkOutput(t *testing.T, out []byte, e *Environment) {
	expected, err := ioutil.ReadFile(path.Join(TESTS_DIR, t.Name()))
	if err != nil {
		panic(err)
	}
	if bytes.Compare(out, expected) != 0 {
		t.Errorf("expected:\n%s\nreceived:\n%s\n", string(expected), string(out))
		logs := string(e.RunSuccess("cat", "/var/run/i3tmux/i3tmux.log"))
		t.Log("Logs:\n", logs)
	}
}

func checkScreen(t *testing.T, e *Environment) {
	screen := e.RunSuccess("/bin/bash", "-c", "xwd -root | convert xwd:- png:-")
	expected, err := ioutil.ReadFile(path.Join(TESTS_DIR, t.Name()))
	if err != nil {
		panic(err)
	}
	if bytes.Compare(screen, expected) != 0 {
		tmpFileName := "/tmp/screen-" + t.Name() + ".png"
		err := ioutil.WriteFile(tmpFileName, screen, 0644)
		if err != nil {
			panic(err)
		}
		t.Errorf("screen %s different than expected %s", tmpFileName, t.Name())
	}
}

func TestNoGroups(t *testing.T) {
	// t.Parallel()
	e := newEnvironment()
	defer e.Close()

	out := e.RunSuccess(I3TMUX, "-host", SSH_HOSTNAME, "-list")
	checkOutput(t, out, e)
}

func TestCreateGroup(t *testing.T) {
	// t.Parallel()
	e := newEnvironment()
	defer e.Close()

	e.RunSuccess(I3TMUX, "-host", SSH_HOSTNAME, "-create", "foo")
	out := e.RunSuccess(I3TMUX, "-host", SSH_HOSTNAME, "-list")
	checkOutput(t, out, e)
}

func TestResumeGroupWithSingleSession(t *testing.T) {
	// t.Parallel()
	e := newEnvironment()
	defer e.Close()

	e.StartSuccess("i3")
	e.RunSuccess(I3TMUX, "-host", SSH_HOSTNAME, "-create", "foo")
	e.RunSuccess(I3TMUX, "-host", SSH_HOSTNAME, "-resume", "foo")
	out := e.RunSuccess("i3-save-tree")
	checkOutput(t, out, e)
}

func TestAddSessionToGroup(t *testing.T) {
	// t.Parallel()
	e := newEnvironment()
	defer e.Close()

	e.StartSuccess("i3")
	e.RunSuccess(I3TMUX, "-host", SSH_HOSTNAME, "-create", "foo")
	e.RunSuccess(I3TMUX, "-host", SSH_HOSTNAME, "-resume", "foo")
	e.RunSuccess(I3TMUX, "-host", SSH_HOSTNAME, "-add")
	out := e.RunSuccess("i3-save-tree")
	checkOutput(t, out, e)
}

func TestKillSessionOfGroup(t *testing.T) {
	// t.Parallel()
	e := newEnvironment()
	defer e.Close()

	e.StartSuccess("i3")
	e.RunSuccess(I3TMUX, "-host", SSH_HOSTNAME, "-create", "foo")
	e.RunSuccess(I3TMUX, "-host", SSH_HOSTNAME, "-resume", "foo")
	e.RunSuccess(I3TMUX, "-host", SSH_HOSTNAME, "-add")
	e.RunSuccess(I3TMUX, "-host", SSH_HOSTNAME, "-kill")
	out := e.RunSuccess("i3-save-tree")
	checkOutput(t, out, e)
}

func TestDetachResumeGroup(t *testing.T) {
	// t.Parallel()
	e := newEnvironment()
	defer e.Close()

	e.StartSuccess("i3")
	e.RunSuccess(I3TMUX, "-host", SSH_HOSTNAME, "-create", "foo")
	e.RunSuccess(I3TMUX, "-host", SSH_HOSTNAME, "-resume", "foo")
	e.RunSuccess(I3TMUX, "-host", SSH_HOSTNAME, "-add")
	e.RunSuccess(I3TMUX, "-host", SSH_HOSTNAME, "-detach")
	e.RunSuccess(I3TMUX, "-host", SSH_HOSTNAME, "-resume", "foo")
	out := e.RunSuccess("i3-save-tree")
	checkOutput(t, out, e)
}
