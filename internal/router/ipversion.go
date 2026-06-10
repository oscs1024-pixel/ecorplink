package router

import (
	"net"
)

// isIPv6 returns true if the given IP string is an IPv6 address.
func isIPv6(ipStr string) bool {
	ip := net.ParseIP(ipStr)
	if ip == nil {
		return false
	}
	return ip.To4() == nil
}

// routePrefix returns the host route prefix for the given IP.
// "/32" for IPv4, "/128" for IPv6.
func routePrefix(ipStr string) string {
	if isIPv6(ipStr) {
		return "/128"
	}
	return "/32"
}
