package database

import (
	"strings"
	"testing"

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
