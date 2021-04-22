package main

import (
	"encoding/gob"
	"fmt"
	"log"
	"net"
	"os"
	"os/exec"
	"time"
)

type Client struct {
	conn net.Conn
	enc  *gob.Encoder
	dec  *gob.Decoder
}

func newClient() (*Client, error) {
	_, err := os.Stat(SERVER_SOCK_PATH)
	if err != nil {
		if os.IsNotExist(err) {
			fmt.Println("Server not up, starting it ...")
			cmd := exec.Command(I3TMUX_BIN, "-server")
			err := cmd.Start()
			if err != nil {
				return nil, fmt.Errorf("starting server: %w", err)
			}
			time.Sleep(1 * time.Second) // FIXME: choose better way to check for server being up
		} else {
			return nil, fmt.Errorf("checking existing of server socket: %w", err)
		}
	}
	_, err = os.Stat(SERVER_SOCK_PATH)
	if err != nil {
		if os.IsNotExist(err) {
			log.Fatalf("Server did not start: %s\n", err)
		}
	}
	// Check if server socket exists

	conn, err := net.Dial("unix", SERVER_SOCK_PATH)
	if err != nil {
		return nil, fmt.Errorf("dialling server: %+v", err)
	}
	return &Client{
		conn: conn,
		enc:  gob.NewEncoder(conn),
		dec:  gob.NewDecoder(conn),
	}, nil
}

func (c *Client) Close() error {
	return c.conn.Close()
}

func (c *Client) Request(r Request) error {
	return c.enc.Encode(&r)
}

func (c *Client) Response() (Response, error) {
	var res Response
	err := c.dec.Decode(&res)
	if err != nil {
		return nil, fmt.Errorf("decoding request: %w", err)
	}
	return res, err
}

func (c *Client) RequestResponse(r Request) (Response, error) {
	if err := c.Request(r); err != nil {
		return nil, fmt.Errorf("encoding request %s: %w", r, err)
	}
	return c.Response()
}

// oldState, err := term.MakeRaw(int(os.Stdin.Fd()))
// if err != nil {
// log.Fatal("making raw terminal: %w", err)
// }
// fd := int(os.Stdin.Fd())
// defer term.Restore(fd, oldState)
// Make raw terminal

// if err := Put(conn.(*net.UnixConn), os.Stdin, os.Stdout, os.Stderr); err != nil {
// log.Fatal(err)
// }
// b := make([]byte, 1)
// conn.Read(b)
// os.Exit(0)
