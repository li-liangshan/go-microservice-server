package store

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/go-sql-driver/mysql"
)

const (
	qGetByID = "SELECT " + userColumns + " FROM users WHERE id = ?"
	qList    = "SELECT " + userColumns + " FROM users ORDER BY id LIMIT ? OFFSET ?"
	qInsert  = "INSERT INTO users (name, email, created_at, updated_at) VALUES (?, ?, ?, ?)"
	qUpdate  = "UPDATE users SET name = ?, email = ?, updated_at = ? WHERE id = ?"
	qDelete  = "DELETE FROM users WHERE id = ?"
)

// newTestStore wires *UserStore.db to sqlmock so callers don't need a real
// MySQL. The cleanup func must be deferred.
func newTestStore(t *testing.T) (*UserStore, sqlmock.Sqlmock, func()) {
	t.Helper()
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherEqual))
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	return &UserStore{db: db}, mock, func() { _ = db.Close() }
}

// ── GetByID ──────────────────────────────────────────────────────────────────

func TestUserStore_GetByID_Success(t *testing.T) {
	s, mock, cleanup := newTestStore(t)
	defer cleanup()

	now := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	mock.ExpectQuery(qGetByID).
		WithArgs(int64(1)).
		WillReturnRows(sqlmock.NewRows([]string{"id", "name", "email", "created_at", "updated_at"}).
			AddRow(1, "alice", "a@b.com", now, now))

	got, err := s.GetByID(context.Background(), 1)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if got.ID != 1 || got.Name != "alice" || got.Email != "a@b.com" {
		t.Errorf("got %+v", got)
	}
	if !got.CreatedAt.Equal(now) || !got.UpdatedAt.Equal(now) {
		t.Errorf("timestamps: %+v", got)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("expectations: %v", err)
	}
}

func TestUserStore_GetByID_NotFound(t *testing.T) {
	s, mock, cleanup := newTestStore(t)
	defer cleanup()

	mock.ExpectQuery(qGetByID).WithArgs(int64(99)).WillReturnError(sql.ErrNoRows)

	_, err := s.GetByID(context.Background(), 99)
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("want ErrNotFound, got %v", err)
	}
}

// ── List ─────────────────────────────────────────────────────────────────────

func TestUserStore_List_Success(t *testing.T) {
	s, mock, cleanup := newTestStore(t)
	defer cleanup()

	now := time.Now()
	rows := sqlmock.NewRows([]string{"id", "name", "email", "created_at", "updated_at"}).
		AddRow(1, "alice", "a@b.com", now, now).
		AddRow(2, "bob", "b@b.com", now, now)
	mock.ExpectQuery(qList).WithArgs(10, 0).WillReturnRows(rows)

	got, err := s.List(context.Background(), 10, 0)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("got %d users", len(got))
	}
	if got[0].Name != "alice" || got[1].Name != "bob" {
		t.Errorf("ordering broken: %+v", got)
	}
}

func TestUserStore_List_Empty(t *testing.T) {
	s, mock, cleanup := newTestStore(t)
	defer cleanup()

	mock.ExpectQuery(qList).WithArgs(20, 100).
		WillReturnRows(sqlmock.NewRows([]string{"id", "name", "email", "created_at", "updated_at"}))

	got, err := s.List(context.Background(), 20, 100)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 0 {
		t.Errorf("got %d users, want 0", len(got))
	}
}

// ── Create ───────────────────────────────────────────────────────────────────

func TestUserStore_Create_Success(t *testing.T) {
	s, mock, cleanup := newTestStore(t)
	defer cleanup()

	mock.ExpectExec(qInsert).
		WithArgs("alice", "a@b.com", sqlmock.AnyArg(), sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(42, 1))

	u := &User{Name: "alice", Email: "a@b.com"}
	before := time.Now()
	if err := s.Create(context.Background(), u); err != nil {
		t.Fatalf("err: %v", err)
	}
	if u.ID != 42 {
		t.Errorf("ID: got %d, want 42", u.ID)
	}
	if u.CreatedAt.Before(before) || u.CreatedAt.After(time.Now().Add(time.Second)) {
		t.Errorf("CreatedAt out of range: %v", u.CreatedAt)
	}
	if !u.UpdatedAt.Equal(u.CreatedAt) {
		t.Errorf("CreatedAt/UpdatedAt should match: %v / %v", u.CreatedAt, u.UpdatedAt)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("expectations: %v", err)
	}
}

