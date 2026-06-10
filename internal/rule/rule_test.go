package rule

import (
	"net"
	"testing"
)

type mockGeoIP struct{}

func (m *mockGeoIP) Country(ip net.IP) (string, error) {
	// 1.2.4.0/24 → CN
	masked := ip.Mask(net.CIDRMask(24, 32))
	if masked != nil && masked.Equal(net.ParseIP("1.2.4.0").Mask(net.CIDRMask(24, 32))) {
		return "CN", nil
	}
	return "", nil
}

func TestMatchDomain(t *testing.T) {
	rules := []string{
		"DOMAIN,github.com,DIRECT",
		"DOMAIN-SUFFIX,google.com,DIRECT",
		"DOMAIN-KEYWORD,notion,DIRECT",
	}
	eng, err := NewEngine(rules, &mockGeoIP{})
	if err != nil {
		t.Fatal(err)
	}

	cases := []struct {
		domain  string
		wantOK  bool
		wantAct ActionType
	}{
		{"github.com", true, ActionDirect},
		{"GITHUB.COM", true, ActionDirect}, // case-insensitive
		{"www.google.com", true, ActionDirect},
		{"google.com", true, ActionDirect},
		{"notgoogle.com", false, 0},
		{"mynotion.io", true, ActionDirect},
		{"example.com", false, 0},
	}
	for _, c := range cases {
		a, ok := eng.MatchDomain(c.domain)
		if ok != c.wantOK {
			t.Errorf("MatchDomain(%q): ok=%v want %v", c.domain, ok, c.wantOK)
			continue
		}
		if !ok {
			continue
		}
		if a.Type != c.wantAct {
			t.Errorf("MatchDomain(%q): action type %v want %v", c.domain, a.Type, c.wantAct)
		}
	}
}

func TestMatchIP(t *testing.T) {
	rules := []string{
		"GEOIP,CN,DIRECT",
		"IP-CIDR,10.0.0.0/8,DIRECT",
	}
	eng, err := NewEngine(rules, &mockGeoIP{})
	if err != nil {
		t.Fatal(err)
	}

	a, ok := eng.MatchIP(net.ParseIP("1.2.4.5"))
	if !ok || a.Type != ActionDirect {
		t.Error("CN IP should match GEOIP,CN,DIRECT")
	}
	a2, ok2 := eng.MatchIP(net.ParseIP("10.1.2.3"))
	if !ok2 || a2.Type != ActionDirect {
		t.Error("10.1.2.3 should match IP-CIDR,10.0.0.0/8")
	}
	_, ok3 := eng.MatchIP(net.ParseIP("8.8.8.8"))
	if ok3 {
		t.Error("8.8.8.8 should not match any rule")
	}
}

func TestNewEngineInvalidRule(t *testing.T) {
	_, err := NewEngine([]string{"BADTYPE,x.com,DIRECT"}, &mockGeoIP{})
	if err == nil {
		t.Error("expected error for unknown rule type")
	}
}

func TestActionVPN(t *testing.T) {
	eng, err := NewEngine([]string{
		"IP-CIDR,10.0.0.0/8,VPN",
		"DOMAIN-SUFFIX,corp.internal,VPN",
	}, &mockGeoIP{})
	if err != nil {
		t.Fatal(err)
	}
	a, ok := eng.MatchIP(net.ParseIP("10.1.2.3"))
	if !ok || a.Type != ActionVPN {
		t.Fatalf("10.1.2.3 should match VPN, got %+v ok=%v", a, ok)
	}
	a2, ok2 := eng.MatchDomain("api.corp.internal")
	if !ok2 || a2.Type != ActionVPN {
		t.Fatalf("corp.internal domain should match VPN, got %+v ok=%v", a2, ok2)
	}
}

func TestRuntimeInjection(t *testing.T) {
	eng, err := NewEngine([]string{"GEOIP,CN,DIRECT"}, &mockGeoIP{})
	if err != nil {
		t.Fatal(err)
	}
	_, ok := eng.MatchIP(net.ParseIP("10.0.0.1"))
	if ok {
		t.Fatal("10.0.0.1 should not match before injection")
	}
	eng.InjectRules([]string{"IP-CIDR,10.0.0.0/8,VPN"}) //nolint:errcheck
	a, ok := eng.MatchIP(net.ParseIP("10.0.0.1"))
	if !ok || a.Type != ActionVPN {
		t.Fatalf("after injection 10.0.0.1 should be VPN, got %+v ok=%v", a, ok)
	}
	// User rule takes priority over injected rule
	eng2, _ := NewEngine([]string{"IP-CIDR,10.0.0.0/8,DIRECT"}, &mockGeoIP{})
	eng2.InjectRules([]string{"IP-CIDR,10.0.0.0/8,VPN"}) //nolint:errcheck
	a3, ok3 := eng2.MatchIP(net.ParseIP("10.0.0.1"))
	if !ok3 || a3.Type != ActionDirect {
		t.Fatalf("user DIRECT rule should override injected VPN, got %+v ok=%v", a3, ok3)
	}
	// Clear injection
	eng.ClearInjectedRules()
	_, ok2 := eng.MatchIP(net.ParseIP("10.0.0.1"))
	if ok2 {
		t.Fatal("after clear, 10.0.0.1 should not match")
	}
}

func TestMatcherAdapter(t *testing.T) {
	rules := []string{
		"DOMAIN,github.com,DIRECT",
		"DOMAIN,google.com,DIRECT",
	}
	eng, _ := NewEngine(rules, &mockGeoIP{})
	adapter := &MatcherAdapter{eng}

	// DIRECT rule should get fakeip so the TCP flow can reach the forwarder
	// and be dialed via the physical-interface outbound.
	if !adapter.MatchDomain("github.com") {
		t.Error("DIRECT domain should get fakeip")
	}
	// DIRECT rule should also get fakeip for domain-based routing.
	if !adapter.MatchDomain("google.com") {
		t.Error("DIRECT domain should get fakeip")
	}
	// No matching rule → no fakeip
	if adapter.MatchDomain("example.com") {
		t.Error("unmatched domain should not get fakeip")
	}
}
