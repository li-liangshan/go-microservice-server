package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"microservice-server/internal/config"
	"microservice-server/internal/database"
	"microservice-server/internal/logger"
	"microservice-server/internal/server"
	"microservice-server/internal/service"
	"microservice-server/internal/store"
	"microservice-server/internal/telemetry"
)

// @title Go Microservice Template
// @version 1.0.0
// @description Production-ready Go microservice template with graceful shutdown,
//              context propagation, structured logging and database integration.
// @BasePath /api/v1

func main() {
	if err := run(os.Args[1:]); err != nil {
		slog.Error("service failed", "err", err)
		os.Exit(1)
	}
}

// run wires up and serves the application. It takes the command line
// arguments explicitly so that it can be exercised from tests.
func run(args []string) error {
	flags := flag.NewFlagSet("server", flag.ContinueOnError)
	cfgPath := flags.String("config", "configs/config.yaml", "path to config file")
	if err := flags.Parse(args); err != nil {
		return fmt.Errorf("parse flags: %w", err)
	}

	// ── 1. Load configuration ──────────────────────────────────────────────
	cfg, err := config.Load(*cfgPath)
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	// ── 2. Initialize structured logger ───────────────────────────────────
	log := logger.New(cfg.LogLevel, cfg.LogFormat)
	slog.SetDefault(log)
	log.Info("service starting",
		"name", cfg.ServiceName,
		"version", cfg.Version,
		"environment", cfg.Environment,
	)

	// ── 3. Initialize OpenTelemetry ────────────────────────────────────────
	telemetryProvider, err := telemetry.New(context.Background(), cfg, log)
	if err != nil {
		return fmt.Errorf("initialize telemetry: %w", err)
	}
	defer func() {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), cfg.Telemetry.ShutdownTimeout)
		defer cancel()
		if err := telemetryProvider.Shutdown(shutdownCtx); err != nil {
			log.Error("failed to shutdown telemetry", "err", err)
		}
	}()
	if cfg.Telemetry.Enabled {
		log.Info("telemetry enabled", "endpoint", cfg.Telemetry.Endpoint)
	} else {
		log.Info("telemetry disabled")
	}

	// ── 4. Connect to database ─────────────────────────────────────────────
	db, err := database.New(context.Background(), cfg.Database)
	if err != nil {
		return fmt.Errorf("connect database: %w", err)
	}
	defer func() {
		if err := db.Close(); err != nil {
			log.Error("failed to close database", "err", err)
		}
	}()
	log.Info("database connected", "host", cfg.Database.Host, "port", cfg.Database.Port)

	// ── 5. Build dependency graph ─────────────────────────────────────────
	userStore := store.NewUserStore(db)
	userService := service.NewUserService(userStore, log)
	httpServer, err := server.New(cfg.HTTP, userService, db, log)
	if err != nil {
		return fmt.Errorf("build http server: %w", err)
	}
	if cfg.HTTP.AuthToken == "" {
		log.Warn("api authentication is disabled: /api/v1 is open to any caller")
	} else {
		log.Info("api authentication enabled")
	}

	// ── 6. Graceful startup & shutdown ────────────────────────────────────
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	serverErr := make(chan error, 1)
	go func() {
		log.Info("http server listening", "addr", cfg.HTTP.Addr)
		serverErr <- httpServer.ListenAndServe()
	}()

	select {
	case <-ctx.Done():
		log.Info("shutdown signal received, draining...")
	case err := <-serverErr:
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			return fmt.Errorf("http server: %w", err)
		}
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), cfg.HTTP.ShutdownTimeout)
	defer cancel()
	if err := httpServer.Shutdown(shutdownCtx); err != nil {
		_ = httpServer.Close()
		return fmt.Errorf("shutdown http server: %w", err)
	}
	log.Info("service stopped")
	return nil
}
