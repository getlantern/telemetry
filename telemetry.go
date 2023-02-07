package telemetry

import (
	"context"
	"os"
	"strconv"

	"github.com/getlantern/golog"
	hostMetrics "go.opentelemetry.io/contrib/instrumentation/host"
	runtimeMetrics "go.opentelemetry.io/contrib/instrumentation/runtime"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetrichttp"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/metric/global"
	"go.opentelemetry.io/otel/propagation"
	controller "go.opentelemetry.io/otel/sdk/metric/controller/basic"
	"go.opentelemetry.io/otel/sdk/metric/export/aggregation"
	processor "go.opentelemetry.io/otel/sdk/metric/processor/basic"
	selector "go.opentelemetry.io/otel/sdk/metric/selector/simple"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.4.0"
)

var log = golog.LoggerFor("telemetry")

// Enable enables OTEL tracing and metrics for HTTP requests with the OTEL provider configured via
// environment variables. This allows us to switch providers purely by changing the environment variables.
func Enable(ctx context.Context, serviceName string, headers map[string]string) func() {
	log.Debug("Enabling OTEL tracing and metrics")
	shutdownTracing := EnableOTELTracing(ctx)
	shutdownMetrics := EnableOTELMetrics(ctx, serviceName, headers)
	return func() {
		shutdownTracing(ctx)
		shutdownMetrics(ctx)
	}
}

// EnableOTELTracing enables OTEL tracing for HTTP requests with the OTEL provider configured via
// environment variables. This allows us to switch providers purely by changing the environment variables.

// Sample rates should be configured via environment variables. See
// https://opentelemetry-python.readthedocs.io/en/latest/sdk/trace.sampling.html
// For example:
// OTEL_TRACES_SAMPLER=traceidratio OTEL_TRACES_SAMPLER_ARG=0.001
func EnableOTELTracing(ctx context.Context) func(context.Context) error {
	exp, err := newExporter(ctx)
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

func newExporter(ctx context.Context) (*otlptrace.Exporter, error) {
	client := otlptracehttp.NewClient()
	return otlptrace.New(ctx, client)
}

func newTraceProvider(exp *otlptrace.Exporter) *sdktrace.TracerProvider {
	sampleRate, found := os.LookupEnv("OTEL_TRACES_SAMPLER_ARG")
	if !found {
		sampleRate = "1.0"
	}
	sr, err := strconv.ParseFloat(sampleRate, 64)
	if err != nil {
		log.Errorf("failed to parse sample rate: %v", err)
		sr = 1.0
	}
	return sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exp),
		// This enables Honeycomb in particular to see the sample rate so that it can scale things appropriately.
		// See https://docs.honeycomb.io/manage-data-volume/sampling/
		sdktrace.WithResource(resource.NewWithAttributes(semconv.SchemaURL, attribute.Key("SampleRate").Int64(int64(1.0/sr)))),
	)
}

// EnableOTELMetrics enables OTEL metrics for HTTP requests with the OTEL provider configured via
// environment variables. This allows us to switch providers purely by changing the environment variables.
func EnableOTELMetrics(ctx context.Context, serviceName string, headers map[string]string) func(context.Context) {
	/*
		opts := []otlpmetrichttp.Option{
			otlpmetrichttp.WithHeaders(headers),
		}
		client := otlpmetrichttp.NewClient(opts...)
	*/
	client := otlpmetrichttp.NewClient()
	exporter, err := otlpmetric.New(ctx, client)
	//exporter, err := otlpmetric.New(ctx, client)
	if err != nil {
		log.Fatalf("Unable to initialize metrics, will not report metrics")
		return func(context.Context) {}
	}

	resource :=
		resource.NewWithAttributes(
			semconv.SchemaURL,
			semconv.ServiceNameKey.String(serviceName),
		)

	c := controller.New(
		processor.NewFactory(
			selector.NewWithInexpensiveDistribution(),
			aggregation.CumulativeTemporalitySelector(),
			processor.WithMemory(true),
		),
		controller.WithExporter(exporter),
		controller.WithResource(resource),
	)
	if err = c.Start(ctx); err != nil {
		log.Fatalf("Unable to start metrics controller, will not report metrics: %v", err)
		return func(context.Context) {}
	}
	if err := runtimeMetrics.Start(runtimeMetrics.WithMeterProvider(c)); err != nil {
		log.Fatalf("Failed to start runtime metrics: %v", err)
		return func(context.Context) {}
	}

	if err := hostMetrics.Start(hostMetrics.WithMeterProvider(c)); err != nil {
		log.Fatalf("Failed to start host metrics: %v", err)
		return func(context.Context) {}
	}

	log.Debug("Metrics reporting enabled")
	global.SetMeterProvider(c)

	return func(ctx context.Context) {
		c.Stop(ctx)
		exporter.Shutdown(ctx)
	}
}
