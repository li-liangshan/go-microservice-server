package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestMain(m *testing.M) {
	for _, key := range []string{
		"SERVICE_NAME", "SERVICE_VERSION", "LOG_LEVEL", "LOG_FORMAT",
		"HTTP_ADDR", "HTTP_AUTH_TOKEN", "HTTP_READ_HEADER_TIMEOUT", "HTTP_READ_TIMEOUT",
		"HTTP_WRITE_TIMEOUT", "HTTP_IDLE_TIMEOUT", "HTTP_REQUEST_TIMEOUT",
		"HTTP_SHUTDOWN_TIMEOUT", "HTTP_READINESS_TIMEOUT", "HTTP_MAX_HEADER_BYTES",
		"HTTP_MAX_BODY_BYTES", "DATABASE_HOST", "DATABASE_PORT", "DATABASE_USER",
		"DATABASE_PASSWORD", "DATABASE_NAME", "DATABASE_MAX_OPEN_CONNS",
		"DATABASE_MAX_IDLE_CONNS", "DATABASE_CONN_MAX_LIFETIME", "DATABASE_CONNECT_TIMEOUT",
		"DATABASE_TLS_MODE", "DATABASE_CONNECT_RETRIES", "DATABASE_CONNECT_RETRY_INTERVAL",
		"TELEMETRY_ENDPOINT", "TELEMETRY_ENVIRONMENT", "TELEMETRY_ENABLED",
		"TELEMETRY_INSECURE", "TELEMETRY_SAMPLE_RATIO", "TELEMETRY_EXPORT_INTERVAL",
		"TELEMETRY_EXPORT_TIMEOUT", "TELEMETRY_SHUTDOWN_TIMEOUT",
	} {
		_ = os.Unsetenv(key)
	}
	os.Exit(m.Run())
}

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
		{"HTTP.ReadHeaderTimeout", cfg.HTTP.ReadHeaderTimeout, 5 * time.Second},
		{"HTTP.ReadTimeout", cfg.HTTP.ReadTimeout, 15 * time.Second},
		{"HTTP.WriteTimeout", cfg.HTTP.WriteTimeout, 15 * time.Second},
		{"HTTP.IdleTimeout", cfg.HTTP.IdleTimeout, 60 * time.Second},
		{"HTTP.RequestTimeout", cfg.HTTP.RequestTimeout, 10 * time.Second},
		{"HTTP.ShutdownTimeout", cfg.HTTP.ShutdownTimeout, 10 * time.Second},
		{"HTTP.ReadinessTimeout", cfg.HTTP.ReadinessTimeout, 2 * time.Second},
		{"HTTP.MaxHeaderBytes", cfg.HTTP.MaxHeaderBytes, 1 << 20},
		{"HTTP.MaxBodyBytes", cfg.HTTP.MaxBodyBytes, int64(1 << 20)},
		{"HTTP.AuthToken", cfg.HTTP.AuthToken, ""},
		{"DB.Driver", cfg.Database.Driver, "mysql"},
		{"DB.Host", cfg.Database.Host, "localhost"},
		{"DB.Port", cfg.Database.Port, 3306},
		{"DB.User", cfg.Database.User, "root"},
		{"DB.Name", cfg.Database.Name, "microservice"},
		{"DB.MaxOpenConns", cfg.Database.MaxOpenConns, 50},
		{"DB.MaxIdleConns", cfg.Database.MaxIdleConns, 10},
		{"DB.ConnMaxLifetime", cfg.Database.ConnMaxLifetime, 30 * time.Minute},
		{"DB.ConnectTimeout", cfg.Database.ConnectTimeout, 5 * time.Second},
		{"DB.ConnectRetries", cfg.Database.ConnectRetries, 5},
		{"DB.ConnectRetryInterval", cfg.Database.ConnectRetryInterval, 2 * time.Second},
		{"DB.TLSMode", cfg.Database.TLSMode, "false"},
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
  tls_mode: "true"
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
	if cfg.Database.TLSMode != "true" {
		t.Errorf("TLSMode: %q", cfg.Database.TLSMode)
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

func TestLoad_ProjectConfig(t *testing.T) {
	cfg, err := Load("../../configs/config.yaml")
	if err != nil {
		t.Fatalf("project config is invalid: %v", err)
	}
	if cfg.Database.TLSMode != "false" {
		t.Errorf("unexpected local TLS mode: %q", cfg.Database.TLSMode)
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

func TestLoad_AppliesEnvironmentOverrides(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "empty.yaml")
	if err := os.WriteFile(path, []byte(""), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("SERVICE_NAME", "env-service")
	t.Setenv("DATABASE_HOST", "mysql.internal")
	t.Setenv("DATABASE_PASSWORD", "from-env")
	t.Setenv("HTTP_AUTH_TOKEN", "secret-token")
	t.Setenv("HTTP_MAX_BODY_BYTES", "2048")
	t.Setenv("DATABASE_TLS_MODE", "true")
	t.Setenv("TELEMETRY_ENABLED", "false")

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.ServiceName != "env-service" {
		t.Errorf("ServiceName: %q", cfg.ServiceName)
	}
	if cfg.Database.Host != "mysql.internal" || cfg.Database.Password != "from-env" {
		t.Errorf("database env overrides lost: %+v", cfg.Database)
	}
	if cfg.HTTP.AuthToken != "secret-token" || cfg.HTTP.MaxBodyBytes != 2048 {
		t.Errorf("http env overrides lost: %+v", cfg.HTTP)
	}
	if cfg.Database.TLSMode != "true" {
		t.Errorf("TLS env override lost: %q", cfg.Database.TLSMode)
	}
	if cfg.Telemetry.Enabled {
		t.Error("telemetry should be disabled by environment")
	}
}

func TestLoad_RejectsInvalidEnvironmentOverride(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "empty.yaml")
	if err := os.WriteFile(path, []byte(""), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("DATABASE_PORT", "not-a-port")
	if _, err := Load(path); err == nil {
		t.Fatal("expected invalid environment override error")
	}
}

func TestValidate_RejectsUnsafeValues(t *testing.T) {
	cfg := &Config{
		ServiceName: "svc",
		Version:     "1",
		LogLevel:    "info",
		LogFormat:   "json",
		HTTP: HTTPConfig{
			Addr:              ":8080",
			ReadHeaderTimeout: time.Second,
			ReadTimeout:       time.Second,
			WriteTimeout:      time.Second,
			IdleTimeout:       time.Second,
			RequestTimeout:    time.Second,
			ShutdownTimeout:   time.Second,
			ReadinessTimeout:  time.Second,
			MaxHeaderBytes:    1,
			MaxBodyBytes:      0,
		},
		Database: DBConfig{
			Driver:          "postgres",
			Host:            "localhost",
			Port:            70000,
			User:            "app",
			Name:            "db",
			MaxOpenConns:    5,
			MaxIdleConns:    10,
			ConnMaxLifetime: time.Minute,
			ConnectTimeout:  time.Second,
			TLSMode:         "false",
		},
		Telemetry: TelemetryConfig{
			Enabled:         true,
			Endpoint:        "",
			SampleRatio:     2,
			ExportInterval:  time.Second,
			ExportTimeout:   time.Second,
			ShutdownTimeout: time.Second,
		},
	}
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected validation error")
	}
}

// productionConfig returns a configuration that satisfies every production
// requirement; individual tests break one rule at a time.
func productionConfig() *Config {
	return &Config{
		ServiceName: "svc",
		Version:     "1",
		Environment: EnvProduction,
		LogLevel:    "info",
		LogFormat:   "json",
		HTTP: HTTPConfig{
			Addr:              ":8080",
			ReadHeaderTimeout: time.Second,
			ReadTimeout:       time.Second,
			WriteTimeout:      time.Second,
			IdleTimeout:       time.Second,
			RequestTimeout:    time.Second,
			ShutdownTimeout:   time.Second,
			ReadinessTimeout:  time.Second,
			MaxHeaderBytes:    1,
			MaxBodyBytes:      1,
			AuthToken:         "a-strong-token",
		},
		Database: DBConfig{
			Driver:               "mysql",
			Host:                 "db.internal",
			Port:                 3306,
			User:                 "app",
			Password:             "a-strong-password",
			Name:                 "microservice",
			MaxOpenConns:         5,
			MaxIdleConns:         5,
			ConnMaxLifetime:      time.Minute,
			ConnectTimeout:       time.Second,
			ConnectRetries:       3,
			ConnectRetryInterval: time.Second,
			TLSMode:              "true",
		},
		Telemetry: TelemetryConfig{
			Enabled:         true,
			Endpoint:        "collector:4317",
			SampleRatio:     1,
			ExportInterval:  time.Second,
			ExportTimeout:   time.Second,
			ShutdownTimeout: time.Second,
		},
	}
}

func TestValidate_ProductionAcceptsSafeConfiguration(t *testing.T) {
	if err := productionConfig().Validate(); err != nil {
		t.Fatalf("Validate: %v", err)
	}
}

func TestValidate_ProductionRequiresSafeSettings(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*Config)
		want   string
	}{
		{"no auth token", func(c *Config) { c.HTTP.AuthToken = "" }, "auth_token"},
		{"root database user", func(c *Config) { c.Database.User = "root" }, "must not be \"root\""},
		{"database tls disabled", func(c *Config) { c.Database.TLSMode = "false" }, "tls_mode"},
		{"short database password", func(c *Config) { c.Database.Password = "short" }, "password"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cfg := productionConfig()
			tc.mutate(cfg)
			err := cfg.Validate()
			if err == nil {
				t.Fatal("expected a validation error")
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Errorf("error %q should mention %q", err, tc.want)
			}
		})
	}
}

