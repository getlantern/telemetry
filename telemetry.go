package telemetry

import (
	"context"

	"log"

	hostMetrics "go.opentelemetry.io/contrib/instrumentation/host"
	runtimeMetrics "go.opentelemetry.io/contrib/instrumentation/runtime"
	"go.opentelemetry.io/otel"
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
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
)

// Enable enables OTEL tracing and metrics for HTTP requests with the OTEL provider configured via
// environment variables. This allows us to switch providers purely by changing the environment variables.
func Enable(ctx context.Context) func() {
	shutdownTracing := EnableOTELTracing(ctx)
	shutdownMetrics := EnableOTELMetrics(ctx)
	return func() {
		shutdownTracing(ctx)
		shutdownMetrics(ctx)
	}
}

// EnableOTELTracing enables OTEL tracing for HTTP requests with the OTEL provider configured via
// environment variables. This allows us to switch providers purely by changing the environment variables.
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
	return sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exp),
	)
}

// EnableOTELMetrics enables OTEL metrics for HTTP requests with the OTEL provider configured via
// environment variables. This allows us to switch providers purely by changing the environment variables.
func EnableOTELMetrics(ctx context.Context) func(context.Context) {
	client := otlpmetrichttp.NewClient()
	exporter, err := otlpmetric.New(context.Background(), client)
	if err != nil {
		log.Fatalf("Unable to initialize metrics, will not report metrics")
		return func(context.Context) {}
	}

	c := controller.New(
		processor.NewFactory(
			selector.NewWithInexpensiveDistribution(),
			aggregation.CumulativeTemporalitySelector(),
			processor.WithMemory(true),
		),
		controller.WithExporter(exporter),
	)
	if err = c.Start(context.Background()); err != nil {
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

	global.SetMeterProvider(c)
	return func(ctx context.Context) {
		c.Stop(ctx)
		exporter.Shutdown(ctx)
	}
}
