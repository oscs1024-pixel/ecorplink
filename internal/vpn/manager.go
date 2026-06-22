// internal/vpn/manager.go
package vpn

import (
	"context"
	"fmt"
	"log"
	"net"
	"slices"
	"strings"
	"sync"
	"time"

	"ecorplink/internal/config"
	"ecorplink/internal/fakeip"
	"ecorplink/internal/forwarder"
	"ecorplink/internal/geoip"
	"ecorplink/internal/outbound"
	"ecorplink/internal/router"
	"ecorplink/internal/routetable"
	"ecorplink/internal/rule"
	"ecorplink/internal/socks5proxy"
	"ecorplink/internal/tun"
	"ecorplink/internal/wgdevice"
)

// ConnectConfig holds everything needed to connect to a VPN node.
type ConnectConfig struct {
	WG wgdevice.Config
	// Routes injected into rule engine after connection (from VPN server)
	SplitRoutes    []string // CIDRs to route via VPN (e.g. "10.0.0.0/8")
	DomainSuffixes []string // domain suffixes to route via VPN
	// For server endpoint route exception
	PhysicalIface string
	ServerIP      string // extracted from WG.ServerEndpoint
	// FollowSplitRoutes: when true (default), server split routes filter VPN
	// traffic (split-tunnel). When false, all traffic goes through VPN (full-tunnel).
	FollowSplitRoutes bool
}

// Status describes the current VPN connection state.
type Status struct {
	Connected    bool
	Reconnecting bool // true while auto-reconnect is in progress
	NodeName     string
	VpnIP        string
	DNS          string // primary DNS server IP
	Protocol     string // "TCP" or "UDP"
	ConnectedAt  int64  // unix timestamp when connection was established
}

// Manager coordinates TUN + WireGuard + forwarder lifecycle.
// Connect starts everything; Disconnect tears it all down.
type Manager struct {
	cfg *config.Config

	mu                       sync.Mutex
	active                   bool
	reconnecting             bool
	currentFollowSplitRoutes bool // tracks current split/full-tunnel mode
	status                   Status

	// stored for real-time FollowSplitRoutes updates
	lastSplitRoutes    []string
	lastDomainSuffixes []string
	lastRxBytes        int64 // last observed rx_bytes for dead-tunnel detection

	tunDev *tun.Device
	wgDev  *wgdevice.Device
	fwd    *forwarder.Forwarder
	eng    *rule.Engine
	rt     *routetable.RouteTable
	rm     *router.RouteManager
	socks5 *socks5proxy.Server
}

// New creates a Manager for the given config.
func New(cfg *config.Config) *Manager {
	return &Manager{cfg: cfg}
}

// Connect starts the TUN, creates the WireGuard device, and starts the forwarder.
// If already connected, the existing connection is torn down first.
func (m *Manager) Connect(nodeName string, cc ConnectConfig) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.active {
		// tear down existing connection first
		m.cleanupLocked()
	}
	if err := m.startLocked(nodeName, cc); err != nil {
		m.cleanupLocked()
		return err
	}
	m.active = true
	proto := "UDP"
	if cc.WG.ProtocolMode == 1 {
		proto = "TCP"
	}
	dns := ""
	if len(cc.WG.DNSServers) > 0 {
		dns = cc.WG.DNSServers[0].String()
	}
	m.lastSplitRoutes = cc.SplitRoutes
	m.lastDomainSuffixes = cc.DomainSuffixes
	m.status = Status{
		Connected:   true,
		NodeName:    nodeName,
		VpnIP:       cc.WG.VpnIP.String(),
		DNS:         dns,
		Protocol:    proto,
		ConnectedAt: time.Now().Unix(),
	}
	return nil
}

// Disconnect tears down the VPN connection.
func (m *Manager) Disconnect() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.cleanupLocked()
	m.active = false
	m.status = Status{}
	return nil
}

// GetStatus returns the current connection state.
func (m *Manager) GetStatus() Status {
	m.mu.Lock()
	defer m.mu.Unlock()
	s := m.status
	s.Reconnecting = m.reconnecting
	return s
}

