package config

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// Config is the top-level service configuration.
type Config struct {
	ServiceName string          `yaml:"service_name"`
	Version     string          `yaml:"version"`
	LogLevel    string          `yaml:"log_level"`
	LogFormat   string          `yaml:"log_format"`
	HTTP        HTTPConfig      `yaml:"http"`
	Database    DBConfig        `yaml:"database"`
	Telemetry   TelemetryConfig `yaml:"telemetry"`
}

// HTTPConfig holds all HTTP server tunables.
type HTTPConfig struct {
	Addr              string        `yaml:"addr"`
	ReadHeaderTimeout time.Duration `yaml:"read_header_timeout"`
	ReadTimeout       time.Duration `yaml:"read_timeout"`
	WriteTimeout      time.Duration `yaml:"write_timeout"`
	IdleTimeout       time.Duration `yaml:"idle_timeout"`
	RequestTimeout    time.Duration `yaml:"request_timeout"`
	ShutdownTimeout   time.Duration `yaml:"shutdown_timeout"`
	ReadinessTimeout  time.Duration `yaml:"readiness_timeout"`
	MaxHeaderBytes    int           `yaml:"max_header_bytes"`
	MaxBodyBytes      int64         `yaml:"max_body_bytes"`
	AuthToken         string        `yaml:"auth_token"`
}

// DBConfig holds database connection parameters.
type DBConfig struct {
	Driver          string        `yaml:"driver"`
	Host            string        `yaml:"host"`
	Port            int           `yaml:"port"`
	User            string        `yaml:"user"`
	Password        string        `yaml:"password"`
	Name            string        `yaml:"name"`
	MaxOpenConns    int           `yaml:"max_open_conns"`
	MaxIdleConns    int           `yaml:"max_idle_conns"`
	ConnMaxLifetime time.Duration `yaml:"conn_max_lifetime"`
	ConnectTimeout  time.Duration `yaml:"connect_timeout"`
	TLSMode         string        `yaml:"tls_mode"`
}

// TelemetryConfig controls OpenTelemetry trace and metric export.
type TelemetryConfig struct {
	Enabled         bool              `yaml:"enabled"`
	Endpoint        string            `yaml:"endpoint"`
	Insecure        bool              `yaml:"insecure"`
	Environment     string            `yaml:"environment"`
	SampleRatio     float64           `yaml:"sample_ratio"`
	ExportInterval  time.Duration     `yaml:"export_interval"`
	ExportTimeout   time.Duration     `yaml:"export_timeout"`
	ShutdownTimeout time.Duration     `yaml:"shutdown_timeout"`
	Headers         map[string]string `yaml:"headers"`
}

// Load reads YAML configuration from disk and applies sensible defaults.
func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	cfg := &Config{
		ServiceName: "go-microservice",
		Version:     "1.0.0",
		LogLevel:    "info",
		LogFormat:   "json",
		HTTP: HTTPConfig{
			Addr:              ":8080",
			ReadHeaderTimeout: 5 * time.Second,
			ReadTimeout:       15 * time.Second,
			WriteTimeout:      15 * time.Second,
			IdleTimeout:       60 * time.Second,
			RequestTimeout:    10 * time.Second,
			ShutdownTimeout:   10 * time.Second,
			ReadinessTimeout:  2 * time.Second,
			MaxHeaderBytes:    1 << 20,
			MaxBodyBytes:      1 << 20,
		},
		Database: DBConfig{
			Driver:          "mysql",
			Host:            "localhost",
			Port:            3306,
			User:            "root",
			Name:            "microservice",
			MaxOpenConns:    50,
			MaxIdleConns:    10,
			ConnMaxLifetime: 30 * time.Minute,
			ConnectTimeout:  5 * time.Second,
			TLSMode:         "false",
		},
		Telemetry: TelemetryConfig{
			Enabled:         true,
			Endpoint:        "localhost:4317",
			Insecure:        true,
			Environment:     "development",
			SampleRatio:     1.0,
			ExportInterval:  10 * time.Second,
			ExportTimeout:   5 * time.Second,
			ShutdownTimeout: 10 * time.Second,
		},
	}

	if err := yaml.Unmarshal(data, cfg); err != nil {
		return nil, err
	}
	if err := applyEnvOverrides(cfg); err != nil {
		return nil, err
	}
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	return cfg, nil
}

