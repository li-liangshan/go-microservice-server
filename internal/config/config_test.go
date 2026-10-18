package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestLoad_AppliesAllDefaults(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "empty.yaml")
	if err := os.WriteFile(path, []byte(""), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	cases := []struct {
		name string
		got  any
		want any
	}{
		{"ServiceName", cfg.ServiceName, "go-microservice"},
		{"Version", cfg.Version, "1.0.0"},
		{"LogLevel", cfg.LogLevel, "info"},
		{"LogFormat", cfg.LogFormat, "json"},
		{"HTTP.Addr", cfg.HTTP.Addr, ":8080"},
		{"HTTP.ReadTimeout", cfg.HTTP.ReadTimeout, 15 * time.Second},
		{"HTTP.WriteTimeout", cfg.HTTP.WriteTimeout, 15 * time.Second},
		{"HTTP.IdleTimeout", cfg.HTTP.IdleTimeout, 60 * time.Second},
		{"HTTP.ShutdownTimeout", cfg.HTTP.ShutdownTimeout, 10 * time.Second},
		{"DB.Driver", cfg.Database.Driver, "mysql"},
		{"DB.Host", cfg.Database.Host, "localhost"},
		{"DB.Port", cfg.Database.Port, 3306},
		{"DB.MaxOpenConns", cfg.Database.MaxOpenConns, 50},
		{"DB.MaxIdleConns", cfg.Database.MaxIdleConns, 10},
		{"DB.ConnMaxLifetime", cfg.Database.ConnMaxLifetime, 30 * time.Minute},
		{"Telemetry.Enabled", cfg.Telemetry.Enabled, true},
		{"Telemetry.Endpoint", cfg.Telemetry.Endpoint, "localhost:4317"},
		{"Telemetry.Insecure", cfg.Telemetry.Insecure, true},
		{"Telemetry.Environment", cfg.Telemetry.Environment, "development"},
		{"Telemetry.SampleRatio", cfg.Telemetry.SampleRatio, 1.0},
		{"Telemetry.ExportInterval", cfg.Telemetry.ExportInterval, 10 * time.Second},
		{"Telemetry.ExportTimeout", cfg.Telemetry.ExportTimeout, 5 * time.Second},
		{"Telemetry.ShutdownTimeout", cfg.Telemetry.ShutdownTimeout, 10 * time.Second},
	}
	for _, c := range cases {
		if c.got != c.want {
			t.Errorf("%s: got %v, want %v", c.name, c.got, c.want)
		}
	}
}

func TestLoad_OverridesFromYAML(t *testing.T) {
	const yaml = `service_name: custom-svc
version: 9.9.9
log_level: debug
log_format: text
http:
  addr: ":9000"
  read_timeout: 5s
  write_timeout: 7s
  idle_timeout: 30s
  shutdown_timeout: 4s
database:
  driver: mysql
  host: db.internal
  port: 3307
  user: app
  password: secret
  name: prod
  max_open_conns: 100
  max_idle_conns: 20
  conn_max_lifetime: 2h
telemetry:
  enabled: true
  endpoint: otel-collector:4317
  insecure: false
  environment: production
  sample_ratio: 0.25
  export_interval: 30s
  export_timeout: 8s
  shutdown_timeout: 12s
  headers:
    x-api-key: secret
`
	dir := t.TempDir()
	path := filepath.Join(dir, "cfg.yaml")
	if err := os.WriteFile(path, []byte(yaml), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	if cfg.ServiceName != "custom-svc" {
		t.Errorf("ServiceName: %q", cfg.ServiceName)
	}
	if cfg.HTTP.Addr != ":9000" || cfg.HTTP.ReadTimeout != 5*time.Second {
		t.Errorf("HTTP overrides lost: %+v", cfg.HTTP)
	}
	if cfg.Database.Host != "db.internal" || cfg.Database.Port != 3307 {
		t.Errorf("DB overrides lost: %+v", cfg.Database)
	}
	if cfg.Database.Password != "secret" {
		t.Errorf("Password: %q", cfg.Database.Password)
	}
	if cfg.Database.ConnMaxLifetime != 2*time.Hour {
		t.Errorf("ConnMaxLifetime: %v", cfg.Database.ConnMaxLifetime)
	}
	if cfg.Telemetry.Endpoint != "otel-collector:4317" || cfg.Telemetry.Insecure {
		t.Errorf("Telemetry overrides lost: %+v", cfg.Telemetry)
	}
	if cfg.Telemetry.SampleRatio != 0.25 || cfg.Telemetry.ExportInterval != 30*time.Second {
		t.Errorf("Telemetry sampling/export lost: %+v", cfg.Telemetry)
	}
	if cfg.Telemetry.Headers["x-api-key"] != "secret" {
		t.Errorf("Telemetry headers: %+v", cfg.Telemetry.Headers)
	}
}

func TestLoad_PartialOverride_KeepsRestAsDefault(t *testing.T) {
	const yaml = `database:
  port: 4444
`
	dir := t.TempDir()
	path := filepath.Join(dir, "cfg.yaml")
	if err := os.WriteFile(path, []byte(yaml), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Database.Port != 4444 {
		t.Errorf("Port: got %d", cfg.Database.Port)
	}
	// Untouched fields keep their defaults.
	if cfg.Database.Host != "localhost" {
		t.Errorf("Host should remain default, got %q", cfg.Database.Host)
	}
	if cfg.HTTP.Addr != ":8080" {
		t.Errorf("Addr should remain default, got %q", cfg.HTTP.Addr)
	}
}

func TestLoad_NotFound(t *testing.T) {
	if _, err := Load(filepath.Join(t.TempDir(), "missing.yaml")); err == nil {
		t.Fatal("expected error for missing file")
	}
}

func TestLoad_InvalidYAML(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "bad.yaml")
	if err := os.WriteFile(path, []byte("foo: : bar\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(path); err == nil {
		t.Fatal("expected yaml parse error")
	}
}

func TestLoad_InvalidTelemetrySampleRatio(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "bad.yaml")
	if err := os.WriteFile(path, []byte("telemetry:\n  sample_ratio: 1.5\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(path); err == nil {
		t.Fatal("expected sample ratio validation error")
	}
}

func TestLoad_RejectsEmptyTelemetryEndpointWhenEnabled(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "bad.yaml")
	if err := os.WriteFile(path, []byte("telemetry:\n  enabled: true\n  endpoint: \"\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(path); err == nil {
		t.Fatal("expected endpoint validation error")
	}
}
