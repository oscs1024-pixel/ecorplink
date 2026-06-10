package fakeip

import (
	"net"
	"testing"
)

func TestPoolAssignAndLookup(t *testing.T) {
	p, err := NewPool("198.18.0.0/15")
	if err != nil {
		t.Fatal(err)
	}

	ip1 := p.Assign("github.com")
	ip2 := p.Assign("google.com")
	if ip1.Equal(ip2) {
		t.Error("different domains should get different IPs")
	}
	ip1b := p.Assign("github.com")
	if !ip1.Equal(ip1b) {
		t.Error("same domain should get same IP on repeated call")
	}
	domain, ok := p.Lookup(ip1)
	if !ok || domain != "github.com" {
		t.Errorf("Lookup: want (github.com, true), got (%q, %v)", domain, ok)
	}
}

func TestPoolInPool(t *testing.T) {
	p, _ := NewPool("198.18.0.0/15")
	ip := p.Assign("example.com")
	if !p.InPool(ip) {
		t.Error("assigned IP should be in pool")
	}
	if p.InPool(net.ParseIP("8.8.8.8")) {
		t.Error("8.8.8.8 should not be in pool")
	}
}

func TestPoolLRUEviction(t *testing.T) {
	// /30 gives 2 usable IPs
	p, err := NewPool("198.18.0.0/30")
	if err != nil {
		t.Fatal(err)
	}

	ip1 := p.Assign("a.com")
	_ = p.Assign("b.com")
	// Third assign must evict oldest (a.com); c.com takes ip1's slot.
	_ = p.Assign("c.com")

	// ip1 slot is now owned by c.com — Lookup should return c.com, not a.com.
	domain, ok := p.Lookup(ip1)
	if !ok {
		t.Error("ip1 slot should now be owned by c.com, but Lookup returned false")
	}
	if domain != "c.com" {
		t.Errorf("ip1 slot should map to c.com after eviction, got %q", domain)
	}

	// a.com must no longer have a forward mapping: re-assigning it should give a fresh IP.
	ipA2 := p.Assign("a.com")
	if ipA2.Equal(ip1) {
		t.Error("a.com was evicted; re-assigning should yield a different (fresh) IP")
	}
}

func TestPoolLookupAfterEviction(t *testing.T) {
	p, _ := NewPool("198.18.0.0/30") // 2 usable IPs
	_ = p.Assign("a.com")
	_ = p.Assign("b.com")
	ip3 := p.Assign("c.com") // evicts a.com's slot
	// c.com should be findable
	domain, ok := p.Lookup(ip3)
	if !ok || domain != "c.com" {
		t.Errorf("Lookup after eviction: want (c.com, true), got (%q, %v)", domain, ok)
	}
}

func TestPoolInvalidCIDR(t *testing.T) {
	_, err := NewPool("not-a-cidr")
	if err == nil {
		t.Error("expected error for invalid CIDR")
	}
	_, err2 := NewPool("198.18.0.0/32")
	if err2 == nil {
		t.Error("expected error for /32 pool (0 usable IPs)")
	}
}
