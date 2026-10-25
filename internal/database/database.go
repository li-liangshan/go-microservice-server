package database

import (
	"context"
	"database/sql"
	"fmt"
	"net/url"

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
func New(ctx context.Context, cfg config.DBConfig) (*DB, error) {
	dsn := buildDSN(cfg)

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

	pingCtx, cancel := context.WithTimeout(ctx, cfg.ConnectTimeout)
	defer cancel()

	if err := conn.PingContext(pingCtx); err != nil {
		_ = conn.Close()
		return nil, fmt.Errorf("ping db: %w", err)
	}

	return &DB{conn}, nil
}

func buildDSN(cfg config.DBConfig) string {
	// parseTime=true lets the driver scan time.Time directly from DATETIME columns.
	return fmt.Sprintf(
		"%s:%s@tcp(%s:%d)/%s?parseTime=true&loc=UTC&charset=utf8mb4&tls=%s",
		cfg.User, cfg.Password, cfg.Host, cfg.Port, url.QueryEscape(cfg.Name), url.QueryEscape(cfg.TLSMode),
	)
}
