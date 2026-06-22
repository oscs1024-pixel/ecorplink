package forwarder

import (
	"context"
	"fmt"
	"log"
	"net"
	"net/netip"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"golang.zx2c4.com/wireguard/tun"
	"gvisor.dev/gvisor/pkg/buffer"
	"gvisor.dev/gvisor/pkg/tcpip"
	"gvisor.dev/gvisor/pkg/tcpip/header"
	"gvisor.dev/gvisor/pkg/tcpip/link/channel"
	"gvisor.dev/gvisor/pkg/tcpip/network/ipv4"
	"gvisor.dev/gvisor/pkg/tcpip/network/ipv6"
	"gvisor.dev/gvisor/pkg/tcpip/stack"
	"gvisor.dev/gvisor/pkg/tcpip/transport/tcp"
	"gvisor.dev/gvisor/pkg/tcpip/transport/udp"

	"ecorplink/internal/fakeip"
	"ecorplink/internal/outbound"
	"ecorplink/internal/routetable"
	"ecorplink/internal/rule"
)

const nicID = 1

// Config holds forwarder configuration.
type Config struct {
	TUNName     string
	TUNIP       string
	TUNMask     int
	TUNMTU      int
	MaxConns    int
	UpstreamDNS string // first upstream addr for DIRECT outbound's resolver
	DefaultDNS  string // upstream addr for DEFAULT/SYSTEM domain resolution
	DNSHijack   []string
}

// Forwarder manages the netstack and dispatches connections via the rule engine.
type Forwarder struct {
	stack      *stack.Stack
	config     Config
	linkEP     *channel.Endpoint
	wg         sync.WaitGroup
	closed     atomic.Bool
	cancelLoop context.CancelFunc
	connCount  atomic.Int64
	maxConns   int
	tunDev     tun.Device

	dnsServer   *fakeip.Server
	pool        *fakeip.Pool
	engine      *rule.Engine
	outbounds   map[string]*outbound.Direct
	routeTable  *routetable.RouteTable
	ifaceCache  sync.Map // string → *outbound.Direct
	routeCache  sync.Map // ip string → struct{}
	dnsHijack   []netip.AddrPort
	vpnOutbound packetDialer
	vpnMu       sync.RWMutex
	defaultVPN  atomic.Bool
}

func NewForwarder(cfg Config, dnsServer *fakeip.Server, pool *fakeip.Pool,
	engine *rule.Engine, outs map[string]*outbound.Direct,
	rt *routetable.RouteTable) (*Forwarder, error) {

	f := &Forwarder{
		config:     cfg,
		maxConns:   cfg.MaxConns,
		dnsServer:  dnsServer,
		pool:       pool,
		engine:     engine,
		outbounds:  outs,
		routeTable: rt,
	}
	f.dnsHijack = parseDNSHijack(cfg.DNSHijack)

	// Flush host-route and interface caches whenever the system routing table
	// changes so we don't forward traffic on a stale interface.
	if rt != nil {
		rt.OnRefresh(func() {
			f.routeCache.Range(func(k, _ any) bool {
				f.routeCache.Delete(k)
				return true
			})
			f.ifaceCache.Range(func(k, _ any) bool {
				f.ifaceCache.Delete(k)
				return true
			})
		})
	}

	s := stack.New(stack.Options{
		NetworkProtocols:   []stack.NetworkProtocolFactory{ipv4.NewProtocol, ipv6.NewProtocol},
		TransportProtocols: []stack.TransportProtocolFactory{tcp.NewProtocolCUBIC, udp.NewProtocol},
	})
	f.stack = s

	// TCP tuning: SACK is enabled by default; also enable receive-buffer auto-tuning
	// and shorten the minimum RTO so the stack recovers from losses faster.
	moderateBuf := tcpip.TCPModerateReceiveBufferOption(true)
	s.SetTransportProtocolOption(tcp.ProtocolNumber, &moderateBuf) //nolint:errcheck
	minRTO := tcpip.TCPMinRTOOption(20 * time.Millisecond)
	s.SetTransportProtocolOption(tcp.ProtocolNumber, &minRTO) //nolint:errcheck

	mtu := cfg.TUNMTU
	if mtu == 0 {
		mtu = 1420
	}
	f.linkEP = channel.New(4096, uint32(mtu), "")
	if err := s.CreateNIC(nicID, f.linkEP); err != nil {
		return nil, fmt.Errorf("create nic: %s", err)
	}
	s.SetPromiscuousMode(nicID, true) //nolint:errcheck
	s.SetSpoofing(nicID, true)        //nolint:errcheck
	s.AddRoute(tcpip.Route{Destination: header.IPv4EmptySubnet, NIC: nicID})
	s.AddRoute(tcpip.Route{Destination: header.IPv6EmptySubnet, NIC: nicID})

	return f, nil
}

