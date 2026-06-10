package vpn

import (
	"context"
	"net"
	"testing"

	"ecorplink/internal/config"
)

func TestEvaluateConnectionDeadRequiresStaleHandshakeAndStableRx(t *testing.T) {
	dead, lastRx := evaluateConnectionDead(1_000, 100, 699, 42, 42)
	if !dead {
		t.Fatal("connection should be dead when handshake is stale and rx_bytes did not grow")
	}
	if lastRx != 42 {
		t.Fatalf("lastRx = %d, want 42", lastRx)
	}
}

func TestEvaluateConnectionDeadKeepsAliveWhenRxGrows(t *testing.T) {
	dead, lastRx := evaluateConnectionDead(1_000, 100, 699, 43, 42)
	if dead {
		t.Fatal("connection should not be dead when rx_bytes grows despite stale handshake")
	}
	if lastRx != 43 {
		t.Fatalf("lastRx = %d, want 43", lastRx)
	}
}

func TestEvaluateConnectionDeadUpdatesRxForFreshHandshake(t *testing.T) {
	dead, lastRx := evaluateConnectionDead(1_000, 100, 800, 7, 0)
	if dead {
		t.Fatal("connection should not be dead while handshake is fresh")
	}
	if lastRx != 7 {
		t.Fatalf("lastRx = %d, want 7", lastRx)
	}
}

func TestEvaluateConnectionDeadWaitsForInitialHandshakeGrace(t *testing.T) {
	dead, lastRx := evaluateConnectionDead(1_000, 940, 0, 9, 3)
	if dead {
		t.Fatal("connection should get an initial grace period before first handshake")
	}
	if lastRx != 9 {
		t.Fatalf("lastRx = %d, want 9", lastRx)
	}
}

func TestEvaluateConnectionDeadDetectsMissingInitialHandshake(t *testing.T) {
	dead, lastRx := evaluateConnectionDead(1_000, 899, 0, 0, 0)
	if !dead {
		t.Fatal("connection should be dead when no handshake arrives after grace period")
	}
	if lastRx != 0 {
		t.Fatalf("lastRx = %d, want 0", lastRx)
	}
}

func TestSOCKS5ListenerStartsAndStopsWithManagerCleanup(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.SOCKS5.Enabled = true
	cfg.SOCKS5.BindHost = "127.0.0.1"
	cfg.SOCKS5.Port = 0
	m := New(cfg)

	err := m.startSOCKS5Locked(func(ctx context.Context, network, address string) (net.Conn, error) {
		var d net.Dialer
		return d.DialContext(ctx, network, address)
	}, staticVPNResolver{})
	if err != nil {
		t.Fatal(err)
	}
	addr := m.socks5.Addr()
	conn, err := net.Dial("tcp", addr)
	if err != nil {
		t.Fatalf("socks5 listener did not accept connections: %v", err)
	}
	conn.Close()

	m.cleanupLocked()
	if conn, err := net.Dial("tcp", addr); err == nil {
		conn.Close()
		t.Fatal("socks5 listener still accepts connections after cleanup")
	}
}

type staticVPNResolver struct{}

func (staticVPNResolver) Resolve(ctx context.Context, name string) (context.Context, net.IP, error) {
	return ctx, net.ParseIP("127.0.0.1"), nil
}
