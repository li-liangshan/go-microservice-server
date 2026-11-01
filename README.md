# Go Microservice Server

A production-ready Go microservice Server demonstrating context propagation,
graceful shutdown, structured logging, database connection pooling, and
clean layered architecture.

## Architecture

```
cmd/server/main.go          ← bootstrap: config → logger → db → deps → server
internal/config/            ← YAML configuration with sensible defaults
internal/logger/            ← slog-based structured JSON logger
internal/database/          ← *sql.DB wrapper with pool tuning + ping + retry
internal/telemetry/         ← OpenTelemetry providers, OTLP exporters, shutdown
internal/store/             ← repository layer (interface + SQL implementation)
internal/service/           ← business logic, error translation
internal/server/            ← HTTP layer (chi router, middleware, handlers)
deploy/k8s/                 ← Kubernetes manifests (deployment, service, PDB)
```

## Quick Start

```bash
# 1. start the full stack: app, MySQL, OpenTelemetry Collector, Jaeger, Prometheus
cp .env.example .env      # replace HTTP_AUTH_TOKEN with a random value
docker compose up -d --build

# 2. verify the service
curl http://localhost:8080/healthz
curl http://localhost:8080/readyz
curl -H "Authorization: Bearer $HTTP_AUTH_TOKEN" http://localhost:8080/api/v1/users

# 3. inspect traces and metrics
open http://localhost:16686          # Jaeger
open http://localhost:9090           # Prometheus
```

To run against a local MySQL instance instead:

```bash
export DATABASE_PASSWORD=admin
make run
```

## Features

- **Context propagation** — every I/O call carries `context.Context`; cancellation reaches the DB driver.
- **Graceful shutdown** — `SIGINT`/`SIGTERM` triggers draining; HTTP server shuts down within timeout.
- **Structured logging** — JSON logs via `slog` with request-level middleware (method, path, status, latency, request id).
- **OpenTelemetry** — OTLP traces and metrics for HTTP, MySQL, and connection pool statistics.
- **Production health checks** — `/healthz` is liveness, `/readyz` verifies MySQL and drains on shutdown. Both are exempt from rate limiting.
- **HTTP hardening** — header/body limits, a request timeout that really answers with `504`, security headers, structured panic recovery, and optional Bearer token auth.
- **Per-client rate limiting** — token bucket keyed on the resolved client IP; health probes are never throttled.
- **Trusted-proxy client IP** — `X-Forwarded-For` is only read when the immediate peer is a listed proxy; otherwise it is ignored entirely.
- **Environment-aware configuration** — secrets and deployment values can be injected without rebuilding the image.
- **Fail-fast startup** — `APP_ENV=production` refuses to start without an auth token, a non-root database account, a database password and database TLS.
- **Database migrations** — a version ledger (`schema_migrations`) tracks what has been applied; `make migrate` applies migrations to existing databases.
- **Database pool tuning** — `SetMaxOpenConns`, `SetMaxIdleConns`, `SetConnMaxLifetime` and the connection retry budget are all configurable.
- **Error wrapping** — store → service → handler with `errors.Is` sentinel checks.
- **Chi middleware** — RequestID, structured panic recovery, request timeout, body size limit.
- **Multi-stage Dockerfile** — minimal Alpine runtime, non-root user, stripped binary, container health check.
- **Kubernetes manifests** — `deploy/k8s` ships a deployment with probes, resource limits, a read-only root filesystem, a PodDisruptionBudget and topology spreading.
- **Makefile** — `build`, `run`, `test`, `test-integration`, `coverage`, `fmt`, `vet`, `lint`, `tidy`, `migrate`, `seed`, `clean`.
- **CI workflow** — formatting, vet, lint, race tests with a MySQL service, coverage gate, vulnerability scan, image build, image scan and SBOM on every push/PR.

## Testing

```bash
make test              # unit tests with the race detector
make test-integration  # CRUD against a real MySQL (TEST_MYSQL_* variables)
make coverage          # total coverage
```

