//go:build darwin

package outbound

import "testing"

func TestParseRouteInterface(t *testing.T) {
	out := []byte(`   route to: default
destination: default
    gateway: 192.168.1.1
  interface: en0
`)

	if got := parseRouteInterface(out); got != "en0" {
		t.Fatalf("interface = %q, want en0", got)
	}
}
