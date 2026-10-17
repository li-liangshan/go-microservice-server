# Go Microservice Template

A production-ready Go microservice template demonstrating context propagation,
graceful shutdown, structured logging, database connection pooling, and
clean layered architecture.

## Architecture

```
cmd/server/main.go          ← bootstrap: config → logger → db → deps → server
internal/config/            ← YAML configuration with sensible defaults
internal/logger/            ← slog-based structured JSON logger
internal/database/          ← *sql.DB wrapper with pool tuning + ping
internal/store/             ← repository layer (interface + SQL implementation)
internal/service/           ← business logic, error translation
internal/server/            ← HTTP layer (chi router, middleware, handlers)
```

## Quick Start

```bash
# 1. start postgres
docker compose up -d postgres

# 2. build & run
make build
make run

# 3. test
curl http://localhost:8080/healthz
curl http://localhost:8080/api/v1/users/1
```

## Features

- **Context propagation** — every I/O call carries `context.Context`; cancellation reaches the DB driver.
- **Graceful shutdown** — `SIGINT`/`SIGTERM` triggers draining; HTTP server shuts down within timeout.
- **Structured logging** — JSON logs via `slog` with request-level middleware (method, path, status, latency).
- **Database pool tuning** — `SetMaxOpenConns`, `SetMaxIdleConns`, `SetConnMaxLifetime` all configurable.
- **Error wrapping** — store → service → handler with `errors.Is` sentinel checks.
- **Chi middleware** — RequestID, RealIP, Recoverer (no crashes), timeout.
- **Multi-stage Dockerfile** — minimal Alpine runtime, non-root user, stripped binary.
- **Makefile** — `build`, `run`, `test`, `fmt`, `vet`, `tidy`, `clean`.

## Testing

```bash
make test   # runs go test -v -race -cover
```

## Configuration

All settings live in `configs/config.yaml`. Override with the `--config` flag.
