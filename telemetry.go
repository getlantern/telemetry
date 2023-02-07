package telemetry

import (
	"context"
	"os"
	"strconv"

	"github.com/getlantern/golog"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/propagation"

	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.4.0"
)

var log = golog.LoggerFor("telemetry")

// Enable enables OTEL tracing and metrics for HTTP requests with the OTEL provider configured via
// environment variables. This allows us to switch providers purely by changing the environment variables.
func Enable(ctx context.Context, serviceName string, headers map[string]string) func() {
	log.Debug("Enabling OTEL tracing")
	shutdownTracing := EnableOTELTracing(ctx)
	return func() {
		shutdownTracing(ctx)
	}
}

// EnableOTELTracing enables OTEL tracing for HTTP requests with the OTEL provider configured via
// environment variables. This allows us to switch providers purely by changing the environment variables.

// Sample rates should be configured via environment variables. See
// https://opentelemetry-python.readthedocs.io/en/latest/sdk/trace.sampling.html
// For example:
// OTEL_TRACES_SAMPLER=traceidratio OTEL_TRACES_SAMPLER_ARG=0.001
func EnableOTELTracing(ctx context.Context) func(context.Context) error {
	exp, err := otlptrace.New(ctx, otlptracehttp.NewClient())
	if err != nil {
		log.Fatalf("failed to initialize exporter: %v", err)
	}

	// Create a new tracer provider with a batch span processor and the otlp exporter.
	tp := newTraceProvider(exp)

	// Set the Tracer Provider and the W3C Trace Context propagator as globals
	otel.SetTracerProvider(tp)

	// Register the trace context and baggage propagators so data is propagated across services/processes.
	otel.SetTextMapPropagator(
		propagation.NewCompositeTextMapPropagator(
			propagation.TraceContext{},
			propagation.Baggage{},
		),
	)
	return tp.Shutdown
}

func newTraceProvider(exp *otlptrace.Exporter) *sdktrace.TracerProvider {
	sampleRate, found := os.LookupEnv("OTEL_TRACES_SAMPLER_ARG")
	if !found {
		log.Errorf("OTEL_TRACES_SAMPLER_ARG not found, defaulting sample rate to 1.0")
		sampleRate = "1.0"
	}
	sr, err := strconv.ParseFloat(sampleRate, 64)
	if err != nil {
		log.Errorf("failed to parse sample rate: %v", err)
		sr = 1.0
	}
	return sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exp),
		//sdktrace.WithSampler(sdktrace.ParentBased(sdktrace.TraceIDRatioBased(sr))),
		// This enables Honeycomb in particular to see the sample rate so that it can scale things appropriately.
		// See https://docs.honeycomb.io/manage-data-volume/sampling/
		sdktrace.WithResource(resource.NewWithAttributes(semconv.SchemaURL, attribute.Key("SampleRate").Int64(int64(1.0/sr)))),
	)
}
