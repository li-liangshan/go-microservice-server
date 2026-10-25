package server

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"mime"
	"net/http"
	"runtime/debug"
	"strconv"
	"strings"
	"sync/atomic"

	"microservice-server/internal/config"
	"microservice-server/internal/service"
	"microservice-server/internal/store"

	"github.com/go-chi/chi/v5"
	chimw "github.com/go-chi/chi/v5/middleware"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

// Pinger verifies downstream dependencies for readiness checks.
type Pinger interface {
	PingContext(context.Context) error
}

// Server is the HTTP server wrapper.
type Server struct {
	cfg    config.HTTPConfig
	svc    *service.UserService
	pinger Pinger
	log    *slog.Logger
	ready  atomic.Bool
	http   *http.Server
}

// New builds the router and HTTP server.
func New(cfg config.HTTPConfig, svc *service.UserService, pinger Pinger, log *slog.Logger) *Server {
	r := chi.NewRouter()

	// ── Standard production middleware ────────────────────────────────────
	r.Use(securityHeaders)
	r.Use(otelhttp.NewMiddleware("http.server",
		otelhttp.WithSpanNameFormatter(func(_ string, r *http.Request) string {
			return r.Method + " " + r.URL.Path
		}),
		otelhttp.WithFilter(func(r *http.Request) bool {
			return r.URL.Path != "/healthz" && r.URL.Path != "/readyz"
		}),
	))
	r.Use(chimw.RequestID)
	r.Use(chimw.RealIP)
	r.Use(requestLogger(log))
	r.Use(chimw.Timeout(cfg.RequestTimeout))
	r.Use(recoverer(log))
	r.Use(chimw.RequestSize(cfg.MaxBodyBytes))
	r.Use(otelRouteName())

	s := &Server{
		cfg:    cfg,
		svc:    svc,
		pinger: pinger,
		log:    log,
		http: &http.Server{
			Addr:              cfg.Addr,
			Handler:           r,
			ReadHeaderTimeout: cfg.ReadHeaderTimeout,
			ReadTimeout:       cfg.ReadTimeout,
			WriteTimeout:      cfg.WriteTimeout,
			IdleTimeout:       cfg.IdleTimeout,
			MaxHeaderBytes:    cfg.MaxHeaderBytes,
			ErrorLog:          slog.NewLogLogger(log.Handler(), slog.LevelError),
		},
	}
	s.ready.Store(true)

	// ── Routes ────────────────────────────────────────────────────────────
	r.Get("/healthz", s.healthCheck)
	r.Get("/readyz", s.readinessCheck)

	r.Route("/api/v1", func(r chi.Router) {
		if cfg.AuthToken != "" {
			r.Use(apiKeyAuth(cfg.AuthToken))
		}
		r.Get("/users", ListHandler(svc))
		r.Post("/users", CreateHandler(svc))
		r.Get("/users/{id}", GetHandler(svc))
		r.Put("/users/{id}", UpdateHandler(svc))
		r.Delete("/users/{id}", DeleteHandler(svc))
	})
	r.NotFound(func(w http.ResponseWriter, _ *http.Request) {
		writeError(w, http.StatusNotFound, "not found")
	})
	r.MethodNotAllowed(func(w http.ResponseWriter, _ *http.Request) {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
	})

	return s
}

// ListenAndServe starts the HTTP server.
func (s *Server) ListenAndServe() error {
	return s.http.ListenAndServe()
}

// Shutdown gracefully shuts down the server.
func (s *Server) Shutdown(ctx context.Context) error {
	s.ready.Store(false)
	return s.http.Shutdown(ctx)
}

// Close immediately closes the HTTP server and all active connections.
func (s *Server) Close() error {
	s.ready.Store(false)
	return s.http.Close()
}

func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("Content-Security-Policy", "default-src 'none'; frame-ancestors 'none'")
		w.Header().Set("Permissions-Policy", "camera=(), microphone=(), geolocation=()")
		next.ServeHTTP(w, r)
	})
}

func recoverer(log *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			defer func() {
				recovered := recover()
				if recovered == nil {
					return
				}
				if recovered == http.ErrAbortHandler {
					return
				}
				log.ErrorContext(r.Context(), "http handler panic",
					"panic", recovered,
					"request_id", chimw.GetReqID(r.Context()),
					"stack", string(debug.Stack()),
				)
				writeError(w, http.StatusInternalServerError, "internal error")
			}()
			next.ServeHTTP(w, r)
		})
	}
}

