package outbound

import (
	"context"
	"fmt"
	"net"
	"strings"
	"sync"
)

// Direct dials TCP/UDP connections bound to a specific network interface,
// bypassing VPN default routes.
type Direct struct {
	IfaceName string // "" = auto-detect on Init()
	mu        sync.RWMutex
	iface     *net.Interface // resolved interface
	upstream  []string       // DNS upstream servers ("ip:port" or "SYSTEM")
}

// NewDirect creates a new Direct dialer with the given interface name and upstream DNS servers.
func NewDirect(ifaceName string, upstream []string) *Direct {
	return &Direct{
		IfaceName: ifaceName,
		upstream:  upstream,
	}
}

// vpnPrefixes lists interface name prefixes that are VPN/virtual/loopback.
var vpnPrefixes = []string{"utun", "tun", "tap", "lo", "veth", "docker", "br-", "virbr"}

func isPhysical(iface net.Interface) bool {
	if iface.Flags&net.FlagUp == 0 {
		return false
	}
	name := iface.Name
	for _, prefix := range vpnPrefixes {
		if strings.HasPrefix(name, prefix) {
			return false
		}
	}
	addrs, err := iface.Addrs()
	if err != nil {
		return false
	}
	for _, addr := range addrs {
		var ip net.IP
		switch v := addr.(type) {
		case *net.IPNet:
			ip = v.IP
		case *net.IPAddr:
			ip = v.IP
		}
		if ip != nil && ip.To4() != nil && !ip.IsLoopback() {
			return true
		}
	}
	return false
}

// ResolvedIfaceName returns the actual interface name used (after auto-detect).
func (d *Direct) ResolvedIfaceName() string {
	d.refreshAutoIface()
	d.mu.RLock()
	defer d.mu.RUnlock()
	if d.iface != nil {
		return d.iface.Name
	}
	return d.IfaceName
}

// Init resolves and locks the interface. Must be called before Dialer()/Resolver().
// If IfaceName is "", auto-detects the first physical interface:
//   - Has at least one IPv4 unicast address
//   - Name does not start with: utun, tun, tap, lo, veth, docker, br-, virbr
//   - Flag net.FlagUp is set
func (d *Direct) Init() error {
	if d.IfaceName != "" {
		iface, err := net.InterfaceByName(d.IfaceName)
		if err != nil {
			return fmt.Errorf("outbound: interface %q not found: %w", d.IfaceName, err)
		}
		d.mu.Lock()
		d.iface = iface
		d.mu.Unlock()
		return nil
	}

	iface, err := detectPhysicalInterface()
	if err != nil {
		return err
	}
	d.mu.Lock()
	d.iface = iface
	d.mu.Unlock()
	return nil
}

func detectPhysicalInterface() (*net.Interface, error) {
	if iface, err := defaultRouteInterface(); err == nil && iface != nil && isPhysical(*iface) {
		return iface, nil
	}
	ifaces, err := net.Interfaces()
	if err != nil {
		return nil, fmt.Errorf("outbound: failed to list interfaces: %w", err)
	}
	for _, iface := range ifaces {
		iface := iface
		if isPhysical(iface) {
			return &iface, nil
		}
	}
	return nil, fmt.Errorf("outbound: no suitable physical interface found")
}

func (d *Direct) refreshAutoIface() {
	if d.IfaceName != "" {
		return
	}
	iface, err := detectPhysicalInterface()
	if err != nil || iface == nil {
		return
	}
	d.mu.Lock()
	d.iface = iface
	d.mu.Unlock()
}

func (d *Direct) currentIface() *net.Interface {
	d.refreshAutoIface()
	d.mu.RLock()
	defer d.mu.RUnlock()
	if d.iface == nil {
		return nil
	}
	iface := *d.iface
	return &iface
}

// Dialer returns a *net.Dialer whose connections are bound to d's interface.
// If Init() was not called or iface is nil, returns a plain &net.Dialer{}.
func (d *Direct) Dialer() *net.Dialer {
	iface := d.currentIface()
	if iface == nil {
		return &net.Dialer{}
	}
	return &net.Dialer{
		Control: bindToInterface(iface.Index, iface.Name),
	}
}

// DialerWithDNS returns a dialer bound to d's interface that also resolves
// domain names via d's upstream DNS over the same bound interface.
// This prevents domain-name dials (e.g. REDIRECT targets) from going through
// the TUN fakeip DNS, which would return a fake IP unreachable via physical NIC.
func (d *Direct) DialerWithDNS() *net.Dialer {
	iface := d.currentIface()
	if iface == nil {
		return &net.Dialer{}
	}
	dialer := &net.Dialer{
		Control: bindToInterface(iface.Index, iface.Name),
	}
	// d.Resolver uses d.Dialer() (no custom resolver), so no recursion.
	for _, u := range d.upstream {
		if u != "" && u != "SYSTEM" {
			dialer.Resolver = d.Resolver(u)
			break
		}
	}
	return dialer
}

// Resolver returns a *net.Resolver whose DNS queries are sent via d's dialer.
// upstreamAddr: "ip:port". Pass "" or "SYSTEM" for the system resolver (unbound).
func (d *Direct) Resolver(upstreamAddr string) *net.Resolver {
	if upstreamAddr == "" || upstreamAddr == "SYSTEM" {
		return net.DefaultResolver
	}
	dialer := d.Dialer()
	return &net.Resolver{
		PreferGo: true,
		Dial: func(ctx context.Context, network, address string) (net.Conn, error) {
			return dialer.DialContext(ctx, "udp", upstreamAddr)
		},
	}
}
