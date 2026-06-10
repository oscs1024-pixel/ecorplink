package main

import (
	"encoding/binary"
	"io"
	"log"
	"net"
	"time"

	"ecorplink/internal/fakeip"
)

type localDNSServer struct {
	addr    string
	dns     *fakeip.Server
	udp     *net.UDPConn
	tcp     net.Listener
	workers chan struct{} // bounded concurrency for DNS handlers
}

const dnsWorkerLimit = 256

type localDNSServers []*localDNSServer

func startLocalDNSAll(addrs []string, dnsServer *fakeip.Server) (localDNSServers, error) {
	var servers localDNSServers
	for _, addr := range addrs {
		s, err := startLocalDNS(addr, dnsServer)
		if err != nil {
			servers.Close()
			return nil, err
		}
		servers = append(servers, s)
	}
	return servers, nil
}

func (servers localDNSServers) Close() {
	for _, s := range servers {
		s.Close()
	}
}

func startLocalDNS(addr string, dnsServer *fakeip.Server) (*localDNSServer, error) {
	udpAddr, err := net.ResolveUDPAddr("udp", addr)
	if err != nil {
		return nil, err
	}
	udpConn, err := net.ListenUDP("udp", udpAddr)
	if err != nil {
		return nil, err
	}
	tcpLn, err := net.Listen("tcp", addr)
	if err != nil {
		udpConn.Close()
		return nil, err
	}

	s := &localDNSServer{addr: addr, dns: dnsServer, udp: udpConn, tcp: tcpLn, workers: make(chan struct{}, dnsWorkerLimit)}
	go s.serveUDP()
	go s.serveTCP()
	return s, nil
}

func (s *localDNSServer) Close() {
	if s.udp != nil {
		s.udp.Close()
	}
	if s.tcp != nil {
		s.tcp.Close()
	}
}

func (s *localDNSServer) serveUDP() {
	buf := make([]byte, 65535)
	for {
		n, client, err := s.udp.ReadFromUDP(buf)
		if err != nil {
			return
		}
		raw := make([]byte, n)
		copy(raw, buf[:n])
		select {
		case s.workers <- struct{}{}:
		default:
			// worker pool full — drop packet rather than spawn unbounded goroutines
			log.Printf("[DNS/local UDP] worker pool full, dropping query from %s", client)
			continue
		}
		go func() {
			defer func() { <-s.workers }()
			resp, err := s.dns.HandleDNS(raw)
			if err != nil {
				log.Printf("[DNS/local UDP] handle: %v", err)
			}
			if len(resp) > 0 {
				s.udp.WriteToUDP(resp, client) //nolint:errcheck
			}
		}()
	}
}

func (s *localDNSServer) serveTCP() {
	for {
		conn, err := s.tcp.Accept()
		if err != nil {
			return
		}
		go s.handleTCP(conn)
	}
}

func (s *localDNSServer) handleTCP(conn net.Conn) {
	defer conn.Close()
	conn.SetDeadline(time.Now().Add(10 * time.Second)) //nolint:errcheck

	var length uint16
	if err := binary.Read(conn, binary.BigEndian, &length); err != nil {
		return
	}
	raw := make([]byte, length)
	if _, err := io.ReadFull(conn, raw); err != nil {
		return
	}
	resp, err := s.dns.HandleDNS(raw)
	if err != nil {
		log.Printf("[DNS/local TCP] handle: %v", err)
	}
	if len(resp) == 0 {
		return
	}
	out := make([]byte, 2+len(resp))
	binary.BigEndian.PutUint16(out, uint16(len(resp)))
	copy(out[2:], resp)
	conn.Write(out) //nolint:errcheck
}
