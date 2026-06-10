package config

import (
	"runtime"
	"testing"
)

func TestParseRuleString(t *testing.T) {
	cases := []struct {
		input   string
		wantErr bool
	}{
		{"DOMAIN,github.com,DIRECT", false},
		{"DOMAIN-SUFFIX,google.com,DIRECT", false},
		{"DOMAIN-KEYWORD,notion,DIRECT_WORK", false},
		{"GEOIP,CN,DIRECT", false},
		{"IP-CIDR,192.168.0.0/16,DIRECT", false},
		{"DOMAIN,myip.ipip.net,REDIRECT:1.2.3.4:80", true},
		{"DOMAIN,myip.ipip.net,REDIRECT:target.example.com:80", true},
		{"INVALID", true},
		{"DOMAIN,,DIRECT", true},
		{"IP-CIDR,notacidr,DIRECT", true},
		{"DOMAIN,x.com,REDIRECT:no-port", true},
		{"DOMAIN,github.com,", true},
		{"UNKNOWN,x.com,DIRECT", true},
		{"DOMAIN,internal.corp,VPN", false},
		{"DOMAIN-SUFFIX,corp.net,VPN", false},
		{"IP-CIDR,10.0.0.0/8,VPN", false},
	}
	for _, c := range cases {
		_, err := ParseRuleString(c.input)
		if (err != nil) != c.wantErr {
			t.Errorf("ParseRuleString(%q): err=%v, wantErr=%v", c.input, err, c.wantErr)
		}
	}
}

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()
	if cfg.FakeIP.Pool == "" {
		t.Error("default pool should not be empty")
	}
	if cfg.DirectOutbound.Interface != "" {
		t.Error("default direct outbound interface should be empty")
	}
	if cfg.Corplink.CompanyName != "" {
		t.Errorf("default company_name = %q, want empty", cfg.Corplink.CompanyName)
	}
	if !cfg.Corplink.InsecureSkipVerify {
		t.Fatal("corplink.insecure_skip_verify should default to true for self-signed deployments")
	}
	if cfg.Corplink.DebugHTTPBody {
		t.Fatal("corplink.debug_http_body should default to false")
	}
	if len(cfg.DNS.Hijack) != 1 || cfg.DNS.Hijack[0] != "0.0.0.0:53" {
		t.Fatalf("default dns hijack = %v, want [0.0.0.0:53]", cfg.DNS.Hijack)
	}
	if !cfg.DNS.SystemHijack {
		t.Fatal("system_hijack should be enabled in the generated template")
	}
	if len(cfg.DNS.Upstream) != 2 || cfg.DNS.Upstream[0] != "223.5.5.5:53" || cfg.DNS.Upstream[1] != "223.6.6.6:53" {
		t.Fatalf("default upstream = %v", cfg.DNS.Upstream)
	}
	if cfg.TUN.IP != "172.30.77.1" || cfg.TUN.Mask != 30 {
		t.Fatalf("default tun = %s/%d", cfg.TUN.IP, cfg.TUN.Mask)
	}
	if cfg.SOCKS5.Enabled || cfg.SOCKS5.BindHost != "127.0.0.1" || cfg.SOCKS5.Port != 1080 {
		t.Fatalf("default socks5 = %+v, want disabled 127.0.0.1:1080", cfg.SOCKS5)
	}
	if runtime.GOOS == "darwin" && cfg.TUN.Name != "utun" {
		t.Fatalf("darwin default tun name = %q, want utun", cfg.TUN.Name)
	}
	if len(cfg.Rules) != 1 {
		t.Fatalf("default rules len = %d, want 1", len(cfg.Rules))
	}
	rule := cfg.Rules[0]
	if !rule.Enabled || rule.Type != "GEOIP" || rule.Value != "CN" || rule.Policy != "DIRECT" {
		t.Fatalf("default rule = %+v, want GEOIP CN DIRECT", rule)
	}
}

func TestDefaultTunNameByPlatform(t *testing.T) {
	tests := map[string]string{
		"darwin":  "utun",
		"linux":   "ecorplink0",
		"windows": "ECorpLink",
	}
	for goos, want := range tests {
		if got := defaultTunName(goos); got != want {
			t.Fatalf("defaultTunName(%q) = %q, want %q", goos, got, want)
		}
	}
}

