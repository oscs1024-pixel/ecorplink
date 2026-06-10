//go:build darwin

package router

import (
	"net"
	"testing"
)

func TestRouteDestinationMatchesCIDRDarwinShorthand(t *testing.T) {
	_, n, err := net.ParseCIDR("128.0.0.0/1")
	if err != nil {
		t.Fatal(err)
	}
	if !routeDestinationMatchesCIDR("128.0/1", n) {
		t.Fatal("128.0/1 should match 128.0.0.0/1")
	}
}

func TestDarwinRouteGetOutputMatchesCIDRWithMask(t *testing.T) {
	_, n, err := net.ParseCIDR("128.0.0.0/1")
	if err != nil {
		t.Fatal(err)
	}
	out := `   route to: 128.0.0.0
destination: 128.0.0.0
       mask: 128.0.0.0
  interface: utun4
`
	iface, ok := parseDarwinRouteGet(out, n)
	if !ok {
		t.Fatal("route get output should match 128.0.0.0/1")
	}
	if iface != "utun4" {
		t.Fatalf("iface = %q, want utun4", iface)
	}
}

func TestDarwinRouteGetOutputRejectsMoreSpecificMask(t *testing.T) {
	_, n, err := net.ParseCIDR("128.0.0.0/1")
	if err != nil {
		t.Fatal(err)
	}
	out := `   route to: 154.94.237.169
destination: 128.0.0.0
       mask: 192.0.0.0
  interface: utun5
`
	if _, ok := parseDarwinRouteGet(out, n); ok {
		t.Fatal("128.0.0.0/2 must not match 128.0.0.0/1")
	}
}

func TestRouteDestinationMatchesCIDRRejectsMoreSpecific(t *testing.T) {
	_, n, err := net.ParseCIDR("128.0.0.0/1")
	if err != nil {
		t.Fatal(err)
	}
	if routeDestinationMatchesCIDR("128.0/2", n) {
		t.Fatal("128.0/2 must not match 128.0.0.0/1")
	}
}
