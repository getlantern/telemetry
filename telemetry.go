package telemetry

import (
	"context"
	"os"
	"strconv"

	"github.com/getlantern/golog"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/propagation"

	sdktrace "go.opentelemetry.io/otel/sdk/trace"
)

var log = golog.LoggerFor("telemetry")

// EnableOTELTracing enables OTEL tracing for HTTP requests with the OTEL provider configured via
// environment variables. This allows us to switch providers purely by changing the environment variables.

// Sample rates should be configured via environment variables. See
// https://opentelemetry-python.readthedocs.io/en/latest/sdk/trace.sampling.html
// For example:
// OTEL_TRACES_SAMPLER=traceidratio OTEL_TRACES_SAMPLER_ARG=0.001
func EnableOTELTracing(ctx context.Context) func(context.Context) error {
	log.Debug("Enabling OTEL tracing")
	err := sampleRate()
	if err != nil {
		return func(ctx context.Context) error { return nil }
	}
	exp, err := otlptrace.New(ctx, otlptracehttp.NewClient())
	if err != nil {
		log.Errorf("telemetry failed to initialize exporter: %w", err)
		return func(ctx context.Context) error { return nil }
	}
	envSampler, err := samplerFromEnv()
	if err != nil {
		log.Errorf("telemetry failed to initialize sampler: %w", err)
		return func(ctx context.Context) error { return nil }
	}

	// Create a new tracer provider with a batch span processor and the otlp exporter.
	tp := sdktrace.NewTracerProvider(
		sdktrace.WithSampler(overrideSampler(envSampler)),
		sdktrace.WithBatcher(exp),
	)

	// Set the Tracer Provider and the W3C Trace Context propagator as globals
	otel.SetTracerProvider(tp)

	// Register the trace context and baggage propagators so data is propagated across services/processes.
	otel.SetTextMapPropagator(
		propagation.NewCompositeTextMapPropagator(
			propagation.TraceContext{},
			propagation.Baggage{},
		),
	)
	return func(ctx context.Context) error {
		tp.Shutdown(ctx)
		return exp.Shutdown(ctx)
	}
}

// sampleRate ensures that the OTEL sampling environment variables are set and are valid
func sampleRate() error {
	_, found := os.LookupEnv("OTEL_TRACES_SAMPLER")
	if !found {
		return log.Errorf("telemetry OTEL_TRACES_SAMPLER not found, required for running")
	}
	sampleRate, found := os.LookupEnv("OTEL_TRACES_SAMPLER_ARG")
	if !found {
		return log.Errorf("telemetry OTEL_TRACES_SAMPLER_ARG not found, required for running")
	}
	_, err := strconv.ParseFloat(sampleRate, 64)
	if err != nil {
		return log.Errorf("telemetry otel failed to parse sample rate: %w", err)
	}
	return nil
}

func AlwaysSample(ctx context.Context) context.Context {
	return context.WithValue(ctx, alwaysSample, true)
}

type overrideType string

const alwaysSample = overrideType("always-sample")

type override struct {
	wrapped sdktrace.Sampler
}

func (os override) ShouldSample(p sdktrace.SamplingParameters) sdktrace.SamplingResult {
	if p.ParentContext.Value(alwaysSample) == true {
		return sdktrace.AlwaysSample().ShouldSample(p)
	}
	return os.wrapped.ShouldSample(p)
}

func (os override) Description() string {
	return "OverrideSampler"
}

// overrideSampler returns a Sampler that uses the sampler from the environment but
// that checks the parent context for a special key that overrides the sampler to
// always sample.
func overrideSampler(wrapped sdktrace.Sampler) sdktrace.Sampler {
	return override{wrapped: wrapped}
}
