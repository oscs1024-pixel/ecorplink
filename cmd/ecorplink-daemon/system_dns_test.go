//go:build darwin

package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestScutilSubkeys(t *testing.T) {
	out := `  subKey [0] = State:/Network/Service/A/DNS
  subKey [1] = State:/Network/Service/B/DNS
`
	got := scutilSubkeys(out)
	want := []string{"State:/Network/Service/A/DNS", "State:/Network/Service/B/DNS"}
	if len(got) != len(want) {
		t.Fatalf("scutilSubkeys = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("scutilSubkeys = %v, want %v", got, want)
		}
	}
}

func TestScutilRestoreScript(t *testing.T) {
	raw := `<dictionary> {
  ConfirmedServiceID : utun4
  SearchDomains : <array> {
  }
  ServerAddresses : <array> {
    0 : 127.0.0.1
    1 : ::1
  }
  ServerPort : 53
  SupplementalMatchDomains : <array> {
    0 : 
  }
}`
	script, err := scutilRestoreScript("State:/Network/Service/utun4/DNS", raw)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"d.add ConfirmedServiceID utun4",
		"d.add ServerAddresses * 127.0.0.1 ::1",
		"d.add ServerPort # 53",
		"d.add SupplementalMatchDomains * \"\"",
		"set State:/Network/Service/utun4/DNS",
	} {
		if !strings.Contains(script, want) {
			t.Fatalf("restore script missing %q:\n%s", want, script)
		}
	}
}

func TestOwnServiceDNSScriptWritesOnlyModifydstService(t *testing.T) {
	script := ownServiceDNSScript("utun5", "172.30.77.1", 28)
	for _, want := range []string{
		"d.add ConfirmedServiceID utun5",
		"d.add ServerAddresses * 172.30.77.1",
		"d.add SupplementalMatchDomains * \"\"",
		"d.add SearchOrder # 0",
		"d.add __IF_INDEX__ # 28",
		"set State:/Network/Service/utun5/DNS",
		"Supplemental: utun5 0",
	} {
		if !strings.Contains(script, want) {
			t.Fatalf("own service DNS script missing %q:\n%s", want, script)
		}
	}
	for _, forbidden := range []string{
		"State:/Network/Service/utun4/DNS",
		"set State:/Network/Global/DNS",
	} {
		if strings.Contains(script, forbidden) {
			t.Fatalf("own service DNS script must not contain %q:\n%s", forbidden, script)
		}
	}
}

func TestSystemDNSOriginalUpstreamSkipsLoopback(t *testing.T) {
	raw := `<dictionary> {
  ServerAddresses : <array> {
    0 : 127.0.0.1
    1 : ::1
  }
  ServerPort : 53
}`
	if got := firstUsableSystemDNSUpstream(raw); got != "" {
		t.Fatalf("loopback upstream = %q, want empty", got)
	}
}

func TestSystemDNSOriginalUpstreamUsesNonLoopback(t *testing.T) {
	raw := `<dictionary> {
  ServerAddresses : <array> {
    0 : 127.0.0.1
    1 : 10.8.0.1
  }
  ServerPort : 5353
}`
	if got := firstUsableSystemDNSUpstream(raw); got != "10.8.0.1:5353" {
		t.Fatalf("upstream = %q, want 10.8.0.1:5353", got)
	}
}

func TestPFDNSRedirectRulesUseAppleAnchorAndLoopbackOnly(t *testing.T) {
	rules := pfDNSRedirectRules("172.30.77.1")
	for _, want := range []string{
		"rdr pass on lo0 inet proto udp from any to 127.0.0.1 port 53 -> 172.30.77.1 port 53",
		"rdr pass on lo0 inet proto tcp from any to 127.0.0.1 port 53 -> 172.30.77.1 port 53",
	} {
		if !strings.Contains(rules, want) {
			t.Fatalf("pf rules missing %q:\n%s", want, rules)
		}
	}
	if strings.Contains(rules, "utun4") || strings.Contains(rules, "State:/Network/Service/utun4/DNS") {
		t.Fatalf("pf rules must not reference feilian service:\n%s", rules)
	}
}

func TestResolvConfContentPointsDigAtTunDNS(t *testing.T) {
	got := resolvConfContent("172.30.77.1")
	for _, want := range []string{
		"# ecorplink managed; restored on stop",
		"nameserver 172.30.77.1",
		"options timeout:1 attempts:2",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("resolv.conf content missing %q:\n%s", want, got)
		}
	}
}

func TestEnsureResolvConfDNSRewritesWhenOverwritten(t *testing.T) {
	path := filepath.Join(t.TempDir(), "resolv.conf")
	if err := os.WriteFile(path, []byte("nameserver 127.0.0.1\n"), 0644); err != nil {
		t.Fatal(err)
	}

	changed, err := ensureResolvConfDNS(path, "172.30.77.1")
	if err != nil {
		t.Fatal(err)
	}
	if !changed {
		t.Fatal("ensureResolvConfDNS changed = false, want true")
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != resolvConfContent("172.30.77.1") {
		t.Fatalf("resolv.conf = %q, want managed content", got)
	}
}

func TestEnsureResolvConfDNSDoesNotRewriteWhenAlreadyManaged(t *testing.T) {
	path := filepath.Join(t.TempDir(), "resolv.conf")
	want := resolvConfContent("172.30.77.1")
	if err := os.WriteFile(path, []byte(want), 0644); err != nil {
		t.Fatal(err)
	}

	changed, err := ensureResolvConfDNS(path, "172.30.77.1")
	if err != nil {
		t.Fatal(err)
	}
	if changed {
		t.Fatal("ensureResolvConfDNS changed = true, want false")
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != want {
		t.Fatalf("resolv.conf = %q, want unchanged", got)
	}
}

func TestScutilArrayAndScalar(t *testing.T) {
	raw := `<dictionary> {
  ServerAddresses : <array> {
    0 : 127.0.0.1
    1 : ::1
  }
  ServerPort : 53
  SupplementalMatchDomains : <array> {
    0 : 
  }
}`
	addrs := scutilArray(raw, "ServerAddresses")
	if len(addrs) != 2 || addrs[0] != "127.0.0.1" || addrs[1] != "::1" {
		t.Fatalf("ServerAddresses = %v, want [127.0.0.1 ::1]", addrs)
	}
	if got := scutilScalar(raw, "ServerPort"); got != "53" {
		t.Fatalf("ServerPort = %q, want 53", got)
	}
	domains := scutilArray(raw, "SupplementalMatchDomains")
	if len(domains) != 1 || domains[0] != "" {
		t.Fatalf("SupplementalMatchDomains = %v, want empty string entry", domains)
	}
}
