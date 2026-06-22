//go:build darwin

package forwarder

import (
	"fmt"
	"net"
	"os/exec"
	"strings"
)

func addScopedHostRoute(ip, iface string) error {
	gateway, err := scopedGateway("default", iface)
	if err != nil {
		return err
	}
	if gateway != nil && !gatewayReachableOnInterface(gateway, iface) {
		gateway = nil
		if fallback, ferr := defaultGatewayForInterface(iface); ferr == nil && fallback != nil {
			gateway = fallback
		}
	}
	if gateway == nil && !isTunnelInterface(iface) {
		return fmt.Errorf("no gateway for physical iface %s", iface)
	}
	if routeNeedsReplace(ip, iface, gateway) {
		deleteHostRoute(ip) //nolint:errcheck
		forgetHostRoute(ip)
	}
	return addHostRoute(ip, iface, gateway)
}

func addHostRoute(ip, iface string, gateway net.IP) error {
	if net.ParseIP(ip).To4() == nil {
		return nil
	}
	if routeNeedsReplace(ip, iface, gateway) {
		deleteHostRoute(ip) //nolint:errcheck
		forgetHostRoute(ip)
	}
	var args []string
	if gateway != nil && gateway.To4() != nil && !gateway.IsUnspecified() {
		args = []string{"add", "-host", ip, gateway.String()}
	} else {
		args = []string{"add", "-host", ip, "-interface", iface}
	}
	out, err := exec.Command("route", args...).CombinedOutput()
	if err != nil && strings.Contains(string(out), "File exists") {
		if delErr := exec.Command("route", "delete", "-host", ip).Run(); delErr != nil {
			return fmt.Errorf("route delete %s (before retry): %w", ip, delErr)
		}
		out, err = exec.Command("route", args...).CombinedOutput()
	}
	if err != nil {
		return fmt.Errorf("route %v: %s: %w", args, out, err)
	}
	return nil
}

func deleteHostRoute(ip string) error {
	exec.Command("route", "delete", "-host", ip).Run() //nolint:errcheck
	return nil
}

func routeNeedsReplace(ip, iface string, gateway net.IP) bool {
	current, ok := hostRouteCurrent(ip, iface, gateway)
	return ok && !current
}

func hostRouteCurrent(ip, iface string, gateway net.IP) (bool, bool) {
	out, err := exec.Command("route", "-n", "get", ip).CombinedOutput()
	if err != nil {
		return false, false
	}
	return !routeOutputNeedsReplace(out, iface, gateway), true
}

func scopedHostRouteCurrent(ip, iface string) (bool, bool) {
	gateway, err := scopedGateway("default", iface)
	if err != nil {
		return false, false
	}
	if gateway != nil && !gatewayReachableOnInterface(gateway, iface) {
		gateway = nil
		if fallback, ferr := defaultGatewayForInterface(iface); ferr == nil && fallback != nil {
			gateway = fallback
		}
	}
	return hostRouteCurrent(ip, iface, gateway)
}

func routeOutputNeedsReplace(out []byte, iface string, gateway net.IP) bool {
	gotIface := parseRouteInterface(out)
	if gotIface != "" && gotIface != iface {
		return true
	}
	gotGateway := parseRouteGateway(out)
	if gateway == nil || gateway.To4() == nil || gateway.IsUnspecified() {
		return false
	}
	return gotGateway == nil || !gotGateway.Equal(gateway)
}

func scopedGateway(ip, iface string) (net.IP, error) {
	out, err := exec.Command("route", "-n", "get", ip, "-ifscope", iface).CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("route get %s ifscope %s: %s: %w", ip, iface, out, err)
	}
	return parseRouteGateway(out), nil
}

func defaultGatewayForInterface(iface string) (net.IP, error) {
	out, err := exec.Command("route", "-n", "get", "default").CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("route get default: %s: %w", out, err)
	}
	gotIface := parseRouteInterface(out)
	if gotIface != "" && gotIface != iface {
		return nil, nil
	}
	return parseRouteGateway(out), nil
}

func parseRouteGateway(out []byte) net.IP {
	for _, line := range strings.Split(string(out), "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "gateway:") {
			continue
		}
		gw := net.ParseIP(strings.TrimSpace(strings.TrimPrefix(line, "gateway:")))
		if gw == nil {
			return nil
		}
		return gw
	}
	return nil
}

func parseRouteInterface(out []byte) string {
	for _, line := range strings.Split(string(out), "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "interface:") {
			return strings.TrimSpace(strings.TrimPrefix(line, "interface:"))
		}
	}
	return ""
}

func gatewayReachableOnInterface(gateway net.IP, ifaceName string) bool {
	if gateway == nil || gateway.To4() == nil {
		return false
	}
	iface, err := net.InterfaceByName(ifaceName)
	if err != nil {
		return false
	}
	addrs, err := iface.Addrs()
	if err != nil {
		return false
	}
	for _, addr := range addrs {
		ipNet, ok := addr.(*net.IPNet)
		if !ok || ipNet.IP.To4() == nil {
			continue
		}
		if ipNet.Contains(gateway) {
			return true
		}
	}
	return false
}

func isTunnelInterface(iface string) bool {
	return strings.HasPrefix(iface, "utun") ||
		strings.HasPrefix(iface, "tun") ||
		strings.HasPrefix(iface, "tap")
}

// AddScopedHostRoute adds a /32 host route for ip via iface,
// bypassing the TUN (used for WireGuard server endpoint).
func AddScopedHostRoute(ip, iface string) error {
	if err := addScopedHostRoute(ip, iface); err != nil {
		return err
	}
	if parsed := net.ParseIP(ip); parsed != nil {
		rememberHostRoute(parsed)
	}
	return nil
}