// SetReconnecting marks whether auto-reconnect is in progress.
func (m *Manager) SetReconnecting(v bool) {
	m.mu.Lock()
	m.reconnecting = v
	m.mu.Unlock()
}

// GetStats returns cumulative WireGuard byte counters, or zeros if not connected.
func (m *Manager) GetStats() wgdevice.Stats {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.wgDev == nil {
		return wgdevice.Stats{}
	}
	s, _ := m.wgDev.GetStats()
	return s
}

// SetFollowSplitRoutes switches between split-tunnel and full-tunnel mode at
// runtime without reconnecting. Has no effect when not connected.
func (m *Manager) SetFollowSplitRoutes(v bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if !m.active || m.eng == nil || m.fwd == nil {
		return
	}
	m.currentFollowSplitRoutes = v
	m.eng.ClearInjectedRules()
	var injected []string
	if v {
		for _, cidr := range m.lastSplitRoutes {
			injected = append(injected, "IP-CIDR,"+cidr+",VPN")
		}
		for _, suffix := range m.lastDomainSuffixes {
			injected = append(injected, "DOMAIN-SUFFIX,"+suffix+",VPN")
		}
		m.fwd.SetDefaultVPN(false)
	} else {
		injected = append(injected, "IP-CIDR,0.0.0.0/0,VPN")
		m.fwd.SetDefaultVPN(true)
	}
	if len(injected) > 0 {
		if err := m.eng.InjectRules(injected); err != nil {
			log.Printf("[vpn] set follow_split_routes=%v inject: %v", v, err)
		}
	}
	log.Printf("[vpn] follow_split_routes=%v applied", v)
}

// UpdateSOCKS5 applies SOCKS5 listener settings without reconnecting the VPN.
// If the tunnel is not active, the new config is stored and used on next connect.
func (m *Manager) UpdateSOCKS5(cfg config.SOCKS5Config) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.cfg.SOCKS5 = cfg
	if !m.active {
		return nil
	}
	if m.socks5 != nil {
		m.socks5.Close() //nolint:errcheck
		m.socks5 = nil
	}
	if !cfg.Enabled {
		return nil
	}
	if m.wgDev == nil {
		return fmt.Errorf("wireguard device is not active")
	}
	return m.startSOCKS5Locked(wgdevice.NewOutbound(m.wgDev).DialContext, vpnResolver{resolver: m.wgDev.VPNResolver()})
}

// UpdateSplitRoutes applies updated routing settings received from the VPN server
// (e.g. via keep-alive response). It is a no-op when nothing changed or when the
// VPN is in full-tunnel mode (FollowSplitRoutes=false).
func (m *Manager) UpdateSplitRoutes(routes, suffixes []string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if !m.active || m.eng == nil || m.fwd == nil {
		return
	}
	if slices.Equal(routes, m.lastSplitRoutes) && slices.Equal(suffixes, m.lastDomainSuffixes) {
		return
	}
	log.Printf("[vpn] split routes updated from server: %d CIDRs, %d domains", len(routes), len(suffixes))
	m.lastSplitRoutes = routes
	m.lastDomainSuffixes = suffixes
	if !m.currentFollowSplitRoutes {
		return // full-tunnel: split routes not active
	}
	m.eng.ClearInjectedRules()
	var injected []string
	for _, cidr := range routes {
		injected = append(injected, "IP-CIDR,"+cidr+",VPN")
	}
	for _, suffix := range suffixes {
		injected = append(injected, "DOMAIN-SUFFIX,"+suffix+",VPN")
	}
	if len(injected) > 0 {
		if err := m.eng.InjectRules(injected); err != nil {
			log.Printf("[vpn] update split routes inject: %v", err)
		}
	}
}

// IsConnectionDead returns true when the WireGuard tunnel never handshakes
// within the initial grace period, or when the most recent handshake is stale.
// WireGuard renegotiates every ~180s (REKEY_AFTER_TIME), so 300s gives 1.5+
// full cycles of headroom before declaring the tunnel dead.
func (m *Manager) IsConnectionDead() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	if !m.active || m.wgDev == nil {
		return false
	}
	s, err := m.wgDev.GetStats()
	if err != nil {
		m.lastRxBytes = s.RxBytes
		return false
	}
	dead, lastRxBytes := evaluateConnectionDead(time.Now().Unix(), m.status.ConnectedAt, s.LastHandshakeSec, s.RxBytes, m.lastRxBytes)
	m.lastRxBytes = lastRxBytes
	return dead
}

