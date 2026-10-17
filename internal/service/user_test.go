package service

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"

	"microservice-server/internal/store"
)

// fakeStore is a test double for store.UserStoreInterface. Each method
// delegates to a configurable Fn; tests set only the functions they need.
type fakeStore struct {
	GetByIDFn func(ctx context.Context, id int64) (*store.User, error)
	ListFn    func(ctx context.Context, l, o int) ([]*store.User, error)
	CreateFn  func(ctx context.Context, u *store.User) error
	UpdateFn  func(ctx context.Context, u *store.User) error
	DeleteFn  func(ctx context.Context, id int64) error
}

func (f *fakeStore) GetByID(ctx context.Context, id int64) (*store.User, error) {
	if f.GetByIDFn == nil {
		return nil, errors.New("fakeStore.GetByID not configured")
	}
	return f.GetByIDFn(ctx, id)
}
func (f *fakeStore) List(ctx context.Context, l, o int) ([]*store.User, error) {
	if f.ListFn == nil {
		return nil, errors.New("fakeStore.List not configured")
	}
	return f.ListFn(ctx, l, o)
}
func (f *fakeStore) Create(ctx context.Context, u *store.User) error {
	if f.CreateFn == nil {
		return errors.New("fakeStore.Create not configured")
	}
	return f.CreateFn(ctx, u)
}
func (f *fakeStore) Update(ctx context.Context, u *store.User) error {
	if f.UpdateFn == nil {
		return errors.New("fakeStore.Update not configured")
	}
	return f.UpdateFn(ctx, u)
}
func (f *fakeStore) Delete(ctx context.Context, id int64) error {
	if f.DeleteFn == nil {
		return errors.New("fakeStore.Delete not configured")
	}
	return f.DeleteFn(ctx, id)
}

func newServiceWithStore(s store.UserStoreInterface) *UserService {
	// Discard logger so tests don't pollute stdout.
	return NewUserService(s, slog.New(slog.NewTextHandler(io.Discard, nil)))
}

// ── Get ──────────────────────────────────────────────────────────────────────

func TestUserService_Get_Success(t *testing.T) {
	want := &store.User{ID: 7, Name: "alice"}
	fake := &fakeStore{GetByIDFn: func(_ context.Context, id int64) (*store.User, error) {
		if id != 7 {
			t.Errorf("got id %d, want 7", id)
		}
		return want, nil
	}}
	got, err := newServiceWithStore(fake).Get(context.Background(), 7)
	if err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestUserService_Get_NotFound_Translated(t *testing.T) {
	fake := &fakeStore{GetByIDFn: func(context.Context, int64) (*store.User, error) {
		return nil, store.ErrNotFound
	}}
	if _, err := newServiceWithStore(fake).Get(context.Background(), 1); !errors.Is(err, ErrUserNotFound) {
		t.Errorf("want ErrUserNotFound, got %v", err)
	}
}

func TestUserService_Get_OtherError_PassesThrough(t *testing.T) {
	other := errors.New("db exploded")
	fake := &fakeStore{GetByIDFn: func(context.Context, int64) (*store.User, error) {
		return nil, other
	}}
	_, err := newServiceWithStore(fake).Get(context.Background(), 1)
	if !errors.Is(err, other) {
		t.Errorf("want %v, got %v", other, err)
	}
}

// ── List ─────────────────────────────────────────────────────────────────────

func TestUserService_List_PassesLimitAndOffset(t *testing.T) {
	called := false
	var gotL, gotO int
	fake := &fakeStore{ListFn: func(_ context.Context, l, o int) ([]*store.User, error) {
		called = true
		gotL, gotO = l, o
		return []*store.User{{ID: 1}}, nil
	}}
	users, err := newServiceWithStore(fake).List(context.Background(), 5, 10)
	if !called {
		t.Fatal("List not called")
	}
	if gotL != 5 || gotO != 10 {
		t.Errorf("got limit=%d offset=%d, want 5/10", gotL, gotO)
	}
	if err != nil || len(users) != 1 {
		t.Errorf("err=%v users=%v", err, users)
	}
}

// ── Create (validation) ──────────────────────────────────────────────────────

func TestUserService_Create_ValidationRejects(t *testing.T) {
	cases := []struct {
		name string
		in   store.User
	}{
		{"empty name", store.User{Name: "", Email: "a@b.com"}},
		{"whitespace-only name", store.User{Name: "   ", Email: "a@b.com"}},
		{"empty email", store.User{Name: "Alice", Email: ""}},
		{"missing @", store.User{Name: "Alice", Email: "noatsign"}},
		{"missing dot", store.User{Name: "Alice", Email: "no@dot"}},
		{"trailing space in email", store.User{Name: "Alice", Email: " "}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			storeCalled := false
			fake := &fakeStore{CreateFn: func(context.Context, *store.User) error {
				storeCalled = true
				return nil
			}}
			svc := newServiceWithStore(fake)
			in := c.in
			_, err := svc.Create(context.Background(), &in)
			if !errors.Is(err, ErrInvalidInput) {
				t.Errorf("err: got %v, want ErrInvalidInput", err)
			}
			if storeCalled {
				t.Error("store.Create should not have been called")
			}
		})
	}
}

