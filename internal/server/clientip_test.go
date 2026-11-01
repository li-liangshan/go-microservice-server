package server

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestNewClientIPResolver_RejectsInvalidProxy(t *testing.T) {
	if _, err := newClientIPResolver([]string{"not-an-ip"}); err == nil {
		t.Fatal("expected an error for an invalid trusted proxy")
	}
}

func TestClientIPResolver_IgnoresForwardingHeadersFromUntrustedPeer(t *testing.T) {
	resolver, err := newClientIPResolver(nil)
	if err != nil {
		t.Fatalf("resolver: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/users", nil)
	req.RemoteAddr = "203.0.113.7:51000"
	req.Header.Set("X-Forwarded-For", "1.2.3.4")
	req.Header.Set("X-Real-IP", "5.6.7.8")

	if got := resolver.resolve(req); got != "203.0.113.7" {
		t.Errorf("client ip: got %q, want %q", got, "203.0.113.7")
	}
}

func TestClientIPResolver_ReadsForwardedChainFromTrustedProxy(t *testing.T) {
	resolver, err := newClientIPResolver([]string{"10.0.0.0/8"})
	if err != nil {
		t.Fatalf("resolver: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/users", nil)
	req.RemoteAddr = "10.0.0.5:51000"
	req.Header.Set("X-Forwarded-For", "1.2.3.4, 10.0.0.9")

	if got := resolver.resolve(req); got != "1.2.3.4" {
		t.Errorf("client ip: got %q, want %q", got, "1.2.3.4")
	}
}

// Within a chain of trusted proxies the right-most untrusted hop is the
// client.
func TestClientIPResolver_UsesRightMostUntrustedHop(t *testing.T) {
	resolver, err := newClientIPResolver([]string{"10.0.0.0/8", "192.168.0.0/16"})
	if err != nil {
		t.Fatalf("resolver: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/users", nil)
	req.RemoteAddr = "10.0.0.5:51000"
	req.Header.Set("X-Forwarded-For", "203.0.113.9, 192.168.1.1")

	if got := resolver.resolve(req); got != "203.0.113.9" {
		t.Errorf("client ip: got %q, want %q", got, "203.0.113.9")
	}
}

// When every hop is trusted the left-most recorded address is the client.
func TestClientIPResolver_UsesLeftMostHopWhenAllTrusted(t *testing.T) {
	resolver, err := newClientIPResolver([]string{"10.0.0.0/8", "192.168.0.0/16"})
	if err != nil {
		t.Fatalf("resolver: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/users", nil)
	req.RemoteAddr = "10.0.0.5:51000"
	req.Header.Set("X-Forwarded-For", "192.168.1.1, 10.0.0.9")

	if got := resolver.resolve(req); got != "192.168.1.1" {
		t.Errorf("client ip: got %q, want %q", got, "192.168.1.1")
	}
}

func TestClientIPResolver_SkipsMalformedForwardedEntries(t *testing.T) {
	resolver, err := newClientIPResolver([]string{"10.0.0.0/8"})
	if err != nil {
		t.Fatalf("resolver: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/users", nil)
	req.RemoteAddr = "10.0.0.5:51000"
	req.Header.Set("X-Forwarded-For", "garbage, 1.2.3.4")

	if got := resolver.resolve(req); got != "1.2.3.4" {
		t.Errorf("client ip: got %q, want %q", got, "1.2.3.4")
	}
}

// IPv4-mapped IPv6 addresses must match IPv4 CIDR blocks.
func TestClientIPResolver_NormalisesIPv4MappedAddresses(t *testing.T) {
	resolver, err := newClientIPResolver([]string{"10.0.0.0/8"})
	if err != nil {
		t.Fatalf("resolver: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/users", nil)
	req.RemoteAddr = "[::ffff:10.0.0.5]:51000"
	req.Header.Set("X-Forwarded-For", "1.2.3.4")

	if got := resolver.resolve(req); got != "1.2.3.4" {
		t.Errorf("client ip: got %q, want %q", got, "1.2.3.4")
	}
}

func TestClientIPMiddleware_StoresAddressInContext(t *testing.T) {
	resolver, err := newClientIPResolver(nil)
	if err != nil {
		t.Fatalf("resolver: %v", err)
	}

	var seen string
	handler := clientIPMiddleware(resolver)(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		seen = ClientIP(r.Context())
	}))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/users", nil)
	req.RemoteAddr = "203.0.113.7:51000"
	handler.ServeHTTP(httptest.NewRecorder(), req)

	if seen != "203.0.113.7" {
		t.Errorf("context client ip: got %q, want %q", seen, "203.0.113.7")
	}
}
