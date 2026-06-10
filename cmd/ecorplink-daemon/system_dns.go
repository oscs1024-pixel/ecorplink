//go:build darwin

package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"
)

const (
	systemDNSStateFile = "dns-state.json"
	systemDNSKeyPrefix = "State:/Network/Service/"
	systemDNSKeySuffix = "/DNS"
)

var resolvConfMaintainer = struct {
	sync.Mutex
	stop chan struct{}
}{}

type systemDNSState struct {
	ServiceKey        string `json:"service_key"`
	ManagedServiceKey bool   `json:"managed_service_key"`
	GlobalScutil      string `json:"global_scutil"`
	PFEnabled         bool   `json:"pf_enabled"`
	ResolvConfPath    string `json:"resolv_conf_path"`
	ResolvConfContent string `json:"resolv_conf_content"`
	Applied           bool   `json:"applied"`
}

func systemDNSAddr() string {
	return ""
}

func systemDNSOriginalUpstream() string {
	_, raw, err := findSupplementalDNSKey()
	if err != nil {
		return ""
	}
	return firstUsableSystemDNSUpstream(raw)
}

func firstUsableSystemDNSUpstream(raw string) string {
	addrs := scutilArray(raw, "ServerAddresses")
	port := scutilScalar(raw, "ServerPort")
	if port == "" {
		port = "53"
	}
	for _, addr := range addrs {
		ip := net.ParseIP(addr)
		if ip != nil && !ip.IsLoopback() {
			return net.JoinHostPort(addr, port)
		}
	}
	return ""
}

func setupSystemDNS(tunName, serverIP string) (*systemDNSState, error) {
	if strings.TrimSpace(tunName) == "" {
		return nil, fmt.Errorf("empty TUN name")
	}
	if net.ParseIP(serverIP) == nil {
		return nil, fmt.Errorf("invalid DNS server IP %q", serverIP)
	}
	cleanupSystemDNS()
	ifIndex, err := interfaceIndexForIP(serverIP)
	if err != nil {
		return nil, err
	}
	key := ownServiceDNSKey(tunName)
	resolvContent, err := os.ReadFile(systemResolvConfPath())
	if err != nil {
		return nil, err
	}
	state := &systemDNSState{
		ServiceKey:        key,
		ManagedServiceKey: true,
		ResolvConfPath:    systemResolvConfPath(),
		ResolvConfContent: string(resolvContent),
		Applied:           true,
	}
	if err := saveSystemDNSState(state); err != nil {
		return nil, err
	}
	if _, err := runScutil(ownServiceDNSScript(tunName, serverIP, ifIndex)); err != nil {
		return nil, err
	}
	if err := setupResolvConfDNS(serverIP); err != nil {
		return nil, err
	}
	startResolvConfMaintainer(systemResolvConfPath(), serverIP, 500*time.Millisecond)
	return state, nil
}

func cleanupSystemDNS() {
	stopResolvConfMaintainer()
	state, err := loadSystemDNSState()
	if err != nil {
		return
	}
	if state.GlobalScutil != "" {
		_ = restoreScutilDictionary("State:/Network/Global/DNS", state.GlobalScutil)
	} else if !state.ManagedServiceKey {
		_ = removeScutilKey("State:/Network/Global/DNS")
	}
	if state.ManagedServiceKey && state.ServiceKey != "" {
		_ = removeScutilKey(state.ServiceKey)
	}
	if state.PFEnabled {
		cleanupPFDNSRedirect()
	}
	if state.ResolvConfPath != "" {
		_ = os.WriteFile(state.ResolvConfPath, []byte(state.ResolvConfContent), 0644)
	}
	_ = os.Remove(systemDNSStatePath())
}

func prepareSystemDNSListener() error {
	return nil
}

func cleanupSystemDNSListener() {
}

func flushDNSCache() {
	exec.Command("dscacheutil", "-flushcache").Run()       //nolint:errcheck
	exec.Command("killall", "-HUP", "mDNSResponder").Run() //nolint:errcheck
	log.Printf("[dns] flushed DNS cache")
}

func systemResolvConfPath() string {
	return "/etc/resolv.conf"
}

func setupResolvConfDNS(serverIP string) error {
	_, err := ensureResolvConfDNS(systemResolvConfPath(), serverIP)
	return err
}

