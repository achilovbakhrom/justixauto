package telemetry

import (
	"context"
	"testing"
)

// Exporters connect lazily, so Setup with an endpoint must succeed offline.
func TestSetupWithEndpoint(t *testing.T) {
	t.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", "http://127.0.0.1:1")
	t.Setenv("OTEL_RESOURCE_ATTRIBUTES", "deployment.environment=test")
	stop, err := Setup(context.Background(), "test")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // do not wait for the unreachable collector
	_ = stop(ctx)
}
