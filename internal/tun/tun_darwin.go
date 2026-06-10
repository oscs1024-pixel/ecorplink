//go:build darwin

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
	peerIP := make(net.IP, len(ip))
	copy(peerIP, ip)
	peerIP[3]++
	cmd := exec.Command("ifconfig", name, "inet", ip.String(), peerIP.String(), "netmask", net.IP(mask).String(), "up")
	if err := cmd.Run(); err != nil {
		return err
	}
	return nil
}