func ensureResolvConfDNS(path, serverIP string) (bool, error) {
	next := resolvConfContent(serverIP)
	got, err := os.ReadFile(path)
	if err == nil && string(got) == next {
		return false, nil
	}
	if err := os.WriteFile(path, []byte(next), 0644); err != nil {
		return false, err
	}
	got, err = os.ReadFile(path)
	if err != nil {
		return false, err
	}
	if string(got) != next {
		return false, fmt.Errorf("resolv.conf write did not persist")
	}
	return true, nil
}

func startResolvConfMaintainer(path, serverIP string, interval time.Duration) {
	stopResolvConfMaintainer()
	resolvConfMaintainer.Lock()
	stop := make(chan struct{})
	resolvConfMaintainer.stop = stop
	resolvConfMaintainer.Unlock()
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				ensureResolvConfDNS(path, serverIP) //nolint:errcheck
			case <-stop:
				return
			}
		}
	}()
}

func stopResolvConfMaintainer() {
	resolvConfMaintainer.Lock()
	stop := resolvConfMaintainer.stop
	resolvConfMaintainer.stop = nil
	resolvConfMaintainer.Unlock()
	if stop != nil {
		close(stop)
	}
}

func setupPFDNSRedirect(serverIP string) error {
	cmd := exec.Command("pfctl", "-a", "com.ecorplink", "-f", "-")
	cmd.Stdin = strings.NewReader(pfDNSRedirectRules(serverIP))
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("pfctl load com.ecorplink: %s: %w", out.String(), err)
	}
	cmd = exec.Command("pfctl", "-E")
	cmd.Stdout = &out
	cmd.Stderr = &out
	_ = cmd.Run()
	return nil
}

func cleanupPFDNSRedirect() {
	exec.Command("pfctl", "-a", "com.ecorplink", "-F", "all").Run() //nolint:errcheck
}

func pfDNSRedirectRules(serverIP string) string {
	return fmt.Sprintf(`rdr pass on lo0 inet proto udp from any to 127.0.0.1 port 53 -> %s port 53
rdr pass on lo0 inet proto tcp from any to 127.0.0.1 port 53 -> %s port 53
`, serverIP, serverIP)
}

func findSupplementalDNSKey() (string, string, error) {
	out, err := runScutil("list State:/Network/Service/.*/DNS\n")
	if err != nil {
		return "", "", err
	}
	keys := scutilSubkeys(out)
	for _, key := range keys {
		raw, err := showScutilKey(key)
		if err != nil {
			continue
		}
		if strings.Contains(raw, "SupplementalMatchDomains") &&
			strings.Contains(raw, "127.0.0.1") {
			return key, raw, nil
		}
	}
	return "", "", fmt.Errorf("no supplemental DNS service found")
}

func ownServiceDNSKey(tunName string) string {
	return systemDNSKeyPrefix + tunName + systemDNSKeySuffix
}

func ownServiceDNSScript(tunName, serverIP string, ifIndex int) string {
	return fmt.Sprintf(`open
d.init
d.add ConfirmedServiceID %s
d.add SearchOrder # 0
d.add ServerAddresses * %s
d.add ServerPort # 53
d.add SupplementalMatchDomains * ""
d.add __CONFIGURATION_ID__ "Supplemental: %s 0"
d.add __FLAGS__ # 16390
d.add __IF_INDEX__ # %d
d.add __ORDER__ # 0
set %s
quit
`, tunName, serverIP, tunName, ifIndex, ownServiceDNSKey(tunName))
}
func resolvConfContent(serverIP string) string {
	return fmt.Sprintf("# ecorplink managed; restored on stop\nnameserver %s\noptions timeout:1 attempts:2\n", serverIP)
}

func interfaceIndexForIP(ip string) (int, error) {
	target := net.ParseIP(ip)
	if target == nil {
		return 0, fmt.Errorf("invalid interface IP %q", ip)
	}
	ifaces, err := net.Interfaces()
	if err != nil {
		return 0, err
	}
	for _, iface := range ifaces {
		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}
		for _, addr := range addrs {
			var current net.IP
			switch v := addr.(type) {
			case *net.IPNet:
				current = v.IP
			case *net.IPAddr:
				current = v.IP
			}
			if current != nil && current.Equal(target) {
				return iface.Index, nil
			}
		}
	}
	return 0, fmt.Errorf("no interface owns IP %s", ip)
}