func parseDNSHijack(values []string) []netip.AddrPort {
	out := make([]netip.AddrPort, 0, len(values))
	for _, value := range values {
		if _, after, ok := strings.Cut(value, "://"); ok {
			value = after
		}
		value = strings.Replace(value, "any", "0.0.0.0", 1)
		addrPort, err := netip.ParseAddrPort(value)
		if err == nil {
			out = append(out, addrPort)
		}
	}
	return out
}

func (f *Forwarder) shouldHijackDNS(ip net.IP, port uint16) bool {
	if len(f.dnsHijack) == 0 {
		return false
	}
	addr, ok := netip.AddrFromSlice(ip)
	if !ok {
		return false
	}
	target := netip.AddrPortFrom(addr.Unmap(), port)
	for _, hijack := range f.dnsHijack {
		if hijack == target || (hijack.Addr().IsUnspecified() && hijack.Port() == target.Port()) {
			return true
		}
	}
	return false
}

func (f *Forwarder) Start(tunDev tun.Device) {
	f.tunDev = tunDev
	ctx, cancel := context.WithCancel(context.Background())
	f.cancelLoop = cancel
	f.wg.Add(2)
	go func() { defer f.wg.Done(); f.tunReadLoop(tunDev) }()
	go func() { defer f.wg.Done(); f.tunWriteLoop(ctx) }()

	tcpFwd := tcp.NewForwarder(f.stack, 0, 4096, f.handleTCP)
	f.stack.SetTransportProtocolHandler(tcp.ProtocolNumber, tcpFwd.HandlePacket)
	udpFwd := udp.NewForwarder(f.stack, f.handleUDP)
	f.stack.SetTransportProtocolHandler(udp.ProtocolNumber, udpFwd.HandlePacket)
}

func (f *Forwarder) Close() {
	f.closed.Store(true)
	if f.cancelLoop != nil {
		f.cancelLoop()
	}
	if f.tunDev != nil {
		f.tunDev.Close()
	}
	f.stack.Close()
	f.wg.Wait()
	f.routeCache.Range(func(key, value any) bool {
		ip := key.(string)
		deleteHostRoute(ip) //nolint:errcheck
		forgetHostRoute(ip)
		return true
	})
}

// ConnCount returns the current number of active connections.
func (f *Forwarder) ConnCount() int {
	return int(f.connCount.Load())
}

// SetVPNOutbound sets the VPN outbound dialer. Pass nil to clear.
func (f *Forwarder) SetVPNOutbound(d packetDialer) {
	f.vpnMu.Lock()
	f.vpnOutbound = d
	f.vpnMu.Unlock()
}

// SetDefaultVPN controls whether unmatched traffic falls through to VPN (true)
// or to the direct/route-table path (false, default).
func (f *Forwarder) SetDefaultVPN(v bool) {
	f.defaultVPN.Store(v)
}

func (f *Forwarder) getVPNOutbound() packetDialer {
	f.vpnMu.RLock()
	defer f.vpnMu.RUnlock()
	return f.vpnOutbound
}

func (f *Forwarder) trackConn() bool {
	if f.maxConns <= 0 {
		return true
	}
	if int(f.connCount.Add(1)) > f.maxConns {
		f.connCount.Add(-1)
		return false
	}
	return true
}

func (f *Forwarder) untrackConn() {
	if f.maxConns > 0 {
		f.connCount.Add(-1)
	}
}

// dialerForIface returns a bound dialer for ifaceName, caching the result.
func (f *Forwarder) dialerForIface(ifaceName string) *net.Dialer {
	if v, ok := f.ifaceCache.Load(ifaceName); ok {
		return v.(*outbound.Direct).Dialer()
	}
	d := outbound.NewDirect(ifaceName, nil)
	if err := d.Init(); err != nil {
		log.Printf("[warn] dialerForIface %s: %v — using unbound dialer", ifaceName, err)
	}
	f.ifaceCache.Store(ifaceName, d)
	return d.Dialer()
}