func evaluateConnectionDead(now, connectedAt, lastHandshakeSec, rxBytes, lastRxBytes int64) (bool, int64) {
	if lastHandshakeSec == 0 {
		if connectedAt > 0 && now-connectedAt > 90 {
			return true, rxBytes
		}
		return false, rxBytes
	}
	age := now - lastHandshakeSec
	if age <= 300 {
		return false, rxBytes
	}
	return rxBytes == lastRxBytes, rxBytes
}

func (m *Manager) startLocked(nodeName string, cc ConnectConfig) error {
	cfg := m.cfg

	ruleStrings, err := cfg.RuleStrings()
	if err != nil {
		return fmt.Errorf("rules: %w", err)
	}

	geoDB, err := geoip.Open(cfg.GeoIP.File)
	if err != nil {
		return fmt.Errorf("geoip: %w", err)
	}

	eng, err := rule.NewEngine(ruleStrings, geoDB)
	if err != nil {
		geoDB.Close()
		return fmt.Errorf("rule engine: %w", err)
	}
	m.eng = eng

	// Add server endpoint route exception BEFORE starting WG device
	// so its encrypted UDP/TCP packets bypass the TUN.
	if cc.ServerIP != "" && cc.PhysicalIface != "" {
		if err := forwarder.AddScopedHostRoute(cc.ServerIP, cc.PhysicalIface); err != nil {
			log.Printf("[vpn] server route exception %s via %s: %v (continuing)", cc.ServerIP, cc.PhysicalIface, err)
		}
	}

	wgDev, err := wgdevice.New(cc.WG)
	if err != nil {
		return fmt.Errorf("wg device: %w", err)
	}
	m.wgDev = wgDev

	rt := routetable.New(cfg.TUN.Name, 30*time.Second)
	if err := rt.Start(); err != nil {
		log.Printf("[vpn] routetable: %v (TTL fallback active)", err)
	}
	m.rt = rt

	tunCfg := tun.Config{
		Name: cfg.TUN.Name,
		IP:   cfg.TUN.IP,
		Mask: cfg.TUN.Mask,
		MTU:  cfg.TUN.MTU,
	}
	tunDev, err := tun.NewDevice(tunCfg)
	if err != nil {
		return fmt.Errorf("create tun: %w", err)
	}
	m.tunDev = tunDev

	actualName, _ := tunDev.Name()
	if actualName != "" && actualName != cfg.TUN.Name {
		cfg.TUN.Name = actualName
		rt.SetSkipIface(actualName)
	}

	pool, err := fakeip.NewPool(cfg.FakeIP.Pool)
	if err != nil {
		return fmt.Errorf("fakeip pool: %w", err)
	}

	directOut := outbound.NewDirect(cfg.DirectOutbound.Interface, cfg.DNS.Upstream)
	if err := directOut.Init(); err != nil {
		return fmt.Errorf("direct outbound init: %w", err)
	}

	// Add host routes for upstream DNS servers via the physical interface so
	// fakeip server and forwarder DNS queries don't loop back through the TUN.
	physIface := directOut.ResolvedIfaceName()
	if physIface == "" {
		physIface = cc.PhysicalIface
	}
	for _, upstream := range cfg.DNS.Upstream {
		host, _, err := net.SplitHostPort(upstream)
		if err != nil || net.ParseIP(host) == nil {
			continue
		}
		if rerr := forwarder.AddScopedHostRoute(host, physIface); rerr != nil {
			log.Printf("[vpn] dns route %s via %s: %v (continuing)", host, physIface, rerr)
		}
	}

	// Real-IP lookup bound to physical interface for fakeip pool logging.
	upstreamDNS := firstNonSystem(cfg.DNS.Upstream)
	realIPLookup := func(domain string) []string {
		if upstreamDNS == "" {
			return nil
		}
		r := &net.Resolver{
			PreferGo: true,
			Dial: func(ctx context.Context, network, address string) (net.Conn, error) {
				return directOut.Dialer().DialContext(ctx, "udp", upstreamDNS)
			},
		}
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		addrs, err := r.LookupHost(ctx, domain)
		if err != nil {
			return nil
		}
		return addrs
	}
	pool.OnAssign = func(domain string, fakeIP net.IP) {
		addrs := realIPLookup(domain)
		if len(addrs) > 0 {
			log.Printf("[fakeip] %s → %s (real: %s)", fakeIP, domain, strings.Join(addrs, ", "))
		} else {
			log.Printf("[fakeip] %s → %s", fakeIP, domain)
		}
	}

	matcher := &rule.MatcherAdapter{Engine: eng}
	dnsServer := fakeip.NewServer(pool, matcher, nil, cfg.DNS.Upstream)
	if len(cfg.DNS.Hijack) > 0 {
		dnsServer.SetFakeAllA(true)
	}
	// Bind fakeip server's upstream DNS exchange to the physical interface so
	// non-A queries (HTTPS, MX, TXT, etc.) don't loop back through the TUN.
	dnsServer.SetDialFn(func(ctx context.Context, network, address string) (net.Conn, error) {
		return directOut.Dialer().DialContext(ctx, network, address)
	})

	outs := map[string]*outbound.Direct{"DIRECT": directOut}
	upstream := firstNonSystem(cfg.DNS.Upstream)
	// When config TUN MTU is 0 (auto), use the VPN server's MTU so both sides
	// of the proxy use the same segment size and avoid unnecessary segmentation.
	tunMTU := cfg.TUN.MTU
	if tunMTU == 0 && cc.WG.MTU > 0 {
		tunMTU = cc.WG.MTU
	}
	fwdCfg := forwarder.Config{
		TUNName:     cfg.TUN.Name,
		TUNIP:       cfg.TUN.IP,
		TUNMask:     cfg.TUN.Mask,
		TUNMTU:      tunMTU,
		UpstreamDNS: upstream,
		DefaultDNS:  upstream, // prevent forwarder from falling back to system DNS
		DNSHijack:   cfg.DNS.Hijack,
	}
	fwd, err := forwarder.NewForwarder(fwdCfg, dnsServer, pool, eng, outs, rt)
	if err != nil {
		return fmt.Errorf("forwarder: %w", err)
	}
	m.fwd = fwd
	vpnOutbound := wgdevice.NewOutbound(wgDev)
	fwd.SetVPNOutbound(vpnOutbound)

	if err := m.startSOCKS5Locked(vpnOutbound.DialContext, vpnResolver{resolver: wgDev.VPNResolver()}); err != nil {
		return err
	}

	rt2 := router.NewPlatformRouter(cfg.TUN.Name)
	rm := router.NewRouteManager(rt2)
	m.rm = rm
	refined := RefinedCaptureCIDRs(rt.Entries(), cfg.TUN.Name)
	if err := rm.AddRoutes(cfg.FakeIP.Pool, refined); err != nil {
		return fmt.Errorf("add routes: %w", err)
	}

	fwd.Start(tunDev)

	// Inject VPN routes from corplink server response
	m.currentFollowSplitRoutes = cc.FollowSplitRoutes
	var injected []string
	if cc.FollowSplitRoutes {
		// Split-tunnel: only matching CIDRs/domains go through VPN.
		for _, cidr := range cc.SplitRoutes {
			injected = append(injected, "IP-CIDR,"+cidr+",VPN")
		}
		for _, suffix := range cc.DomainSuffixes {
			injected = append(injected, "DOMAIN-SUFFIX,"+suffix+",VPN")
		}
	} else {
		// Full-tunnel: everything goes through VPN; user DIRECT rules still win.
		injected = append(injected, "IP-CIDR,0.0.0.0/0,VPN")
		fwd.SetDefaultVPN(true)
	}
	if len(injected) > 0 {
		if err := eng.InjectRules(injected); err != nil {
			log.Printf("[vpn] inject rules: %v (continuing)", err)
		}
	}

	log.Printf("[vpn] connected: node=%s vpnIP=%s", nodeName, cc.WG.VpnIP)
	return nil
}

