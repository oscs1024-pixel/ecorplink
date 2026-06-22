//go:build darwin

package forwarder

import "testing"

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
