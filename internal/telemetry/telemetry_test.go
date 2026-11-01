package telemetry

import (
	"context"
	"io"
	"log/slog"
	"testing"
	"time"

	"microservice-server/internal/config"
)

func TestNew_DisabledReturnsNoopProvider(t *testing.T) {
	cfg := &config.Config{Telemetry: config.TelemetryConfig{Enabled: false}}
	log := slog.New(slog.NewTextHandler(io.Discard, nil))

	provider, err := New(context.Background(), cfg, log)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if provider == nil {
		t.Fatal("New returned nil provider")
	}
	if provider.tracerProvider != nil || provider.meterProvider != nil {
		t.Fatalf("disabled telemetry should not create providers: %+v", provider)
	}
	if err := provider.Shutdown(context.Background()); err != nil {
		t.Fatalf("Shutdown: %v", err)
	}
}

func TestNewSampler(t *testing.T) {
	for _, ratio := range []float64{-1, 0, 0.25, 1, 2} {
		if sampler := newSampler(ratio); sampler == nil {
			t.Fatalf("newSampler(%v) returned nil", ratio)
		}
	}
}

// Telemetry must start even when the collector is unreachable: the OTLP
// exporters connect lazily and retry in the background.
func TestNew_EnabledWithUnreachableCollector(t *testing.T) {
	cfg := &config.Config{
		ServiceName: "svc",
		Version:     "1",
		Telemetry: config.TelemetryConfig{
			Enabled:         true,
			Endpoint:        "127.0.0.1:1",
			Insecure:        true,
			Environment:     "test",
			SampleRatio:     1,
			ExportInterval:  time.Hour,
			ExportTimeout:   200 * time.Millisecond,
			ShutdownTimeout: time.Second,
		},
	}
	log := slog.New(slog.NewTextHandler(io.Discard, nil))

	provider, err := New(context.Background(), cfg, log)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if provider.tracerProvider == nil || provider.meterProvider == nil {
		t.Fatalf("enabled telemetry should create providers: %+v", provider)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	// Flushing to an unreachable collector may fail; the important part is
	// that it terminates.
	_ = provider.Shutdown(ctx)
}
