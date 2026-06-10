// Package rule implements a routing rule engine that matches domains and IPs
// against a prioritized list of rules.
package rule

import (
	"fmt"
	"net"
	"strings"
	"sync"
)

// ActionType describes what should happen when a rule matches.
type ActionType int

const (
	ActionDirect ActionType = iota // route via physical interface
	ActionVPN                      // route via WireGuard tunnel
)

// Action is the result of a rule match.
type Action struct {
	Type     ActionType
	Outbound string // "DIRECT" or "VPN"
}

// GeoIPDB is the interface the engine uses for IP-to-country lookups.
// internal/geoip.DB satisfies this interface.
type GeoIPDB interface {
	Country(ip net.IP) (string, error)
}

// ruleKind is the type of a parsed rule entry.
type ruleKind int

const (
	kindDomain        ruleKind = iota
	kindDomainSuffix           // matches domain and *.domain
	kindDomainKeyword          // contains keyword
	kindGeoIP
	kindIPCIDR
)

// entry is a single compiled rule.
type entry struct {
	kind    ruleKind
	value   string     // domain / keyword / country-code / (unused for CIDR)
	network *net.IPNet // for IP-CIDR
	action  Action
}

// Engine holds the compiled rule list and evaluates matches.
type Engine struct {
	rules        []entry
	runtimeRules []entry
	mu           sync.RWMutex
	geoip        GeoIPDB
}

// NewEngine parses ruleStrings and returns an Engine.
//
// Format: "<TYPE>,<value>,<action>"
//
// Supported types: DOMAIN, DOMAIN-SUFFIX, DOMAIN-KEYWORD, GEOIP, IP-CIDR.
// Actions: DIRECT, VPN.
//
// Returns an error if any rule is unparseable or has an unknown type.
func NewEngine(ruleStrings []string, geoip GeoIPDB) (*Engine, error) {
	rules := make([]entry, 0, len(ruleStrings))
	for _, s := range ruleStrings {
		e, err := parseRule(s)
		if err != nil {
			return nil, err
		}
		rules = append(rules, e)
	}
	return &Engine{rules: rules, geoip: geoip}, nil
}

// parseRule converts a rule string into an entry.
func parseRule(s string) (entry, error) {
	parts := strings.SplitN(s, ",", 3)
	if len(parts) != 3 {
		return entry{}, fmt.Errorf("rule: invalid format %q (want TYPE,value,action)", s)
	}
	typ, value, actionStr := strings.ToUpper(strings.TrimSpace(parts[0])),
		strings.TrimSpace(parts[1]),
		strings.TrimSpace(parts[2])

	action, err := parseAction(actionStr)
	if err != nil {
		return entry{}, fmt.Errorf("rule: %q: %w", s, err)
	}

	switch typ {
	case "DOMAIN":
		return entry{kind: kindDomain, value: strings.ToLower(value), action: action}, nil

	case "DOMAIN-SUFFIX":
		return entry{kind: kindDomainSuffix, value: strings.ToLower(value), action: action}, nil

	case "DOMAIN-KEYWORD":
		return entry{kind: kindDomainKeyword, value: strings.ToLower(value), action: action}, nil

	case "GEOIP":
		return entry{kind: kindGeoIP, value: strings.ToUpper(value), action: action}, nil

	case "IP-CIDR":
		_, network, err := net.ParseCIDR(value)
		if err != nil {
			return entry{}, fmt.Errorf("rule: invalid CIDR %q: %w", value, err)
		}
		return entry{kind: kindIPCIDR, network: network, action: action}, nil

	default:
		return entry{}, fmt.Errorf("rule: unknown rule type %q", typ)
	}
}

// parseAction converts an action string such as "DIRECT" or "VPN" into an Action.
func parseAction(s string) (Action, error) {
	upper := strings.ToUpper(strings.TrimSpace(s))
	switch upper {
	case "VPN":
		return Action{Type: ActionVPN, Outbound: "VPN"}, nil
	default:
		// DIRECT or any DIRECT-prefixed variant
		if upper == "DIRECT" || strings.HasPrefix(upper, "DIRECT") {
			return Action{Type: ActionDirect, Outbound: "DIRECT"}, nil
		}
		return Action{}, fmt.Errorf("action must be DIRECT or VPN, got %q", s)
	}
}

// InjectRules adds runtime rules checked AFTER user rules (e.g., from VPN server).
func (e *Engine) InjectRules(ruleStrings []string) error {
	entries := make([]entry, 0, len(ruleStrings))
	for _, s := range ruleStrings {
		en, err := parseRule(s)
		if err != nil {
			return err
		}
		entries = append(entries, en)
	}
	e.mu.Lock()
	e.runtimeRules = entries
	e.mu.Unlock()
	return nil
}

// ClearInjectedRules removes all runtime-injected rules.
func (e *Engine) ClearInjectedRules() {
	e.mu.Lock()
	e.runtimeRules = nil
	e.mu.Unlock()
}

func matchDomainInList(rules []entry, d string) (*Action, bool) {
	for i := range rules {
		r := &rules[i]
		switch r.kind {
		case kindDomain:
			if d == r.value {
				return &r.action, true
			}
		case kindDomainSuffix:
			if d == r.value || strings.HasSuffix(d, "."+r.value) {
				return &r.action, true
			}
		case kindDomainKeyword:
			if strings.Contains(d, r.value) {
				return &r.action, true
			}
		}
	}
	return nil, false
}

func matchIPInList(rules []entry, ip net.IP, geoip GeoIPDB) (*Action, bool) {
	for i := range rules {
		r := &rules[i]
		switch r.kind {
		case kindGeoIP:
			if geoip == nil {
				continue
			}
			cc, err := geoip.Country(ip)
			if err != nil {
				continue
			}
			if strings.EqualFold(cc, r.value) {
				return &r.action, true
			}
		case kindIPCIDR:
			if r.network.Contains(ip) {
				return &r.action, true
			}
		}
	}
	return nil, false
}

// MatchDomain returns the first matching Action for a domain query.
// Only DOMAIN, DOMAIN-SUFFIX, and DOMAIN-KEYWORD rules are evaluated.
// Matching is case-insensitive; trailing dots are stripped.
// User rules are checked first; runtime-injected rules are checked second.
func (e *Engine) MatchDomain(domain string) (*Action, bool) {
	d := strings.ToLower(strings.TrimSuffix(domain, "."))
	e.mu.RLock()
	defer e.mu.RUnlock()
	if a, ok := matchDomainInList(e.rules, d); ok {
		return a, true
	}
	return matchDomainInList(e.runtimeRules, d)
}

// MatchIP returns the first matching Action for a destination IP.
// Only GEOIP and IP-CIDR rules are evaluated.
// User rules are checked first; runtime-injected rules are checked second.
func (e *Engine) MatchIP(ip net.IP) (*Action, bool) {
	e.mu.RLock()
	defer e.mu.RUnlock()
	if a, ok := matchIPInList(e.rules, ip, e.geoip); ok {
		return a, true
	}
	return matchIPInList(e.runtimeRules, ip, e.geoip)
}
