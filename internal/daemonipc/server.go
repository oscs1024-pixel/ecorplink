package daemonipc

import (
	"bufio"
	"encoding/json"
	"log"
	"net"
	"os"
	"path/filepath"
	"sync"
)

// Handler processes a command and returns a response.
type Handler func(cmd Cmd) Response

// Server listens on a Unix socket and dispatches commands.
type Server struct {
	path    string
	handler Handler
	ln      net.Listener
	wg      sync.WaitGroup
}

// NewServer creates a server that will listen on socketPath.
func NewServer(socketPath string, handler Handler) *Server {
	return &Server{path: socketPath, handler: handler}
}

// Start begins listening. Non-blocking.
func (s *Server) Start() error {
	os.Remove(s.path) //nolint:errcheck
	ln, err := net.Listen("unix", s.path)
	if err != nil {
		return err
	}
	// Owner-only: this socket accepts privileged daemon actions.
	setSocketOwnerToDirOwner(s.path) //nolint:errcheck
	os.Chmod(s.path, 0600)           //nolint:errcheck
	s.ln = ln
	s.wg.Add(1)
	go s.accept()
	return nil
}

// Stop closes the listener and waits for connections to finish.
func (s *Server) Stop() {
	if s.ln != nil {
		s.ln.Close()
	}
	s.wg.Wait()
}

func (s *Server) accept() {
	defer s.wg.Done()
	for {
		conn, err := s.ln.Accept()
		if err != nil {
			return
		}
		s.wg.Add(1)
		go func(c net.Conn) {
			defer s.wg.Done()
			defer c.Close()
			s.handle(c)
		}(conn)
	}
}

func (s *Server) handle(conn net.Conn) {
	if err := validatePeer(conn, filepath.Dir(s.path)); err != nil {
		log.Printf("[daemonipc] rejected peer: %v", err)
		return
	}
	scanner := bufio.NewScanner(conn)
	enc := json.NewEncoder(conn)
	for scanner.Scan() {
		var cmd Cmd
		if err := json.Unmarshal(scanner.Bytes(), &cmd); err != nil {
			enc.Encode(Response{OK: false, Error: "invalid JSON: " + err.Error()}) //nolint:errcheck
			continue
		}
		resp := s.handler(cmd)
		if err := enc.Encode(resp); err != nil {
			log.Printf("[daemonipc] write response: %v", err)
			return
		}
	}
}
