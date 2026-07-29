// Copyright 2025 Guidewire Software, Inc.
// SPDX-License-Identifier: Apache-2.0

package telemetry

import (
	"context"
	"os"
	"time"

	"github.com/rs/zerolog/log"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetricgrpc"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/resource"
	"go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.24.0"
)

const (
	serviceName    = "teams360-api"
	serviceVersion = "1.0.0"
)

// Config holds telemetry configuration
type Config struct {
	Enabled         bool
	OTLPEndpoint    string
	SampleRate      float64
	Environment     string
	MetricsInterval time.Duration
}

// DefaultConfig returns configuration from environment variables
func DefaultConfig() Config {
	// Disabled by default - must explicitly enable with OTEL_ENABLED=true
	enabled := os.Getenv("OTEL_ENABLED") == "true"
	endpoint := os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT")
	if endpoint == "" {
		endpoint = "localhost:4317" // Default OTel collector endpoint
	}

	env := os.Getenv("ENVIRONMENT")
	if env == "" {
		env = "development"
	}

	return Config{
		Enabled:         enabled,
		OTLPEndpoint:    endpoint,
		SampleRate:      1.0, // 100% sampling in dev, adjust for production
		Environment:     env,
		MetricsInterval: 15 * time.Second,
	}
}

// Telemetry holds the initialized telemetry providers
type Telemetry struct {
	tracerProvider *trace.TracerProvider
	meterProvider  *metric.MeterProvider
	config         Config
}

var globalTelemetry *Telemetry

// Init initializes OpenTelemetry with the given configuration
// Returns a shutdown function that should be called on application exit
func Init(ctx context.Context, cfg Config) (shutdown func(context.Context) error, err error) {
	if !cfg.Enabled {
		log.Info().Msg("Telemetry disabled via configuration")
		return func(context.Context) error { return nil }, nil
	}

	// Create resource describing this service
	// Note: We create a new resource directly instead of merging with Default()
	// to avoid schema URL version conflicts between different OTel SDK versions
	res, err := resource.New(ctx,
		resource.WithAttributes(
			semconv.ServiceName(serviceName),
			semconv.ServiceVersion(serviceVersion),
			semconv.DeploymentEnvironment(cfg.Environment),
		),
		resource.WithProcessRuntimeDescription(),
		resource.WithHost(),
	)
	if err != nil {
		return nil, err
	}

	// Initialize trace provider
	tracerProvider, err := initTracerProvider(ctx, cfg, res)
	if err != nil {
		return nil, err
	}

	// Initialize meter provider
	meterProvider, err := initMeterProvider(ctx, cfg, res)
	if err != nil {
		tracerProvider.Shutdown(ctx)
		return nil, err
	}

	// Set global providers
	otel.SetTracerProvider(tracerProvider)
	otel.SetMeterProvider(meterProvider)
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	))

	globalTelemetry = &Telemetry{
		tracerProvider: tracerProvider,
		meterProvider:  meterProvider,
		config:         cfg,
	}

	// Initialize business metrics
	if err := initBusinessMetrics(); err != nil {
		log.Warn().Err(err).Msg("Failed to initialize some business metrics")
	}

	log.Info().
		Str("endpoint", cfg.OTLPEndpoint).
		Str("environment", cfg.Environment).
		Float64("sample_rate", cfg.SampleRate).
		Msg("OpenTelemetry initialized successfully")

	// Return shutdown function
	shutdown = func(ctx context.Context) error {
		var errs []error
		if err := tracerProvider.Shutdown(ctx); err != nil {
			errs = append(errs, err)
		}
		if err := meterProvider.Shutdown(ctx); err != nil {
			errs = append(errs, err)
		}
		if len(errs) > 0 {
			return errs[0]
		}
		return nil
	}

	return shutdown, nil
}

// initTracerProvider creates and configures the trace provider
// Uses non-blocking connection to avoid startup failures when collector is unavailable
func initTracerProvider(ctx context.Context, cfg Config, res *resource.Resource) (*trace.TracerProvider, error) {
	// Create OTLP trace exporter with built-in connection management
	// The exporter handles connection retries and non-blocking behavior internally
	exporter, err := otlptracegrpc.New(ctx,
		otlptracegrpc.WithEndpoint(cfg.OTLPEndpoint),
		otlptracegrpc.WithInsecure(), // Use insecure connection for local dev
		otlptracegrpc.WithRetry(otlptracegrpc.RetryConfig{
			Enabled:         true,
			InitialInterval: 1 * time.Second,
			MaxInterval:     30 * time.Second,
			MaxElapsedTime:  5 * time.Minute,
		}),
	)
	if err != nil {
		log.Warn().Err(err).Str("endpoint", cfg.OTLPEndpoint).
			Msg("Failed to create trace exporter, continuing without tracing")
		// Return a no-op tracer provider
		return trace.NewTracerProvider(trace.WithResource(res)), nil
	}

	// Create sampler based on config
	var sampler trace.Sampler
	if cfg.SampleRate >= 1.0 {
		sampler = trace.AlwaysSample()
	} else if cfg.SampleRate <= 0 {
		sampler = trace.NeverSample()
	} else {
		sampler = trace.ParentBased(trace.TraceIDRatioBased(cfg.SampleRate))
	}

	// Create trace provider
	tp := trace.NewTracerProvider(
		trace.WithBatcher(exporter),
		trace.WithResource(res),
		trace.WithSampler(sampler),
	)

	return tp, nil
}

// initMeterProvider creates and configures the meter provider
// Uses non-blocking connection to avoid startup failures when collector is unavailable
func initMeterProvider(ctx context.Context, cfg Config, res *resource.Resource) (*metric.MeterProvider, error) {
	// Create OTLP metric exporter with built-in connection management
	// The exporter handles connection retries and non-blocking behavior internally
	exporter, err := otlpmetricgrpc.New(ctx,
		otlpmetricgrpc.WithEndpoint(cfg.OTLPEndpoint),
		otlpmetricgrpc.WithInsecure(), // Use insecure connection for local dev
		otlpmetricgrpc.WithRetry(otlpmetricgrpc.RetryConfig{
			Enabled:         true,
			InitialInterval: 1 * time.Second,
			MaxInterval:     30 * time.Second,
			MaxElapsedTime:  5 * time.Minute,
		}),
	)
	if err != nil {
		log.Warn().Err(err).Str("endpoint", cfg.OTLPEndpoint).
			Msg("Failed to create metric exporter, continuing without metrics export")
		// Return a no-op meter provider
		return metric.NewMeterProvider(metric.WithResource(res)), nil
	}

	// Create meter provider with periodic reader
	mp := metric.NewMeterProvider(
		metric.WithResource(res),
		metric.WithReader(metric.NewPeriodicReader(exporter,
			metric.WithInterval(cfg.MetricsInterval),
		)),
	)

	return mp, nil
}

// GetTracerProvider returns the global tracer provider
func GetTracerProvider() *trace.TracerProvider {
	if globalTelemetry == nil {
		return nil
	}
	return globalTelemetry.tracerProvider
}

// GetMeterProvider returns the global meter provider
func GetMeterProvider() *metric.MeterProvider {
	if globalTelemetry == nil {
		return nil
	}
	return globalTelemetry.meterProvider
}