// Validate checks configuration invariants required for safe startup.
func (c *Config) Validate() error {
	var errs []error
	add := func(format string, args ...any) {
		errs = append(errs, fmt.Errorf(format, args...))
	}

	if strings.TrimSpace(c.ServiceName) == "" {
		add("service_name is required")
	}
	if strings.TrimSpace(c.Version) == "" {
		add("version is required")
	}
	switch c.LogLevel {
	case "debug", "info", "warn", "error":
	default:
		add("log_level must be one of debug, info, warn, error")
	}
	if c.LogFormat != "json" && c.LogFormat != "text" {
		add("log_format must be json or text")
	}

	if strings.TrimSpace(c.HTTP.Addr) == "" {
		add("http.addr is required")
	}
	if c.HTTP.ReadHeaderTimeout <= 0 {
		add("http.read_header_timeout must be positive")
	}
	if c.HTTP.ReadTimeout <= 0 {
		add("http.read_timeout must be positive")
	}
	if c.HTTP.WriteTimeout <= 0 {
		add("http.write_timeout must be positive")
	}
	if c.HTTP.IdleTimeout <= 0 {
		add("http.idle_timeout must be positive")
	}
	if c.HTTP.RequestTimeout <= 0 {
		add("http.request_timeout must be positive")
	}
	if c.HTTP.ShutdownTimeout <= 0 {
		add("http.shutdown_timeout must be positive")
	}
	if c.HTTP.ReadinessTimeout <= 0 {
		add("http.readiness_timeout must be positive")
	}
	if c.HTTP.MaxHeaderBytes <= 0 {
		add("http.max_header_bytes must be positive")
	}
	if c.HTTP.MaxBodyBytes <= 0 {
		add("http.max_body_bytes must be positive")
	}

	if c.Database.Driver != "mysql" {
		add("database.driver must be mysql")
	}
	if strings.TrimSpace(c.Database.Host) == "" {
		add("database.host is required")
	}
	if c.Database.Port < 1 || c.Database.Port > 65535 {
		add("database.port must be between 1 and 65535")
	}
	if strings.TrimSpace(c.Database.User) == "" {
		add("database.user is required")
	}
	if strings.TrimSpace(c.Database.Name) == "" {
		add("database.name is required")
	}
	if c.Database.MaxOpenConns <= 0 {
		add("database.max_open_conns must be positive")
	}
	if c.Database.MaxIdleConns < 0 || c.Database.MaxIdleConns > c.Database.MaxOpenConns {
		add("database.max_idle_conns must be between 0 and max_open_conns")
	}
	if c.Database.ConnMaxLifetime <= 0 {
		add("database.conn_max_lifetime must be positive")
	}
	if c.Database.ConnectTimeout <= 0 {
		add("database.connect_timeout must be positive")
	}
	if strings.TrimSpace(c.Database.TLSMode) == "" {
		add("database.tls_mode is required")
	}

	if c.Telemetry.Enabled && strings.TrimSpace(c.Telemetry.Endpoint) == "" {
		add("telemetry.endpoint is required when telemetry is enabled")
	}
	if c.Telemetry.SampleRatio < 0 || c.Telemetry.SampleRatio > 1 {
		add("telemetry.sample_ratio must be between 0 and 1")
	}
	if c.Telemetry.ExportInterval <= 0 {
		add("telemetry.export_interval must be positive")
	}
	if c.Telemetry.ExportTimeout <= 0 {
		add("telemetry.export_timeout must be positive")
	}
	if c.Telemetry.ShutdownTimeout <= 0 {
		add("telemetry.shutdown_timeout must be positive")
	}

	return errors.Join(errs...)
}

