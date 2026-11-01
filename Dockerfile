# ── Build stage ──────────────────────────────────────────────────────────
# The builder image must ship a Go toolchain at least as new as the `go`
# directive in go.mod: the official images pin GOTOOLCHAIN=local, so an older
# toolchain fails with "go.mod requires go >= ..." instead of downloading one.
FROM golang:1.26-alpine AS builder

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build \
    -trimpath \
    -buildvcs=false \
    -ldflags="-s -w" \
    -o /out/server ./cmd/server

# ── Runtime stage ─────────────────────────────────────────────────────────
FROM alpine:3.22

RUN apk --no-cache add ca-certificates tzdata && \
    addgroup -S app && adduser -S app -G app

WORKDIR /app
COPY --from=builder /out/server /app/server
COPY --from=builder /src/configs /app/configs

USER app
EXPOSE 8080
STOPSIGNAL SIGTERM

HEALTHCHECK --interval=30s --timeout=3s --start-period=10s --retries=3 \
    CMD wget -qO- http://127.0.0.1:8080/healthz >/dev/null || exit 1

ENTRYPOINT ["/app/server", "--config", "/app/configs/config.yaml"]