// The production rules must not apply to the other environments.
func TestValidate_NonProductionAllowsUnsafeSettings(t *testing.T) {
	cfg := productionConfig()
	cfg.Environment = EnvDevelopment
	cfg.HTTP.AuthToken = ""
	cfg.Database.User = "root"
	cfg.Database.TLSMode = "false"
	cfg.Database.Password = ""

	if err := cfg.Validate(); err != nil {
		t.Fatalf("Validate: %v", err)
	}
}

func TestValidate_RejectsInvalidConnectRetrySettings(t *testing.T) {
	cfg := productionConfig()
	cfg.Environment = EnvDevelopment
	cfg.Database.ConnectRetries = -1
	cfg.Database.ConnectRetryInterval = 0

	err := cfg.Validate()
	if err == nil {
		t.Fatal("expected a validation error")
	}
	for _, want := range []string{"connect_retries", "connect_retry_interval"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q should mention %q", err, want)
		}
	}
}

func TestLoad_AppliesConnectRetryEnvironmentOverrides(t *testing.T) {
	t.Setenv("DATABASE_CONNECT_RETRIES", "7")
	t.Setenv("DATABASE_CONNECT_RETRY_INTERVAL", "3s")

	path := filepath.Join(t.TempDir(), "empty.yaml")
	if err := os.WriteFile(path, []byte(""), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Database.ConnectRetries != 7 {
		t.Errorf("ConnectRetries: got %d, want 7", cfg.Database.ConnectRetries)
	}
	if cfg.Database.ConnectRetryInterval != 3*time.Second {
		t.Errorf("ConnectRetryInterval: got %v, want 3s", cfg.Database.ConnectRetryInterval)
	}
}
