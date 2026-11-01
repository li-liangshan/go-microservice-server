package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// writeConfig writes a minimal configuration file and returns its path.
func writeConfig(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}
	return path
}

const baseConfig = `
service_name: go-microservice
version: 1.0.0
environment: development
log_level: info
log_format: json

http:
  addr: ":0"
  read_header_timeout: 5s
  read_timeout: 15s
  write_timeout: 15s
  idle_timeout: 60s
  request_timeout: 10s
  shutdown_timeout: 10s
  readiness_timeout: 2s
  max_header_bytes: 1048576
  max_body_bytes: 1048576

database:
  driver: mysql
  host: 127.0.0.1
  port: 3306
  user: app
  password: "supersecret"
  name: microservice
  max_open_conns: 50
  max_idle_conns: 10
  conn_max_lifetime: 30m
  connect_timeout: 5s
  connect_retries: 0
  connect_retry_interval: 1s
  tls_mode: "false"

telemetry:
  enabled: false
  endpoint: localhost:4317
  insecure: true
  environment: development
  sample_ratio: 1.0
  export_interval: 10s
  export_timeout: 5s
  shutdown_timeout: 10s
`

func TestRun_UnknownFlag(t *testing.T) {
	t.Setenv("APP_ENV", "development")

	err := run([]string{"--not-a-flag"})
	if err == nil {
		t.Fatal("expected an error for an unknown flag")
	}
	if !strings.Contains(err.Error(), "parse flags") {
		t.Errorf("error: %v", err)
	}
}

func TestRun_MissingConfigFile(t *testing.T) {
	t.Setenv("APP_ENV", "development")

	err := run([]string{"--config", filepath.Join(t.TempDir(), "absent.yaml")})
	if err == nil {
		t.Fatal("expected an error for a missing config file")
	}
	if !strings.Contains(err.Error(), "load config") {
		t.Errorf("error: %v", err)
	}
}

func TestRun_InvalidConfig(t *testing.T) {
	t.Setenv("APP_ENV", "development")

	path := writeConfig(t, "service_name: [unclosed\n")
	err := run([]string{"--config", path})
	if err == nil {
		t.Fatal("expected an error for a malformed config file")
	}
	if !strings.Contains(err.Error(), "load config") {
		t.Errorf("error: %v", err)
	}
}

// The production environment refuses to start without an auth token, a
// non-root database account and database TLS.
func TestRun_ProductionRequiresSafeSettings(t *testing.T) {
	t.Setenv("APP_ENV", "production")

	path := writeConfig(t, baseConfig)
	err := run([]string{"--config", path})
	if err == nil {
		t.Fatal("expected the service to refuse to start in production")
	}
	for _, want := range []string{"auth_token", "tls_mode"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q should mention %q", err, want)
		}
	}
}

// The production environment also refuses to use the database root account.
func TestRun_ProductionRejectsRootDatabaseUser(t *testing.T) {
	t.Setenv("APP_ENV", "production")

	path := writeConfig(t, strings.Replace(baseConfig, "user: app", "user: root", 1))
	err := run([]string{"--config", path})
	if err == nil {
		t.Fatal("expected the service to refuse to start in production")
	}
	if !strings.Contains(err.Error(), "must not be \"root\"") {
		t.Errorf("error %q should mention the root account", err)
	}
}

// A database that cannot be reached must not be reported as a success, and
// the failure must name the database.
func TestRun_ReportsDatabaseFailure(t *testing.T) {
	t.Setenv("APP_ENV", "development")
	t.Setenv("DATABASE_HOST", "127.0.0.1")
	t.Setenv("DATABASE_PORT", "1")
	t.Setenv("DATABASE_CONNECT_RETRIES", "0")

	path := writeConfig(t, baseConfig)
	err := run([]string{"--config", path})
	if err == nil {
		t.Fatal("expected an error when the database is unreachable")
	}
	if !strings.Contains(err.Error(), "connect database") {
		t.Errorf("error: %v", err)
	}
}
