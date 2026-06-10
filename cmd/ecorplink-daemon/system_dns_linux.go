//go:build linux

package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net"
	"os"
	"os/exec"
	"path/filepath"
)

const systemDNSStateFile = "dns-state.json"

type systemDNSState struct {
	Method  string `json:"method"`
	TunName string `json:"tun_name,omitempty"`
	Path    string `json:"path,omitempty"`
	Content string `json:"content,omitempty"`
	Applied bool   `json:"applied"`
}

func systemDNSAddr() string { return "" }

func systemDNSOriginalUpstream() string { return "" }

func prepareSystemDNSListener() error { return nil }

func cleanupSystemDNSListener() {}

func flushDNSCache() {
	if exec.Command("systemd-resolve", "--flush-caches").Run() != nil {
		exec.Command("nscd", "-i", "hosts").Run() //nolint:errcheck
	}
	log.Printf("[dns] flushed DNS cache")
}

func setupSystemDNS(tunName string, serverIP string) (*systemDNSState, error) {
	if net.ParseIP(serverIP) == nil {
		return nil, fmt.Errorf("invalid DNS server IP %q", serverIP)
	}
	if tunName != "" {
		if state, err := setupSystemDNSResolved(tunName, serverIP); err == nil {
			return state, nil
		} else {
			log.Printf("[dns] resolvectl setup failed, falling back to /etc/resolv.conf: %v", err)
		}
	}
	return setupSystemDNSResolvConf(serverIP)
}

func setupSystemDNSResolved(tunName, serverIP string) (*systemDNSState, error) {
	resolvectl, err := exec.LookPath("resolvectl")
	if err != nil {
		resolvectl, err = exec.LookPath("systemd-resolve")
		if err != nil {
			return nil, err
		}
	}
	state := &systemDNSState{Method: "resolvectl", TunName: tunName, Applied: true}
	if err := saveSystemDNSState(state); err != nil {
		return nil, err
	}
	commands := [][]string{
		{"dns", tunName, serverIP},
		{"domain", tunName, "~."},
		{"default-route", tunName, "yes"},
	}
	for _, args := range commands {
		if out, err := exec.Command(resolvectl, args...).CombinedOutput(); err != nil {
			return nil, fmt.Errorf("%s %v: %s: %w", resolvectl, args, out, err)
		}
	}
	return state, nil
}

func setupSystemDNSResolvConf(serverIP string) (*systemDNSState, error) {
	const resolvConf = "/etc/resolv.conf"
	data, err := os.ReadFile(resolvConf)
	if err != nil {
		return nil, err
	}
	state := &systemDNSState{Method: "resolv_conf", Path: resolvConf, Content: string(data), Applied: true}
	if err := saveSystemDNSState(state); err != nil {
		return nil, err
	}
	next := fmt.Sprintf("# ecorplink managed; restored on stop\nnameserver %s\noptions timeout:1 attempts:2\n", serverIP)
	if err := os.WriteFile(resolvConf, []byte(next), 0644); err != nil {
		return nil, err
	}
	return state, nil
}

func cleanupSystemDNS() {
	state, err := loadSystemDNSState()
	if err != nil {
		return
	}
	switch state.Method {
	case "resolvectl":
		if state.TunName != "" {
			if resolvectl, err := exec.LookPath("resolvectl"); err == nil {
				exec.Command(resolvectl, "revert", state.TunName).Run() //nolint:errcheck
			} else if legacy, err := exec.LookPath("systemd-resolve"); err == nil {
				exec.Command(legacy, "revert", state.TunName).Run() //nolint:errcheck
			}
		}
	default:
		if state.Path != "" {
			_ = os.WriteFile(state.Path, []byte(state.Content), 0644)
		}
	}
	_ = os.Remove(systemDNSStatePath())
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
