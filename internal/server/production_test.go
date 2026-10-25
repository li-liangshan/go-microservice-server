package server

import (
	"context"
	"errors"
	"io"
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

type fakePinger struct {
	err error
}

func (p fakePinger) PingContext(context.Context) error {
	return p.err
}

func newProductionTestServer(t *testing.T, fs *fakeStore, pinger Pinger, mutate func(*config.HTTPConfig)) *Server {
	t.Helper()
	cfg := config.HTTPConfig{
		Addr:              ":0",
		ReadHeaderTimeout: time.Second,
		ReadTimeout:       time.Second,
		WriteTimeout:      time.Second,
		IdleTimeout:       time.Second,
		RequestTimeout:    time.Second,
		ShutdownTimeout:   time.Second,
		ReadinessTimeout:  100 * time.Millisecond,
		MaxHeaderBytes:    1 << 20,
		MaxBodyBytes:      1 << 20,
	}
	if mutate != nil {
		mutate(&cfg)
	}
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	svc := service.NewUserService(fs, log)
	return New(cfg, svc, pinger, log)
}

func serve(t *testing.T, srv *Server, method, target string, body string, headers map[string]string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, target, strings.NewReader(body))
	for key, value := range headers {
		req.Header.Set(key, value)
	}
	recorder := httptest.NewRecorder()
	srv.http.Handler.ServeHTTP(recorder, req)
	return recorder
}

func TestHealthAndReadiness(t *testing.T) {
	srv := newProductionTestServer(t, &fakeStore{}, fakePinger{}, nil)

	if got := serve(t, srv, http.MethodGet, "/healthz", "", nil).Code; got != http.StatusOK {
		t.Errorf("healthz status: %d", got)
	}
	if got := serve(t, srv, http.MethodGet, "/readyz", "", nil).Code; got != http.StatusOK {
		t.Errorf("readyz status: %d", got)
	}
}

func TestReadinessFailsWhenDatabaseUnavailable(t *testing.T) {
	srv := newProductionTestServer(t, &fakeStore{}, fakePinger{err: errors.New("db down")}, nil)
	recorder := serve(t, srv, http.MethodGet, "/readyz", "", nil)
	if recorder.Code != http.StatusServiceUnavailable {
		t.Errorf("readyz status: got %d, want %d", recorder.Code, http.StatusServiceUnavailable)
	}
}

func TestReadinessStopsAfterShutdown(t *testing.T) {
	srv := newProductionTestServer(t, &fakeStore{}, fakePinger{}, nil)
	srv.ready.Store(false)
	if got := serve(t, srv, http.MethodGet, "/readyz", "", nil).Code; got != http.StatusServiceUnavailable {
		t.Errorf("readyz status: got %d, want %d", got, http.StatusServiceUnavailable)
	}
}

func TestAPIKeyAuth(t *testing.T) {
	fs := &fakeStore{listFn: func(context.Context, int, int) ([]*store.User, error) {
		return nil, nil
	}}
	srv := newProductionTestServer(t, fs, fakePinger{}, func(cfg *config.HTTPConfig) {
		cfg.AuthToken = "secret"
	})

	if got := serve(t, srv, http.MethodGet, "/api/v1/users", "", nil).Code; got != http.StatusUnauthorized {
		t.Errorf("missing token status: %d", got)
	}
	wrong := map[string]string{"Authorization": "Bearer wrong"}
	if got := serve(t, srv, http.MethodGet, "/api/v1/users", "", wrong).Code; got != http.StatusUnauthorized {
		t.Errorf("wrong token status: %d", got)
	}
	valid := map[string]string{"Authorization": "Bearer secret"}
	if got := serve(t, srv, http.MethodGet, "/api/v1/users", "", valid).Code; got != http.StatusOK {
		t.Errorf("valid token status: %d", got)
	}
	if got := serve(t, srv, http.MethodGet, "/healthz", "", nil).Code; got != http.StatusOK {
		t.Errorf("health endpoint should not require auth, got %d", got)
	}
}

func TestJSONBodyValidation(t *testing.T) {
	fs := &fakeStore{createFn: func(context.Context, *store.User) error { return nil }}
	srv := newProductionTestServer(t, fs, fakePinger{}, nil)

	tests := []struct {
		name        string
		contentType string
		body        string
		wantStatus  int
	}{
		{"unsupported content type", "text/plain", `{"name":"a","email":"a@b.com"}`, http.StatusUnsupportedMediaType},
		{"unknown field", "application/json", `{"name":"a","email":"a@b.com","role":"admin"}`, http.StatusBadRequest},
		{"trailing json", "application/json", `{"name":"a","email":"a@b.com"}{}`, http.StatusBadRequest},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			headers := map[string]string{"Content-Type": tc.contentType}
			recorder := serve(t, srv, http.MethodPost, "/api/v1/users", tc.body, headers)
			if recorder.Code != tc.wantStatus {
				t.Errorf("status: got %d, want %d; body=%s", recorder.Code, tc.wantStatus, recorder.Body.String())
			}
		})
	}
}

func TestRequestBodyLimit(t *testing.T) {
	srv := newProductionTestServer(t, &fakeStore{}, fakePinger{}, func(cfg *config.HTTPConfig) {
		cfg.MaxBodyBytes = 16
	})
	headers := map[string]string{"Content-Type": "application/json"}
	recorder := serve(t, srv, http.MethodPost, "/api/v1/users", `{"name":"alice","email":"a@b.com"}`, headers)
	if recorder.Code != http.StatusRequestEntityTooLarge {
		t.Errorf("status: got %d, want %d; body=%s", recorder.Code, http.StatusRequestEntityTooLarge, recorder.Body.String())
	}
}

func TestPanicRecoveryReturnsJSONError(t *testing.T) {
	fs := &fakeStore{getByIDFn: func(context.Context, int64) (*store.User, error) {
		panic("boom")
	}}
	srv := newProductionTestServer(t, fs, fakePinger{}, nil)
	recorder := serve(t, srv, http.MethodGet, "/api/v1/users/1", "", nil)
	if recorder.Code != http.StatusInternalServerError {
		t.Errorf("status: got %d, want %d", recorder.Code, http.StatusInternalServerError)
	}
	if got := decodeError(t, recorder.Body.Bytes()); got != "internal error" {
		t.Errorf("error: %q", got)
	}
}

func TestSecurityHeadersAndJSONErrors(t *testing.T) {
	srv := newProductionTestServer(t, &fakeStore{}, fakePinger{}, nil)

	recorder := serve(t, srv, http.MethodGet, "/healthz", "", nil)
	if recorder.Header().Get("X-Content-Type-Options") != "nosniff" {
		t.Error("missing X-Content-Type-Options")
	}
	if recorder.Header().Get("Cache-Control") != "no-store" {
		t.Error("missing Cache-Control: no-store")
	}

	recorder = serve(t, srv, http.MethodGet, "/missing", "", nil)
	if recorder.Code != http.StatusNotFound {
		t.Errorf("not found status: %d", recorder.Code)
	}
	if got := decodeError(t, recorder.Body.Bytes()); got != "not found" {
		t.Errorf("not found error: %q", got)
	}
}
