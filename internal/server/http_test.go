package server

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"microservice-server/internal/service"
	"microservice-server/internal/store"

	"github.com/go-chi/chi/v5"
)

// fakeStore satisfies store.UserStoreInterface for HTTP tests.
type fakeStore struct {
	getByIDFn func(ctx context.Context, id int64) (*store.User, error)
	listFn    func(ctx context.Context, l, o int) ([]*store.User, error)
	createFn  func(ctx context.Context, u *store.User) error
	updateFn  func(ctx context.Context, u *store.User) error
	deleteFn  func(ctx context.Context, id int64) error
}

func (f *fakeStore) GetByID(ctx context.Context, id int64) (*store.User, error) {
	return f.getByIDFn(ctx, id)
}
func (f *fakeStore) List(ctx context.Context, l, o int) ([]*store.User, error) {
	return f.listFn(ctx, l, o)
}
func (f *fakeStore) Create(ctx context.Context, u *store.User) error {
	return f.createFn(ctx, u)
}
func (f *fakeStore) Update(ctx context.Context, u *store.User) error {
	return f.updateFn(ctx, u)
}
func (f *fakeStore) Delete(ctx context.Context, id int64) error {
	return f.deleteFn(ctx, id)
}

// newTestRouter returns a chi router with the same routes the production
// server registers, so chi.URLParam / middleware behave identically.
func newTestRouter(svc *service.UserService) *chi.Mux {
	r := chi.NewRouter()
	r.Route("/api/v1", func(r chi.Router) {
		r.Get("/users", ListHandler(svc))
		r.Post("/users", CreateHandler(svc))
		r.Get("/users/{id}", GetHandler(svc))
		r.Put("/users/{id}", UpdateHandler(svc))
		r.Delete("/users/{id}", DeleteHandler(svc))
	})
	return r
}

func doRequest(t *testing.T, fs *fakeStore, method, url string, body []byte) *httptest.ResponseRecorder {
	t.Helper()
	svc := service.NewUserService(fs, slog.New(slog.NewTextHandler(io.Discard, nil)))
	router := newTestRouter(svc)

	var bodyReader io.Reader
	if body != nil {
		bodyReader = bytes.NewReader(body)
	}
	req := httptest.NewRequest(method, url, bodyReader)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	return w
}

func decodeError(t *testing.T, body []byte) string {
	t.Helper()
	var env map[string]string
	if err := json.Unmarshal(body, &env); err != nil {
		t.Fatalf("invalid error json: %v / %q", err, body)
	}
	return env["error"]
}

// ── GET /users/{id} ──────────────────────────────────────────────────────────

func TestGetHandler_Success(t *testing.T) {
	fs := &fakeStore{getByIDFn: func(_ context.Context, id int64) (*store.User, error) {
		return &store.User{ID: id, Name: "alice", Email: "a@b.com"}, nil
	}}
	w := doRequest(t, fs, http.MethodGet, "/api/v1/users/1", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("status %d", w.Code)
	}
	var got store.User
	if err := json.NewDecoder(w.Body).Decode(&got); err != nil {
		t.Fatal(err)
	}
	if got.ID != 1 || got.Name != "alice" {
		t.Errorf("got %+v", got)
	}
}

func TestGetHandler_BadID(t *testing.T) {
	w := doRequest(t, &fakeStore{}, http.MethodGet, "/api/v1/users/not-a-number", nil)
	if w.Code != http.StatusBadRequest {
		t.Errorf("status %d", w.Code)
	}
	if got := decodeError(t, w.Body.Bytes()); got != "invalid user id" {
		t.Errorf("msg: %q", got)
	}
}

func TestGetHandler_NotFound(t *testing.T) {
	fs := &fakeStore{getByIDFn: func(context.Context, int64) (*store.User, error) {
		return nil, store.ErrNotFound
	}}
	w := doRequest(t, fs, http.MethodGet, "/api/v1/users/77", nil)
	if w.Code != http.StatusNotFound {
		t.Errorf("status %d", w.Code)
	}
	if got := decodeError(t, w.Body.Bytes()); got != "user not found" {
		t.Errorf("msg: %q", got)
	}
}

