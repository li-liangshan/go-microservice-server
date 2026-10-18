package config

import (
	"fmt"
	"os"
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
	Addr            string        `yaml:"addr"`
	ReadTimeout     time.Duration `yaml:"read_timeout"`
	WriteTimeout    time.Duration `yaml:"write_timeout"`
	IdleTimeout     time.Duration `yaml:"idle_timeout"`
	ShutdownTimeout time.Duration `yaml:"shutdown_timeout"`
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
			Addr:            ":8080",
			ReadTimeout:     15 * time.Second,
			WriteTimeout:    15 * time.Second,
			IdleTimeout:     60 * time.Second,
			ShutdownTimeout: 10 * time.Second,
		},
		Database: DBConfig{
			Driver:          "mysql",
			Host:            "localhost",
			Port:            3306,
			MaxOpenConns:    50,
			MaxIdleConns:    10,
			ConnMaxLifetime: 30 * time.Minute,
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
	if cfg.Telemetry.Enabled && cfg.Telemetry.Endpoint == "" {
		return nil, fmt.Errorf("telemetry.endpoint is required when telemetry is enabled")
	}
	if cfg.Telemetry.SampleRatio < 0 || cfg.Telemetry.SampleRatio > 1 {
		return nil, fmt.Errorf("telemetry.sample_ratio must be between 0 and 1")
	}
	return cfg, nil
}
