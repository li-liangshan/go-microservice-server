package service

import (
	"context"
	"errors"
	"log/slog"

	"microservice-server/internal/store"
)

// Errors returned by the service layer.
var (
	ErrUserNotFound = errors.New("user not found")
)

// UserService encapsulates business logic for users.
type UserService struct {
	store store.UserStoreInterface
	log   *slog.Logger
}

// NewUserService constructs a UserService.
func NewUserService(s store.UserStoreInterface, log *slog.Logger) *UserService {
	return &UserService{store: s, log: log}
}

// Get retrieves a user by ID, translating store-level errors into service-level ones.
func (s *UserService) Get(ctx context.Context, id int64) (*store.User, error) {
	user, err := s.store.GetByID(ctx, id)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return nil, ErrUserNotFound
		}
		s.log.Error("user store get failed", "id", id, "err", err)
		return nil, err
	}
	return user, nil
}

// List returns a paginated list of users.
func (s *UserService) List(ctx context.Context, limit, offset int) ([]*store.User, error) {
	users, err := s.store.List(ctx, limit, offset)
	if err != nil {
		s.log.Error("user store list failed", "err", err)
		return nil, err
	}
	return users, nil
}
