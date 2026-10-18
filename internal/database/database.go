package database

import (
	"context"
	"database/sql"
	"fmt"
	"net/url"
	"time"

	"microservice-server/internal/config"

	"github.com/XSAM/otelsql"
	_ "github.com/go-sql-driver/mysql"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"
)

// DB wraps *sql.DB with lifecycle helpers.
type DB struct {
	*sql.DB
}

// New opens a database connection, configures the connection pool and verifies connectivity.
func New(cfg config.DBConfig) (*DB, error) {
	// Build a MySQL DSN: user:password@tcp(host:port)/dbname?parseTime=true&loc=Local&charset=utf8mb4
	// parseTime=true lets the driver scan time.Time directly from DATETIME columns.
	dsn := fmt.Sprintf(
		"%s:%s@tcp(%s:%d)/%s?parseTime=true&loc=Local&charset=utf8mb4",
		cfg.User, cfg.Password, cfg.Host, cfg.Port, url.QueryEscape(cfg.Name),
	)

	conn, err := otelsql.Open("mysql", dsn, otelsql.WithAttributes(semconv.DBSystemMySQL))
	if err != nil {
		return nil, fmt.Errorf("open db: %w", err)
	}

	if err := otelsql.RegisterDBStatsMetrics(conn, otelsql.WithAttributes(semconv.DBSystemMySQL)); err != nil {
		_ = conn.Close()
		return nil, fmt.Errorf("register db metrics: %w", err)
	}

	conn.SetMaxOpenConns(cfg.MaxOpenConns)
	conn.SetMaxIdleConns(cfg.MaxIdleConns)
	conn.SetConnMaxLifetime(cfg.ConnMaxLifetime)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := conn.PingContext(ctx); err != nil {
		_ = conn.Close()
		return nil, fmt.Errorf("ping db: %w", err)
	}

	return &DB{conn}, nil
}
