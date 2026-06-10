package forwarder

import (
	"log"
	"net"
	"time"

	"gvisor.dev/gvisor/pkg/tcpip/adapters/gonet"
	"gvisor.dev/gvisor/pkg/tcpip/transport/udp"
	"gvisor.dev/gvisor/pkg/waiter"
)

func (f *Forwarder) handleUDP(r *udp.ForwarderRequest) {
	id := r.ID()
	dstIP := net.IP(id.LocalAddress.AsSlice())
	dstPort := uint16(id.LocalPort)

	w := &waiter.Queue{}
	ep, err := r.CreateEndpoint(w)
	if err != nil {
		return
	}
	conn := gonet.NewUDPConn(w, ep)

	if f.shouldHijackDNS(dstIP, dstPort) {
		go f.handleDNSUDP(conn)
		return
	}
	go f.dispatchUDP(conn, dstIP, dstPort)
}

func (f *Forwarder) dispatchUDP(conn *gonet.UDPConn, dstIP net.IP, dstPort uint16) {
	defer conn.Close()
	if !f.trackConn() {
		return
	}
	defer f.untrackConn()

	target, dialer, err := f.resolve(dstIP, dstPort)
	if err != nil {
		log.Printf("[UDP] resolve %s:%d: %v", dstIP, dstPort, err)
		return
	}

	out, err := dialer.Dial("udp", target)
	if err != nil {
		log.Printf("[UDP] dial %s: %v", target, err)
		return
	}
	defer out.Close()

	const idleTimeout = 2 * time.Minute
	buf1 := make([]byte, 65535)
	buf2 := make([]byte, 65535)
	done := make(chan struct{}, 2)

	go func() {
		defer func() { done <- struct{}{} }()
		for {
			conn.SetReadDeadline(time.Now().Add(idleTimeout))
			n, err := conn.Read(buf1)
			if err != nil || f.closed.Load() {
				return
			}
			out.Write(buf1[:n]) //nolint:errcheck
		}
	}()
	go func() {
		defer func() { done <- struct{}{} }()
		for {
			out.SetReadDeadline(time.Now().Add(idleTimeout))
			n, err := out.Read(buf2)
			if err != nil || f.closed.Load() {
				return
			}
			conn.Write(buf2[:n]) //nolint:errcheck
		}
	}()
	<-done
	// close both connections so the other goroutine unblocks promptly
	conn.Close()
	out.Close()
	<-done
}
