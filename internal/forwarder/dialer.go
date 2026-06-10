package forwarder

import (
	"context"
	"net"
)

type packetDialer interface {
	Dial(network, address string) (net.Conn, error)
	DialContext(ctx context.Context, network, address string) (net.Conn, error)
}

type lookupIPAddrFunc func(context.Context, string) ([]net.IPAddr, error)

type hostRouteDialer struct {
	base   *net.Dialer
	ensure func(net.IP)
}

func (d *hostRouteDialer) Dial(network, address string) (net.Conn, error) {
	return d.DialContext(context.Background(), network, address)
}

func (d *hostRouteDialer) DialContext(ctx context.Context, network, address string) (net.Conn, error) {
	resolver := net.DefaultResolver
	if d.base != nil && d.base.Resolver != nil {
		resolver = d.base.Resolver
	}
	if err := ensureResolvedHostRoutes(ctx, address, resolver.LookupIPAddr, d.ensure); err != nil {
		return nil, err
	}
	return d.base.DialContext(ctx, network, address)
}

func ensureResolvedHostRoutes(ctx context.Context, address string, lookup lookupIPAddrFunc, ensure func(net.IP)) error {
	host, _, err := net.SplitHostPort(address)
	if err != nil {
		return err
	}
	if ip := net.ParseIP(host); ip != nil {
		ensure(ip)
		return nil
	}
	addrs, err := lookup(ctx, host)
	if err != nil {
		return err
	}
	for _, addr := range addrs {
		if addr.IP != nil {
			ensure(addr.IP)
		}
	}
	return nil
}
