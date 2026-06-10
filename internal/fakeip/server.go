package fakeip

import (
	"context"
	"fmt"
	"net"
	"strings"

	"github.com/miekg/dns"
)

// DomainMatcher is satisfied by rule.MatcherAdapter.
type DomainMatcher interface {
	MatchDomain(domain string) bool
}

// Upstream forwards DNS queries to a real upstream server.
// Satisfied by the boundUpstream in cmd/ecorplink-daemon.
type Upstream interface {
	Exchange(msg *dns.Msg) (*dns.Msg, error)
}

// DialFunc dials a network connection (used to bypass TUN for upstream DNS).
type DialFunc func(ctx context.Context, network, addr string) (net.Conn, error)

// Server processes raw DNS UDP/TCP payloads from the TUN forwarder.
type Server struct {
	pool          *Pool
	matcher       DomainMatcher
	upstream      Upstream // nil → SERVFAIL for unmatched domains
	upstreamAddrs []string // fallback if upstream is nil: direct DNS client
	fakeAllA      bool
	dialFn        DialFunc // non-nil → use for upstream UDP exchange (bypasses TUN)
}

// SetFakeAllA makes every A query receive a fake IP. Domain rules still decide
// the outbound action later in the TCP/UDP forwarder; unmatched domains fall
// back to the current system route.
func (s *Server) SetFakeAllA(enabled bool) {
	s.fakeAllA = enabled
}

// SetDialFn sets a custom dial function for upstream UDP DNS exchanges.
// Use this to bind DNS queries to the physical interface, bypassing TUN capture.
func (s *Server) SetDialFn(fn DialFunc) {
	s.dialFn = fn
}

// NewServer creates a DNS intercept server.
// upstream may be nil (unmatched queries return SERVFAIL).
// upstreamAddrs is used only when upstream is nil — direct UDP exchange.
func NewServer(pool *Pool, matcher DomainMatcher, upstream Upstream, upstreamAddrs []string) *Server {
	return &Server{
		pool:          pool,
		matcher:       matcher,
		upstream:      upstream,
		upstreamAddrs: upstreamAddrs,
	}
}

// HandleDNS processes raw DNS bytes (UDP payload or TCP payload without the
// 2-byte length prefix). Returns raw DNS response bytes.
func (s *Server) HandleDNS(raw []byte) ([]byte, error) {
	req := new(dns.Msg)
	if err := req.Unpack(raw); err != nil {
		return s.packServfail(req), fmt.Errorf("fakeip: unpack DNS message: %w", err)
	}

	// No questions → SERVFAIL
	if len(req.Question) == 0 {
		return s.packServfail(req), nil
	}

	q := req.Question[0]
	domain := strings.ToLower(strings.TrimSuffix(q.Name, "."))

	if s.fakeAllA || s.matcher.MatchDomain(domain) {
		switch q.Qtype {
		case dns.TypeA:
			ip := s.pool.Assign(domain)
			resp := new(dns.Msg)
			resp.SetReply(req)
			resp.Answer = []dns.RR{
				&dns.A{
					Hdr: dns.RR_Header{
						Name:   q.Name,
						Rrtype: dns.TypeA,
						Class:  dns.ClassINET,
						Ttl:    1,
					},
					A: ip,
				},
			}
			return resp.Pack()

		case dns.TypeAAAA:
			// No NAT64 — return NOERROR with empty answer section.
			resp := new(dns.Msg)
			resp.SetReply(req)
			resp.Answer = nil
			return resp.Pack()

		default:
			// Other record types for matched domain: forward upstream.
			return s.forward(req)
		}
	}

	// Domain not matched: forward to upstream.
	return s.forward(req)
}

// forward sends req to the configured upstream and returns the packed response.
// Returns SERVFAIL bytes if no upstream is configured.
func (s *Server) forward(req *dns.Msg) ([]byte, error) {
	if s.upstream != nil {
		resp, err := s.upstream.Exchange(req)
		if err != nil {
			return s.packServfail(req), fmt.Errorf("fakeip: upstream exchange: %w", err)
		}
		return resp.Pack()
	}

	if len(s.upstreamAddrs) > 0 {
		addr := s.upstreamAddrs[0]
		if s.dialFn != nil {
			// Use dialFn so the UDP query bypasses TUN capture routes.
			conn, err := s.dialFn(context.Background(), "udp", addr)
			if err != nil {
				return s.packServfail(req), fmt.Errorf("fakeip: direct DNS exchange dial: %w", err)
			}
			defer conn.Close() //nolint:errcheck
			dconn := &dns.Conn{Conn: conn}
			if err := dconn.WriteMsg(req); err != nil {
				return s.packServfail(req), fmt.Errorf("fakeip: direct DNS exchange write: %w", err)
			}
			resp, err := dconn.ReadMsg()
			if err != nil {
				return s.packServfail(req), fmt.Errorf("fakeip: direct DNS exchange read: %w", err)
			}
			return resp.Pack()
		}
		c := &dns.Client{Net: "udp"}
		resp, _, err := c.Exchange(req, addr)
		if err != nil {
			return s.packServfail(req), fmt.Errorf("fakeip: direct DNS exchange: %w", err)
		}
		return resp.Pack()
	}

	return s.packServfail(req), nil
}

// packServfail returns a packed SERVFAIL response for req.
// If req is nil or packing fails, returns a minimal hard-coded SERVFAIL.
func (s *Server) packServfail(req *dns.Msg) []byte {
	resp := new(dns.Msg)
	if req != nil {
		resp.SetRcode(req, dns.RcodeServerFailure)
	} else {
		resp.SetRcode(&dns.Msg{}, dns.RcodeServerFailure)
	}
	b, _ := resp.Pack()
	return b
}
