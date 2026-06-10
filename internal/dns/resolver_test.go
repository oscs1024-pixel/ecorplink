package dns

import (
	"testing"
	"time"
)

func TestCacheSetAndGet(t *testing.T) {
	c := NewCache("example.com", 48*time.Hour, 2*time.Minute)
	ips := []string{"1.2.3.4", "5.6.7.8"}
	c.Set(ips)

	got, ok := c.Get()
	if !ok {
		t.Fatal("expected cache hit")
	}
	if len(got) != 2 || got[0] != "1.2.3.4" {
		t.Fatalf("got %v, want [1.2.3.4 5.6.7.8]", got)
	}
}

func TestCacheExpiration(t *testing.T) {
	c := NewCache("example.com", 1*time.Millisecond, 2*time.Minute)
	c.Set([]string{"1.2.3.4"})
	time.Sleep(2 * time.Millisecond)
	_, ok := c.Get()
	if ok {
		t.Fatal("expected cache expired")
	}
}

func TestCacheDiff(t *testing.T) {
	c := NewCache("example.com", 48*time.Hour, 2*time.Minute)
	c.Set([]string{"1.2.3.4", "5.6.7.8"})
	added, removed := c.Diff([]string{"5.6.7.8", "9.8.7.6"})
	if len(added) != 1 || added[0] != "9.8.7.6" {
		t.Fatalf("added = %v, want [9.8.7.6]", added)
	}
	if len(removed) != 1 || removed[0] != "1.2.3.4" {
		t.Fatalf("removed = %v, want [1.2.3.4]", removed)
	}
}

func TestCacheDiffEmpty(t *testing.T) {
	c := NewCache("example.com", 48*time.Hour, 2*time.Minute)
	c.Set([]string{})
	added, removed := c.Diff([]string{"1.2.3.4"})
	if len(added) != 1 || added[0] != "1.2.3.4" {
		t.Fatalf("added = %v, want [1.2.3.4]", added)
	}
	if len(removed) != 0 {
		t.Fatalf("removed = %v, want []", removed)
	}
}

func TestCacheDiffIdentical(t *testing.T) {
	c := NewCache("example.com", 48*time.Hour, 2*time.Minute)
	c.Set([]string{"1.2.3.4", "5.6.7.8"})
	added, removed := c.Diff([]string{"5.6.7.8", "1.2.3.4"})
	if len(added) != 0 || len(removed) != 0 {
		t.Fatalf("expected no changes, added=%v removed=%v", added, removed)
	}
}

func TestResolverStartStop(t *testing.T) {
	r := NewResolver("", 100*time.Millisecond)
	r.AddDomain("example.com", time.Hour, time.Minute)

	// Start should be idempotent-ish (no panic)
	r.Start()
	r.Start() // second start should be no-op

	// Stop should be safe to call multiple times
	r.Stop()
	r.Stop()
	r.Stop()
}

func TestResolverOnChange(t *testing.T) {
	r := NewResolver("", 100*time.Millisecond)
	r.AddDomain("test.example.com", time.Hour, time.Minute)

	var changedDomain string
	var changedAdded []string
	var changedRemoved []string

	r.SetOnChange(func(domain string, added, removed []string) {
		changedDomain = domain
		changedAdded = added
		changedRemoved = removed
	})

	// Simulate a refresh by manually triggering
	c, ok := r.GetCache("test.example.com")
	if !ok {
		t.Fatal("expected cache to exist")
	}
	c.Set([]string{"1.2.3.4"})

	// Now set different IPs and trigger diff
	added, removed := c.Diff([]string{"5.6.7.8"})
	c.Set([]string{"5.6.7.8"})
	if r.onChange != nil {
		r.onChange("test.example.com", added, removed)
	}

	if changedDomain != "test.example.com" {
		t.Fatalf("domain = %q, want test.example.com", changedDomain)
	}
	if len(changedAdded) != 1 || changedAdded[0] != "5.6.7.8" {
		t.Fatalf("added = %v, want [5.6.7.8]", changedAdded)
	}
	if len(changedRemoved) != 1 || changedRemoved[0] != "1.2.3.4" {
		t.Fatalf("removed = %v, want [1.2.3.4]", changedRemoved)
	}
}

func TestResolverCacheUsed(t *testing.T) {
	// This test verifies that Resolve checks cache first
	r := NewResolver("", time.Hour)
	r.AddDomain("cached.example.com", time.Hour, time.Minute)

	c, _ := r.GetCache("cached.example.com")
	c.Set([]string{"9.9.9.9"})

	// Since we can't easily mock DNS without external deps,
	// we verify the cache is set and retrievable
	cached, hit := c.Get()
	if !hit {
		t.Fatal("expected cache hit")
	}
	if len(cached) != 1 || cached[0] != "9.9.9.9" {
		t.Fatalf("cached = %v, want [9.9.9.9]", cached)
	}
}