// resolve decides the dial target and dialer for an inbound connection.
// Returns target "host:port", dialer, error.
func (f *Forwarder) resolve(dstIP net.IP, dstPort uint16) (target string, dialer packetDialer, err error) {
	port := strconv.Itoa(int(dstPort))

	if f.pool != nil && f.pool.InPool(dstIP) {
		domain, ok := f.pool.Lookup(dstIP)
		if !ok {
			return "", nil, fmt.Errorf("fakeip: no mapping for %s", dstIP)
		}
		if f.engine != nil {
			action, matched := f.engine.MatchDomain(domain)
			if !matched {
				if f.defaultVPN.Load() {
					vpnAction := &rule.Action{Type: rule.ActionVPN, Outbound: "VPN"}
					return f.actionToTargetWithLog(vpnAction, domain, port, "default")
				}
				target, dialer, err := f.defaultDomainTarget(domain, port)
				logForwardDecision(domain, "DIRECT", "default", target, err)
				return target, dialer, err
			}
			return f.actionToTargetWithLog(action, domain, port, "rule")
		}
		if f.defaultVPN.Load() {
			vpnAction := &rule.Action{Type: rule.ActionVPN, Outbound: "VPN"}
			return f.actionToTargetWithLog(vpnAction, domain, port, "default")
		}
		target, dialer, err := f.defaultDomainTarget(domain, port)
		logForwardDecision(domain, "DIRECT", "default", target, err)
		return target, dialer, err
	}

	// Real IP: try IP rules first
	if f.engine != nil {
		if action, ok := f.engine.MatchIP(dstIP); ok {
			return f.actionToTarget(action, dstIP.String(), port)
		}
	}

	// Fallback: full-tunnel mode sends to VPN, otherwise route table.
	if f.defaultVPN.Load() {
		vpnAction := &rule.Action{Type: rule.ActionVPN, Outbound: "VPN"}
		return f.actionToTarget(vpnAction, dstIP.String(), port)
	}
	if f.routeTable != nil {
		ifaceName, gateway, err := f.routeTable.Lookup(dstIP)
		if err != nil {
			return "", nil, fmt.Errorf("no route for %s: %w", dstIP, err)
		}
		f.ensureHostRoute(dstIP, ifaceName, gateway)
		return net.JoinHostPort(dstIP.String(), port), f.dialerForIface(ifaceName), nil
	}

	return net.JoinHostPort(dstIP.String(), port), &net.Dialer{}, nil
}

func (f *Forwarder) defaultDomainTarget(domain, port string) (string, packetDialer, error) {
	base := &net.Dialer{}
	if f.config.DefaultDNS != "" {
		base.Resolver = f.routeDNSResolver(f.config.DefaultDNS)
	}
	dialer := &hostRouteDialer{
		base: base,
		ensure: func(ip net.IP) {
			f.ensureDefaultHostRoute(ip)
		},
	}
	return net.JoinHostPort(domain, port), dialer, nil
}

func (f *Forwarder) routeDNSResolver(upstream string) *net.Resolver {
	return &net.Resolver{
		PreferGo: true,
		Dial: func(ctx context.Context, network, address string) (net.Conn, error) {
			// Always use the DIRECT outbound's interface-bound dialer for DNS,
			// so upstream queries bypass the TUN capture routes entirely.
			if ob := f.outbounds["DIRECT"]; ob != nil {
				return ob.Dialer().DialContext(ctx, network, upstream)
			}
			return (&net.Dialer{}).DialContext(ctx, network, upstream)
		},
	}
}

