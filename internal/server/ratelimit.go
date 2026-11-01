package server

import (
	"math"
	"net/http"
	"sync"
	"time"
)

// rateLimiter is a per-client token bucket.
//
// Each client owns a bucket that refills at a fixed rate and holds at most
// `burst` tokens. Buckets are created lazily and the total number of tracked
// clients is bounded, so a rotating set of source addresses cannot exhaust
// memory.
//
// When the table is full, buckets that have been idle long enough to refill
// are dropped first; if none qualifies, the least recently used bucket is
// evicted. That eviction is a deliberate trade-off: the evicted client gets a
// fresh allowance, so a source-address rotation can exceed the configured
// rate. Without the bound, memory usage would be attacker controlled.
type rateLimiter struct {
	mu      sync.Mutex
	buckets map[string]*tokenBucket
	rate    float64
	burst   float64
	maxKeys int
}

// tokenBucket holds the remaining allowance and the last refill timestamp.
type tokenBucket struct {
	tokens float64
	last   time.Time
}

// newRateLimiter builds a limiter allowing requestsPerSecond requests per
// client with the given burst allowance.
func newRateLimiter(requestsPerSecond float64, burst, maxKeys int) *rateLimiter {
	if burst <= 0 {
		burst = 1
	}
	if maxKeys <= 0 {
		maxKeys = 1
	}
	return &rateLimiter{
		buckets: make(map[string]*tokenBucket),
		rate:    requestsPerSecond,
		burst:   float64(burst),
		maxKeys: maxKeys,
	}
}

// allow consumes one token for key and reports whether the request may
// proceed.
func (l *rateLimiter) allow(key string, now time.Time) bool {
	l.mu.Lock()
	defer l.mu.Unlock()

	bucket, ok := l.buckets[key]
	if !ok {
		if len(l.buckets) >= l.maxKeys {
			l.evictIdleLocked(now)
		}
		if len(l.buckets) >= l.maxKeys {
			l.evictOldestLocked()
		}
		bucket = &tokenBucket{tokens: l.burst, last: now}
		l.buckets[key] = bucket
	} else {
		if elapsed := now.Sub(bucket.last).Seconds(); elapsed > 0 {
			bucket.tokens = math.Min(l.burst, bucket.tokens+elapsed*l.rate)
		}
		bucket.last = now
	}

	if bucket.tokens < 1 {
		return false
	}
	bucket.tokens--
	return true
}

// evictIdleLocked drops buckets that have refilled completely. Such a bucket
// carries no state that differs from a freshly created one.
func (l *rateLimiter) evictIdleLocked(now time.Time) {
	idleAfter := l.idleAfter()
	for key, bucket := range l.buckets {
		if now.Sub(bucket.last) >= idleAfter {
			delete(l.buckets, key)
		}
	}
}

// evictOldestLocked drops the least recently used bucket.
func (l *rateLimiter) evictOldestLocked() {
	var oldestKey string
	var oldest time.Time
	found := false
	for key, bucket := range l.buckets {
		if !found || bucket.last.Before(oldest) {
			oldestKey = key
			oldest = bucket.last
			found = true
		}
	}
	if found {
		delete(l.buckets, oldestKey)
	}
}

// idleAfter is the time a bucket needs to refill from empty to full.
func (l *rateLimiter) idleAfter() time.Duration {
	if l.rate <= 0 {
		return time.Hour
	}
	return time.Duration(l.burst/l.rate*float64(time.Second)) + time.Second
}

// size reports the number of tracked clients.
func (l *rateLimiter) size() int {
	l.mu.Lock()
	defer l.mu.Unlock()
	return len(l.buckets)
}

// rateLimit rejects requests from clients that exceed their allowance.
//
// Health probes are exempt: liveness and readiness probes are called
// frequently and often from the same address as the API traffic, so charging
// them against the bucket would make a busy instance look unhealthy.
func rateLimit(limiter *rateLimiter) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if isHealthPath(r.URL.Path) {
				next.ServeHTTP(w, r)
				return
			}

			key := ClientIP(r.Context())
			if key == "" {
				key = r.RemoteAddr
			}
			if !limiter.allow(key, time.Now()) {
				w.Header().Set("Retry-After", "1")
				writeError(w, http.StatusTooManyRequests, "rate limit exceeded")
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}