func showScutilKey(key string) (string, error) {
	return runScutil(fmt.Sprintf("open\nshow %s\nquit\n", key))
}

func restoreScutilDictionary(key, raw string) error {
	script, err := scutilRestoreScript(key, raw)
	if err != nil {
		return err
	}
	_, err = runScutil(script)
	return err
}

func removeScutilKey(key string) error {
	_, err := runScutil(fmt.Sprintf("open\nremove %s\nquit\n", key))
	return err
}

func runScutil(script string) (string, error) {
	cmd := exec.Command("scutil")
	cmd.Stdin = strings.NewReader(script)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return stdout.String(), fmt.Errorf("scutil: %s: %w", stderr.String(), err)
	}
	return stdout.String(), nil
}

func scutilSubkeys(out string) []string {
	re := regexp.MustCompile(`subKey \[\d+\] = (.+)`)
	var keys []string
	for _, m := range re.FindAllStringSubmatch(out, -1) {
		keys = append(keys, strings.TrimSpace(m[1]))
	}
	return keys
}

func scutilArray(raw, name string) []string {
	lines := strings.Split(raw, "\n")
	for i := 0; i < len(lines); i++ {
		line := strings.TrimSpace(lines[i])
		if !strings.HasPrefix(line, name+" : <array>") {
			continue
		}
		var vals []string
		for i++; i < len(lines); i++ {
			entry := strings.TrimSpace(lines[i])
			if entry == "}" {
				break
			}
			entryParts := strings.SplitN(entry, ":", 2)
			if len(entryParts) == 2 {
				vals = append(vals, strings.TrimSpace(entryParts[1]))
			}
		}
		return vals
	}
	return nil
}

func scutilScalar(raw, name string) string {
	for _, line := range strings.Split(raw, "\n") {
		line = strings.TrimSpace(line)
		parts := strings.SplitN(line, ":", 2)
		if len(parts) == 2 && strings.TrimSpace(parts[0]) == name {
			return strings.TrimSpace(parts[1])
		}
	}
	return ""
}

func scutilRestoreScript(key, raw string) (string, error) {
	var b strings.Builder
	b.WriteString("open\nd.init\n")
	lines := strings.Split(raw, "\n")
	for i := 0; i < len(lines); i++ {
		line := strings.TrimSpace(lines[i])
		if line == "" || line == "<dictionary> {" || line == "}" {
			continue
		}
		parts := strings.SplitN(line, ":", 2)
		if len(parts) != 2 {
			continue
		}
		name := strings.TrimSpace(parts[0])
		value := strings.TrimSpace(parts[1])
		if value == "<array> {" {
			var vals []string
			for i++; i < len(lines); i++ {
				entry := strings.TrimSpace(lines[i])
				if entry == "}" {
					break
				}
				entryParts := strings.SplitN(entry, ":", 2)
				if len(entryParts) == 2 {
					vals = append(vals, strings.TrimSpace(entryParts[1]))
				}
			}
			b.WriteString("d.add " + name + " *")
			for _, v := range vals {
				b.WriteString(" " + scutilQuote(v))
			}
			b.WriteString("\n")
			continue
		}
		if isDecimal(value) {
			b.WriteString("d.add " + name + " # " + value + "\n")
		} else {
			b.WriteString("d.add " + name + " " + scutilQuote(value) + "\n")
		}
	}
	b.WriteString("set " + key + "\nquit\n")
	return b.String(), nil
}

func isDecimal(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

func scutilQuote(s string) string {
	if s == "" {
		return `""`
	}
	if strings.ContainsAny(s, " \t\n\"") {
		return `"` + strings.ReplaceAll(s, `"`, `\"`) + `"`
	}
	return s
}

func saveSystemDNSState(state *systemDNSState) error {
	dir := filepath.Dir(systemDNSStatePath())
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(systemDNSStatePath(), data, 0644)
}

func loadSystemDNSState() (*systemDNSState, error) {
	data, err := os.ReadFile(systemDNSStatePath())
	if err != nil {
		return nil, err
	}
	var state systemDNSState
	if err := json.Unmarshal(data, &state); err != nil {
		return nil, err
	}
	return &state, nil
}

func systemDNSStatePath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return filepath.Join(".ecorplink", systemDNSStateFile)
	}
	return filepath.Join(home, ".ecorplink", systemDNSStateFile)
}
