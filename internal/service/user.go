package service

import (
	"context"
	"errors"
	"log/slog"
	"regexp"
	"strings"

	"microservice-server/internal/store"
)

// Errors returned by the service layer.
var (
	ErrUserNotFound = errors.New("user not found")
	ErrInvalidInput = errors.New("invalid input")
	ErrEmailExists  = errors.New("email already exists")
)

// Tiny email shape check; full RFC-5322 validation belongs to the caller.
var emailRegex = regexp.MustCompile(`^[^@\s]+@[^@\s]+\.[^@\s]+$`)

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

// Create validates the input and persists a new user. Translates store-level
// ErrDuplicate into ErrEmailExists and returns a fully populated record.
func (s *UserService) Create(ctx context.Context, u *store.User) (*store.User, error) {
	if !validateUser(u) {
		return nil, ErrInvalidInput
	}
	if err := s.store.Create(ctx, u); err != nil {
		if errors.Is(err, store.ErrDuplicate) {
			return nil, ErrEmailExists
		}
		s.log.Error("user store create failed", "err", err)
		return nil, err
	}
	return u, nil
}

// Update modifies an existing user. Validates input, runs the UPDATE, then
// re-fetches so the returned record reflects the actual DB state.
func (s *UserService) Update(ctx context.Context, id int64, u *store.User) (*store.User, error) {
	u.ID = id
	if !validateUser(u) {
		return nil, ErrInvalidInput
	}
	if err := s.store.Update(ctx, u); err != nil {
		if errors.Is(err, store.ErrDuplicate) {
			return nil, ErrEmailExists
		}
		if errors.Is(err, store.ErrNotFound) {
			return nil, ErrUserNotFound
		}
		s.log.Error("user store update failed", "id", id, "err", err)
		return nil, err
	}
	return s.store.GetByID(ctx, id)
}

// Delete removes a user by id. Translates ErrNotFound into ErrUserNotFound.
func (s *UserService) Delete(ctx context.Context, id int64) error {
	if err := s.store.Delete(ctx, id); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return ErrUserNotFound
		}
		s.log.Error("user store delete failed", "id", id, "err", err)
		return err
	}
	return nil
}

// validateUser normalises and checks name + email shape. Returns true when the
// input is acceptable.
func validateUser(u *store.User) bool {
	u.Name = strings.TrimSpace(u.Name)
	u.Email = strings.TrimSpace(u.Email)
	return u.Name != "" && emailRegex.MatchString(u.Email)
}
