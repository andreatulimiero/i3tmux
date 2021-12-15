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

type SSHClient struct {
	*ssh.Client
	conn            net.Conn
	isClosed        bool
	reconnectionNo  uint
	conf            *Conf
	sshClientConfig *ssh.ClientConfig
}

func getSSHClientConfig(host string) (*Conf, *ssh.ClientConfig, error) {
	conf, err := getConfForHost(host)
	if err != nil {
		return nil, nil, fmt.Errorf("Error parsing ~/.ssh/config: %w", err)
	}
	key, err := ioutil.ReadFile(conf.IdentityFile)
	if err != nil {
		return nil, nil, fmt.Errorf("unable to read private key: %w", err)
	}
	signer, err := ssh.ParsePrivateKey(key)
	if err != nil {
		return nil, nil, fmt.Errorf("unable to parse private key: %w", err)
	}
	sshClientConfig := ssh.ClientConfig{
		User: conf.User,
		Auth: []ssh.AuthMethod{
			ssh.PublicKeys(signer),
		},
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
		// FIXME: check server's key
	}
	return conf, &sshClientConfig, nil
}

func getSSHClient(conf *Conf, sshClientConfig *ssh.ClientConfig) (*SSHClient, error) {
	var d net.Dialer
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	serverAddr := fmt.Sprintf("%s:%d", conf.Hostname, conf.PortNo)
	conn, err := d.DialContext(ctx, "tcp", serverAddr)
	if err != nil {
		return nil, err
	}
	sshConn, newChan, reqChan, err := ssh.NewClientConn(
		conn,
		serverAddr,
		sshClientConfig)
	client := ssh.NewClient(sshConn, newChan, reqChan)
	return &SSHClient{client, conn, false, 0, conf, sshClientConfig}, nil
}

func newSSHClient(host string) (*SSHClient, error) {
	conf, sshClientConfig, err := getSSHClientConfig(host)
	if err != nil {
		return nil, err
	}
	sshClient, err := getSSHClient(conf, sshClientConfig)
	if err != nil {
		return nil, err
	}
	go sshClient.keepAlive()
	return sshClient, nil
}

func (c *SSHClient) restartConnection() error {
	for {
		var d net.Dialer
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		serverAddr := fmt.Sprintf("%s:%d", c.conf.Hostname, c.conf.PortNo)
		conn, err := d.DialContext(ctx, "tcp", serverAddr)
		if err != nil {
			log.Printf("%#v", err)
			if _, ok := err.(*net.OpError); ok {
				log.Println("Host still down, retrying in 1s")
				time.Sleep(1 * time.Second)
				continue
			} else {
				log.Printf("Unexpected error: %#v", err)
				c.isClosed = true
				return err
			}
		}
		c.conn = conn
		sshConn, newChan, reqChan, err := ssh.NewClientConn(
			c.conn,
			serverAddr,
			c.sshClientConfig)
		c.Client = ssh.NewClient(sshConn, newChan, reqChan)
		c.reconnectionNo += 1
		return nil
	}
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

func (c *SSHClient) keepAlive() {
	keepaliveInterval := 5000 * time.Millisecond
	keepaliveTimeout := 2000 * time.Millisecond
	ticker := time.NewTicker(keepaliveInterval)
	for !c.isClosed {
		<-ticker.C
		err := c.conn.SetDeadline(time.Now().Add(keepaliveTimeout))
		if err != nil {
			log.Printf("failed to set deadline, connection might be closed: %#v", err)
			c.restartConnection()
		}
		start := time.Now()
		_, _, err = c.SendRequest("keepalive@andreatulimiero.com", true, nil)
		if err != nil {
			log.Printf("Error sending keepalive request: %#v", err)
			c.restartConnection()
		}
		log.Printf("Keepalive RTT: %s", time.Now().Sub(start))
		err = c.conn.SetDeadline(time.Time{})
		if err != nil {
			log.Printf("failed to reset deadline, connection might be closed: %v", err)
			c.restartConnection()
		}
	}
}
