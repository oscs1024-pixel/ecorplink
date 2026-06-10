//go:build darwin

package router

import (
	"fmt"
	"net"
	"os/exec"
	"strings"
)

type darwinRouter struct{ tunName string }

func NewPlatformRouter(tunName string) Router  { return &darwinRouter{tunName: tunName} }
func (r *darwinRouter) SetTunName(name string) { r.tunName = name }
func (r *darwinRouter) TunName() string        { return r.tunName }

func (r *darwinRouter) AddRoute(cidr string) error {
	if owned, exists := r.existingRouteOwnedByTun(cidr); exists {
		if !owned {
			return ErrRouteExists
		}
		// Already owned by our TUN — skip the add to avoid a spurious "File exists".
		return nil
	}
	out, err := exec.Command("route", "add", "-net", cidr, "-interface", r.tunName).CombinedOutput()
	if err != nil {
		if strings.Contains(string(out), "File exists") {
			return ErrRouteExists
		}
		return fmt.Errorf("route add %s: %s: %w", cidr, out, err)
	}
	return nil
}

func (r *darwinRouter) DelRoute(cidr string) error {
	if r.tunName == "" && (cidr == "0.0.0.0/1" || cidr == "128.0.0.0/1") {
		return nil
	}
	if r.tunName != "" {
		owned, exists := r.existingRouteOwnedByTun(cidr)
		if exists && !owned {
			return nil
		}
	}
	exec.Command("route", "delete", "-net", cidr).Run() //nolint:errcheck
	return nil
}

func (r *darwinRouter) RouteInterface(cidr string) (string, bool) {
	_, ipNet, err := net.ParseCIDR(cidr)
	if err != nil || ipNet == nil || ipNet.IP.To4() == nil {
		return "", false
	}
	out, err := exec.Command("route", "-n", "get", ipNet.IP.String()).CombinedOutput()
	if err != nil {
		return "", false
	}
	return parseDarwinRouteGet(string(out), ipNet)
}

func (r *darwinRouter) existingRouteOwnedByTun(cidr string) (bool, bool) {
	iface, ok := r.RouteInterface(cidr)
	if !ok {
		return false, false
	}
	return iface == r.tunName, true
}

func parseDarwinRouteGet(out string, ipNet *net.IPNet) (string, bool) {
	routeDest := ""
	routeMask := ""
	iface := ""
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(line)
		switch {
		case strings.HasPrefix(line, "destination:"):
			routeDest = strings.TrimSpace(strings.TrimPrefix(line, "destination:"))
		case strings.HasPrefix(line, "mask:"):
			routeMask = strings.TrimSpace(strings.TrimPrefix(line, "mask:"))
		case strings.HasPrefix(line, "interface:"):
			iface = strings.TrimSpace(strings.TrimPrefix(line, "interface:"))
		}
	}
	if routeDest == "" || iface == "" {
		return "", false
	}
	if routeMask != "" && routeDestinationMaskMatchesCIDR(routeDest, routeMask, ipNet) {
		return iface, true
	}
	if !routeDestinationMatchesCIDR(routeDest, ipNet) {
		return "", false
	}
	return iface, true
}

func routeDestinationMaskMatchesCIDR(routeDest, routeMask string, ipNet *net.IPNet) bool {
	destIP := net.ParseIP(normalizeDarwinRouteIP(routeDest))
	maskIP := net.ParseIP(normalizeDarwinRouteIP(routeMask))
	if destIP == nil || maskIP == nil {
		return false
	}
	mask := net.IPv4Mask(maskIP[12], maskIP[13], maskIP[14], maskIP[15])
	routeNet := &net.IPNet{IP: destIP.To4().Mask(mask), Mask: mask}
	routeOnes, routeBits := routeNet.Mask.Size()
	ones, bits := ipNet.Mask.Size()
	return routeBits == bits && routeOnes == ones && routeNet.IP.Equal(ipNet.IP)
}

func routeDestinationMatchesCIDR(routeDest string, ipNet *net.IPNet) bool {
	ones, bits := ipNet.Mask.Size()
	if bits != 32 {
		return false
	}
	if routeDest == "default" {
		return ones == 0
	}
	if !strings.Contains(routeDest, "/") {
		if ip := net.ParseIP(routeDest); ip != nil {
			return ones == 32 && ip.Equal(ipNet.IP)
		}
	}
	routeDest = normalizeDarwinRouteDest(routeDest)
	_, routeNet, err := net.ParseCIDR(routeDest)
	if err != nil || routeNet == nil {
		return false
	}
	routeOnes, routeBits := routeNet.Mask.Size()
	return routeBits == 32 && routeOnes == ones && routeNet.IP.Equal(ipNet.IP)
}

func normalizeDarwinRouteDest(routeDest string) string {
	prefix := ""
	ipPart := routeDest
	if before, after, ok := strings.Cut(routeDest, "/"); ok {
		ipPart = before
		prefix = "/" + after
	}
	parts := strings.Split(ipPart, ".")
	if len(parts) >= 4 {
		return routeDest
	}
	for len(parts) < 4 {
		parts = append(parts, "0")
	}
	return strings.Join(parts, ".") + prefix
}

func normalizeDarwinRouteIP(ip string) string {
	return normalizeDarwinRouteDest(ip)
}
