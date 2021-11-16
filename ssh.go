package main

import (
	"bytes"
	"context"
	"fmt"
	"log"
	"time"

	"golang.org/x/crypto/ssh"

	"io/ioutil"
	"net"
)

const (
	TCP_USER_TIMEOUT_VALUE = 5 // in sec
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

	var d net.Dialer
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	serverAddr := fmt.Sprintf("%s:%d", conf.Hostname, conf.PortNo)
	conn, err := d.DialContext(ctx, "tcp", serverAddr)
	if err != nil {
		return nil, err
	}

	sshConn, newChan, reqChan, err := ssh.NewClientConn(conn, serverAddr, config)
	client := ssh.NewClient(sshConn, newChan, reqChan)

	go func() {
		keepaliveInterval := 5000 * time.Millisecond
		keepaliveTimeout := 500 * time.Millisecond
		ticker := time.NewTicker(keepaliveInterval)
		for {
			<-ticker.C
			err := conn.SetDeadline(time.Now().Add(keepaliveTimeout))
			if err != nil {
				log.Printf("failed to set deadline, connection might be closed: %#v", err)
				return
			}
			start := time.Now()
			_, _, err = client.SendRequest("keepalive@andreatulimiero.com", true, nil)
			if err != nil {
				log.Printf("Error sending keepalive request: %#v", err)
				return
			}
			log.Printf("Keepalive RTT: %s", time.Now().Sub(start))
			err = conn.SetDeadline(time.Time{})
			if err != nil {
				log.Printf("failed to reset deadline, connection might be closed: %v", err)
				return
			}
		}
	}()

	return &SSHClient{client}, nil
}

func (c *SSHClient) Run(cmd string) (string, string, error) {
	// Handy function to run a cmd remotely and get the answer
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
