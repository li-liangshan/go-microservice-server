package store

import "context"

// UserStoreInterface decouples the service layer from the concrete SQL implementation.
// It allows easy substitution for tests or other backends.
type UserStoreInterface interface {
	GetByID(ctx context.Context, id int64) (*User, error)
	List(ctx context.Context, limit, offset int) ([]*User, error)
	Create(ctx context.Context, u *User) error
	Update(ctx context.Context, u *User) error
	Delete(ctx context.Context, id int64) error
}
