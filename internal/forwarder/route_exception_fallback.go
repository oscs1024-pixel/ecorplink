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

func hostRouteCurrent(ip, iface string, gateway net.IP) (bool, bool) {
	return false, false
}

func scopedHostRouteCurrent(ip, iface string) (bool, bool) {
	return false, false
}

// AddScopedHostRoute is a no-op on non-darwin platforms.
func AddScopedHostRoute(ip, iface string) error { return nil }
