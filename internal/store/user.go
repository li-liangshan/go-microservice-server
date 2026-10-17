package store

import (
	"context"
	"database/sql"
	"errors"
	"microservice-server/internal/database"
	"time"

	"github.com/go-sql-driver/mysql"
)

// Common sentinel errors for repository operations.
var (
	ErrNotFound  = errors.New("record not found")
	ErrDuplicate = errors.New("duplicate record")
)

// User is the persistence model.
type User struct {
	ID        int64     `json:"id"`
	Name      string    `json:"name"`
	Email     string    `json:"email"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// UserStore handles user persistence. All methods accept context.Context
// so call cancellation and deadlines can propagate to the database driver.
type UserStore struct {
	db *sql.DB
}

// NewUserStore returns a new UserStore.
func NewUserStore(db *database.DB) *UserStore {
	return &UserStore{db: db.DB}
}

const userColumns = "id, name, email, created_at, updated_at"

func (s *UserStore) GetByID(ctx context.Context, id int64) (*User, error) {
	row := s.db.QueryRowContext(ctx,
		"SELECT "+userColumns+" FROM users WHERE id = ?", id)

	var u User
	if err := row.Scan(&u.ID, &u.Name, &u.Email, &u.CreatedAt, &u.UpdatedAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return &u, nil
}

func (s *UserStore) List(ctx context.Context, limit, offset int) ([]*User, error) {
	rows, err := s.db.QueryContext(ctx,
		"SELECT "+userColumns+" FROM users ORDER BY id LIMIT ? OFFSET ?", limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var users []*User
	for rows.Next() {
		var u User
		if err := rows.Scan(&u.ID, &u.Name, &u.Email, &u.CreatedAt, &u.UpdatedAt); err != nil {
			return nil, err
		}
		users = append(users, &u)
	}
	return users, rows.Err()
}

// Create inserts a new user and stamps it with the generated ID and timestamps.
// On insert the driver reports ER_DUP_ENTRY (1062) as ErrDuplicate.
func (s *UserStore) Create(ctx context.Context, u *User) error {
	now := time.Now().UTC()
	res, err := s.db.ExecContext(ctx,
		"INSERT INTO users (name, email, created_at, updated_at) VALUES (?, ?, ?, ?)",
		u.Name, u.Email, now, now,
	)
	if err != nil {
		var me *mysql.MySQLError
		if errors.As(err, &me) && me.Number == 1062 {
			return ErrDuplicate
		}
		return err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return err
	}
	u.ID = id
	u.CreatedAt = now
	u.UpdatedAt = now
	return nil
}

// Update modifies an existing user. Returns ErrNotFound when no row matches id,
// ErrDuplicate on email unique-constraint violation.
func (s *UserStore) Update(ctx context.Context, u *User) error {
	now := time.Now().UTC()
	res, err := s.db.ExecContext(ctx,
		"UPDATE users SET name = ?, email = ?, updated_at = ? WHERE id = ?",
		u.Name, u.Email, now, u.ID,
	)
	if err != nil {
		var me *mysql.MySQLError
		if errors.As(err, &me) && me.Number == 1062 {
			return ErrDuplicate
		}
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return ErrNotFound
	}
	u.UpdatedAt = now
	return nil
}

// Delete removes a user by id. Returns ErrNotFound when no row matches.
func (s *UserStore) Delete(ctx context.Context, id int64) error {
	res, err := s.db.ExecContext(ctx, "DELETE FROM users WHERE id = ?", id)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return ErrNotFound
	}
	return nil
}
