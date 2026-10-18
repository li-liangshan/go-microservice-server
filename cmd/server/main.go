package main

import (
	"context"
	"flag"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

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
	cfgPath := flag.String("config", "configs/config.yaml", "path to config file")
	flag.Parse()

	// ── 1. Load configuration ──────────────────────────────────────────────
	cfg, err := config.Load(*cfgPath)
	if err != nil {
		slog.Error("failed to load config", "err", err)
		os.Exit(1)
	}

	// ── 2. Initialize structured logger ───────────────────────────────────
	log := logger.New(cfg.LogLevel, cfg.LogFormat)
	slog.SetDefault(log)
	log.Info("service starting", "name", cfg.ServiceName, "version", cfg.Version)

	// ── 3. Initialize OpenTelemetry ────────────────────────────────────────
	telemetryProvider, err := telemetry.New(context.Background(), cfg, log)
	if err != nil {
		log.Error("failed to initialize telemetry", "err", err)
		os.Exit(1)
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
	db, err := database.New(cfg.Database)
	if err != nil {
		log.Error("failed to connect database", "err", err)
		os.Exit(1)
	}
	defer db.Close()
	log.Info("database connected", "host", cfg.Database.Host, "port", cfg.Database.Port)

	// ── 5. Build dependency graph ─────────────────────────────────────────
	userStore := store.NewUserStore(db)
	userService := service.NewUserService(userStore, log)
	httpServer := server.New(cfg.HTTP, userService, log)

	// ── 6. Graceful startup & shutdown ────────────────────────────────────
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	go func() {
		log.Info("http server listening", "addr", cfg.HTTP.Addr)
		if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Error("http server error", "err", err)
			stop()
		}
	}()

	<-ctx.Done()
	log.Info("shutdown signal received, draining...")

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := httpServer.Shutdown(shutdownCtx); err != nil {
		log.Error("forced shutdown", "err", err)
	}
	log.Info("service stopped")
}
