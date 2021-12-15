package main

import (
	"encoding/gob"
	"fmt"
	"log"
	"net"
	"os"
	"os/signal"
	"sync"
	"syscall"
)

var (
	sshClients     = make(map[string]*SSHClient)
	sshClientsLock = sync.RWMutex{}
)

type Server struct{}

func newServer() *Server {
	return &Server{}
}

type ServerClient struct {
	Client
}

func newServerClient(conn net.Conn) *ServerClient {
	enc := gob.NewEncoder(conn)
	dec := gob.NewDecoder(conn)
	return &ServerClient{Client{conn: conn, enc: enc, dec: dec}}
}

func (c *ServerClient) Close() {
	c.conn.Close()
}

func (s *Server) Run() error {
	_, err := os.Stat(SERVER_SOCK)
	if err != nil {
		if os.IsNotExist(err) {
			fmt.Println("Creating directory for server socket ...")
			err = os.MkdirAll(SERVER_SOCK, 0700)
			if err != nil {
				return fmt.Errorf("creating directory for server socket: %w", err)
			}
		} else {
			return fmt.Errorf("checking existence of server socket directory: %w", err)
		}
	}
	// Check if server socket exists
	log.Println("Starting server ...")

	termSignals := make(chan os.Signal, 1)
	signal.Notify(termSignals, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-termSignals
		s.Stop()
	}()

	os.Remove(SERVER_SOCK)
	// listener, err := net.Listen("tcp", "localhost:5050")
	listener, err := net.Listen("unix", SERVER_SOCK)
	if err != nil {
		return fmt.Errorf("listening on socket file %s: %w", SERVER_SOCK, err)
	}
	defer listener.Close()
	log.Println("Listening for connections ...")
	for {
		conn, err := listener.Accept()
		if err != nil {
			return fmt.Errorf("accepting client: %w\n", err)
		}
		go s.handleClient(newServerClient(conn))
	}
}

func (s *Server) Stop() {
	log.Println("Stopping server ...")
	if err := os.Remove(SERVER_SOCK); err != nil {
		log.Fatal(err)
	}
	log.Println("Stopped server")
	os.Exit(0)
}

func ensureSSHClient(host string) (*SSHClient, error) {
	// ensureSSHClient ensures that an sshClient is present for host.
	// Since, this method can be invoked by different callers concurrently,
	// and we want that an SSHClient is created only once per host, we lock
	// at the very beginning so that if two calls are performed at the same time,
	// and an SSHClient does not exist, the following happens:
	// - the first caller checks for the SSHClient's existence
	// - creates a new SSHClient since it does not exist
	// - the new SSHClient is returned
	// - the second caller checks for the SSHClient's existence
	// - the SSHClient exists and it is returned
	sshClientsLock.Lock()
	defer sshClientsLock.Unlock()
	sshClient, ok := sshClients[host]
	if ok {
		return sshClient, nil
	}
	log.Println("Creating sshClient for", host)
	sshClient, err := newSSHClient(host)
	if err != nil {
		return nil, err
	}
	sshClients[host] = sshClient
	return sshClient, nil
}

func removeSSHClient(host string) {
	sshClientsLock.Lock()
	defer sshClientsLock.Unlock()

	sshClient, ok := sshClients[host]
	if ok {
		sshClient.Close()
		delete(sshClients, host)
	}
}

func (s *Server) handleClient(client *ServerClient) {
	defer func() {
		log.Println("Closing connection with client")
		client.Close()
	}()
	var err error
	var r Request
	for {
		log.Println("Waiting...")
		err = client.dec.Decode(&r)
		if err != nil {
			log.Println("Error decoding client request", err)
			return
		}
		log.Printf("%T →", r)
		sshClient, err := ensureSSHClient(r.GetHost())
		if err != nil {
			log.Printf("Error ensuring SSHClient: %#v", err)
			r := (Response)(&ResponseBase{UnknownError, err.Error()})
			if err = client.enc.Encode(&r); err != nil {
				log.Printf("Error encoding response: %+v\n", err)
			}
			return
		}
		res := r.Do(sshClient, client)
		log.Printf("%T ←", res)
		if err = client.enc.Encode(&res); err != nil {
			log.Printf("Error encoding response: %+v\n", err)
		}
	}
}
