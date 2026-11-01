package store

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"microservice-server/internal/config"
	"microservice-server/internal/database"
)

// The integration test runs against a real MySQL instance so that the
// behaviour of the SQL layer — affected-row semantics, unique-constraint
// handling, schema compatibility — is verified against the server rather than
// against a mock.
//
// It is skipped unless TEST_MYSQL_HOST is set:
//
//	TEST_MYSQL_HOST=127.0.0.1 TEST_MYSQL_USER=root TEST_MYSQL_PASSWORD=secret \
//	    go test ./internal/store -run Integration
func integrationConfig(t *testing.T) config.DBConfig {
	t.Helper()

	host := os.Getenv("TEST_MYSQL_HOST")
	if host == "" {
		t.Skip("TEST_MYSQL_HOST is not set; skipping the MySQL integration test")
	}

	port := 3306
	if value := os.Getenv("TEST_MYSQL_PORT"); value != "" {
		parsed, err := strconv.Atoi(value)
		if err != nil {
			t.Fatalf("TEST_MYSQL_PORT: %v", err)
		}
		port = parsed
	}

	return config.DBConfig{
		Driver:               "mysql",
		Host:                 host,
		Port:                 port,
		User:                 envOr("TEST_MYSQL_USER", "root"),
		Password:             envOr("TEST_MYSQL_PASSWORD", ""),
		Name:                 envOr("TEST_MYSQL_DATABASE", "microservice_it"),
		MaxOpenConns:         5,
		MaxIdleConns:         5,
		ConnMaxLifetime:      time.Minute,
		ConnectTimeout:       5 * time.Second,
		ConnectRetries:       3,
		ConnectRetryInterval: time.Second,
		TLSMode:              envOr("TEST_MYSQL_TLS_MODE", "false"),
	}
}

func envOr(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}

// newIntegrationStore connects to MySQL, applies the migration and returns a
// store backed by an empty users table.
func newIntegrationStore(t *testing.T) *UserStore {
	t.Helper()

	cfg := integrationConfig(t)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// The database itself must exist before the connection can be opened.
	admin, err := sql.Open("mysql", adminDSN(cfg))
	if err != nil {
		t.Fatalf("open admin connection: %v", err)
	}
	defer func() { _ = admin.Close() }()

	if _, err := admin.ExecContext(ctx, "CREATE DATABASE IF NOT EXISTS `"+cfg.Name+"`"); err != nil {
		t.Fatalf("create database: %v", err)
	}

	db, err := database.New(ctx, cfg)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	migration, err := os.ReadFile(filepath.Join("..", "..", "migrations", "0001_init.sql"))
	if err != nil {
		t.Fatalf("read migration: %v", err)
	}
	if _, err := db.ExecContext(ctx, string(migration)); err != nil {
		t.Fatalf("apply migration: %v", err)
	}
	if _, err := db.ExecContext(ctx, "TRUNCATE TABLE users"); err != nil {
		t.Fatalf("truncate: %v", err)
	}

	return NewUserStore(db)
}

// adminDSN builds a DSN that is not tied to a database, for the CREATE
// DATABASE statement.
func adminDSN(cfg config.DBConfig) string {
	return cfg.User + ":" + cfg.Password + "@tcp(" + cfg.Host + ":" + strconv.Itoa(cfg.Port) + ")/?parseTime=true"
}

func TestIntegration_UserStoreCRUD(t *testing.T) {
	s := newIntegrationStore(t)
	ctx := context.Background()

	// Create
	created := &User{Name: "alice", Email: "alice@example.com"}
	if err := s.Create(ctx, created); err != nil {
		t.Fatalf("create: %v", err)
	}
	if created.ID == 0 {
		t.Fatal("create did not populate the id")
	}
	if created.CreatedAt.IsZero() || created.UpdatedAt.IsZero() {
		t.Fatalf("create did not populate the timestamps: %+v", created)
	}

	// Duplicate email
	if err := s.Create(ctx, &User{Name: "other", Email: "alice@example.com"}); !errors.Is(err, ErrDuplicate) {
		t.Errorf("duplicate create: want ErrDuplicate, got %v", err)
	}

	// Get
	got, err := s.GetByID(ctx, created.ID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.Email != "alice@example.com" {
		t.Errorf("get: %+v", got)
	}

	// Get missing
	if _, err := s.GetByID(ctx, created.ID+1000); !errors.Is(err, ErrNotFound) {
		t.Errorf("get missing: want ErrNotFound, got %v", err)
	}

	// Update
	updated := &User{ID: created.ID, Name: "alice2", Email: "alice2@example.com"}
	if err := s.Update(ctx, updated); err != nil {
		t.Fatalf("update: %v", err)
	}
	refetched, err := s.GetByID(ctx, created.ID)
	if err != nil {
		t.Fatalf("get after update: %v", err)
	}
	if refetched.Name != "alice2" {
		t.Errorf("update was not persisted: %+v", refetched)
	}

	// An update that writes the values already stored must succeed. MySQL
	// reports zero affected rows for such an UPDATE, which must not be
	// mistaken for a missing row.
	noop := &User{ID: created.ID, Name: "alice2", Email: "alice2@example.com"}
	if err := s.Update(ctx, noop); err != nil {
		t.Fatalf("no-op update: %v", err)
	}

	// Update missing
	if err := s.Update(ctx, &User{ID: created.ID + 1000, Name: "x", Email: "x@example.com"}); !errors.Is(err, ErrNotFound) {
		t.Errorf("update missing: want ErrNotFound, got %v", err)
	}

	// List
	if err := s.Create(ctx, &User{Name: "bob", Email: "bob@example.com"}); err != nil {
		t.Fatalf("create second: %v", err)
	}
	users, err := s.List(ctx, 10, 0)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(users) != 2 {
		t.Errorf("list: got %d users, want 2", len(users))
	}

	// Delete
	if err := s.Delete(ctx, created.ID); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if err := s.Delete(ctx, created.ID); !errors.Is(err, ErrNotFound) {
		t.Errorf("delete missing: want ErrNotFound, got %v", err)
	}
}
