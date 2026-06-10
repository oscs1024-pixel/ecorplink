package socks5proxy

import (
	"context"
	"errors"
	"io"
	"net"
	"strconv"
	"testing"
	"time"
)

func TestServerRelaysTCPConnectThroughProvidedDialer(t *testing.T) {
	target := startTCPEcho(t)
	dialed := make(chan string, 1)
	server, err := Start(Config{
		Enabled:  true,
		BindHost: "127.0.0.1",
		Port:     0,
	}, func(ctx context.Context, network, address string) (net.Conn, error) {
		dialed <- network + " " + address
		var d net.Dialer
		return d.DialContext(ctx, network, address)
	}, staticResolver{})
	if err != nil {
		t.Fatal(err)
	}
	defer server.Close()

	conn, err := net.Dial("tcp", server.Addr())
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	if err := socksConnect(conn, "127.0.0.1", target.Port); err != nil {
		t.Fatal(err)
	}
	if _, err := conn.Write([]byte("ping")); err != nil {
		t.Fatal(err)
	}
	buf := make([]byte, 4)
	if _, err := io.ReadFull(conn, buf); err != nil {
		t.Fatal(err)
	}
	if string(buf) != "ping" {
		t.Fatalf("echo = %q, want ping", string(buf))
	}
	select {
	case got := <-dialed:
		want := "tcp " + target.String()
		if got != want {
			t.Fatalf("dialed = %q, want %q", got, want)
		}
	default:
		t.Fatal("dialer was not called")
	}
}

func TestServerRejectsUDPAssociate(t *testing.T) {
	server, err := Start(Config{Enabled: true, BindHost: "127.0.0.1", Port: 0}, nil, staticResolver{})
	if err != nil {
		t.Fatal(err)
	}
	defer server.Close()

	conn, err := net.Dial("tcp", server.Addr())
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	if _, err := conn.Write([]byte{0x05, 0x01, 0x00}); err != nil {
		t.Fatal(err)
	}
	method := make([]byte, 2)
	if _, err := io.ReadFull(conn, method); err != nil {
		t.Fatal(err)
	}
	if method[1] != 0x00 {
		t.Fatalf("auth method = %#x, want no-auth", method[1])
	}
	req := []byte{0x05, 0x03, 0x00, 0x01, 127, 0, 0, 1, 0, 0}
	if _, err := conn.Write(req); err != nil {
		t.Fatal(err)
	}
	resp := make([]byte, 10)
	if _, err := io.ReadFull(conn, resp); err != nil {
		t.Fatal(err)
	}
	if resp[1] == 0x00 {
		t.Fatalf("UDP ASSOCIATE unexpectedly succeeded: %#v", resp)
	}
}

func TestDisabledConfigDoesNotListen(t *testing.T) {
	server, err := Start(Config{Enabled: false, BindHost: "127.0.0.1", Port: 0}, nil, staticResolver{})
	if err != nil {
		t.Fatal(err)
	}
	if server != nil {
		t.Fatal("disabled config should not start server")
	}
}

type staticResolver struct{}

func (staticResolver) Resolve(ctx context.Context, name string) (context.Context, net.IP, error) {
	ip := net.ParseIP(name)
	if ip == nil {
		return ctx, nil, errors.New("test resolver only accepts IP literals")
	}
	return ctx, ip, nil
}

func socksConnect(conn net.Conn, host string, port int) error {
	if _, err := conn.Write([]byte{0x05, 0x01, 0x00}); err != nil {
		return err
	}
	method := make([]byte, 2)
	if _, err := io.ReadFull(conn, method); err != nil {
		return err
	}
	if method[0] != 0x05 || method[1] != 0x00 {
		return errors.New("unexpected auth response")
	}
	ip := net.ParseIP(host).To4()
	if ip == nil {
		return errors.New("test helper only supports IPv4")
	}
	req := []byte{0x05, 0x01, 0x00, 0x01, ip[0], ip[1], ip[2], ip[3], byte(port >> 8), byte(port)}
	if _, err := conn.Write(req); err != nil {
		return err
	}
	resp := make([]byte, 10)
	if _, err := io.ReadFull(conn, resp); err != nil {
		return err
	}
	if resp[1] != 0x00 {
		return errors.New("connect failed with reply " + strconv.Itoa(int(resp[1])))
	}
	return nil
}

func startTCPEcho(t *testing.T) *net.TCPAddr {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { ln.Close() })
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			go func() {
				defer conn.Close()
				conn.SetDeadline(time.Now().Add(5 * time.Second)) //nolint:errcheck
				io.Copy(conn, conn)                               //nolint:errcheck
			}()
		}
	}()
	return ln.Addr().(*net.TCPAddr)
}
