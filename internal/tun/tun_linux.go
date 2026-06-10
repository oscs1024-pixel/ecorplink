//go:build linux

package tun

import (
	"fmt"
	"net"
	"os/exec"
	"strconv"
)

func (d *Device) configureIP() error {
	name, _ := d.dev.Name()
	ip := net.ParseIP(d.config.IP).To4()
	if ip == nil {
		return fmt.Errorf("invalid IPv4 address: %s", d.config.IP)
	}
	if err := exec.Command("ip", "addr", "add", ip.String()+"/"+strconv.Itoa(d.config.Mask), "dev", name).Run(); err != nil {
		return err
	}
	return exec.Command("ip", "link", "set", "dev", name, "up").Run()
}
