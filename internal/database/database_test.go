package database

import (
	"context"
	"strings"
	"testing"
	"time"

	"microservice-server/internal/config"
)

func TestBuildDSN_UsesUTCAndTLS(t *testing.T) {
	dsn := buildDSN(config.DBConfig{
		User:     "app",
		Password: "secret",
		Host:     "db.internal",
		Port:     3307,
		Name:     "my service",
		TLSMode:  "true",
	})

	for _, want := range []string{
		"app:secret@tcp(db.internal:3307)/my+service",
		"parseTime=true",
		"loc=UTC",
		"charset=utf8mb4",
		"tls=true",
	} {
		if !strings.Contains(dsn, want) {
			t.Errorf("DSN %q missing %q", dsn, want)
		}
	}
}

// clientFoundRows makes the driver report matched rows, so that an UPDATE
// which writes identical values is not mistaken for a missing row.
func TestBuildDSN_ReportsMatchedRows(t *testing.T) {
	dsn := buildDSN(config.DBConfig{User: "app", Host: "db", Port: 3306, Name: "db", TLSMode: "false"})
	if !strings.Contains(dsn, "clientFoundRows=true") {
		t.Errorf("DSN %q missing clientFoundRows=true", dsn)
	}
}

// A database that is not reachable must be reported as an error rather than
// silently returning an unusable pool.
func TestNew_UnreachableDatabaseFails(t *testing.T) {
	cfg := config.DBConfig{
		Driver:               "mysql",
		Host:                 "127.0.0.1",
		Port:                 1,
		User:                 "app",
		Password:             "secret",
		Name:                 "db",
		MaxOpenConns:         1,
		MaxIdleConns:         1,
		ConnMaxLifetime:      time.Minute,
		ConnectTimeout:       time.Second,
		ConnectRetries:       1,
		ConnectRetryInterval: 10 * time.Millisecond,
		TLSMode:              "false",
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	db, err := New(ctx, cfg)
	if err == nil {
		_ = db.Close()
		t.Fatal("expected an error for an unreachable database")
	}
	if !strings.Contains(err.Error(), "ping db") {
		t.Errorf("error: %v", err)
	}
}

// The initial connection is retried, so the service can start next to a
// database that is still booting.
func TestNew_RetriesUntilTheContextIsCancelled(t *testing.T) {
	cfg := config.DBConfig{
		Driver:               "mysql",
		Host:                 "127.0.0.1",
		Port:                 1,
		User:                 "app",
		Password:             "secret",
		Name:                 "db",
		MaxOpenConns:         1,
		MaxIdleConns:         1,
		ConnMaxLifetime:      time.Minute,
		ConnectTimeout:       200 * time.Millisecond,
		ConnectRetries:       100,
		ConnectRetryInterval: 50 * time.Millisecond,
		TLSMode:              "false",
	}

	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()

	start := time.Now()
	if _, err := New(ctx, cfg); err == nil {
		t.Fatal("expected an error for an unreachable database")
	}
	// The retry loop must stop when the context is done rather than run for
	// the full retry budget.
	if elapsed := time.Since(start); elapsed > 5*time.Second {
		t.Errorf("retry loop ran for %v, expected it to stop with the context", elapsed)
	}
}
