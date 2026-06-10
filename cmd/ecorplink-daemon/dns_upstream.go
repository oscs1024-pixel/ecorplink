package main

import (
	"fmt"
	"log"
	"net"
	"strings"
	"sync"
	"time"

	"ecorplink/internal/outbound"
	"ecorplink/internal/routetable"
	"github.com/miekg/dns"
)

// routeUpstream implements fakeip.Upstream using the monitored system route
// table. It is used for unmatched domains, whose DNS should follow DEFAULT /
// SYSTEM routing instead of the physical DIRECT outbound.
type routeUpstream struct {
	rt     *routetable.RouteTable
	addr   string
	direct *outbound.Direct // physical interface fallback when tunnel route is selected

	mu    sync.Mutex
	cache map[string]*outbound.Direct // iface name → cached Direct
}

func newRouteUpstream(rt *routetable.RouteTable, addr string, direct *outbound.Direct) *routeUpstream {
	return &routeUpstream{
		rt:     rt,
		addr:   addr,
		direct: direct,
		cache:  make(map[string]*outbound.Direct),
	}
}

func isTunnelIfaceName(name string) bool {
	return strings.HasPrefix(name, "utun") ||
		strings.HasPrefix(name, "tun") ||
		strings.HasPrefix(name, "tap")
}

func (u *routeUpstream) Exchange(msg *dns.Msg) (*dns.Msg, error) {
	host, _, err := net.SplitHostPort(u.addr)
	if err != nil {
		return nil, fmt.Errorf("dns upstream %q: %w", u.addr, err)
	}
	ip := net.ParseIP(host)
	if ip == nil {
		return nil, fmt.Errorf("dns upstream %q is not an IP address", host)
	}
	iface, _, err := u.rt.Lookup(ip)
	if err != nil {
		return nil, fmt.Errorf("dns upstream route lookup %s: %w", ip, err)
	}

	// When the route to the DNS server goes through a VPN/tunnel interface,
	// the tunnel may not forward arbitrary DNS queries (e.g. split-tunnel VPNs).
	// Use the physical interface directly instead.
	var d *outbound.Direct
	if isTunnelIfaceName(iface) {
		d = u.direct
	} else {
		d = u.getOrCreate(iface)
	}

	client := &dns.Client{Net: "udp", Dialer: d.Dialer(), Timeout: 5 * time.Second}
	resp, _, err := client.Exchange(msg, u.addr)
	return resp, err
}

func (u *routeUpstream) getOrCreate(iface string) *outbound.Direct {
	u.mu.Lock()
	if d, ok := u.cache[iface]; ok {
		u.mu.Unlock()
		return d
	}
	u.mu.Unlock()

	// Init() does a syscall — keep it outside the lock to avoid serializing
	// concurrent DNS queries while one goroutine waits for interface resolution.
	d := outbound.NewDirect(iface, nil)
	if err := d.Init(); err != nil {
		log.Printf("[dns upstream] iface %s init: %v — using physical fallback", iface, err)
		return u.direct
	}

	u.mu.Lock()
	if existing, ok := u.cache[iface]; ok {
		// Another goroutine raced and already cached; use its result.
		u.mu.Unlock()
		return existing
	}
	u.cache[iface] = d
	u.mu.Unlock()
	return d
}
