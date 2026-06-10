package main

import (
	"context"
	"net"
	"strings"
	"testing"
	"time"

	"ecorplink/internal/config"
	"ecorplink/internal/routetable"
	vpnpkg "ecorplink/internal/vpn"
)

func TestWaitForProcessExitReturnsAfterProcessStops(t *testing.T) {
	checks := 0
	err := waitForProcessExit(1234, 100*time.Millisecond, time.Millisecond, func(pid int) bool {
		if pid != 1234 {
			t.Fatalf("pid = %d, want 1234", pid)
		}
		checks++
		return checks < 3
	})
	if err != nil {
		t.Fatalf("waitForProcessExit returned error: %v", err)
	}
	if checks != 3 {
		t.Fatalf("checks = %d, want 3", checks)
	}
}

func TestWaitForProcessExitTimesOut(t *testing.T) {
	err := waitForProcessExit(1234, 3*time.Millisecond, time.Millisecond, func(pid int) bool {
		return true
	})
	if err == nil {
		t.Fatal("waitForProcessExit should time out while process is still running")
	}
}

func TestDaemonArgsStripStartSubcommand(t *testing.T) {
	got := daemonArgs([]string{"ecorplink", "start", "-c", "config.json", "--pid-file", "/tmp/ecorplink.pid"})
	want := []string{"ecorplink", "-c", "config.json", "--pid-file", "/tmp/ecorplink.pid"}
	if len(got) != len(want) {
		t.Fatalf("daemonArgs = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("daemonArgs = %v, want %v", got, want)
		}
	}
}

func TestDaemonArgsKeepFlagOnlyInvocation(t *testing.T) {
	got := daemonArgs([]string{"ecorplink", "-c", "config.json"})
	want := []string{"ecorplink", "-c", "config.json"}
	if len(got) != len(want) {
		t.Fatalf("daemonArgs = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("daemonArgs = %v, want %v", got, want)
		}
	}
}

func TestDaemonCommandFromArgs(t *testing.T) {
	tests := []struct {
		args []string
		want string
	}{
		{[]string{"ecorplink-daemon", "install-service", "-c", "config.json"}, "install-service"},
		{[]string{"ecorplink-daemon", "uninstall-service"}, "uninstall-service"},
		{[]string{"ecorplink-daemon", "stop"}, "stop"},
		{[]string{"ecorplink-daemon", "-c", "config.json"}, ""},
	}
	for _, tt := range tests {
		if got := daemonCommandFromArgs(tt.args); got != tt.want {
			t.Fatalf("daemonCommandFromArgs(%v) = %q, want %q", tt.args, got, tt.want)
		}
	}
}

func TestLinuxSystemdServiceUnit(t *testing.T) {
	unit := linuxSystemdServiceUnit("/opt/ECorpLink/ecorplink-daemon", "/home/alice/.ecorplink/config.json", "/home/alice/.ecorplink/ecorplink.pid", "/home/alice/.ecorplink")
	for _, want := range []string{
		"Description=ECorpLink daemon",
		`Environment="ECORPLINK_DAEMON=1"`,
		`Environment="HOME=/home/alice"`,
		`WorkingDirectory="/home/alice/.ecorplink"`,
		`ExecStart="/opt/ECorpLink/ecorplink-daemon" -c "/home/alice/.ecorplink/config.json" --pid-file "/home/alice/.ecorplink/ecorplink.pid"`,
		"WantedBy=multi-user.target",
	} {
		if !strings.Contains(unit, want) {
			t.Fatalf("linux systemd unit missing %q:\n%s", want, unit)
		}
	}
}

func TestCleanupAfterReconnectDisconnectRunsFullNetworkCleanup(t *testing.T) {
	called := false
	orig := cleanupRoutesAndDNS
	cleanupRoutesAndDNS = func() {
		called = true
	}
	defer func() {
		cleanupRoutesAndDNS = orig
	}()

	cleanupAfterReconnectDisconnect()

	if !called {
		t.Fatal("reconnect teardown should run full route and DNS cleanup")
	}
}

func TestRefinedCaptureCIDRsSplitsOtherTunnelRoutes(t *testing.T) {
	entries := []routetable.Entry{
		{Dest: mustTestCIDR("64.0.0.0/2"), Iface: "utun6"},
		{Dest: mustTestCIDR("192.168.1.0/24"), Iface: "en0"},
		{Dest: mustTestCIDR("198.18.0.0/15"), Iface: "utun100"},
		{Dest: mustTestCIDR("127.0.0.0/8"), Iface: "utun6"},
	}
	got := vpnpkg.RefinedCaptureCIDRs(entries, "utun100")
	want := []string{"64.0.0.0/3", "96.0.0.0/3"}
	if len(got) != len(want) {
		t.Fatalf("refinedCaptureCIDRs = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("refinedCaptureCIDRs = %v, want %v", got, want)
		}
	}
}

func mustTestCIDR(s string) *net.IPNet {
	_, n, err := net.ParseCIDR(s)
	if err != nil {
		panic(err)
	}
	return n
}

func TestShouldSetupSystemDNSHonorsConfig(t *testing.T) {
	if shouldSetupSystemDNS(nil) {
		t.Fatal("nil config should not set system DNS")
	}
	cfg := config.DefaultConfig()
	cfg.DNS.SystemHijack = false
	if shouldSetupSystemDNS(cfg) {
		t.Fatal("system_hijack=false should not set system DNS")
	}
	cfg.DNS.SystemHijack = true
	if !shouldSetupSystemDNS(cfg) {
		t.Fatal("system_hijack=true should set system DNS")
	}
}

func TestConnectionSupervisorCancelsPreviousSession(t *testing.T) {
	var sup connectionSupervisor
	ctx1, gen1 := sup.Start(context.Background())
	ctx2, gen2 := sup.Start(context.Background())
	if gen2 <= gen1 {
		t.Fatalf("generation did not advance: %d -> %d", gen1, gen2)
	}
	select {
	case <-ctx1.Done():
	case <-time.After(time.Second):
		t.Fatal("previous connection context was not cancelled")
	}
	select {
	case <-ctx2.Done():
		t.Fatal("current connection context should still be active")
	default:
	}
	if sup.IsCurrent(gen1) {
		t.Fatal("old generation should not be current")
	}
	if !sup.IsCurrent(gen2) {
		t.Fatal("new generation should be current")
	}
	sup.Stop()
	select {
	case <-ctx2.Done():
	case <-time.After(time.Second):
		t.Fatal("current connection context was not cancelled by Stop")
	}
}
