# Go Microservice Server

A production-ready Go microservice Server demonstrating context propagation,
graceful shutdown, structured logging, database connection pooling, and
clean layered architecture.

## Architecture

```
cmd/server/main.go          ← bootstrap: config → logger → db → deps → server
internal/config/            ← YAML configuration with sensible defaults
internal/logger/            ← slog-based structured JSON logger
internal/database/          ← *sql.DB wrapper with pool tuning + ping
internal/telemetry/         ← OpenTelemetry providers, OTLP exporters, shutdown
internal/store/             ← repository layer (interface + SQL implementation)
internal/service/           ← business logic, error translation
internal/server/            ← HTTP layer (chi router, middleware, handlers)
```

## Quick Start

```bash
# 1. start mysql, OpenTelemetry Collector and Jaeger
docker compose up -d

# 2. build & run
make build
make run

# 3. test
curl http://localhost:8080/healthz
curl http://localhost:8080/api/v1/users/1

# 4. inspect traces
open http://localhost:16686
```

## Features

- **Context propagation** — every I/O call carries `context.Context`; cancellation reaches the DB driver.
- **Graceful shutdown** — `SIGINT`/`SIGTERM` triggers draining; HTTP server shuts down within timeout.
- **Structured logging** — JSON logs via `slog` with request-level middleware (method, path, status, latency).
- **OpenTelemetry** — OTLP traces and metrics for HTTP, MySQL, and connection pool statistics.
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

## OpenTelemetry

Telemetry is configured under `telemetry` and exports OTLP/gRPC to an
OpenTelemetry Collector by default:

`docker compose up -d` starts the Collector on `localhost:4317` and Jaeger at
http://localhost:16686. The Collector forwards traces to Jaeger and logs
metrics through its `debug` exporter.

```yaml
telemetry:
  enabled: true
  endpoint: localhost:4317
  insecure: true
  environment: development
  sample_ratio: 1.0
  export_interval: 10s
  export_timeout: 5s
  shutdown_timeout: 10s
  headers: {}
```

Instrumentation includes:

- HTTP server spans with method, route, status code, and latency; incoming W3C `traceparent`/`baggage` headers are propagated.
- MySQL spans for queries and executions, plus `db.sql.*` metrics and connection pool gauges.
- HTTP server metrics emitted by `otelhttp`.
- `trace_id` and `span_id` fields in context-aware `slog` records.
- Graceful exporter flush during service shutdown.

Set `enabled: false` to run without an OTLP endpoint. `headers` can carry
collector authentication metadata. Resource attributes can also be supplied
through the standard `OTEL_RESOURCE_ATTRIBUTES` environment variable.