func (m *Manager) cleanupLocked() {
	if m.socks5 != nil {
		m.socks5.Close() //nolint:errcheck
		m.socks5 = nil
	}
	if m.eng != nil {
		m.eng.ClearInjectedRules()
		m.eng = nil
	}
	if m.fwd != nil {
		m.fwd.SetDefaultVPN(false)
		m.fwd.Close()
		m.fwd = nil
	}
	if m.rm != nil {
		m.rm.Cleanup()
		m.rm = nil
	}
	if m.wgDev != nil {
		m.wgDev.Close()
		m.wgDev = nil
	}
	if m.tunDev != nil {
		m.tunDev.Close()
		m.tunDev = nil
	}
	if m.rt != nil {
		m.rt.Stop()
		m.rt = nil
	}
	forwarder.CleanupPersistedHostRoutes()
	log.Printf("[vpn] disconnected")
}

func (m *Manager) startSOCKS5Locked(dial socks5proxy.DialFunc, resolver socks5proxy.Resolver) error {
	srv, err := socks5proxy.Start(socks5proxy.Config{
		Enabled:  m.cfg.SOCKS5.Enabled,
		BindHost: m.cfg.SOCKS5.BindHost,
		Port:     m.cfg.SOCKS5.Port,
	}, dial, resolver)
	if err != nil {
		return fmt.Errorf("socks5: %w", err)
	}
	m.socks5 = srv
	if srv != nil {
		log.Printf("[socks5] listening on %s", srv.Addr())
	}
	return nil
}

