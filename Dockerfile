# ── Build stage ──────────────────────────────────────────────────────────
FROM golang:1.22-alpine AS builder

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /out/server ./cmd/server

# ── Runtime stage ─────────────────────────────────────────────────────────
FROM alpine:3.20

RUN apk --no-cache add ca-certificates tzdata && \
    addgroup -S app && adduser -S app -G app

WORKDIR /app
COPY --from=builder /out/server /app/server
COPY --from=builder /src/configs /app/configs

USER app
EXPOSE 8080

ENTRYPOINT ["/app/server", "--config", "/app/configs/config.yaml"]