func apiKeyAuth(token string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			const prefix = "Bearer "
			header := r.Header.Get("Authorization")
			if !strings.HasPrefix(header, prefix) ||
				subtle.ConstantTimeCompare([]byte(strings.TrimPrefix(header, prefix)), []byte(token)) != 1 {
				w.Header().Set("WWW-Authenticate", `Bearer realm="api"`)
				writeError(w, http.StatusUnauthorized, "unauthorized")
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// otelRouteName refines the span name once chi has resolved the route pattern.
func otelRouteName() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			defer func() {
				routeContext := chi.RouteContext(r.Context())
				if routeContext == nil {
					return
				}
				route := routeContext.RoutePattern()
				if route == "" {
					return
				}
				span := trace.SpanFromContext(r.Context())
				span.SetName(r.Method + " " + route)
				span.SetAttributes(attribute.String("http.route", route))
				if labeler, ok := otelhttp.LabelerFromContext(r.Context()); ok {
					labeler.Add(attribute.String("http.route", route))
				}
			}()

			next.ServeHTTP(w, r)
		})
	}
}

// ── Handlers ──────────────────────────────────────────────────────────────

func GetHandler(s *service.UserService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		idStr := chi.URLParam(r, "id")
		id, err := strconv.ParseInt(idStr, 10, 64)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid user id")
			return
		}

		user, err := s.Get(ctx, id)
		if err != nil {
			if errors.Is(err, service.ErrUserNotFound) {
				writeError(w, http.StatusNotFound, "user not found")
				return
			}
			writeError(w, http.StatusInternalServerError, "internal error")
			return
		}
		writeJSON(w, http.StatusOK, user)
	}
}

func ListHandler(s *service.UserService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
		offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))
		if limit <= 0 || limit > 100 {
			limit = 20
		}
		if offset < 0 {
			offset = 0
		}

		users, err := s.List(ctx, limit, offset)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "internal error")
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"users": users, "count": len(users)})
	}
}

func (s *Server) healthCheck(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *Server) readinessCheck(w http.ResponseWriter, r *http.Request) {
	if !s.ready.Load() {
		writeError(w, http.StatusServiceUnavailable, "not ready")
		return
	}
	if s.pinger == nil {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ready"})
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), s.cfg.ReadinessTimeout)
	defer cancel()
	if err := s.pinger.PingContext(ctx); err != nil {
		s.log.WarnContext(r.Context(), "readiness check failed", "err", err)
		writeError(w, http.StatusServiceUnavailable, "database unavailable")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ready"})
}

// request DTOs keep store types out of the HTTP wire format.
type createUserRequest struct {
	Name  string `json:"name"`
	Email string `json:"email"`
}

type updateUserRequest struct {
	Name  string `json:"name"`
	Email string `json:"email"`
}

func CreateHandler(s *service.UserService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req createUserRequest
		if !decodeJSON(w, r, &req) {
			return
		}
		u, err := s.Create(r.Context(), &store.User{Name: req.Name, Email: req.Email})
		if err != nil {
			writeUserError(w, err)
			return
		}
		writeJSON(w, http.StatusCreated, u)
	}
}

func UpdateHandler(s *service.UserService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, err := parseID(r)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid user id")
			return
		}
		var req updateUserRequest
		if !decodeJSON(w, r, &req) {
			return
		}
		u, err := s.Update(r.Context(), id, &store.User{Name: req.Name, Email: req.Email})
		if err != nil {
			writeUserError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, u)
	}
}

func DeleteHandler(s *service.UserService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, err := parseID(r)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid user id")
			return
		}
		if err := s.Delete(r.Context(), id); err != nil {
			writeUserError(w, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

func parseID(r *http.Request) (int64, error) {
	return strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
}

func decodeJSON(w http.ResponseWriter, r *http.Request, dst any) bool {
	contentType := r.Header.Get("Content-Type")
	mediaType, _, err := mime.ParseMediaType(contentType)
	if err != nil || mediaType != "application/json" {
		writeError(w, http.StatusUnsupportedMediaType, "content-type must be application/json")
		return false
	}

	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(dst); err != nil {
		writeJSONDecodeError(w, err)
		return false
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		writeError(w, http.StatusBadRequest, "invalid json body")
		return false
	}
	return true
}

func writeJSONDecodeError(w http.ResponseWriter, err error) {
	var maxBytesError *http.MaxBytesError
	if errors.As(err, &maxBytesError) {
		writeError(w, http.StatusRequestEntityTooLarge, "request body too large")
		return
	}
	writeError(w, http.StatusBadRequest, "invalid json body")
}

// writeUserError centralises the error→status mapping for user handlers.
func writeUserError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, service.ErrInvalidInput):
		writeError(w, http.StatusBadRequest, "invalid input: name and a valid email are required")
	case errors.Is(err, service.ErrUserNotFound):
		writeError(w, http.StatusNotFound, "user not found")
	case errors.Is(err, service.ErrEmailExists):
		writeError(w, http.StatusConflict, "email already exists")
	default:
		writeError(w, http.StatusInternalServerError, "internal error")
	}
}
