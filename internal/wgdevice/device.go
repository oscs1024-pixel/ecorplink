// internal/wgdevice/device.go
package wgdevice

import (
	"context"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"log"
	"net"
	"net/netip"
	"strconv"
	"strings"

	"golang.zx2c4.com/wireguard/conn"
	"golang.zx2c4.com/wireguard/device"
	"golang.zx2c4.com/wireguard/tun/netstack"
)

// Config holds parameters for a WireGuard netstack device.
type Config struct {
	PrivateKeyB64      string     // base64-encoded X25519 private key
	ServerPublicKeyB64 string     // base64-encoded server public key
	ServerEndpoint     string     // "ip:port"
	ProtocolMode       int        // 1=TCP, 2=UDP
	VpnIP              netip.Addr
	DNSServers         []netip.Addr
	FallbackDNS        netip.Addr // used when DNSServers is empty
	MTU                int
}

// Device wraps a wireguard-go netstack device.
type Device struct {
	dev  *device.Device
	tnet *netstack.Net
}

// New creates and starts a WireGuard device in netstack (userspace) mode.
func New(cfg Config) (*Device, error) {
	mtu := cfg.MTU
	if mtu == 0 {
		mtu = 1420
	}

	privKeyBytes, err := base64.StdEncoding.DecodeString(cfg.PrivateKeyB64)
	if err != nil {
		return nil, fmt.Errorf("decode private key: %w", err)
	}
	pubKeyBytes, err := base64.StdEncoding.DecodeString(cfg.ServerPublicKeyB64)
	if err != nil {
		return nil, fmt.Errorf("decode server public key: %w", err)
	}

	dnsServers := cfg.DNSServers
	if len(dnsServers) == 0 && cfg.FallbackDNS.IsValid() {
		dnsServers = []netip.Addr{cfg.FallbackDNS}
		log.Printf("[wg] VPN provided no DNS servers, using fallback %s", cfg.FallbackDNS)
	}
	tdev, tnet, err := netstack.CreateNetTUN([]netip.Addr{cfg.VpnIP}, dnsServers, mtu)
	if err != nil {
		return nil, fmt.Errorf("create netstack tun: %w", err)
	}

	var bind conn.Bind
	if cfg.ProtocolMode == 1 {
		bind = conn.NewTCPBind()
	} else {
		bind = conn.NewDefaultBind()
	}

	logger := device.NewLogger(device.LogLevelSilent, "wg: ")
	dev := device.NewDevice(tdev, bind, logger)

	ipc := buildIPC(privKeyBytes, pubKeyBytes, cfg.ServerEndpoint)
	if err := dev.IpcSet(ipc); err != nil {
		dev.Close()
		return nil, fmt.Errorf("wg ipc set: %w", err)
	}
	if err := dev.Up(); err != nil {
		dev.Close()
		return nil, fmt.Errorf("wg up: %w", err)
	}

	configureWGNetstack(tnet)
	return &Device{dev: dev, tnet: tnet}, nil
}

func buildIPC(privKey, serverPubKey []byte, endpoint string) string {
	var b strings.Builder
	// Note: do NOT include "set=1" here — that prefix is for the UNIX socket
	// UAPI protocol. device.IpcSet() expects the body directly.
	b.WriteString("private_key=" + hex.EncodeToString(privKey) + "\n")
	b.WriteString("replace_peers=true\n")
	b.WriteString("public_key=" + hex.EncodeToString(serverPubKey) + "\n")
	b.WriteString("endpoint=" + endpoint + "\n")
	b.WriteString("persistent_keepalive_interval=25\n")
	b.WriteString("replace_allowed_ips=true\n")
	b.WriteString("allowed_ip=0.0.0.0/0\n")
	b.WriteString("allowed_ip=::/0\n")
	b.WriteString("\n") // UAPI requires a blank line to terminate
	return b.String()
}

// Close shuts down the WireGuard device.
func (d *Device) Close() {
	d.dev.Close()
}

// Net returns the netstack.Net for dialing through the tunnel.
func (d *Device) Net() *netstack.Net {
	return d.tnet
}

// VPNResolver returns a net.Resolver that resolves hostnames through the
// WireGuard tunnel (via the VPN's own DNS servers). Use it with a short
// deadline — the VPN DNS only services its own corporate domains; external
// hostnames may produce NXDOMAIN or time out.
func (d *Device) VPNResolver() *net.Resolver {
	return &net.Resolver{
		PreferGo: true,
		Dial: func(ctx context.Context, network, address string) (net.Conn, error) {
			return d.tnet.DialContext(ctx, network, address)
		},
	}
}

// configureWGNetstack switches the WireGuard userspace TCP stack from the
// default Reno congestion control to CUBIC and tunes RTO for lossy paths.
func configureWGNetstack(tnet *netstack.Net) {
	tnet.ConfigureCubic()
	log.Printf("[wg] netstack configured: CUBIC, minRTO=20ms, moderateRecvBuf")
}

// Stats holds cumulative byte counters for the WireGuard tunnel.
type Stats struct {
	TxBytes          int64
	RxBytes          int64
	LastHandshakeSec int64 // unix timestamp of last successful handshake, 0 if none
}

// GetStats reads cumulative tx/rx bytes from the WireGuard device via IPC.
func (d *Device) GetStats() (Stats, error) {
	out, err := d.dev.IpcGet()
	if err != nil {
		return Stats{}, fmt.Errorf("ipc get: %w", err)
	}
	var s Stats
	for _, line := range strings.Split(out, "\n") {
		k, v, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		n, err := strconv.ParseInt(strings.TrimSpace(v), 10, 64)
		if err != nil {
			continue
		}
		switch k {
		case "tx_bytes":
			s.TxBytes += n
		case "rx_bytes":
			s.RxBytes += n
		case "last_handshake_time_sec":
			if n > s.LastHandshakeSec {
				s.LastHandshakeSec = n
			}
		}
	}
	return s, nil
}
