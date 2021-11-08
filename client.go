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

// Client events
const (
	ConnectionClosed = iota
)

type Client struct {
	conn net.Conn
	enc  *gob.Encoder
	dec  *gob.Decoder
}

func newClient() (*Client, error) {
	_, err := os.Stat(SERVER_SOCK)
	if err != nil {
		if os.IsNotExist(err) {
			fmt.Println("Server not up, starting it ...")
			cmd := exec.Command(I3TMUX_BIN, "-server")
			err := cmd.Start()
			if err != nil {
				return nil, fmt.Errorf("starting server: %w", err)
			}
			time.Sleep(100 * time.Millisecond) // FIXME: choose better way to check for server being up
		} else {
			return nil, fmt.Errorf("checking existing of server socket: %w", err)
		}
	}
	_, err = os.Stat(SERVER_SOCK)
	if err != nil {
		if os.IsNotExist(err) {
			// log.Fatalf("Server did not start: %s\n", err) FIXME: Enable
		}
	}
	// Check if server socket exists

	conn, err := net.Dial("tcp", "localhost:5050")
	if err != nil {
		return nil, fmt.Errorf("dialling server: %+v", err)
	}
	log.Println("Dialled ", SERVER_SOCK)
	return &Client{
		conn: conn,
		enc:  gob.NewEncoder(conn),
		dec:  gob.NewDecoder(conn),
	}, nil
}

func (c *Client) Close() error {
	log.Printf("Closing client %#v", c.conn)
	return c.conn.Close()
}

func (c *Client) Request(r Request) error {
	return c.enc.Encode(&r)
}

func (c *Client) Response() (Response, error) {
	var res Response
	log.Println("Waiting ...")
	err := c.dec.Decode(&res)
	if err != nil {
		return nil, fmt.Errorf("decoding request: %w", err)
	}
	log.Printf("← %T", res)
	return res, err
}

func (c *Client) RequestResponse(r Request) (Response, error) {
	log.Printf("→ %T", r)
	if err := c.Request(r); err != nil {
		return nil, fmt.Errorf("encoding request %s: %w", r, err)
	}
	return c.Response()
}
