package config

import (
	"encoding/json"
	"fmt"
	"net"
	"os"
	"runtime"
	"strings"
)

// Config holds the application configuration.
type Config struct {
	TUN            TUNConfig      `json:"tun"`
	FakeIP         FakeIPConfig   `json:"fakeip"`
	DNS            DNSConfig      `json:"dns"`
	SOCKS5         SOCKS5Config   `json:"socks5"`
	GeoIP          GeoIPConfig    `json:"geoip"`
	DirectOutbound OutboundConfig `json:"direct_outbound"`
	Corplink       CorplinkConfig `json:"corplink"`
	Log            LogConfig      `json:"log"`
	Rules          RuleList       `json:"rules"`
}

// CorplinkConfig holds corplink/飞连 connection settings.
type CorplinkConfig struct {
	CompanyName        string `json:"company_name"`
	InsecureSkipVerify bool   `json:"insecure_skip_verify"`
	DebugHTTPBody      bool   `json:"debug_http_body"`
}

// TUNConfig configures the TUN interface.
type TUNConfig struct {
	Name string `json:"name"`
	IP   string `json:"ip"`
	Mask int    `json:"mask"`
	MTU  int    `json:"mtu"`
}

// FakeIPConfig configures the FakeIP address pool.
type FakeIPConfig struct {
	Pool string `json:"pool"`
}

// DNSConfig configures upstream DNS resolvers.
// Upstream entries may be "SYSTEM" or "ip:port".
type DNSConfig struct {
	Upstream     []string `json:"upstream"`
	Hijack       []string `json:"hijack"`
	SystemHijack bool     `json:"system_hijack"`
}

// SOCKS5Config configures the optional local SOCKS5 ingress.
type SOCKS5Config struct {
	Enabled  bool   `json:"enabled"`
	BindHost string `json:"bind_host"`
	Port     int    `json:"port"`
}

// GeoIPConfig configures the GeoIP database.
type GeoIPConfig struct {
	File string `json:"file"` // path to .mmdb; "" = use embedded
}

// OutboundConfig configures a named outbound.
type OutboundConfig struct {
	Interface string `json:"interface"` // network interface name; "" = auto-detect
}

// LogConfig configures logging.
type LogConfig struct {
	Level   string `json:"level"`
	File    string `json:"file"`
	MaxSize string `json:"max_size"`
	MaxAge  int    `json:"max_age"`
}

// RuleList stores editable rule objects.
type RuleList []RuleConfig

// RuleConfig is the editable GUI representation of one routing rule.
type RuleConfig struct {
	ID      string `json:"id,omitempty"`
	Enabled bool   `json:"enabled"`
	Type    string `json:"type"`
	Value   string `json:"value"`
	Policy  string `json:"policy"`
	Group   string `json:"group,omitempty"`
	Comment string `json:"comment,omitempty"`
}

// DefaultConfig returns a Config pre-filled with sensible defaults.
func DefaultConfig() *Config {
	return &Config{
		TUN: TUNConfig{
			Name: defaultTunName(runtime.GOOS),
			IP:   "172.30.77.1",
			Mask: 30,
			MTU:  1420,
		},
		FakeIP: FakeIPConfig{
			Pool: "198.18.0.0/15",
		},
		DNS: DNSConfig{
			Upstream:     []string{"223.5.5.5:53", "223.6.6.6:53"},
			Hijack:       []string{"0.0.0.0:53"},
			SystemHijack: true,
		},
		SOCKS5: SOCKS5Config{
			Enabled:  false,
			BindHost: "127.0.0.1",
			Port:     1080,
		},
		GeoIP: GeoIPConfig{
			File: "",
		},
		DirectOutbound: OutboundConfig{Interface: ""},
		Corplink: CorplinkConfig{
			CompanyName:        "",
			InsecureSkipVerify: true,
			DebugHTTPBody:      false,
		},
		Log: LogConfig{
			Level:   "info",
			File:    "~/.ecorplink/ecorplink.log",
			MaxSize: "100MB",
			MaxAge:  7,
		},
		Rules: RuleList{
			{
				ID:      "geoip_cn",
				Enabled: true,
				Type:    "GEOIP",
				Value:   "CN",
				Policy:  "DIRECT",
			},
		},
	}
}

func defaultTunName(goos string) string {
	switch goos {
	case "darwin":
		return "utun"
	case "windows":
		return "ECorpLink"
	case "linux":
		return "ecorplink0"
	default:
		return "ecorplink0"
	}
}

// LoadConfig reads a config file from path, merges it into DefaultConfig, and
// validates all rule strings. If path is empty, common default paths are tried.
func LoadConfig(path string) (*Config, error) {
	cfg := DefaultConfig()
	if path == "" {
		for _, p := range []string{"config.json", ".ecorplink.json"} {
			if _, err := os.Stat(p); err == nil {
				path = p
				break
			}
		}
	}
	if path == "" {
		return cfg, nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read config %q: %w", path, err)
	}
	return mergeAndValidate(cfg, data)
}

// LoadConfigFromBytes parses config from raw JSON bytes, merging into DefaultConfig.
func LoadConfigFromBytes(data []byte) (*Config, error) {
	return mergeAndValidate(DefaultConfig(), data)
}

