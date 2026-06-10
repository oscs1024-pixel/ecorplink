//go:build !darwin

package forwarder

import "net"

func addScopedHostRoute(ip, iface string) error {
	return nil
}

func addHostRoute(ip, iface string, gateway net.IP) error {
	return nil
}

func deleteHostRoute(ip string) error {
	return nil
}

// AddScopedHostRoute is a no-op on non-darwin platforms.
func AddScopedHostRoute(ip, iface string) error { return nil }
