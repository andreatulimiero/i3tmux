package main

import (
	"bytes"
	"fmt"
	"golang.org/x/crypto/ssh"
	"io/ioutil"
	"os"
)

type SSHClient struct {
	*ssh.Client
}

func newSSHClient(host string) (*SSHClient, error) {
	conf, err := getConfForHost(host)
	if err != nil {
		return nil, fmt.Errorf("Error parsing ~/.ssh/config: %w", err)
	}
	key, err := ioutil.ReadFile(conf.IdentityFile)
	if err != nil {
		return nil, fmt.Errorf("unable to read private key: %w", err)
	}
	signer, err := ssh.ParsePrivateKey(key)
	if err != nil {
		return nil, fmt.Errorf("unable to parse private key: %w", err)
	}
	config := &ssh.ClientConfig{
		User: conf.User,
		Auth: []ssh.AuthMethod{
			ssh.PublicKeys(signer),
		},
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
		// FIXME: check server's key
	}
	conn, err := ssh.Dial("tcp", fmt.Sprintf("%s:%d", conf.Hostname, conf.PortNo), config)
	if err != nil {
		return nil, fmt.Errorf("unable to dial: %w", err)
	}
	return &SSHClient{conn}, nil
}

func (c *SSHClient) Run(cmd string) (string, string, error) {
	session, err := c.NewSession()
	if err != nil {
		return "", "", fmt.Errorf("unable to create session: %w", err)
	}
	defer session.Close()

	var stdout, stderr bytes.Buffer
	session.Stdout = &stdout
	session.Stderr = &stderr
	err = session.Run(cmd)
	return stdout.String(), stderr.String(), err
}

func (c *SSHClient) Shell(files []*os.File) error {
	session, err := c.NewSession()
	if err != nil {
		return fmt.Errorf("unable to create session: %w", err)
	}
	defer session.Close()

	modes := ssh.TerminalModes{
		ssh.ECHO:          1,     // disable echoing
		ssh.TTY_OP_ISPEED: 14400, // input speed = 14.4kbaud
		ssh.TTY_OP_OSPEED: 14400, // output speed = 14.4kbaud
	}
	// FIXME: Retrieve real width and height of the terminal
	// w, h, err := term.GetSize(fd)
	// if err != nil {
	// return fmt.Errorf("getting terminal width and height: %w", err)
	// }
	if err := session.RequestPty("xterm-256color", 40, 80, modes); err != nil {
		return fmt.Errorf("request for pseudo terminal failed: %w", err)
	}

	session.Stdout = files[0]
	session.Stderr = files[1]
	session.Stdin = files[2]

	// Start remote shell
	fmt.Println("Starting shell")
	if err := session.Shell(); err != nil {
		return fmt.Errorf("failed to start shell: %w", err)
	}

	if err := session.Wait(); err != nil {
		if e, ok := err.(*ssh.ExitError); ok {
			switch e.ExitStatus() {
			case 130:
				return nil
			}
		}
		return fmt.Errorf("ssh: %s", err)
	}
	return nil
}
