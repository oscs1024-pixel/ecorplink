package fakeip

import (
	"encoding/binary"
	"fmt"
	"net"
	"sync"
)

// Pool manages a fake IP address pool with LRU eviction.
// Default CIDR: "198.18.0.0/15"
type Pool struct {
	mu   sync.Mutex
	cidr *net.IPNet

	// base is the first usable IP (network address + 1), as uint32.
	base uint32
	// size is the number of usable IPs in the pool (2^(32-prefix) - 2).
	size uint32
	// next is the circular allocation counter (offset from base).
	next uint32

	// domainToOffset maps domain name to its assigned offset.
	domainToOffset map[string]uint32
	// offsetToDomain maps offset back to domain name (reverse lookup).
	// Always kept in sync with domainToOffset.
	offsetToDomain map[uint32]string

	// OnAssign is called in a new goroutine when a domain is assigned a
	// fake IP for the first time. May be nil.
	OnAssign func(domain string, fakeIP net.IP)
}

// ipToUint32 converts a 4-byte net.IP to uint32.
func ipToUint32(ip net.IP) uint32 {
	ip = ip.To4()
	return binary.BigEndian.Uint32(ip)
}

// uint32ToIP converts uint32 to a 4-byte net.IP.
func uint32ToIP(n uint32) net.IP {
	ip := make(net.IP, 4)
	binary.BigEndian.PutUint32(ip, n)
	return ip
}

// NewPool creates a pool from a CIDR string (must be IPv4 /N with N < 32).
func NewPool(cidr string) (*Pool, error) {
	_, ipNet, err := net.ParseCIDR(cidr)
	if err != nil {
		return nil, fmt.Errorf("fakeip: invalid CIDR %q: %w", cidr, err)
	}
	if ipNet.IP.To4() == nil {
		return nil, fmt.Errorf("fakeip: CIDR %q is not IPv4", cidr)
	}

	ones, bits := ipNet.Mask.Size()
	if bits != 32 {
		return nil, fmt.Errorf("fakeip: CIDR %q is not IPv4", cidr)
	}
	if ones >= 32 {
		return nil, fmt.Errorf("fakeip: CIDR %q has no usable IPs (prefix >= 32)", cidr)
	}

	networkAddr := ipToUint32(ipNet.IP.To4())
	base := networkAddr + 1
	size := (uint32(1) << uint(32-ones)) - 2

	if size == 0 {
		return nil, fmt.Errorf("fakeip: CIDR %q has no usable IPs", cidr)
	}

	return &Pool{
		cidr:           ipNet,
		base:           base,
		size:           size,
		next:           0,
		domainToOffset: make(map[string]uint32),
		offsetToDomain: make(map[uint32]string),
	}, nil
}

// Assign returns the fake IP for domain. Allocates a new one if needed.
// If the pool is full, evicts the oldest entry (circular/LRU).
// Same domain always returns same IP until evicted.
func (p *Pool) Assign(domain string) net.IP {
	p.mu.Lock()

	// If domain already has an assignment, return it.
	if offset, ok := p.domainToOffset[domain]; ok {
		ip := uint32ToIP(p.base + offset)
		p.mu.Unlock()
		return ip
	}

	// Allocate the next slot (circular).
	offset := p.next
	p.next = (p.next + 1) % p.size

	if oldDomain, occupied := p.offsetToDomain[offset]; occupied {
		delete(p.domainToOffset, oldDomain)
	}

	p.domainToOffset[domain] = offset
	p.offsetToDomain[offset] = domain
	ip := uint32ToIP(p.base + offset)
	onAssign := p.OnAssign
	p.mu.Unlock()

	if onAssign != nil {
		go onAssign(domain, ip)
	}
	return ip
}

// Lookup returns the domain for a fake IP. Returns ("", false) if not found.
func (p *Pool) Lookup(ip net.IP) (string, bool) {
	p.mu.Lock()
	defer p.mu.Unlock()

	ip4 := ip.To4()
	if ip4 == nil {
		return "", false
	}
	ipUint := ipToUint32(ip4)
	if ipUint < p.base || ipUint >= p.base+p.size {
		return "", false
	}
	offset := ipUint - p.base
	domain, ok := p.offsetToDomain[offset]
	return domain, ok
}

// InPool reports whether ip falls within this pool's CIDR.
func (p *Pool) InPool(ip net.IP) bool {
	return p.cidr.Contains(ip)
}