func (f *Forwarder) actionToTarget(action *rule.Action, host, port string) (string, packetDialer, error) {
	switch action.Type {
	case rule.ActionDirect:
		ob := f.outbounds[action.Outbound]
		if ob == nil {
			ob = f.outbounds["DIRECT"]
		}
		if ob == nil {
			return "", nil, fmt.Errorf("outbound %q not found", action.Outbound)
		}
		baseDialer := ob.DialerWithDNS()
		var dialer packetDialer = baseDialer
		if ip := net.ParseIP(host); ip != nil {
			f.ensureScopedHostRoute(ip, ob.ResolvedIfaceName())
		} else {
			f.ensureOutboundDNSRoute(ob)
			dialer = &hostRouteDialer{
				base: baseDialer,
				ensure: func(ip net.IP) {
					f.ensureScopedHostRoute(ip, ob.ResolvedIfaceName())
				},
			}
		}
		return net.JoinHostPort(host, port), dialer, nil
	case rule.ActionVPN:
		vpn := f.getVPNOutbound()
		if vpn == nil {
			// VPN not connected: fall through to DIRECT
			ob := f.outbounds["DIRECT"]
			if ob == nil {
				return "", nil, fmt.Errorf("no VPN outbound and no DIRECT outbound")
			}
			return net.JoinHostPort(host, port), ob.DialerWithDNS(), nil
		}
		return net.JoinHostPort(host, port), vpn, nil
	default:
		return "", nil, fmt.Errorf("unknown action type %d", action.Type)
	}
}

func (f *Forwarder) actionToTargetWithLog(action *rule.Action, host, port, source string) (string, packetDialer, error) {
	target, dialer, err := f.actionToTarget(action, host, port)
	policy := "DIRECT"
	if action.Type == rule.ActionVPN {
		policy = "VPN"
	}
	logForwardDecision(host, policy, source, target, err)
	return target, dialer, err
}

func logForwardDecision(host, policy, source, target string, err error) {
	if err != nil {
		log.Printf("[route] domain=%s policy=%s source=%s error=%v", host, policy, source, err)
		return
	}
	log.Printf("[route] domain=%s policy=%s source=%s target=%s", host, policy, source, target)
}

func (f *Forwarder) defaultIPTarget(ip net.IP, port string) (string, packetDialer, error) {
	if f.routeTable != nil {
		ifaceName, gateway, err := f.routeTable.Lookup(ip)
		if err != nil {
			return "", nil, fmt.Errorf("no route for %s: %w", ip, err)
		}
		f.ensureHostRoute(ip, ifaceName, gateway)
		return net.JoinHostPort(ip.String(), port), f.dialerForIface(ifaceName), nil
	}
	return net.JoinHostPort(ip.String(), port), &net.Dialer{}, nil
}

func (f *Forwarder) ensureOutboundDNSRoute(ob *outbound.Direct) {
	host, _, err := net.SplitHostPort(f.config.UpstreamDNS)
	if err != nil {
		return
	}
	ip := net.ParseIP(host)
	if ip == nil {
		return
	}
	f.ensureScopedHostRoute(ip, ob.ResolvedIfaceName())
}

func (f *Forwarder) ensureHostRoute(ip net.IP, iface string, gateway net.IP) {
	if ip == nil || ip.To4() == nil {
		return
	}
	key := ip.String()
	if _, loaded := f.routeCache.Load(key); loaded {
		if current, ok := hostRouteCurrent(key, iface, gateway); ok && current {
			return
		}
		f.routeCache.Delete(key)
	}
	if err := addHostRoute(key, iface, gateway); err != nil {
		log.Printf("[warn] host route %s via %s: %v", key, iface, err)
		return
	}
	f.routeCache.Store(key, struct{}{})
	rememberHostRoute(ip)
}

func (f *Forwarder) ensureDefaultHostRoute(ip net.IP) {
	if ip == nil || ip.To4() == nil || f.routeTable == nil {
		return
	}
	iface, gateway, err := f.routeTable.Lookup(ip)
	if err != nil {
		log.Printf("[warn] default host route %s: %v", ip, err)
		return
	}
	f.ensureHostRoute(ip, iface, gateway)
}

func (f *Forwarder) ensureScopedHostRoute(ip net.IP, iface string) {
	if ip == nil || ip.To4() == nil {
		return
	}
	key := ip.String()
	if _, loaded := f.routeCache.Load(key); loaded {
		if current, ok := scopedHostRouteCurrent(key, iface); ok && current {
			return
		}
		f.routeCache.Delete(key)
	}
	if err := addScopedHostRoute(key, iface); err != nil {
		log.Printf("[warn] scoped host route %s via %s: %v", key, iface, err)
		return
	}
	f.routeCache.Store(key, struct{}{})
	rememberHostRoute(ip)
}

