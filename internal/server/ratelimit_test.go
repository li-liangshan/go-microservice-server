package server

import (
	"bytes"
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"microservice-server/internal/config"
	"microservice-server/internal/service"
	"microservice-server/internal/store"
)

// ── limiter ────────────────────────────────────────────────────────────────

func TestRateLimiter_AllowsUpToBurst(t *testing.T) {
	limiter := newRateLimiter(1, 3, 10)
	now := time.Now()

	for i := 0; i < 3; i++ {
		if !limiter.allow("1.2.3.4", now) {
			t.Fatalf("request %d should have been allowed", i+1)
		}
	}
	if limiter.allow("1.2.3.4", now) {
		t.Error("fourth request should have been rejected")
	}
}

func TestRateLimiter_RefillsOverTime(t *testing.T) {
	limiter := newRateLimiter(1, 2, 10)
	start := time.Now()

	if !limiter.allow("1.2.3.4", start) {
		t.Fatal("first request should have been allowed")
	}
	if !limiter.allow("1.2.3.4", start) {
		t.Fatal("second request should have been allowed")
	}
	if limiter.allow("1.2.3.4", start) {
		t.Fatal("third request should have been rejected")
	}

	// One second later the bucket has refilled by one token.
	later := start.Add(time.Second)
	if !limiter.allow("1.2.3.4", later) {
		t.Error("request after the refill interval should have been allowed")
	}
}

func TestRateLimiter_IsolatesClients(t *testing.T) {
	limiter := newRateLimiter(1, 1, 10)
	now := time.Now()

	if !limiter.allow("1.2.3.4", now) {
		t.Fatal("first request should have been allowed")
	}
	if limiter.allow("1.2.3.4", now) {
		t.Error("second request from the same client should have been rejected")
	}
	if !limiter.allow("5.6.7.8", now) {
		t.Error("a different client should have its own allowance")
	}
}

func TestRateLimiter_EvictsIdleBucketsWhenFull(t *testing.T) {
	// Room for a single client, so the next client forces an eviction.
	limiter := newRateLimiter(1, 1, 1)
	start := time.Now()

	limiter.allow("1.2.3.4", start)
	if got := limiter.size(); got != 1 {
		t.Fatalf("tracked clients: got %d, want 1", got)
	}

	// Long after the bucket has refilled it carries no state, so it is dropped.
	limiter.allow("5.6.7.8", start.Add(2*time.Hour))
	if got := limiter.size(); got != 1 {
		t.Fatalf("idle bucket should have been evicted, tracked clients: %d", got)
	}
	if _, ok := limiter.buckets["1.2.3.4"]; ok {
		t.Error("the idle bucket should have been evicted")
	}
}

func TestRateLimiter_BoundsTrackedClients(t *testing.T) {
	const maxKeys = 3
	limiter := newRateLimiter(1, 1, maxKeys)
	now := time.Now()

	for i := 0; i < 100; i++ {
		limiter.allow(string(rune('a'+i%26))+string(rune('0'+i/26)), now.Add(time.Duration(i)*time.Millisecond))
	}
	if got := limiter.size(); got > maxKeys {
		t.Fatalf("tracked clients: got %d, want <= %d", got, maxKeys)
	}
}

func TestRateLimiter_EvictsLeastRecentlyUsedWhenFull(t *testing.T) {
	limiter := newRateLimiter(1, 1, 2)
	start := time.Now()

	limiter.allow("old", start)
	limiter.allow("new", start.Add(time.Second))
	limiter.allow("newest", start.Add(2*time.Second))

	if got := limiter.size(); got != 2 {
		t.Fatalf("tracked clients: got %d, want 2", got)
	}
	if _, ok := limiter.buckets["old"]; ok {
		t.Error("the least recently used bucket should have been evicted")
	}
}

// ── middleware ─────────────────────────────────────────────────────────────

func TestRateLimitMiddleware_RejectsWith429(t *testing.T) {
	limiter := newRateLimiter(1, 1, 10)
	handler := rateLimit(limiter)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"ok": "yes"})
	}))

	first := httptest.NewRecorder()
	handler.ServeHTTP(first, httptest.NewRequest(http.MethodGet, "/api/v1/users", nil))
	if first.Code != http.StatusOK {
		t.Fatalf("first request status: %d", first.Code)
	}

	second := httptest.NewRecorder()
	handler.ServeHTTP(second, httptest.NewRequest(http.MethodGet, "/api/v1/users", nil))
	if second.Code != http.StatusTooManyRequests {
		t.Fatalf("second request status: got %d, want %d", second.Code, http.StatusTooManyRequests)
	}
	if second.Header().Get("Retry-After") == "" {
		t.Error("missing Retry-After header on 429")
	}
}

// Health probes must never be rate limited: an orchestrator polls them
// frequently and a busy instance would otherwise be declared unhealthy.
func TestRateLimitMiddleware_ExemptsHealthPaths(t *testing.T) {
	limiter := newRateLimiter(1, 1, 10)
	handler := rateLimit(limiter)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"ok": "yes"})
	}))

	for i := 0; i < 5; i++ {
		recorder := httptest.NewRecorder()
		handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/healthz", nil))
		if recorder.Code != http.StatusOK {
			t.Fatalf("healthz request %d status: got %d, want %d", i+1, recorder.Code, http.StatusOK)
		}
	}
}

// A rejected request must still reach the access log, otherwise rate limited
// traffic is invisible.
func TestRateLimitedRequestsAreLogged(t *testing.T) {
	var logs bytes.Buffer
	log := slog.New(slog.NewJSONHandler(&logs, nil))

	cfg := config.HTTPConfig{
		Addr:              ":0",
		ReadHeaderTimeout: time.Second,
		ReadTimeout:       time.Second,
		WriteTimeout:      time.Second,
		IdleTimeout:       time.Second,
		RequestTimeout:    time.Second,
		ShutdownTimeout:   time.Second,
		ReadinessTimeout:  time.Second,
		MaxHeaderBytes:    1 << 20,
		MaxBodyBytes:      1 << 20,
		RateLimit: config.RateLimitConfig{
			Enabled:           true,
			RequestsPerSecond: 1,
			Burst:             1,
			MaxTrackedClients: 10,
		},
	}

	fs := &fakeStore{listFn: func(context.Context, int, int) ([]*store.User, error) {
		return nil, nil
	}}
	svc := service.NewUserService(fs, log)
	srv, err := New(cfg, svc, fakePinger{}, log)
	if err != nil {
		t.Fatalf("build server: %v", err)
	}

	for i := 0; i < 3; i++ {
		recorder := httptest.NewRecorder()
		srv.http.Handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/api/v1/users", nil))
	}

	if !strings.Contains(logs.String(), `"status":429`) {
		t.Errorf("expected a 429 entry in the access log, got:\n%s", logs.String())
	}
}