type vpnResolver struct {
	resolver *net.Resolver
}

func (r vpnResolver) Resolve(ctx context.Context, name string) (context.Context, net.IP, error) {
	if ip := net.ParseIP(name); ip != nil {
		return ctx, ip, nil
	}
	addrs, err := r.resolver.LookupIPAddr(ctx, name)
	if err != nil {
		return ctx, nil, err
	}
	for _, addr := range addrs {
		if addr.IP != nil {
			return ctx, addr.IP, nil
		}
	}
	return ctx, nil, fmt.Errorf("resolve %s: no addresses", name)
}

func firstNonSystem(upstream []string) string {
	for _, u := range upstream {
		if u != "" && u != "SYSTEM" {
			return u
		}
	}
	return ""
}

func RefinedCaptureCIDRs(entries []routetable.Entry, tunName string) []string {
	var out []string
	seen := make(map[string]bool)
	for _, e := range entries {
		if e.Dest == nil || e.Dest.IP.To4() == nil {
			continue
		}
		if e.Iface == tunName || !isTunnelIface(e.Iface) {
			continue
		}
		for _, cidr := range splitMoreSpecific(e.Dest) {
			if !seen[cidr] {
				seen[cidr] = true
				out = append(out, cidr)
			}
		}
	}
	return out
}

func isTunnelIface(name string) bool {
	return strings.HasPrefix(name, "utun") ||
		strings.HasPrefix(name, "tun") ||
		strings.HasPrefix(name, "tap")
}

func splitMoreSpecific(n *net.IPNet) []string {
	ones, bits := n.Mask.Size()
	if bits != 32 || ones >= 32 {
		return nil
	}
	ip := n.IP.To4()
	if ip == nil || ip[0] == 127 || ip[0] >= 224 {
		return nil
	}
	childMask := net.CIDRMask(ones+1, 32)
	first := &net.IPNet{IP: ip.Mask(childMask), Mask: childMask}
	step := uint32(1) << uint32(32-(ones+1))
	secondIP := uint32(ip[0])<<24 | uint32(ip[1])<<16 | uint32(ip[2])<<8 | uint32(ip[3])
	secondIP += step
	second := &net.IPNet{
		IP:   net.IPv4(byte(secondIP>>24), byte(secondIP>>16), byte(secondIP>>8), byte(secondIP)),
		Mask: childMask,
	}
	return []string{first.String(), second.String()}
}
