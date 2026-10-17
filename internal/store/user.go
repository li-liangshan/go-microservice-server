package store

import (
	"context"
	"database/sql"
	"errors"
	"microservice-server/internal/database"
	"time"
)

// Common sentinel errors for repository operations.
var (
	ErrNotFound = errors.New("record not found")
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
		"SELECT "+userColumns+" FROM users WHERE id = $1", id)

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
		"SELECT "+userColumns+" FROM users ORDER BY id LIMIT $1 OFFSET $2", limit, offset)
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