func (f *Forwarder) tunReadLoop(dev tun.Device) {
	mtu := f.config.TUNMTU
	if mtu == 0 {
		mtu = 1420
	}
	buf := make([]byte, mtu+4)
	for !f.closed.Load() {
		bufs := [][]byte{buf}
		sizes := []int{0}
		_, err := dev.Read(bufs, sizes, 4)
		if err != nil {
			if !f.closed.Load() {
				log.Printf("[tun] read error: %v", err)
			}
			return
		}
		n := sizes[0]
		if n == 0 {
			continue
		}
		pkt := make([]byte, n)
		copy(pkt, buf[4:4+n])

		// ICMP packets are not handled by gvisor's TCP/UDP forwarders.
		// Without interception, gvisor returns ICMP unreachable which ping
		// reports as "No route to host". Add a host route for the destination
		// and drop the packet so subsequent ICMP bypasses the TUN device.
		if len(pkt) >= 20 && pkt[0]>>4 == 4 && pkt[9] == 1 {
			f.handleICMPv4(pkt)
			continue
		}

		var proto tcpip.NetworkProtocolNumber
		switch (pkt[0] >> 4) & 0x0f {
		case 4:
			proto = header.IPv4ProtocolNumber
		case 6:
			proto = header.IPv6ProtocolNumber
		default:
			continue
		}
		pb := stack.NewPacketBuffer(stack.PacketBufferOptions{Payload: buffer.MakeWithData(pkt)})
		f.linkEP.InjectInbound(proto, pb)
		pb.DecRef()
	}
}

// handleICMPv4 handles an incoming IPv4 ICMP packet. For echo requests (type 8)
// destined for a DIRECT-policy IP, it adds a host route and drops the packet.
// The first echo will time out; subsequent ones bypass the TUN via the host route.
func (f *Forwarder) handleICMPv4(pkt []byte) {
	if len(pkt) < 20 {
		return
	}
	// IPv4 header length is IHL*4 (IHL is the lower nibble of byte 0).
	ihl := int(pkt[0]&0x0f) * 4
	if ihl < 20 || len(pkt) < ihl+8 { // need at least ICMP header (8 bytes) after IP header
		return
	}
	if pkt[ihl] != 8 { // ICMP echo request type = 8
		return
	}
	dstIP := net.IP(pkt[16:20])
	if f.engine != nil {
		action, ok := f.engine.MatchIP(dstIP)
		if !ok {
			f.ensureDefaultHostRoute(dstIP)
			return
		}
		switch action.Type {
		case rule.ActionDirect:
			ob := f.outbounds[action.Outbound]
			if ob == nil {
				ob = f.outbounds["DIRECT"]
			}
			if ob != nil {
				f.ensureScopedHostRoute(dstIP, ob.ResolvedIfaceName())
			}
		case rule.ActionVPN:
			// ICMP through VPN: no host route needed, packet goes through WG tunnel
			// Do nothing — ICMP to VPN destinations will time out on first packet
			// but subsequent ones will work through the WG netstack
		}
	} else {
		f.ensureDefaultHostRoute(dstIP)
	}
}

func (f *Forwarder) tunWriteLoop(ctx context.Context) {
	// Pool for the 4-byte TUN header prefix + packet body. Each packet has a
	// different size so we pool *[]byte pointers and re-slice as needed.
	pool := &sync.Pool{New: func() any {
		b := make([]byte, 0, f.config.TUNMTU+4)
		return &b
	}}
	for {
		pkt := f.linkEP.ReadContext(ctx)
		if pkt == nil {
			return
		}
		view := pkt.ToView()
		data := view.AsSlice()
		if len(data) > 0 {
			bufPtr := pool.Get().(*[]byte)
			need := len(data) + 4
			if cap(*bufPtr) < need {
				*bufPtr = make([]byte, need)
			}
			out := (*bufPtr)[:need]
			out[0], out[1], out[2], out[3] = 0, 0, 0, 0
			copy(out[4:], data)
			f.tunDev.Write([][]byte{out}, 4) //nolint:errcheck
			pool.Put(bufPtr)
		}
		view.Release()
		pkt.DecRef()
	}
}