func TestGetHandler_InternalError(t *testing.T) {
	fs := &fakeStore{getByIDFn: func(context.Context, int64) (*store.User, error) {
		return nil, errors.New("db down")
	}}
	w := doRequest(t, fs, http.MethodGet, "/api/v1/users/1", nil)
	if w.Code != http.StatusInternalServerError {
		t.Errorf("status %d", w.Code)
	}
}

// ── GET /users (List) ────────────────────────────────────────────────────────

func TestListHandler_Success(t *testing.T) {
	fs := &fakeStore{listFn: func(_ context.Context, l, o int) ([]*store.User, error) {
		return []*store.User{{ID: 1}, {ID: 2}}, nil
	}}
	w := doRequest(t, fs, http.MethodGet, "/api/v1/users", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("status %d", w.Code)
	}
	var resp struct {
		Count int           `json:"count"`
		Users []*store.User `json:"users"`
	}
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatal(err)
	}
	if resp.Count != 2 || len(resp.Users) != 2 {
		t.Errorf("got count=%d users=%v", resp.Count, resp.Users)
	}
}

func TestListHandler_DefaultsLimitAndOffset(t *testing.T) {
	var gotL, gotO int
	fs := &fakeStore{listFn: func(_ context.Context, l, o int) ([]*store.User, error) {
		gotL, gotO = l, o
		return nil, nil
	}}
	w := doRequest(t, fs, http.MethodGet, "/api/v1/users", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("status %d", w.Code)
	}
	if gotL != 20 || gotO != 0 {
		t.Errorf("limit/offset: got %d/%d, want 20/0", gotL, gotO)
	}
}

func TestListHandler_ClampsHugeLimit(t *testing.T) {
	var gotL int
	fs := &fakeStore{listFn: func(_ context.Context, l, o int) ([]*store.User, error) {
		gotL = l
		return nil, nil
	}}
	w := doRequest(t, fs, http.MethodGet, "/api/v1/users?limit=99999", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("status %d", w.Code)
	}
	if gotL != 20 {
		t.Errorf("limit: got %d, want 20 (clamped)", gotL)
	}
}

// ── POST /users ──────────────────────────────────────────────────────────────

func TestCreateHandler_Success(t *testing.T) {
	fs := &fakeStore{createFn: func(_ context.Context, u *store.User) error {
		u.ID = 99
		u.CreatedAt = u.CreatedAt.UTC()
		return nil
	}}
	body := []byte(`{"name":"alice","email":"a@b.com"}`)
	w := doRequest(t, fs, http.MethodPost, "/api/v1/users", body)
	if w.Code != http.StatusCreated {
		t.Fatalf("status %d body=%s", w.Code, w.Body.String())
	}
	var got store.User
	if err := json.NewDecoder(w.Body).Decode(&got); err != nil {
		t.Fatal(err)
	}
	if got.ID != 99 {
		t.Errorf("ID: got %d", got.ID)
	}
}

func TestCreateHandler_InvalidJSON(t *testing.T) {
	w := doRequest(t, &fakeStore{}, http.MethodPost, "/api/v1/users", []byte(`{not json`))
	if w.Code != http.StatusBadRequest {
		t.Errorf("status %d", w.Code)
	}
}

func TestCreateHandler_ValidationError(t *testing.T) {
	storeCalled := false
	fs := &fakeStore{createFn: func(context.Context, *store.User) error {
		storeCalled = true
		return nil
	}}
	w := doRequest(t, fs, http.MethodPost, "/api/v1/users", []byte(`{"name":"","email":"bad"}`))
	if w.Code != http.StatusBadRequest {
		t.Errorf("status %d", w.Code)
	}
	if storeCalled {
		t.Error("store should not have been called for invalid input")
	}
}

func TestCreateHandler_Duplicate(t *testing.T) {
	fs := &fakeStore{createFn: func(context.Context, *store.User) error {
		return store.ErrDuplicate
	}}
	w := doRequest(t, fs, http.MethodPost, "/api/v1/users", []byte(`{"name":"alice","email":"dup@x.com"}`))
	if w.Code != http.StatusConflict {
		t.Errorf("status %d", w.Code)
	}
	if got := decodeError(t, w.Body.Bytes()); got != "email already exists" {
		t.Errorf("msg: %q", got)
	}
}

