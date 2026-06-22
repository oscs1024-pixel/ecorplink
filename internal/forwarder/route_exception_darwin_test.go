//go:build darwin

package forwarder

import (
	"net"
	"testing"
)

func TestParseRouteGatewayAndInterface(t *testing.T) {
	out := []byte(`   route to: default
destination: default
       mask: default
    gateway: 192.168.1.1
  interface: en0
      flags: <UP,GATEWAY,DONE,STATIC,PRCLONING,GLOBAL>
 recvpipe  sendpipe  ssthresh  rtt,msec    rttvar  hopcount      mtu     expire
       0         0         0         0         0         0      1500         0
`)

	if got := parseRouteGateway(out); got == nil || got.String() != "192.168.1.1" {
		t.Fatalf("gateway = %v, want 192.168.1.1", got)
	}
	if got := parseRouteInterface(out); got != "en0" {
		t.Fatalf("interface = %q, want en0", got)
	}
}

func TestRouteOutputNeedsReplaceForStaleGateway(t *testing.T) {
	out := []byte(`   route to: 203.0.113.10
destination: 203.0.113.10
    gateway: 192.168.1.1
  interface: en0
      flags: <UP,GATEWAY,HOST,DONE,STATIC>
`)

	if !routeOutputNeedsReplace(out, "en0", net.ParseIP("172.31.2.1")) {
		t.Fatal("route with stale gateway on same interface should be replaced")
	}
}

func TestRouteOutputNeedsReplaceForCurrentGateway(t *testing.T) {
	out := []byte(`   route to: 203.0.113.10
destination: 203.0.113.10
    gateway: 172.31.2.1
  interface: en0
      flags: <UP,GATEWAY,HOST,DONE,STATIC>
`)

	if routeOutputNeedsReplace(out, "en0", net.ParseIP("172.31.2.1")) {
		t.Fatal("route with current gateway and interface should be kept")
	}
}

func TestRouteOutputNeedsReplaceForDifferentInterface(t *testing.T) {
	out := []byte(`   route to: 203.0.113.10
destination: 203.0.113.10
    gateway: link#26
  interface: utun9
      flags: <UP,HOST,DONE,STATIC>
`)

	if !routeOutputNeedsReplace(out, "en0", net.ParseIP("172.31.2.1")) {
		t.Fatal("route on different interface should be replaced")
	}
}
