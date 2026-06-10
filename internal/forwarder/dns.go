package forwarder

import (
	"encoding/binary"
	"io"
	"log"
	"net"

	"gvisor.dev/gvisor/pkg/tcpip/adapters/gonet"
)

// handleDNSUDP processes a single UDP DNS query from the TUN.
func (f *Forwarder) handleDNSUDP(conn *gonet.UDPConn) {
	defer conn.Close()
	buf := make([]byte, 65535)
	conn.SetReadDeadline(deadlineIn(5))
	n, err := conn.Read(buf)
	if err != nil {
		return
	}
	resp, err := f.dnsServer.HandleDNS(buf[:n])
	if err != nil {
		log.Printf("[DNS/UDP] handle: %v", err)
		return
	}
	conn.Write(resp) //nolint:errcheck
}

// handleDNSTCP processes DNS-over-TCP (2-byte length-prefixed payload).
func (f *Forwarder) handleDNSTCP(conn net.Conn) {
	defer conn.Close()
	conn.SetDeadline(deadlineIn(10))

	var length uint16
	if err := binary.Read(conn, binary.BigEndian, &length); err != nil {
		return
	}
	raw := make([]byte, length)
	if _, err := io.ReadFull(conn, raw); err != nil {
		return
	}
	resp, err := f.dnsServer.HandleDNS(raw)
	if err != nil {
		return
	}
	out := make([]byte, 2+len(resp))
	binary.BigEndian.PutUint16(out, uint16(len(resp)))
	copy(out[2:], resp)
	conn.Write(out) //nolint:errcheck
}
