package server

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"microservice-server/internal/config"
	"microservice-server/internal/service"
	"microservice-server/internal/store"

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
		r.Get("/users", ListHandler(svc))
		r.Post("/users", CreateHandler(svc))
		r.Get("/users/{id}", GetHandler(svc))
		r.Put("/users/{id}", UpdateHandler(svc))
		r.Delete("/users/{id}", DeleteHandler(svc))
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
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, "invalid json body")
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
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, "invalid json body")
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