func TestUserStore_Create_Duplicate(t *testing.T) {
	s, mock, cleanup := newTestStore(t)
	defer cleanup()

	mock.ExpectExec(qInsert).
		WithArgs("alice", "dup@x.com", sqlmock.AnyArg(), sqlmock.AnyArg()).
		WillReturnError(&mysql.MySQLError{Number: 1062, Message: "Duplicate entry"})

	err := s.Create(context.Background(), &User{Name: "alice", Email: "dup@x.com"})
	if !errors.Is(err, ErrDuplicate) {
		t.Errorf("want ErrDuplicate, got %v", err)
	}
}

func TestUserStore_Create_OtherError_PassesThrough(t *testing.T) {
	s, mock, cleanup := newTestStore(t)
	defer cleanup()

	other := errors.New("connection reset")
	mock.ExpectExec(qInsert).
		WillReturnError(other)

	err := s.Create(context.Background(), &User{Name: "x", Email: "x@x.com"})
	if !errors.Is(err, other) {
		t.Errorf("want %v, got %v", other, err)
	}
}

// ── Update ───────────────────────────────────────────────────────────────────

func TestUserStore_Update_Success(t *testing.T) {
	s, mock, cleanup := newTestStore(t)
	defer cleanup()

	mock.ExpectExec(qUpdate).
		WithArgs("alice2", "a2@b.com", sqlmock.AnyArg(), int64(7)).
		WillReturnResult(sqlmock.NewResult(0, 1))

	u := &User{ID: 7, Name: "alice2", Email: "a2@b.com"}
	before := time.Now()
	if err := s.Update(context.Background(), u); err != nil {
		t.Fatalf("err: %v", err)
	}
	if u.UpdatedAt.Before(before) {
		t.Errorf("UpdatedAt not advanced: %v", u.UpdatedAt)
	}
}

func TestUserStore_Update_NotFound(t *testing.T) {
	s, mock, cleanup := newTestStore(t)
	defer cleanup()

	mock.ExpectExec(qUpdate).
		WithArgs("x", "y@z.com", sqlmock.AnyArg(), int64(99)).
		WillReturnResult(sqlmock.NewResult(0, 0))

	err := s.Update(context.Background(), &User{ID: 99, Name: "x", Email: "y@z.com"})
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("want ErrNotFound, got %v", err)
	}
}

func TestUserStore_Update_Duplicate(t *testing.T) {
	s, mock, cleanup := newTestStore(t)
	defer cleanup()

	mock.ExpectExec(qUpdate).
		WillReturnError(&mysql.MySQLError{Number: 1062, Message: "Duplicate entry"})

	err := s.Update(context.Background(), &User{ID: 1, Name: "x", Email: "taken@x.com"})
	if !errors.Is(err, ErrDuplicate) {
		t.Errorf("want ErrDuplicate, got %v", err)
	}
}

// ── Delete ───────────────────────────────────────────────────────────────────

func TestUserStore_Delete_Success(t *testing.T) {
	s, mock, cleanup := newTestStore(t)
	defer cleanup()

	mock.ExpectExec(qDelete).
		WithArgs(int64(5)).
		WillReturnResult(sqlmock.NewResult(0, 1))

	if err := s.Delete(context.Background(), 5); err != nil {
		t.Errorf("err: %v", err)
	}
}

func TestUserStore_Delete_NotFound(t *testing.T) {
	s, mock, cleanup := newTestStore(t)
	defer cleanup()

	mock.ExpectExec(qDelete).
		WithArgs(int64(404)).
		WillReturnResult(sqlmock.NewResult(0, 0))

	err := s.Delete(context.Background(), 404)
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("want ErrNotFound, got %v", err)
	}
}