func TestUserService_Create_TrimsFieldsBeforeValidating(t *testing.T) {
	var captured *store.User
	fake := &fakeStore{CreateFn: func(_ context.Context, u *store.User) error {
		captured = u
		u.ID = 99
		return nil
	}}
	in := store.User{Name: "  alice  ", Email: "  a@b.com  "}
	got, err := newServiceWithStore(fake).Create(context.Background(), &in)
	if err != nil {
		t.Fatal(err)
	}
	if captured.Name != "alice" {
		t.Errorf("Name not trimmed: %q", captured.Name)
	}
	if captured.Email != "a@b.com" {
		t.Errorf("Email not trimmed: %q", captured.Email)
	}
	if got.ID != 99 {
		t.Errorf("got.ID: %d", got.ID)
	}
}

func TestUserService_Create_Duplicate_Translated(t *testing.T) {
	fake := &fakeStore{CreateFn: func(context.Context, *store.User) error {
		return store.ErrDuplicate
	}}
	_, err := newServiceWithStore(fake).Create(context.Background(),
		&store.User{Name: "alice", Email: "dup@x.com"})
	if !errors.Is(err, ErrEmailExists) {
		t.Errorf("want ErrEmailExists, got %v", err)
	}
}

// ── Update ───────────────────────────────────────────────────────────────────

func TestUserService_Update_AssignsID(t *testing.T) {
	var capturedID int64
	fake := &fakeStore{
		UpdateFn: func(_ context.Context, u *store.User) error {
			capturedID = u.ID
			return nil
		},
		GetByIDFn: func(_ context.Context, id int64) (*store.User, error) {
			if id != 42 {
				t.Errorf("re-fetch id: got %d, want 42", id)
			}
			return &store.User{ID: id, Name: "fetched"}, nil
		},
	}
	svc := newServiceWithStore(fake)
	got, err := svc.Update(context.Background(), 42,
		&store.User{Name: "alice", Email: "a@b.com"})
	if err != nil {
		t.Fatal(err)
	}
	if capturedID != 42 {
		t.Errorf("capturedID: got %d, want 42", capturedID)
	}
	if got.Name != "fetched" {
		t.Errorf("got.Name: %q", got.Name)
	}
}

func TestUserService_Update_NotFound_Translated(t *testing.T) {
	fake := &fakeStore{UpdateFn: func(context.Context, *store.User) error {
		return store.ErrNotFound
	}}
	_, err := newServiceWithStore(fake).Update(context.Background(), 1,
		&store.User{Name: "x", Email: "y@z.com"})
	if !errors.Is(err, ErrUserNotFound) {
		t.Errorf("want ErrUserNotFound, got %v", err)
	}
}

func TestUserService_Update_Duplicate_Translated(t *testing.T) {
	fake := &fakeStore{UpdateFn: func(context.Context, *store.User) error {
		return store.ErrDuplicate
	}}
	_, err := newServiceWithStore(fake).Update(context.Background(), 1,
		&store.User{Name: "x", Email: "taken@x.com"})
	if !errors.Is(err, ErrEmailExists) {
		t.Errorf("want ErrEmailExists, got %v", err)
	}
}

// ── Delete ───────────────────────────────────────────────────────────────────

func TestUserService_Delete_Success(t *testing.T) {
	var captured int64
	fake := &fakeStore{DeleteFn: func(_ context.Context, id int64) error {
		captured = id
		return nil
	}}
	if err := newServiceWithStore(fake).Delete(context.Background(), 9); err != nil {
		t.Fatal(err)
	}
	if captured != 9 {
		t.Errorf("captured: got %d, want 9", captured)
	}
}

func TestUserService_Delete_NotFound_Translated(t *testing.T) {
	fake := &fakeStore{DeleteFn: func(context.Context, int64) error {
		return store.ErrNotFound
	}}
	err := newServiceWithStore(fake).Delete(context.Background(), 5)
	if !errors.Is(err, ErrUserNotFound) {
		t.Errorf("want ErrUserNotFound, got %v", err)
	}
}