func applyEnvOverrides(cfg *Config) error {
	var errs []error

	envString("SERVICE_NAME", &cfg.ServiceName)
	envString("SERVICE_VERSION", &cfg.Version)
	envString("LOG_LEVEL", &cfg.LogLevel)
	envString("LOG_FORMAT", &cfg.LogFormat)

	envString("HTTP_ADDR", &cfg.HTTP.Addr)
	envString("HTTP_AUTH_TOKEN", &cfg.HTTP.AuthToken)
	errs = append(errs,
		envDuration("HTTP_READ_HEADER_TIMEOUT", &cfg.HTTP.ReadHeaderTimeout),
		envDuration("HTTP_READ_TIMEOUT", &cfg.HTTP.ReadTimeout),
		envDuration("HTTP_WRITE_TIMEOUT", &cfg.HTTP.WriteTimeout),
		envDuration("HTTP_IDLE_TIMEOUT", &cfg.HTTP.IdleTimeout),
		envDuration("HTTP_REQUEST_TIMEOUT", &cfg.HTTP.RequestTimeout),
		envDuration("HTTP_SHUTDOWN_TIMEOUT", &cfg.HTTP.ShutdownTimeout),
		envDuration("HTTP_READINESS_TIMEOUT", &cfg.HTTP.ReadinessTimeout),
		envInt("HTTP_MAX_HEADER_BYTES", &cfg.HTTP.MaxHeaderBytes),
		envInt64("HTTP_MAX_BODY_BYTES", &cfg.HTTP.MaxBodyBytes),
	)

	envString("DATABASE_HOST", &cfg.Database.Host)
	envString("DATABASE_USER", &cfg.Database.User)
	envString("DATABASE_PASSWORD", &cfg.Database.Password)
	envString("DATABASE_NAME", &cfg.Database.Name)
	envString("DATABASE_TLS_MODE", &cfg.Database.TLSMode)
	errs = append(errs,
		envInt("DATABASE_PORT", &cfg.Database.Port),
		envInt("DATABASE_MAX_OPEN_CONNS", &cfg.Database.MaxOpenConns),
		envInt("DATABASE_MAX_IDLE_CONNS", &cfg.Database.MaxIdleConns),
		envDuration("DATABASE_CONN_MAX_LIFETIME", &cfg.Database.ConnMaxLifetime),
		envDuration("DATABASE_CONNECT_TIMEOUT", &cfg.Database.ConnectTimeout),
	)

	envString("TELEMETRY_ENDPOINT", &cfg.Telemetry.Endpoint)
	envString("TELEMETRY_ENVIRONMENT", &cfg.Telemetry.Environment)
	errs = append(errs,
		envBool("TELEMETRY_ENABLED", &cfg.Telemetry.Enabled),
		envBool("TELEMETRY_INSECURE", &cfg.Telemetry.Insecure),
		envFloat64("TELEMETRY_SAMPLE_RATIO", &cfg.Telemetry.SampleRatio),
		envDuration("TELEMETRY_EXPORT_INTERVAL", &cfg.Telemetry.ExportInterval),
		envDuration("TELEMETRY_EXPORT_TIMEOUT", &cfg.Telemetry.ExportTimeout),
		envDuration("TELEMETRY_SHUTDOWN_TIMEOUT", &cfg.Telemetry.ShutdownTimeout),
	)

	return errors.Join(errs...)
}

func envString(key string, target *string) {
	if value, ok := os.LookupEnv(key); ok {
		*target = value
	}
}

func envInt(key string, target *int) error {
	value, ok := os.LookupEnv(key)
	if !ok {
		return nil
	}
	parsed, err := strconv.Atoi(value)
	if err != nil {
		return fmt.Errorf("%s must be an integer: %w", key, err)
	}
	*target = parsed
	return nil
}

func envInt64(key string, target *int64) error {
	value, ok := os.LookupEnv(key)
	if !ok {
		return nil
	}
	parsed, err := strconv.ParseInt(value, 10, 64)
	if err != nil {
		return fmt.Errorf("%s must be an integer: %w", key, err)
	}
	*target = parsed
	return nil
}

func envBool(key string, target *bool) error {
	value, ok := os.LookupEnv(key)
	if !ok {
		return nil
	}
	parsed, err := strconv.ParseBool(value)
	if err != nil {
		return fmt.Errorf("%s must be a boolean: %w", key, err)
	}
	*target = parsed
	return nil
}

func envFloat64(key string, target *float64) error {
	value, ok := os.LookupEnv(key)
	if !ok {
		return nil
	}
	parsed, err := strconv.ParseFloat(value, 64)
	if err != nil {
		return fmt.Errorf("%s must be a number: %w", key, err)
	}
	*target = parsed
	return nil
}

func envDuration(key string, target *time.Duration) error {
	value, ok := os.LookupEnv(key)
	if !ok {
		return nil
	}
	parsed, err := time.ParseDuration(value)
	if err != nil {
		return fmt.Errorf("%s must be a duration: %w", key, err)
	}
	*target = parsed
	return nil
}
