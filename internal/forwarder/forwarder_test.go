package forwarder

import (
	"context"
	"net"
	"testing"

	"github.com/miekg/dns"

	"ecorplink/internal/fakeip"
	"ecorplink/internal/rule"
)

// TestForwarderCompiles verifies the package compiles with the new API.
func TestForwarderCompiles(t *testing.T) {
	// NewForwarder requires real deps; just verify the package compiles.
	_ = Config{}
}

func TestDNSHijackMatchesAnyIPv4Port53(t *testing.T) {
	f := &Forwarder{dnsHijack: parseDNSHijack([]string{"0.0.0.0:53"})}
	if !f.shouldHijackDNS(net.ParseIP("223.5.5.5"), 53) {
		t.Fatal("0.0.0.0:53 should hijack IPv4 DNS")
	}
	if f.shouldHijackDNS(net.ParseIP("223.5.5.5"), 853) {
		t.Fatal("0.0.0.0:53 should not hijack non-DNS port")
	}
}

func TestDNSHijackMatchesSpecificAddress(t *testing.T) {
	f := &Forwarder{dnsHijack: parseDNSHijack([]string{"1.1.1.1:53"})}
	if !f.shouldHijackDNS(net.ParseIP("1.1.1.1"), 53) {
		t.Fatal("specific DNS address should be hijacked")
	}
	if f.shouldHijackDNS(net.ParseIP("8.8.8.8"), 53) {
		t.Fatal("different DNS address should not be hijacked")
	}
}

func TestVPNDomainTargetKeepsHostnameForWireGuardResolution(t *testing.T) {
	upstream, shutdown := startTestDNS(t, "www.google.com.", "31.13.92.37")
	defer shutdown()

	f := &Forwarder{
		config: Config{UpstreamDNS: upstream},
	}
	f.SetVPNOutbound(fakePacketDialer{})

	target, _, err := f.actionToTarget(&rule.Action{Type: rule.ActionVPN, Outbound: "VPN"}, "www.google.com", "443")
	if err != nil {
		t.Fatal(err)
	}
	if target != "www.google.com:443" {
		t.Fatalf("target = %q, want hostname preserved for WG resolution", target)
	}
}

func TestDefaultVPNDomainKeepsHostnameForWireGuardResolution(t *testing.T) {
	upstream, shutdown := startTestDNS(t, "www.google.com.", "31.13.92.37")
	defer shutdown()

	pool, err := fakeip.NewPool("198.18.0.0/15")
	if err != nil {
		t.Fatal(err)
	}
	fakeGoogle := pool.Assign("www.google.com")

	f := &Forwarder{
		config: Config{UpstreamDNS: upstream},
		pool:   pool,
	}
	f.defaultVPN.Store(true)
	f.SetVPNOutbound(fakePacketDialer{})

	target, _, err := f.resolve(fakeGoogle, 443)
	if err != nil {
		t.Fatal(err)
	}
	if target != "www.google.com:443" {
		t.Fatalf("target = %q, want default VPN hostname preserved for WG resolution", target)
	}
}

type fakePacketDialer struct{}

func (fakePacketDialer) Dial(network, address string) (net.Conn, error) {
	return (&net.Dialer{}).Dial(network, address)
}

func (fakePacketDialer) DialContext(ctx context.Context, network, address string) (net.Conn, error) {
	return (&net.Dialer{}).DialContext(ctx, network, address)
}

func startTestDNS(t *testing.T, domain, answer string) (string, func()) {
	t.Helper()
	pc, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	server := &dns.Server{PacketConn: pc, Handler: dns.HandlerFunc(func(w dns.ResponseWriter, r *dns.Msg) {
		msg := new(dns.Msg)
		msg.SetReply(r)
		for _, q := range r.Question {
			if q.Qtype == dns.TypeA && q.Name == domain {
				msg.Answer = append(msg.Answer, &dns.A{
					Hdr: dns.RR_Header{Name: q.Name, Rrtype: dns.TypeA, Class: dns.ClassINET, Ttl: 60},
					A:   net.ParseIP(answer),
				})
			}
		}
		_ = w.WriteMsg(msg)
	})}
	go func() {
		_ = server.ActivateAndServe()
	}()
	return pc.LocalAddr().String(), func() {
		_ = server.Shutdown()
	}
}
