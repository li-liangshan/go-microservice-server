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
# 1. start the full stack: app, MySQL, OpenTelemetry Collector and Jaeger
cp .env.example .env
docker compose up -d --build

# 2. verify the service
curl http://localhost:8080/healthz
curl http://localhost:8080/readyz
curl http://localhost:8080/api/v1/users

# 3. inspect traces
open http://localhost:16686
```

To run against a local MySQL instance instead:

```bash
export DATABASE_PASSWORD=admin
make run
```

## Features

- **Context propagation** — every I/O call carries `context.Context`; cancellation reaches the DB driver.
- **Graceful shutdown** — `SIGINT`/`SIGTERM` triggers draining; HTTP server shuts down within timeout.
- **Structured logging** — JSON logs via `slog` with request-level middleware (method, path, status, latency).
- **OpenTelemetry** — OTLP traces and metrics for HTTP, MySQL, and connection pool statistics.
- **Production health checks** — `/healthz` is liveness, `/readyz` verifies MySQL and drains on shutdown.
- **HTTP hardening** — header/body limits, request timeouts, security headers, structured panic recovery, and optional Bearer token auth.
- **Environment-aware configuration** — secrets and deployment values can be injected without rebuilding the image.
- **Database migrations** — Compose initializes a fresh MySQL volume; `make migrate` applies migrations to existing databases.
- **Database pool tuning** — `SetMaxOpenConns`, `SetMaxIdleConns`, `SetConnMaxLifetime` all configurable.
- **Error wrapping** — store → service → handler with `errors.Is` sentinel checks.
- **Chi middleware** — RequestID, RealIP, Recoverer (no crashes), timeout.
- **Multi-stage Dockerfile** — minimal Alpine runtime, non-root user, stripped binary, container health check.
- **Makefile** — `build`, `run`, `test`, `fmt`, `vet`, `tidy`, `migrate`, `clean`.
- **CI workflow** — race tests, vet, binary build, Compose validation, and Docker build on every push/PR.

## Testing

```bash
make test   # runs go test -v -race -cover
```

## Configuration

All settings live in `configs/config.yaml`. Override the file with `--config`,
then use environment variables for deployment-specific or secret values.
Configuration is validated at startup and the service fails fast on unsafe
values.

Common production variables:

| Variable | Purpose |
|---|---|
| `SERVICE_NAME`, `SERVICE_VERSION` | OpenTelemetry resource identity |
| `LOG_LEVEL`, `LOG_FORMAT` | `debug`/`info`/`warn`/`error`, `json`/`text` |
| `HTTP_ADDR`, `HTTP_AUTH_TOKEN` | Listen address and optional Bearer token |
| `DATABASE_HOST`, `DATABASE_PORT`, `DATABASE_USER`, `DATABASE_PASSWORD`, `DATABASE_NAME`, `DATABASE_TLS_MODE` | MySQL connection and TLS mode (`false`/`true`/`skip-verify`) |
| `TELEMETRY_ENABLED`, `TELEMETRY_ENDPOINT`, `TELEMETRY_INSECURE`, `TELEMETRY_ENVIRONMENT`, `TELEMETRY_SAMPLE_RATIO` | OTLP export |

When `HTTP_AUTH_TOKEN` is set, all `/api/v1` routes require
`Authorization: Bearer <token>`. Health endpoints remain unauthenticated.

## Migrations

`docker compose up -d --build` mounts `migrations/` into MySQL's
`docker-entrypoint-initdb.d`, so a fresh volume is initialized automatically.
For an existing database, use the configured connection:

```bash
DATABASE_HOST=127.0.0.1 \
DATABASE_USER=root \
DATABASE_PASSWORD=admin \
make migrate
```

For production, create a dedicated least-privilege database user and run
migrations as a separate deployment step instead of using the MySQL root
account.

## Production Notes

- Terminate TLS at an ingress or load balancer; the application serves HTTP internally.
- Set a strong `HTTP_AUTH_TOKEN` and `MYSQL_ROOT_PASSWORD` through your secret manager, never commit `.env`.
- Use `/healthz` for liveness probes and `/readyz` for readiness probes.
- Set `TELEMETRY_ENVIRONMENT=production` and point `TELEMETRY_ENDPOINT` at your OTLP Collector.
- The Compose file is a local integration stack; production should manage MySQL, secrets, replicas, and migrations separately.

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
