// Package telemetry wires OpenTelemetry traces and metrics (OTLP over HTTP)
// and JSON logs that carry the trace of each request. Exporting is on when
// OTEL_EXPORTER_OTLP_ENDPOINT is set; the standard OTEL_* variables apply
// (OTEL_SERVICE_NAME, OTEL_RESOURCE_ATTRIBUTES, OTEL_TRACES_SAMPLER, ...).
package telemetry

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetrichttp"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/propagation"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"
	"go.opentelemetry.io/otel/trace"
)

// ServiceName is used when OTEL_SERVICE_NAME is not set.
const ServiceName = "justixauto-api"

// Setup installs the global tracer and meter providers. The returned function
// flushes and stops them; call it on shutdown.
func Setup(ctx context.Context, version string) (func(context.Context) error, error) {
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(propagation.TraceContext{}, propagation.Baggage{}))
	if os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT") == "" {
		return func(context.Context) error { return nil }, nil
	}
	host, _ := os.Hostname() // the pod name in Kubernetes
	// Schemaless: merging with the SDK default resource fails when schema URLs differ.
	res, err := resource.Merge(resource.Default(), resource.NewSchemaless(
		semconv.ServiceName(envOr("OTEL_SERVICE_NAME", ServiceName)), semconv.ServiceVersion(version), semconv.ServiceInstanceID(host)))
	if err != nil {
		return nil, err
	}
	traces, err := otlptracehttp.New(ctx)
	if err != nil {
		return nil, err
	}
	metrics, err := otlpmetrichttp.New(ctx)
	if err != nil {
		return nil, err
	}
	tp := sdktrace.NewTracerProvider(sdktrace.WithBatcher(traces), sdktrace.WithResource(res))
	mp := sdkmetric.NewMeterProvider(sdkmetric.WithReader(sdkmetric.NewPeriodicReader(metrics, sdkmetric.WithInterval(15*time.Second))),
		sdkmetric.WithResource(res))
	otel.SetTracerProvider(tp)
	otel.SetMeterProvider(mp)
	return func(ctx context.Context) error { return errors.Join(tp.Shutdown(ctx), mp.Shutdown(ctx)) }, nil
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

// Logger returns a JSON logger on stdout whose records carry trace_id and
// span_id when logged with a request context (Loki links them to traces).
func Logger() *slog.Logger {
	return slog.New(traceHandler{slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo})})
}

type traceHandler struct{ slog.Handler }

func (h traceHandler) Handle(ctx context.Context, r slog.Record) error {
	if sc := trace.SpanContextFromContext(ctx); sc.IsValid() {
		r.AddAttrs(slog.String("trace_id", sc.TraceID().String()), slog.String("span_id", sc.SpanID().String()))
	}
	return h.Handler.Handle(ctx, r)
}

func (h traceHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return traceHandler{h.Handler.WithAttrs(attrs)}
}
func (h traceHandler) WithGroup(name string) slog.Handler {
	return traceHandler{h.Handler.WithGroup(name)}
}