func mergeAndValidate(cfg *Config, data []byte) (*Config, error) {
	if err := json.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}
	// Platform-specific validation: macOS requires utun prefix. The bare
	// "utun" name asks the kernel to allocate a free utunN interface.
	if runtime.GOOS == "darwin" && !strings.HasPrefix(cfg.TUN.Name, "utun") {
		cfg.TUN.Name = "utun"
	}
	if err := cfg.SOCKS5.Validate(); err != nil {
		return nil, err
	}
	if _, err := cfg.RuleStrings(); err != nil {
		return nil, err
	}
	return cfg, nil
}

func (c SOCKS5Config) Validate() error {
	if !c.Enabled {
		return nil
	}
	if strings.TrimSpace(c.BindHost) == "" {
		return fmt.Errorf("socks5 bind_host must not be empty")
	}
	if c.Port < 1 || c.Port > 65535 {
		return fmt.Errorf("socks5 port must be between 1 and 65535")
	}
	return nil
}

// RuleStrings returns enabled rules in the string format consumed by the rule
// engine.
func (c *Config) RuleStrings() ([]string, error) {
	return c.Rules.EnabledStrings()
}

// UnmarshalJSON accepts rule objects.
func (rl *RuleList) UnmarshalJSON(data []byte) error {
	var objects []RuleConfig
	if err := json.Unmarshal(data, &objects); err != nil {
		return err
	}
	for i := range objects {
		rule := &objects[i]
		if rule.ID == "" {
			rule.ID = fmt.Sprintf("rule_%d", i+1)
		}
		if rule.Policy == "" {
			rule.Policy = "DIRECT"
		}
		if err := rule.Validate(); err != nil {
			return fmt.Errorf("rule[%d]: %w", i, err)
		}
	}
	*rl = objects
	return nil
}

// EnabledStrings returns enabled rules in rule-engine format.
func (rl RuleList) EnabledStrings() ([]string, error) {
	out := make([]string, 0, len(rl))
	for i, rule := range rl {
		if !rule.Enabled {
			continue
		}
		s, err := rule.ToRuleString()
		if err != nil {
			return nil, fmt.Errorf("rule[%d]: %w", i, err)
		}
		out = append(out, s)
	}
	return out, nil
}

// Validate verifies a GUI rule object.
func (r RuleConfig) Validate() error {
	if _, err := r.ToRuleString(); err != nil {
		return err
	}
	return nil
}

// ToRuleString converts a rule object to the engine string format.
func (r RuleConfig) ToRuleString() (string, error) {
	ruleType := strings.ToUpper(strings.TrimSpace(r.Type))
	value := strings.TrimSpace(r.Value)
	policy := strings.ToUpper(strings.TrimSpace(r.Policy))
	if policy == "" {
		policy = "DIRECT"
	}
	if policy != "DIRECT" && policy != "VPN" {
		return "", fmt.Errorf("policy must be DIRECT or VPN, got %q", policy)
	}
	raw := fmt.Sprintf("%s,%s,%s", ruleType, value, policy)
	if _, err := ParseRuleString(raw); err != nil {
		return "", err
	}
	return raw, nil
}

// ParseRuleString validates and tokenizes a rule string of the form:
//
//	TYPE,VALUE,ACTION
//
// Valid types: DOMAIN, DOMAIN-SUFFIX, DOMAIN-KEYWORD, GEOIP, IP-CIDR
// Valid actions: DIRECT, DIRECT-prefixed outbound names, or VPN.
//
// Returns the tokens [type, value, action] on success.
func ParseRuleString(s string) ([]string, error) {
	parts := strings.SplitN(s, ",", 3)
	if len(parts) != 3 {
		return nil, fmt.Errorf("invalid rule %q: expected TYPE,VALUE,ACTION", s)
	}
	ruleType := strings.TrimSpace(parts[0])
	value := strings.TrimSpace(parts[1])
	action := strings.TrimSpace(parts[2])

	if value == "" {
		return nil, fmt.Errorf("invalid rule %q: value is empty", s)
	}
	if action == "" {
		return nil, fmt.Errorf("invalid rule %q: action is empty", s)
	}

	switch ruleType {
	case "DOMAIN", "DOMAIN-SUFFIX", "DOMAIN-KEYWORD":
		// value is a domain or keyword; no additional validation needed.
	case "GEOIP":
		// value is a country code; no additional validation needed.
	case "IP-CIDR":
		if _, _, err := net.ParseCIDR(value); err != nil {
			return nil, fmt.Errorf("invalid rule %q: bad CIDR %q: %w", s, value, err)
		}
	default:
		return nil, fmt.Errorf("invalid rule %q: unknown type %q", s, ruleType)
	}

	// Validate action: only DIRECT or VPN are allowed.
	upper := strings.ToUpper(action)
	if upper != "DIRECT" && upper != "VPN" {
		// Allow DIRECT-prefixed outbound names for backward compat
		if !strings.HasPrefix(upper, "DIRECT") {
			return nil, fmt.Errorf("invalid rule %q: action must be DIRECT or VPN, got %q", s, action)
		}
	}

	return []string{ruleType, value, action}, nil
}
