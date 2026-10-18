package telemetry

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"microservice-server/internal/config"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetricgrpc"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/propagation"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"
)

// Provider owns the OpenTelemetry trace and metric providers.
type Provider struct {
	tracerProvider *sdktrace.TracerProvider
	meterProvider  *sdkmetric.MeterProvider
}

// New configures OpenTelemetry exporters, providers and global propagation.
// When telemetry is disabled it returns a no-op provider.
func New(ctx context.Context, cfg *config.Config, log *slog.Logger) (*Provider, error) {
	if !cfg.Telemetry.Enabled {
		return &Provider{}, nil
	}

	res, err := resource.New(ctx,
		resource.WithFromEnv(),
		resource.WithTelemetrySDK(),
		resource.WithProcess(),
		resource.WithHost(),
		resource.WithAttributes(
			semconv.ServiceName(cfg.ServiceName),
			semconv.ServiceVersion(cfg.Version),
			semconv.DeploymentEnvironment(cfg.Telemetry.Environment),
		),
	)
	if err != nil {
		return nil, fmt.Errorf("create telemetry resource: %w", err)
	}

	traceOptions := []otlptracegrpc.Option{
		otlptracegrpc.WithEndpoint(cfg.Telemetry.Endpoint),
		otlptracegrpc.WithHeaders(cfg.Telemetry.Headers),
		otlptracegrpc.WithTimeout(cfg.Telemetry.ExportTimeout),
	}
	if cfg.Telemetry.Insecure {
		traceOptions = append(traceOptions, otlptracegrpc.WithInsecure())
	}
	traceExporter, err := otlptracegrpc.New(ctx, traceOptions...)
	if err != nil {
		return nil, fmt.Errorf("create trace exporter: %w", err)
	}

	metricOptions := []otlpmetricgrpc.Option{
		otlpmetricgrpc.WithEndpoint(cfg.Telemetry.Endpoint),
		otlpmetricgrpc.WithHeaders(cfg.Telemetry.Headers),
		otlpmetricgrpc.WithTimeout(cfg.Telemetry.ExportTimeout),
	}
	if cfg.Telemetry.Insecure {
		metricOptions = append(metricOptions, otlpmetricgrpc.WithInsecure())
	}
	metricExporter, err := otlpmetricgrpc.New(ctx, metricOptions...)
	if err != nil {
		_ = traceExporter.Shutdown(ctx)
		return nil, fmt.Errorf("create metric exporter: %w", err)
	}

	tracerProvider := sdktrace.NewTracerProvider(
		sdktrace.WithResource(res),
		sdktrace.WithSampler(newSampler(cfg.Telemetry.SampleRatio)),
		sdktrace.WithBatcher(traceExporter,
			sdktrace.WithBatchTimeout(cfg.Telemetry.ExportInterval),
			sdktrace.WithExportTimeout(cfg.Telemetry.ExportTimeout),
		),
	)
	meterProvider := sdkmetric.NewMeterProvider(
		sdkmetric.WithResource(res),
		sdkmetric.WithReader(sdkmetric.NewPeriodicReader(metricExporter,
			sdkmetric.WithInterval(cfg.Telemetry.ExportInterval),
			sdkmetric.WithTimeout(cfg.Telemetry.ExportTimeout),
		)),
	)

	otel.SetTracerProvider(tracerProvider)
	otel.SetMeterProvider(meterProvider)
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	))
	otel.SetErrorHandler(otel.ErrorHandlerFunc(func(err error) {
		log.Warn("opentelemetry error", "err", err)
	}))

	return &Provider{
		tracerProvider: tracerProvider,
		meterProvider:  meterProvider,
	}, nil
}

// Shutdown flushes pending telemetry and releases exporter resources.
func (p *Provider) Shutdown(ctx context.Context) error {
	var errs []error
	if p.tracerProvider != nil {
		errs = append(errs, p.tracerProvider.Shutdown(ctx))
	}
	if p.meterProvider != nil {
		errs = append(errs, p.meterProvider.Shutdown(ctx))
	}
	return errors.Join(errs...)
}

func newSampler(ratio float64) sdktrace.Sampler {
	switch {
	case ratio >= 1:
		return sdktrace.ParentBased(sdktrace.AlwaysSample())
	case ratio <= 0:
		return sdktrace.ParentBased(sdktrace.NeverSample())
	default:
		return sdktrace.ParentBased(sdktrace.TraceIDRatioBased(ratio))
	}
}
