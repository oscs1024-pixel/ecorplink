package dns

import (
	"context"
	"log"
	"net"
	"sync"
	"time"
)

// Cache holds resolved IPs for a domain with TTL and refresh tracking.
type Cache struct {
	Domain   string
	IPs      []string
	CachedAt time.Time
	TTL      time.Duration
	Refresh  time.Duration
	mu       sync.RWMutex
}

func NewCache(domain string, ttl, refresh time.Duration) *Cache {
	return &Cache{
		Domain:  domain,
		TTL:     ttl,
		Refresh: refresh,
	}
}

func (c *Cache) Set(ips []string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.IPs = ips
	c.CachedAt = time.Now()
}

func (c *Cache) Get() ([]string, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	if time.Since(c.CachedAt) > c.TTL {
		return nil, false
	}
	out := make([]string, len(c.IPs))
	copy(out, c.IPs)
	return out, true
}

func (c *Cache) Diff(newIPs []string) (added, removed []string) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	oldSet := make(map[string]struct{}, len(c.IPs))
	for _, ip := range c.IPs {
		oldSet[ip] = struct{}{}
	}
	newSet := make(map[string]struct{}, len(newIPs))
	for _, ip := range newIPs {
		newSet[ip] = struct{}{}
		if _, ok := oldSet[ip]; !ok {
			added = append(added, ip)
		}
	}
	for _, ip := range c.IPs {
		if _, ok := newSet[ip]; !ok {
			removed = append(removed, ip)
		}
	}
	return
}

// Resolver manages DNS resolution for multiple domains with periodic refresh.
type Resolver struct {
	caches          map[string]*Cache
	resolver        string
	refreshInterval time.Duration
	mu              sync.RWMutex
	ticker          *time.Ticker
	stop            chan struct{}
	stopOnce        sync.Once
	onChange        func(domain string, added, removed []string)
}

func NewResolver(resolver string, refreshInterval time.Duration) *Resolver {
	if refreshInterval <= 0 {
		refreshInterval = 2 * time.Minute
	}
	return &Resolver{
		caches:          make(map[string]*Cache),
		resolver:        resolver,
		refreshInterval: refreshInterval,
		stop:            make(chan struct{}),
	}
}

func (r *Resolver) SetOnChange(cb func(domain string, added, removed []string)) {
	r.onChange = cb
}

func (r *Resolver) AddDomain(domain string, ttl, refresh time.Duration) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.caches[domain] = NewCache(domain, ttl, refresh)
}

func (r *Resolver) Resolve(domain string) ([]string, error) {
	// Check cache first
	if c, ok := r.GetCache(domain); ok {
		if cached, hit := c.Get(); hit {
			return cached, nil
		}
	}

	var addrs []net.IPAddr
	var ips []net.IP
	var err error
	if r.resolver != "" {
		resolver := &net.Resolver{
			PreferGo: true,
			Dial: func(ctx context.Context, network, address string) (net.Conn, error) {
				d := net.Dialer{Timeout: 5 * time.Second}
				return d.DialContext(ctx, network, r.resolver)
			},
		}
		addrs, err = resolver.LookupIPAddr(context.Background(), domain)
		if err != nil {
			return nil, err
		}
		for _, addr := range addrs {
			ips = append(ips, addr.IP)
		}
	} else {
		ips, err = net.LookupIP(domain)
		if err != nil {
			return nil, err
		}
	}
	var result []string
	for _, ip := range ips {
		result = append(result, ip.String())
	}
	return result, nil
}

func (r *Resolver) Start() {
	r.mu.Lock()
	if r.ticker != nil {
		r.mu.Unlock()
		log.Printf("[dns] resolver already started")
		return
	}
	r.ticker = time.NewTicker(r.refreshInterval)
	r.mu.Unlock()

	stopCh := r.stop
	go func() {
		for {
			select {
			case <-r.ticker.C:
				r.refreshAll()
			case <-stopCh:
				return
			}
		}
	}()
}

func (r *Resolver) Stop() {
	r.stopOnce.Do(func() {
		close(r.stop)
		r.mu.Lock()
		if r.ticker != nil {
			r.ticker.Stop()
		}
		r.mu.Unlock()
	})
}

func (r *Resolver) refreshAll() {
	r.mu.RLock()
	domains := make([]string, 0, len(r.caches))
	for d := range r.caches {
		domains = append(domains, d)
	}
	r.mu.RUnlock()

	for _, d := range domains {
		ips, err := r.Resolve(d)
		if err != nil {
			log.Printf("[dns] refresh resolve %s error: %v", d, err)
			continue
		}
		r.mu.RLock()
		c, ok := r.caches[d]
		r.mu.RUnlock()
		if !ok {
			continue
		}
		added, removed := c.Diff(ips)
		if len(added) > 0 || len(removed) > 0 || len(ips) != len(c.IPs) {
			c.Set(ips)
			if r.onChange != nil {
				r.onChange(d, added, removed)
			}
		}
	}
}

func (r *Resolver) GetCache(domain string) (*Cache, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	c, ok := r.caches[domain]
	return c, ok
}
