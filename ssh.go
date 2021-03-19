package main

import (
  "io/ioutil"
  "bytes"
  "fmt"
  "golang.org/x/crypto/ssh"
)

type Client struct{
  *ssh.Client
}

func NewClient(host string, conf *Conf) (*Client, error) {
  key, err := ioutil.ReadFile(conf.privKeyPath)
  if err != nil {
    return nil, fmt.Errorf("unable to read private key: %w", err)
  }
  signer, err := ssh.ParsePrivateKey(key)
  if err != nil {
    return nil, fmt.Errorf("unable to parse private key: %w", err)
  }
  config := &ssh.ClientConfig{
    User: conf.user,
    Auth: []ssh.AuthMethod{
      ssh.PublicKeys(signer),
    },
    HostKeyCallback: ssh.InsecureIgnoreHostKey(),
    // FIXME: check server's key
  }
  client, err := ssh.Dial("tcp", fmt.Sprintf("%s:%d",host,conf.portNo), config)
  if err != nil {
    return nil, fmt.Errorf("unable to dial: %w", err)
  }
  return &Client{client}, nil
}

func (c *Client) Close() error {
  return c.Close()
}

func(c *Client) Run(cmd string) (string, string, error) {
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
