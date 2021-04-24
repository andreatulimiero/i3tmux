package main

import (
	"encoding/gob"
	"fmt"
	"log"
	"net"
	"os"
	"os/signal"
	"syscall"
)

type Server struct{}

func newServer() *Server {
	return &Server{}
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
		go s.handleClient(conn)
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

var (
	sshClients = make(map[string]*SSHClient)
)

func (s *Server) handleClient(conn net.Conn) {
	defer conn.Close()
	var err error
	enc := gob.NewEncoder(conn)
	dec := gob.NewDecoder(conn)
	var r Request
	for {
		if err = dec.Decode(&r); err != nil {
			return
		}
		host := r.GetHost()
		sshClient, ok := sshClients[host]
		if !ok {
			log.Println("Creating client for", host)
			sshClient, err = newSSHClient(host)
			if err != nil {
				log.Println(fmt.Errorf("Error creating client: %w", err))
				return
			}
			sshClients[host] = sshClient
			go func() {
				sshClient.Wait()
				delete(sshClients, host) // FIXME: Implement mutex for concurrent modification
			}()
		}
		// defer sshClient.Close()
		fmt.Println(sshClient)
		fmt.Println(r)
		res := r.Do(sshClient, &Client{conn: conn, enc: enc, dec: dec})
		if err = enc.Encode(&res); err != nil {
			log.Printf("Error encoding response: %+v\n", err)
		}
	}
}
