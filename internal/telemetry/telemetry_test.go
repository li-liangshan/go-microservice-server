package telemetry

import (
	"context"
	"io"
	"log/slog"
	"testing"

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
