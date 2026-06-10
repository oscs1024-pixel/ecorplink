//go:build windows

package tun

import (
	"fmt"
	"net"
	"os/exec"
)

func (d *Device) configureIP() error {
	name, _ := d.dev.Name()
	ip := net.ParseIP(d.config.IP).To4()
	if ip == nil {
		return fmt.Errorf("invalid IPv4 address: %s", d.config.IP)
	}
	mask := net.CIDRMask(d.config.Mask, 32)

	// Set IP and subnet mask only. Do NOT set a default gateway here.
	// Wintun is a point-to-point interface; adding a default gateway would
	// cause Windows to route all traffic (0.0.0.0/0) through it, breaking
	// the network. Per-target /32 routes are added separately by the router.
	cmd := exec.Command("netsh", "interface", "ip", "set", "address", "name="+name, "static", ip.String(), net.IP(mask).String())
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("set address: %w", err)
	}

	// Set high metric so Windows doesn't prefer this interface over physical adapters.
	// This prevents Windows from trying to route general traffic through Wintun.
	cmd = exec.Command("netsh", "interface", "ip", "set", "interface", "interface="+name, "metric=9999")
	_ = cmd.Run() // non-fatal: best-effort to avoid interfering with normal networking

	return nil
}
