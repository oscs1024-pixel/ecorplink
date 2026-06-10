// route-test prints the routing decision for a list of domains and IPs,
// using the rule engine and geoip database from the local config.
package main

import (
	"fmt"
	"net"
	"os"

	"ecorplink/internal/config"
	"ecorplink/internal/geoip"
	"ecorplink/internal/rule"
)

func main() {
	cfg, err := config.LoadConfig("config.json")
	if err != nil {
		fmt.Fprintln(os.Stderr, "load config:", err)
		os.Exit(1)
	}

	geoDB, err := geoip.Open(cfg.GeoIP.File)
	if err != nil {
		fmt.Fprintln(os.Stderr, "open geoip:", err)
		os.Exit(1)
	}
	defer geoDB.Close()

	ruleStrings, err := cfg.RuleStrings()
	if err != nil {
		fmt.Fprintln(os.Stderr, "rules:", err)
		os.Exit(1)
	}
	engine, err := rule.NewEngine(ruleStrings, geoDB)
	if err != nil {
		fmt.Fprintln(os.Stderr, "new engine:", err)
		os.Exit(1)
	}

	fmt.Printf("%-40s  %-12s  %s\n", "目标", "决策", "命中规则")
	fmt.Printf("%-40s  %-12s  %s\n", "------", "------", "------")

	domains := []string{
		"github.com",
		"raw.githubusercontent.com",
		"google.com",
		"www.google.com",
		"youtube.com",
		"telegram.org",
		"baidu.com",
		"www.baidu.com",
		"zhihu.com",
		"bilibili.com",
		"qq.com",
		"example.com",
		"feishu.cn",
		"microsoft.com",
		"bing.com",
		"icloud.com",
		"apple.com",
		"speedtest.net",
		"notion.so",
		"anthropic.com",
	}

	for _, d := range domains {
		action, matched := engine.MatchDomain(d)
		if !matched {
			fmt.Printf("%-40s  %-12s  %s\n", d, "→ 飞连", "(无匹配)")
			continue
		}
		label := actionLabel(action)
		hint := matchHint(d, ruleStrings)
		fmt.Printf("%-40s  %-12s  %s\n", d, label, hint)
	}

	fmt.Println()
	fmt.Printf("%-40s  %-12s  %s\n", "IP", "决策", "GeoIP/CIDR")
	fmt.Printf("%-40s  %-12s  %s\n", "------", "------", "------")

	ips := []struct{ ip, note string }{
		{"180.101.50.188", "baidu.com"},
		{"14.119.104.189", "qq.com"},
		{"110.242.68.66", "bilibili.com"},
		{"220.185.184.16", "failing IP"},
		{"8.8.8.8", "Google DNS"},
		{"140.82.112.3", "github.com"},
		{"172.217.16.46", "google.com"},
		{"91.108.56.130", "Telegram"},
		{"192.168.1.1", "私网"},
		{"10.0.0.1", "私网"},
		{"114.114.114.114", "114 DNS"},
		{"1.1.1.1", "Cloudflare DNS"},
		{"93.184.216.34", "example.com"},
	}

	for _, entry := range ips {
		ip := net.ParseIP(entry.ip)
		if ip == nil {
			continue
		}
		action, matched := engine.MatchIP(ip)
		cc, _ := geoDB.Country(ip)
		if cc == "" {
			cc = "?"
		}
		if !matched {
			fmt.Printf("%-40s  %-12s  GeoIP=%s\n", entry.ip+" ("+entry.note+")", "→ 飞连", cc)
			continue
		}
		label := actionLabel(action)
		fmt.Printf("%-40s  %-12s  GeoIP=%s\n", entry.ip+" ("+entry.note+")", label, cc)
	}
}

func actionLabel(a *rule.Action) string {
	if a.Type == rule.ActionVPN {
		return "VPN"
	}
	return "DIRECT(" + a.Outbound + ")"
}

func matchHint(domain string, rules []string) string {
	dl := lowercase(domain)
	for _, r := range rules {
		parts := splitN3(r)
		if parts == nil {
			continue
		}
		typ, val := parts[0], lowercase(parts[1])
		switch typ {
		case "DOMAIN":
			if dl == val {
				return r
			}
		case "DOMAIN-SUFFIX":
			if dl == val || hasSuffix(dl, "."+val) {
				return r
			}
		case "DOMAIN-KEYWORD":
			if contains(dl, val) {
				return r
			}
		}
	}
	return ""
}

func splitN3(s string) []string {
	out := make([]string, 0, 3)
	rest := s
	for i := 0; i < 2; i++ {
		idx := indexByte(rest, ',')
		if idx < 0 {
			return nil
		}
		out = append(out, rest[:idx])
		rest = rest[idx+1:]
	}
	out = append(out, rest)
	return out
}

func lowercase(s string) string {
	b := []byte(s)
	for i, c := range b {
		if c >= 'A' && c <= 'Z' {
			b[i] = c + 32
		}
	}
	return string(b)
}

func hasSuffix(s, suffix string) bool {
	return len(s) >= len(suffix) && s[len(s)-len(suffix):] == suffix
}

func contains(s, sub string) bool {
	return len(sub) > 0 && len(s) >= len(sub) && func() bool {
		for i := 0; i <= len(s)-len(sub); i++ {
			if s[i:i+len(sub)] == sub {
				return true
			}
		}
		return false
	}()
}

func indexByte(s string, b byte) int {
	for i := 0; i < len(s); i++ {
		if s[i] == b {
			return i
		}
	}
	return -1
}
