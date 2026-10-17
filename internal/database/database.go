package database

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"microservice-server/internal/config"

	_ "github.com/lib/pq"
)

// DB wraps *sql.DB with lifecycle helpers.
type DB struct {
	*sql.DB
}

// New opens a database connection, configures the connection pool and verifies connectivity.
func New(cfg config.DBConfig) (*DB, error) {
	dsn := fmt.Sprintf(
		"host=%s port=%d user=%s password=%s dbname=%s sslmode=disable",
		cfg.Host, cfg.Port, cfg.User, cfg.Password, cfg.Name,
	)

	conn, err := sql.Open(cfg.Driver, dsn)
	if err != nil {
		return nil, fmt.Errorf("open db: %w", err)
	}

	conn.SetMaxOpenConns(cfg.MaxOpenConns)
	conn.SetMaxIdleConns(cfg.MaxIdleConns)
	conn.SetConnMaxLifetime(cfg.ConnMaxLifetime)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := conn.PingContext(ctx); err != nil {
		return nil, fmt.Errorf("ping db: %w", err)
	}

	return &DB{conn}, nil
}