func TestLoadConfigFromBytes(t *testing.T) {
	data := []byte(`{
		"fakeip": {"pool": "198.18.0.0/15"},
		"dns": {"upstream": ["8.8.8.8:53"]},
		"socks5": {"enabled": true, "bind_host": "127.0.0.1", "port": 18080},
		"direct_outbound": {"interface": "en0"},
		"rules": [
			{"enabled": true, "type": "DOMAIN", "value": "github.com", "policy": "DIRECT"},
			{"enabled": true, "type": "GEOIP", "value": "CN", "policy": "DIRECT"}
		]
	}`)
	cfg, err := LoadConfigFromBytes(data)
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.Rules) != 2 {
		t.Errorf("want 2 rules, got %d", len(cfg.Rules))
	}
	if len(cfg.DNS.Hijack) != 1 || cfg.DNS.Hijack[0] != "0.0.0.0:53" {
		t.Fatalf("merged dns hijack = %v, want default [0.0.0.0:53]", cfg.DNS.Hijack)
	}
	if !cfg.SOCKS5.Enabled || cfg.SOCKS5.BindHost != "127.0.0.1" || cfg.SOCKS5.Port != 18080 {
		t.Fatalf("merged socks5 = %+v", cfg.SOCKS5)
	}
	cfg2, err := LoadConfigFromBytes([]byte(`{"corplink":{"insecure_skip_verify":false,"debug_http_body":true}}`))
	if err != nil {
		t.Fatal(err)
	}
	if cfg2.Corplink.InsecureSkipVerify {
		t.Fatal("explicit insecure_skip_verify=false should be preserved")
	}
	if !cfg2.Corplink.DebugHTTPBody {
		t.Fatal("explicit debug_http_body=true should be preserved")
	}

	// bad JSON
	_, err = LoadConfigFromBytes([]byte("not json"))
	if err == nil {
		t.Error("expected error for bad JSON")
	}

	// invalid rule in array
	_, err2 := LoadConfigFromBytes([]byte(`{"rules":[{"enabled":true,"type":"INVALID","value":"x","policy":"DIRECT"}]}`))
	if err2 == nil {
		t.Error("expected error for invalid rule")
	}
}

func TestRuleListRejectsStringRules(t *testing.T) {
	if _, err := LoadConfigFromBytes([]byte(`{"rules":["DOMAIN-SUFFIX,example.invalid,DIRECT"]}`)); err == nil {
		t.Fatal("expected string rules to be rejected")
	}
}

func TestRuleListLoadsObjectRulesAndFiltersDisabled(t *testing.T) {
	cfg, err := LoadConfigFromBytes([]byte(`{
		"rules": [
			{
				"id": "rule_enabled",
				"enabled": true,
				"type": "DOMAIN",
				"value": "github.com",
				"policy": "DIRECT",
				"group": "Dev",
				"comment": "default route"
			},
			{
				"id": "rule_disabled",
				"enabled": false,
				"type": "GEOIP",
				"value": "CN",
				"policy": "DIRECT"
			}
		]
	}`))
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.Rules) != 2 {
		t.Fatalf("len(cfg.Rules) = %d, want 2", len(cfg.Rules))
	}
	if cfg.Rules[0].Group != "Dev" || cfg.Rules[0].Comment != "default route" {
		t.Fatalf("object metadata not preserved: %+v", cfg.Rules[0])
	}
	strings, err := cfg.RuleStrings()
	if err != nil {
		t.Fatal(err)
	}
	if len(strings) != 1 || strings[0] != "DOMAIN,github.com,DIRECT" {
		t.Fatalf("RuleStrings() = %v", strings)
	}
}

func TestObjectRuleMissingEnabledStaysDisabled(t *testing.T) {
	cfg, err := LoadConfigFromBytes([]byte(`{
		"rules": [{
			"type": "DOMAIN",
			"value": "github.com",
			"policy": "DIRECT",
			"comment": "missing enabled field"
		}]
	}`))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Rules[0].Enabled {
		t.Fatalf("rule should require explicit enabled=true: %+v", cfg.Rules[0])
	}
}

func TestObjectRuleValidationRejectsBadCIDR(t *testing.T) {
	if _, err := LoadConfigFromBytes([]byte(`{
		"rules": [{"type": "IP-CIDR", "value": "not-cidr", "policy": "DIRECT"}]
	}`)); err == nil {
		t.Fatal("expected bad CIDR error")
	}
}

func TestSOCKS5Validation(t *testing.T) {
	cases := []struct {
		name string
		json string
	}{
		{
			name: "missing host",
			json: `{"socks5":{"enabled":true,"bind_host":"","port":1080}}`,
		},
		{
			name: "bad port",
			json: `{"socks5":{"enabled":true,"bind_host":"127.0.0.1","port":70000}}`,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := LoadConfigFromBytes([]byte(tc.json)); err == nil {
				t.Fatal("expected socks5 validation error")
			}
		})
	}
}