The integration test in `internal/store` is skipped unless `TEST_MYSQL_HOST` is
set. It applies `migrations/0001_init.sql` to the target database, so a drift
between the schema and the code fails the test.

## Configuration

All settings live in `configs/config.yaml`. Override the file with `--config`,
then use environment variables for deployment-specific or secret values.
Configuration is validated at startup and the service fails fast on unsafe
values.

Common production variables:

| Variable | Purpose |
|---|---|
| `APP_ENV` | `development` / `staging` / `production`; `production` enables the safety rules |
| `SERVICE_NAME`, `SERVICE_VERSION` | OpenTelemetry resource identity |
| `LOG_LEVEL`, `LOG_FORMAT` | `debug`/`info`/`warn`/`error`, `json`/`text` |
| `HTTP_ADDR`, `HTTP_AUTH_TOKEN` | Listen address and Bearer token for `/api/v1` |
| `HTTP_TRUSTED_PROXIES` | Comma-separated IPs/CIDRs whose forwarding headers are trusted |
| `HTTP_RATE_LIMIT_ENABLED`, `HTTP_RATE_LIMIT_RPS`, `HTTP_RATE_LIMIT_BURST`, `HTTP_RATE_LIMIT_MAX_CLIENTS` | Per-client request allowance |
| `DATABASE_HOST`, `DATABASE_PORT`, `DATABASE_USER`, `DATABASE_PASSWORD`, `DATABASE_NAME`, `DATABASE_TLS_MODE` | MySQL connection and TLS mode (`false`/`true`/`skip-verify`) |
| `DATABASE_CONNECT_RETRIES`, `DATABASE_CONNECT_RETRY_INTERVAL` | Retry budget for the initial connection |
| `TELEMETRY_ENABLED`, `TELEMETRY_ENDPOINT`, `TELEMETRY_INSECURE`, `TELEMETRY_ENVIRONMENT`, `TELEMETRY_SAMPLE_RATIO` | OTLP export |

When `HTTP_AUTH_TOKEN` is set, all `/api/v1` routes require
`Authorization: Bearer <token>`. Health endpoints remain unauthenticated.

### Production checklist

`APP_ENV=production` makes the service refuse to start unless:

- `HTTP_AUTH_TOKEN` is set;
- the database account is not `root`;
- the database password is at least 8 characters;
- `DATABASE_TLS_MODE` is not `false`.

## Migrations

`docker compose up -d --build` mounts `migrations/` into MySQL's
`docker-entrypoint-initdb.d`, so a fresh volume is initialized automatically.
For an existing database, apply the migrations explicitly — the script takes
the database name from `DATABASE_NAME`, creates it when missing and records
each applied file in `schema_migrations`:

```bash
DATABASE_HOST=127.0.0.1 \
DATABASE_USER=root \
DATABASE_PASSWORD=admin \
DATABASE_NAME=microservice \
make migrate
```

For production, create a dedicated least-privilege database user and run
migrations as a separate deployment step instead of using the MySQL root
account.

## Production Notes

- Terminate TLS at an ingress or load balancer; the application serves HTTP internally.
- Set a strong `HTTP_AUTH_TOKEN` and database password through your secret manager, never commit `.env`.
- Use `/healthz` for liveness probes and `/readyz` for readiness probes. Neither is rate limited.
- Set `TELEMETRY_ENVIRONMENT=production` and point `TELEMETRY_ENDPOINT` at your OTLP Collector.
- The Collector exports metrics in Prometheus format on `:8889`; point your Prometheus at it.
- Rate limiting is keyed on the client IP. Clients behind a shared NAT or ingress share one allowance; per-client limiting requires per-client credentials.
- The Compose file is a local integration stack; production should manage MySQL, secrets, replicas, and migrations separately. See `deploy/k8s` for a starting point.

## OpenTelemetry

Telemetry is configured under `telemetry` and exports OTLP/gRPC to an
OpenTelemetry Collector by default:

`docker compose up -d` starts the Collector on `localhost:4317`, Jaeger at
http://localhost:16686 and Prometheus at http://localhost:9090. The Collector
forwards traces to Jaeger and exposes the metrics in Prometheus format.

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
