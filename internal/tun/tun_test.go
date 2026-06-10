package tun

import (
	"testing"
)

func TestTUNConfig(t *testing.T) {
	cfg := Config{
		Name: "rtun0",
		IP:   "172.30.77.1",
		Mask: 30,
		MTU:  1420,
	}
	if cfg.MTU != 1420 {
		t.Fatalf("MTU = %d, want 1420", cfg.MTU)
	}
}

func TestDevicePeerIP(t *testing.T) {
	tests := []struct {
		ip   string
		want string
	}{
		{"172.30.77.1", "172.30.77.2"},
		{"10.0.0.1", "10.0.0.2"},
		{"192.168.1.5", "192.168.1.6"},
	}
	for _, tt := range tests {
		d := &Device{config: Config{IP: tt.ip, Mask: 30}}
		got := d.PeerIP()
		if got != tt.want {
			t.Errorf("PeerIP(%q) = %q, want %q", tt.ip, got, tt.want)
		}
	}
}

func TestDevicePeerIPInvalid(t *testing.T) {
	d := &Device{config: Config{IP: "not-an-ip", Mask: 30}}
	if got := d.PeerIP(); got != "" {
		t.Errorf("PeerIP(invalid) = %q, want empty", got)
	}
}
