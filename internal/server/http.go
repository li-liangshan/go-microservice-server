package server

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"microservice-server/internal/config"
	"microservice-server/internal/service"

	"github.com/go-chi/chi/v5"
	chimw "github.com/go-chi/chi/v5/middleware"
)

// Server is the HTTP server wrapper.
type Server struct {
	cfg  config.HTTPConfig
	svc  *service.UserService
	log  *slog.Logger
	http *http.Server
}

// New builds the router and HTTP server.
func New(cfg config.HTTPConfig, svc *service.UserService, log *slog.Logger) *Server {
	r := chi.NewRouter()

	// ── Standard production middleware ────────────────────────────────────
	r.Use(chimw.RequestID)
	r.Use(chimw.RealIP)
	r.Use(chimw.Recoverer) // converts panics into 500 instead of crashing
	r.Use(chimw.Timeout(60 * time.Second))
	r.Use(requestLogger(log))

	// ── Routes ────────────────────────────────────────────────────────────
	r.Get("/healthz", healthCheck)
	r.Get("/readyz", healthCheck)

	r.Route("/api/v1", func(r chi.Router) {
		r.Get("/users/{id}", GetHandler(svc))
		r.Get("/users", ListHandler(svc))
	})

	srv := &Server{
		cfg: cfg,
		svc: svc,
		log: log,
		http: &http.Server{
			Addr:         cfg.Addr,
			Handler:      r,
			ReadTimeout:  cfg.ReadTimeout,
			WriteTimeout: cfg.WriteTimeout,
			IdleTimeout:  cfg.IdleTimeout,
		},
	}
	return srv
}

// ListenAndServe starts the HTTP server.
func (s *Server) ListenAndServe() error {
	return s.http.ListenAndServe()
}

// Shutdown gracefully shuts down the server.
func (s *Server) Shutdown(ctx context.Context) error {
	return s.http.Shutdown(ctx)
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

func healthCheck(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}
