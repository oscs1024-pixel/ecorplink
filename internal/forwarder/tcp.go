package forwarder

import (
	"context"
	"log"
	"net"
	"time"

	"gvisor.dev/gvisor/pkg/tcpip/adapters/gonet"
	"gvisor.dev/gvisor/pkg/tcpip/transport/tcp"
	"gvisor.dev/gvisor/pkg/waiter"
)

func (f *Forwarder) handleTCP(r *tcp.ForwarderRequest) {
	id := r.ID()
	dstIP := net.IP(id.LocalAddress.AsSlice())
	dstPort := uint16(id.LocalPort)

	w := &waiter.Queue{}
	ep, err := r.CreateEndpoint(w)
	if err != nil {
		r.Complete(true)
		return
	}
	r.Complete(false)
	conn := gonet.NewTCPConn(w, ep)

	if f.shouldHijackDNS(dstIP, dstPort) {
		go f.handleDNSTCP(conn)
		return
	}
	go f.dispatchTCP(conn, dstIP, dstPort)
}

func (f *Forwarder) dispatchTCP(conn net.Conn, dstIP net.IP, dstPort uint16) {
	defer conn.Close()
	if !f.trackConn() {
		log.Printf("[TCP] max connections reached")
		return
	}
	defer f.untrackConn()

	target, dialer, err := f.resolve(dstIP, dstPort)
	if err != nil {
		log.Printf("[TCP] resolve %s:%d: %v", dstIP, dstPort, err)
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	out, err := dialer.DialContext(ctx, "tcp", target)
	if err != nil {
		log.Printf("[TCP] dial %s: %v", target, err)
		return
	}
	defer out.Close()

	if tc, ok := out.(*net.TCPConn); ok {
		tc.SetKeepAlive(true)
		tc.SetKeepAlivePeriod(3 * time.Minute)
	}
	relay(conn, out, &f.closed)
}
