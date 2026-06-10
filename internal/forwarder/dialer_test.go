package forwarder

import (
	"context"
	"net"
	"testing"
)

func TestEnsureResolvedHostRoutesAddsRoutesForDomainResults(t *testing.T) {
	var routed []string
	lookup := func(ctx context.Context, host string) ([]net.IPAddr, error) {
		if host != "example.test" {
			t.Fatalf("lookup host %q, want example.test", host)
		}
		return []net.IPAddr{
			{IP: net.ParseIP("203.0.113.10")},
			{IP: net.ParseIP("2001:db8::10")},
		}, nil
	}

	if err := ensureResolvedHostRoutes(context.Background(), "example.test:443", lookup, func(ip net.IP) {
		routed = append(routed, ip.String())
	}); err != nil {
		t.Fatal(err)
	}

	want := []string{"203.0.113.10", "2001:db8::10"}
	if len(routed) != len(want) {
		t.Fatalf("routes %v, want %v", routed, want)
	}
	for i := range want {
		if routed[i] != want[i] {
			t.Fatalf("routes %v, want %v", routed, want)
		}
	}
}

func TestEnsureResolvedHostRoutesAddsRouteForLiteralIP(t *testing.T) {
	var routed []string
	lookup := func(ctx context.Context, host string) ([]net.IPAddr, error) {
		t.Fatalf("lookup should not be called for literal IP %q", host)
		return nil, nil
	}

	if err := ensureResolvedHostRoutes(context.Background(), "203.0.113.20:443", lookup, func(ip net.IP) {
		routed = append(routed, ip.String())
	}); err != nil {
		t.Fatal(err)
	}

	if len(routed) != 1 || routed[0] != "203.0.113.20" {
		t.Fatalf("routes %v, want [203.0.113.20]", routed)
	}
}