func TestCreateHandler_InternalError(t *testing.T) {
	fs := &fakeStore{createFn: func(context.Context, *store.User) error {
		return errors.New("db write failed")
	}}
	w := doRequest(t, fs, http.MethodPost, "/api/v1/users", []byte(`{"name":"alice","email":"a@b.com"}`))
	if w.Code != http.StatusInternalServerError {
		t.Errorf("status %d", w.Code)
	}
}

// ── PUT /users/{id} ──────────────────────────────────────────────────────────

func TestUpdateHandler_Success(t *testing.T) {
	fs := &fakeStore{
		updateFn: func(_ context.Context, u *store.User) error {
			if u.ID != 7 {
				t.Errorf("service should pass id=7, got %d", u.ID)
			}
			return nil
		},
		getByIDFn: func(_ context.Context, id int64) (*store.User, error) {
			return &store.User{ID: id, Name: "fetched", Email: "a@b.com"}, nil
		},
	}
	body := []byte(`{"name":"alice","email":"a@b.com"}`)
	w := doRequest(t, fs, http.MethodPut, "/api/v1/users/7", body)
	if w.Code != http.StatusOK {
		t.Fatalf("status %d body=%s", w.Code, w.Body.String())
	}
}

func TestUpdateHandler_BadID(t *testing.T) {
	body := []byte(`{"name":"alice","email":"a@b.com"}`)
	w := doRequest(t, &fakeStore{}, http.MethodPut, "/api/v1/users/abc", body)
	if w.Code != http.StatusBadRequest {
		t.Errorf("status %d", w.Code)
	}
}

func TestUpdateHandler_NotFound(t *testing.T) {
	fs := &fakeStore{updateFn: func(context.Context, *store.User) error {
		return store.ErrNotFound
	}}
	body := []byte(`{"name":"alice","email":"a@b.com"}`)
	w := doRequest(t, fs, http.MethodPut, "/api/v1/users/9999", body)
	if w.Code != http.StatusNotFound {
		t.Errorf("status %d", w.Code)
	}
}

func TestUpdateHandler_Duplicate(t *testing.T) {
	fs := &fakeStore{updateFn: func(context.Context, *store.User) error {
		return store.ErrDuplicate
	}}
	body := []byte(`{"name":"alice","email":"dup@x.com"}`)
	w := doRequest(t, fs, http.MethodPut, "/api/v1/users/1", body)
	if w.Code != http.StatusConflict {
		t.Errorf("status %d", w.Code)
	}
}

// ── DELETE /users/{id} ───────────────────────────────────────────────────────

func TestDeleteHandler_Success(t *testing.T) {
	var captured int64
	fs := &fakeStore{deleteFn: func(_ context.Context, id int64) error {
		captured = id
		return nil
	}}
	w := doRequest(t, fs, http.MethodDelete, "/api/v1/users/42", nil)
	if w.Code != http.StatusNoContent {
		t.Errorf("status %d", w.Code)
	}
	if w.Body.Len() != 0 {
		t.Errorf("expected empty body, got %q", w.Body.String())
	}
	if captured != 42 {
		t.Errorf("delete id: got %d, want 42", captured)
	}
}

func TestDeleteHandler_NotFound(t *testing.T) {
	fs := &fakeStore{deleteFn: func(context.Context, int64) error {
		return store.ErrNotFound
	}}
	w := doRequest(t, fs, http.MethodDelete, "/api/v1/users/9999", nil)
	if w.Code != http.StatusNotFound {
		t.Errorf("status %d", w.Code)
	}
}

func TestDeleteHandler_BadID(t *testing.T) {
	w := doRequest(t, &fakeStore{}, http.MethodDelete, "/api/v1/users/abc", nil)
	if w.Code != http.StatusBadRequest {
		t.Errorf("status %d", w.Code)
	}
}
