//go:build windows

package router

import (
	"fmt"
	"net"
	"os/exec"
	"strconv"
	"strings"
)

type windowsRouter struct{ tunName string }

func NewPlatformRouter(tunName string) Router   { return &windowsRouter{tunName: tunName} }
func (r *windowsRouter) SetTunName(name string) { r.tunName = name }
func (r *windowsRouter) TunName() string        { return r.tunName }

func (r *windowsRouter) AddRoute(cidr string) error {
	// Get interface index for tunName
	idx, err := getInterfaceIndex(r.tunName)
	if err != nil {
		return fmt.Errorf("router: get iface index %s: %w", r.tunName, err)
	}
	parts := strings.Split(cidr, "/")
	if len(parts) != 2 {
		return fmt.Errorf("router: invalid CIDR %s", cidr)
	}
	script := fmt.Sprintf(
		"New-NetRoute -DestinationPrefix '%s' -InterfaceIndex %d -RouteMetric 1 -PolicyStore ActiveStore",
		cidr, idx,
	)
	out, err := exec.Command("powershell", "-NoProfile", "-Command", script).CombinedOutput()
	if err != nil {
		if strings.Contains(string(out), "already exists") || strings.Contains(string(out), "Instance MSFT_NetRoute already exists") {
			return ErrRouteExists
		}
		return fmt.Errorf("New-NetRoute %s: %s: %w", cidr, out, err)
	}
	return nil
}

func (r *windowsRouter) DelRoute(cidr string) error {
	if r.tunName != "" {
		iface, ok := r.RouteInterface(cidr)
		if ok && !strings.EqualFold(iface, r.tunName) {
			return nil
		}
	}
	script := fmt.Sprintf("Remove-NetRoute -DestinationPrefix '%s' -Confirm:$false -ErrorAction SilentlyContinue", cidr)
	exec.Command("powershell", "-NoProfile", "-Command", script).Run() //nolint:errcheck
	return nil
}

func (r *windowsRouter) RouteInterface(cidr string) (string, bool) {
	ip, _, err := net.ParseCIDR(cidr)
	if err != nil || ip == nil {
		return "", false
	}
	script := fmt.Sprintf(
		"$route = Get-NetRoute -DestinationPrefix '%s' -ErrorAction SilentlyContinue | Sort-Object RouteMetric | Select-Object -First 1; if ($route) { (Get-NetAdapter -InterfaceIndex $route.InterfaceIndex).Name }",
		cidr,
	)
	out, err := exec.Command("powershell", "-NoProfile", "-Command", script).CombinedOutput()
	if err != nil {
		return "", false
	}
	iface := strings.TrimSpace(string(out))
	if iface == "" {
		return "", false
	}
	return iface, true
}

func getInterfaceIndex(name string) (int, error) {
	script := fmt.Sprintf("(Get-NetAdapter -Name '%s').ifIndex", name)
	out, err := exec.Command("powershell", "-NoProfile", "-Command", script).Output()
	if err != nil {
		return 0, err
	}
	idx, err := strconv.Atoi(strings.TrimSpace(string(out)))
	if err != nil {
		return 0, fmt.Errorf("parse iface index %q: %w", string(out), err)
	}
	return idx, nil
}
