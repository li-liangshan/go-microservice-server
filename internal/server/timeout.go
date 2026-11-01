package server

import (
	"bytes"
	"context"
	"net/http"
	"sync"
	"time"
)

// requestTimeout enforces a hard deadline on request handling.
//
// Unlike the upstream chi timeout middleware — which only wraps the context
// and therefore lets slow handlers run to completion — this middleware
// abandons the request when the deadline expires and answers the client with
// 504 Gateway Timeout.
//
// The handler keeps running in the background until it observes the cancelled
// context, but its response is discarded: output is buffered and only
// delivered to the client if the handler finished before the deadline.
func requestTimeout(timeout time.Duration) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx, cancel := context.WithTimeout(r.Context(), timeout)
			defer cancel()

			tw := newTimeoutWriter()
			done := make(chan struct{})
			panicked := make(chan any, 1)

			go func() {
				defer func() {
					if p := recover(); p != nil {
						panicked <- p
						return
					}
					close(done)
				}()
				next.ServeHTTP(tw, r.WithContext(ctx))
			}()

			select {
			case <-done:
				tw.deliver(w)
			case p := <-panicked:
				// Re-raise on the request goroutine so the recoverer
				// middleware can turn it into a 500 response.
				panic(p)
			case <-ctx.Done():
				// A panic that raced with the deadline must still surface.
				select {
				case p := <-panicked:
					panic(p)
				default:
				}
				tw.discard()
				writeError(w, http.StatusGatewayTimeout, "request timeout")
			}
		})
	}
}

// timeoutWriter buffers a handler's response so that it can be discarded when
// the request deadline expires.
//
// It is safe for concurrent use: the handler goroutine writes while the
// request goroutine settles the outcome.
type timeoutWriter struct {
	mu       sync.Mutex
	header   http.Header
	body     bytes.Buffer
	status   int
	wrote    bool
	finished bool
}

func newTimeoutWriter() *timeoutWriter {
	return &timeoutWriter{
		header: make(http.Header),
		status: http.StatusOK,
	}
}

// Header returns the buffered response header map.
func (t *timeoutWriter) Header() http.Header {
	return t.header
}

// WriteHeader records the response status.
func (t *timeoutWriter) WriteHeader(code int) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.wrote || t.finished {
		return
	}
	t.status = code
	t.wrote = true
}

// Write buffers the response body.
func (t *timeoutWriter) Write(b []byte) (int, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.finished {
		// The request has already been settled; discard the late write.
		return len(b), nil
	}
	if !t.wrote {
		t.status = http.StatusOK
		t.wrote = true
	}
	return t.body.Write(b)
}

// Flush is a no-op. The response is buffered until the handler finishes, so
// it cannot be streamed incrementally.
func (t *timeoutWriter) Flush() {}

// deliver flushes the buffered response to the client.
func (t *timeoutWriter) deliver(w http.ResponseWriter) {
	t.mu.Lock()
	status, body := t.status, t.body.Bytes()
	header := t.header
	t.finished = true
	t.mu.Unlock()

	for key, values := range header {
		for _, value := range values {
			w.Header().Add(key, value)
		}
	}
	w.WriteHeader(status)
	if len(body) > 0 {
		_, _ = w.Write(body)
	}
}

// discard marks the buffered response as abandoned.
func (t *timeoutWriter) discard() {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.finished = true
	t.body.Reset()
}
