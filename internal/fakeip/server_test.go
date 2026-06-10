package fakeip

import (
	"net"
	"testing"

	"github.com/miekg/dns"
)

type mockMatcher struct{ domains map[string]bool }

func (m *mockMatcher) MatchDomain(d string) bool { return m.domains[d] }

func buildQuery(domain string, qtype uint16) []byte {
	msg := new(dns.Msg)
	msg.SetQuestion(dns.Fqdn(domain), qtype)
	raw, _ := msg.Pack()
	return raw
}

func TestHandleDNS_MatchedReturnsA(t *testing.T) {
	pool, _ := NewPool("198.18.0.0/15")
	matcher := &mockMatcher{domains: map[string]bool{"github.com": true}}
	srv := NewServer(pool, matcher, nil, nil)

	raw := buildQuery("github.com", dns.TypeA)
	resp, err := srv.HandleDNS(raw)
	if err != nil {
		t.Fatal(err)
	}

	msg := new(dns.Msg)
	if err := msg.Unpack(resp); err != nil {
		t.Fatal(err)
	}
	if len(msg.Answer) == 0 {
		t.Fatal("expected at least one A answer")
	}
	a, ok := msg.Answer[0].(*dns.A)
	if !ok {
		t.Fatalf("expected *dns.A, got %T", msg.Answer[0])
	}
	if !pool.InPool(a.A) {
		t.Errorf("returned IP %s not in fakeip pool", a.A)
	}
	if a.Hdr.Ttl != 1 {
		t.Errorf("want TTL=1, got %d", a.Hdr.Ttl)
	}
}

func TestHandleDNS_AAAAReturnsEmpty(t *testing.T) {
	pool, _ := NewPool("198.18.0.0/15")
	matcher := &mockMatcher{domains: map[string]bool{"github.com": true}}
	srv := NewServer(pool, matcher, nil, nil)

	raw := buildQuery("github.com", dns.TypeAAAA)
	resp, _ := srv.HandleDNS(raw)
	msg := new(dns.Msg)
	msg.Unpack(resp)
	if len(msg.Answer) != 0 {
		t.Error("AAAA for fakeip domain should return empty answer (no NAT64)")
	}
	if msg.Rcode != dns.RcodeSuccess {
		t.Errorf("AAAA should return NOERROR, got rcode %d", msg.Rcode)
	}
}

func TestHandleDNS_UnmatchedNoUpstream_SERVFAIL(t *testing.T) {
	pool, _ := NewPool("198.18.0.0/15")
	matcher := &mockMatcher{domains: map[string]bool{}}
	srv := NewServer(pool, matcher, nil, nil) // no upstream

	raw := buildQuery("example.com", dns.TypeA)
	resp, _ := srv.HandleDNS(raw)
	msg := new(dns.Msg)
	msg.Unpack(resp)
	if msg.Rcode != dns.RcodeServerFailure {
		t.Errorf("want SERVFAIL, got rcode %d", msg.Rcode)
	}
}

func TestHandleDNS_FakeAllAForUnmatchedDomain(t *testing.T) {
	pool, _ := NewPool("198.18.0.0/15")
	matcher := &mockMatcher{domains: map[string]bool{}}
	srv := NewServer(pool, matcher, nil, nil)
	srv.SetFakeAllA(true)

	raw := buildQuery("example.com", dns.TypeA)
	resp, err := srv.HandleDNS(raw)
	if err != nil {
		t.Fatal(err)
	}
	msg := new(dns.Msg)
	msg.Unpack(resp)
	if len(msg.Answer) == 0 {
		t.Fatal("expected fake A answer")
	}
	a := msg.Answer[0].(*dns.A)
	if !pool.InPool(a.A) {
		t.Fatalf("answer %s is not in fake pool", a.A)
	}
}

func TestHandleDNS_SameDomainSameIP(t *testing.T) {
	pool, _ := NewPool("198.18.0.0/15")
	matcher := &mockMatcher{domains: map[string]bool{"github.com": true}}
	srv := NewServer(pool, matcher, nil, nil)

	raw1 := buildQuery("github.com", dns.TypeA)
	resp1, _ := srv.HandleDNS(raw1)
	resp2, _ := srv.HandleDNS(raw1)

	var msg1, msg2 dns.Msg
	msg1.Unpack(resp1)
	msg2.Unpack(resp2)
	ip1 := msg1.Answer[0].(*dns.A).A
	ip2 := msg2.Answer[0].(*dns.A).A
	if !ip1.Equal(ip2) {
		t.Errorf("same domain should get same fake IP: %s != %s", ip1, ip2)
	}
}

func TestHandleDNS_CaseInsensitive(t *testing.T) {
	pool, _ := NewPool("198.18.0.0/15")
	matcher := &mockMatcher{domains: map[string]bool{"github.com": true}}
	srv := NewServer(pool, matcher, nil, nil)

	raw := buildQuery("GITHUB.COM", dns.TypeA)
	resp, err := srv.HandleDNS(raw)
	if err != nil {
		t.Fatal(err)
	}
	msg := new(dns.Msg)
	msg.Unpack(resp)
	if len(msg.Answer) == 0 {
		t.Error("uppercase domain should still be matched")
	}
}

func TestHandleDNS_MockUpstream(t *testing.T) {
	pool, _ := NewPool("198.18.0.0/15")
	matcher := &mockMatcher{domains: map[string]bool{}}

	// Build a mock upstream that returns a real IP
	upstreamResp := new(dns.Msg)
	upstreamResp.SetReply(new(dns.Msg))
	upstreamResp.Answer = append(upstreamResp.Answer, &dns.A{
		Hdr: dns.RR_Header{Name: "example.com.", Rrtype: dns.TypeA, Class: dns.ClassINET, Ttl: 60},
		A:   net.ParseIP("1.2.3.4"),
	})
	upstream := &mockUpstream{resp: upstreamResp}
	srv := NewServer(pool, matcher, upstream, nil)

	raw := buildQuery("example.com", dns.TypeA)
	resp, err := srv.HandleDNS(raw)
	if err != nil {
		t.Fatal(err)
	}
	msg := new(dns.Msg)
	msg.Unpack(resp)
	if len(msg.Answer) == 0 {
		t.Fatal("expected upstream answer")
	}
	a := msg.Answer[0].(*dns.A)
	if !a.A.Equal(net.ParseIP("1.2.3.4")) {
		t.Errorf("want 1.2.3.4 from upstream, got %s", a.A)
	}
}

type mockUpstream struct{ resp *dns.Msg }

func (m *mockUpstream) Exchange(msg *dns.Msg) (*dns.Msg, error) {
	resp := m.resp.Copy()
	resp.SetReply(msg)
	return resp, nil
}
