package config

import (
	"os"
	"time"

	"gopkg.in/yaml.v3"
)

// Config is the top-level service configuration.
type Config struct {
	ServiceName string     `yaml:"service_name"`
	Version     string     `yaml:"version"`
	LogLevel    string     `yaml:"log_level"`
	LogFormat   string     `yaml:"log_format"`
	HTTP        HTTPConfig `yaml:"http"`
	Database    DBConfig   `yaml:"database"`
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
	}

	if err := yaml.Unmarshal(data, cfg); err != nil {
		return nil, err
	}
	return cfg, nil
}
