package rule

// MatcherAdapter wraps Engine to satisfy fakeip.DomainMatcher interface.
// fakeip.Server needs: MatchDomain(domain string) bool
type MatcherAdapter struct{ *Engine }

// MatchDomain reports whether the given domain matches any rule in the engine.
// Matched domains receive fake IPs so the TCP/UDP flow reaches the forwarder,
// where the rule action can choose DIRECT or REDIRECT behavior.
func (m *MatcherAdapter) MatchDomain(domain string) bool {
	_, ok := m.Engine.MatchDomain(domain)
	return ok
}
