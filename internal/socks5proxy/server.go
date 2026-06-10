package socks5proxy

import (
	"context"
	"errors"
	"fmt"
	"net"
	"strconv"
	"sync"

	socks5 "github.com/things-go/go-socks5"
)

// Config controls the local SOCKS5 listener.
type Config struct {
	Enabled  bool
	BindHost string
	Port     int
}

// Resolver matches go-socks5's resolver contract.
type Resolver interface {
	Resolve(ctx context.Context, name string) (context.Context, net.IP, error)
}

// DialFunc dials outbound connections for accepted SOCKS5 CONNECT requests.
type DialFunc func(ctx context.Context, network, address string) (net.Conn, error)

// Server owns a SOCKS5 listener.
type Server struct {
	listener net.Listener
	done     chan struct{}
	once     sync.Once
}

// Start launches a TCP-only SOCKS5 server. UDP ASSOCIATE and BIND are rejected.
func Start(cfg Config, dial DialFunc, resolver Resolver) (*Server, error) {
	if !cfg.Enabled {
		return nil, nil
	}
	if cfg.BindHost == "" {
		return nil, errors.New("socks5 bind host is empty")
	}
	if cfg.Port < 0 || cfg.Port > 65535 {
		return nil, errors.New("socks5 port must be between 0 and 65535")
	}
	if dial == nil {
		var d net.Dialer
		dial = d.DialContext
	}
	if resolver == nil {
		resolver = socks5.DNSResolver{}
	}
	addr := net.JoinHostPort(cfg.BindHost, strconv.Itoa(cfg.Port))
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return nil, fmt.Errorf("listen socks5 %s: %w", addr, err)
	}
	srv := socks5.NewServer(
		socks5.WithRule(&socks5.PermitCommand{EnableConnect: true}),
		socks5.WithDial(dial),
		socks5.WithResolver(resolver),
	)
	s := &Server{listener: ln, done: make(chan struct{})}
	go func() {
		defer close(s.done)
		if err := srv.Serve(ln); err != nil && !errors.Is(err, net.ErrClosed) {
			return
		}
	}()
	return s, nil
}

// Addr returns the listener address.
func (s *Server) Addr() string {
	if s == nil || s.listener == nil {
		return ""
	}
	return s.listener.Addr().String()
}

// Close stops accepting SOCKS5 connections.
func (s *Server) Close() error {
	if s == nil {
		return nil
	}
	var err error
	s.once.Do(func() {
		err = s.listener.Close()
		<-s.done
	})
	return err
}
