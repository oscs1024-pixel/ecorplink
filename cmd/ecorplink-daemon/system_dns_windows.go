//go:build windows

package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
)

const systemDNSStateFile = "dns-state.json"

type windowsDNSRecord struct {
	InterfaceIndex  int      `json:"interface_index"`
	ServerAddresses []string `json:"server_addresses"`
}

type systemDNSState struct {
	Records []windowsDNSRecord `json:"records"`
	Applied bool               `json:"applied"`
}

func systemDNSAddr() string { return "" }

func systemDNSOriginalUpstream() string { return "" }

func prepareSystemDNSListener() error { return nil }

func cleanupSystemDNSListener() {}

func setupSystemDNS(_ string, serverIP string) (*systemDNSState, error) {
	if net.ParseIP(serverIP) == nil {
		return nil, fmt.Errorf("invalid DNS server IP %q", serverIP)
	}
	records, err := readWindowsDNS()
	if err != nil {
		return nil, err
	}
	state := &systemDNSState{Records: records, Applied: true}
	if err := saveSystemDNSState(state); err != nil {
		return nil, err
	}
	script := fmt.Sprintf(`Get-NetAdapter | Where-Object {$_.Status -eq 'Up'} | ForEach-Object { Set-DnsClientServerAddress -InterfaceIndex $_.ifIndex -ServerAddresses %q }`, serverIP)
	if out, err := exec.Command("powershell", "-NoProfile", "-Command", script).CombinedOutput(); err != nil {
		return nil, fmt.Errorf("Set-DnsClientServerAddress: %s: %w", out, err)
	}
	return state, nil
}

func cleanupSystemDNS() {
	state, err := loadSystemDNSState()
	if err != nil {
		return
	}
	for _, rec := range state.Records {
		var script string
		if len(rec.ServerAddresses) == 0 {
			script = fmt.Sprintf("Set-DnsClientServerAddress -InterfaceIndex %d -ResetServerAddresses", rec.InterfaceIndex)
		} else {
			quoted := make([]string, 0, len(rec.ServerAddresses))
			for _, addr := range rec.ServerAddresses {
				quoted = append(quoted, strconv.Quote(addr))
			}
			script = fmt.Sprintf("Set-DnsClientServerAddress -InterfaceIndex %d -ServerAddresses @(%s)", rec.InterfaceIndex, strings.Join(quoted, ","))
		}
		exec.Command("powershell", "-NoProfile", "-Command", script).Run() //nolint:errcheck
	}
	_ = os.Remove(systemDNSStatePath())
}

func readWindowsDNS() ([]windowsDNSRecord, error) {
	script := `Get-DnsClientServerAddress -AddressFamily IPv4 | Select-Object InterfaceIndex,ServerAddresses | ConvertTo-Json -Compress`
	out, err := exec.Command("powershell", "-NoProfile", "-Command", script).Output()
	if err != nil {
		return nil, err
	}
	raw := strings.TrimSpace(string(out))
	if raw == "" {
		return nil, nil
	}
	if strings.HasPrefix(raw, "[") {
		var records []windowsDNSRecord
		return records, json.Unmarshal([]byte(raw), &records)
	}
	var record windowsDNSRecord
	if err := json.Unmarshal([]byte(raw), &record); err != nil {
		return nil, err
	}
	return []windowsDNSRecord{record}, nil
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

func flushDNSCache() {
	exec.Command("ipconfig", "/flushdns").Run() //nolint:errcheck
	log.Printf("[dns] flushed DNS cache")
}
