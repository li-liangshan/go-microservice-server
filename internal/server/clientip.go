package server

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/netip"
	"strings"

	"microservice-server/internal/config"
)

// clientIPContextKey is the context key under which the resolved client
// address is stored.
type clientIPContextKey struct{}

// ClientIP returns the resolved client address stored in ctx, or an empty
// string when the request did not pass through the clientIP middleware.
func ClientIP(ctx context.Context) string {
	if value, ok := ctx.Value(clientIPContextKey{}).(string); ok {
		return value
	}
	return ""
}

// clientIPResolver determines the originating client address of a request.
//
// Forwarding headers such as X-Forwarded-For are attacker-controlled, so they
// are only consulted when the immediate peer is a trusted proxy. Within a
// chain of trusted proxies the right-most untrusted hop is the client; when
// every hop is trusted the left-most recorded address is used.
type clientIPResolver struct {
	trusted []netip.Prefix
}

// newClientIPResolver builds a resolver from a list of IP addresses or CIDR
// blocks. An empty list means no proxy is trusted.
func newClientIPResolver(trustedProxies []string) (*clientIPResolver, error) {
	resolver := &clientIPResolver{}
	for _, entry := range trustedProxies {
		prefix, err := config.ParsePrefix(entry)
		if err != nil {
			return nil, fmt.Errorf("trusted proxy %q: %w", entry, err)
		}
		resolver.trusted = append(resolver.trusted, prefix)
	}
	return resolver, nil
}

// isTrusted reports whether addr belongs to a trusted proxy range.
func (r *clientIPResolver) isTrusted(addr netip.Addr) bool {
	if !addr.IsValid() {
		return false
	}
	for _, prefix := range r.trusted {
		if prefix.Contains(addr) {
			return true
		}
	}
	return false
}

// resolve returns the client address for the request.
func (r *clientIPResolver) resolve(req *http.Request) string {
	peer, ok := parseAddr(req.RemoteAddr)
	if !ok {
		// Unparseable peer: fall back to the raw value so the caller still
		// gets a stable key.
		return req.RemoteAddr
	}
	if !r.isTrusted(peer) {
		// The peer is not a proxy we trust, so any forwarding header is
		// attacker-controlled and must be ignored entirely.
		return peer.String()
	}

	chain := forwardedChain(req)
	chain = append(chain, peer)

	for i := len(chain) - 1; i >= 0; i-- {
		if !r.isTrusted(chain[i]) {
			return chain[i].String()
		}
	}
	// Every hop is trusted: the left-most address is the original client.
	return chain[0].String()
}

// clientIPMiddleware resolves the client address and stores it in the request
// context so that access logs, rate limiting and handlers all agree on who
// the caller is.
func clientIPMiddleware(resolver *clientIPResolver) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx := context.WithValue(r.Context(), clientIPContextKey{}, resolver.resolve(r))
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// forwardedChain parses the X-Forwarded-For headers into addresses, oldest
// first. Malformed entries are skipped.
func forwardedChain(req *http.Request) []netip.Addr {
	var chain []netip.Addr
	for _, header := range req.Header.Values("X-Forwarded-For") {
		for _, part := range strings.Split(header, ",") {
			if addr, ok := parseAddr(part); ok {
				chain = append(chain, addr)
			}
		}
	}
	return chain
}

// parseAddr parses an address that may carry a port, as seen in
// X-Forwarded-For and in net/http's RemoteAddr.
func parseAddr(value string) (netip.Addr, bool) {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return netip.Addr{}, false
	}
	if host, _, err := net.SplitHostPort(trimmed); err == nil {
		trimmed = host
	}
	trimmed = strings.Trim(trimmed, "[]")
	addr, err := netip.ParseAddr(trimmed)
	if err != nil {
		return netip.Addr{}, false
	}
	// Normalise IPv4-mapped IPv6 addresses so they match IPv4 CIDR blocks.
	return addr.Unmap(), true
}
