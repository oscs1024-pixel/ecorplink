//go:build linux

package router

import (
	"fmt"
	"net"
	"os/exec"
	"strings"
)

type linuxRouter struct{ tunName string }

func NewPlatformRouter(tunName string) Router { return &linuxRouter{tunName: tunName} }
func (r *linuxRouter) SetTunName(name string) { r.tunName = name }
func (r *linuxRouter) TunName() string        { return r.tunName }

func (r *linuxRouter) AddRoute(cidr string) error {
	out, err := exec.Command("ip", "route", "add", cidr, "dev", r.tunName, "metric", "1").CombinedOutput()
	if err != nil {
		if strings.Contains(string(out), "File exists") || strings.Contains(string(out), "RTNETLINK answers: File exists") {
			return ErrRouteExists
		}
		return fmt.Errorf("ip route add %s: %s: %w", cidr, out, err)
	}
	return nil
}

func (r *linuxRouter) DelRoute(cidr string) error {
	if r.tunName != "" {
		iface, ok := r.RouteInterface(cidr)
		if ok && iface != r.tunName {
			return nil
		}
	}
	exec.Command("ip", "route", "del", cidr).Run() //nolint:errcheck
	return nil
}

func (r *linuxRouter) RouteInterface(cidr string) (string, bool) {
	ip, _, err := net.ParseCIDR(cidr)
	if err != nil || ip == nil {
		return "", false
	}
	out, err := exec.Command("ip", "-o", "route", "get", ip.String()).CombinedOutput()
	if err != nil {
		return "", false
	}
	fields := strings.Fields(string(out))
	for i := 0; i+1 < len(fields); i++ {
		if fields[i] == "dev" {
			return fields[i+1], true
		}
	}
	return "", false
}
