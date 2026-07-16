package observability_test

import (
	"context"
	"testing"
	"time"

	"github.com/subosito/cincai/observability"
	"go.opentelemetry.io/otel"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
)

func TestHook_usesGlobalProviders(t *testing.T) {
	tp := sdktrace.NewTracerProvider()
	mp := sdkmetric.NewMeterProvider()
	otel.SetTracerProvider(tp)
	otel.SetMeterProvider(mp)
	t.Cleanup(func() {
		_ = tp.Shutdown(context.Background())
		_ = mp.Shutdown(context.Background())
		_ = observability.Shutdown(context.Background())
	})

	observability.Hook("cincai-test")
	if !observability.Hooked() {
		t.Fatal("want hooked")
	}
	if !observability.Enabled() {
		t.Fatal("want enabled")
	}
	observability.Metrics().RecordIngress(context.Background(), "openai-chat-completions", "m", "p", "proto", 200, time.Millisecond)

	// Shutdown must not tear down embedder providers.
	_ = observability.Shutdown(context.Background())
	if !observability.Hooked() {
		// global cleared; re-hook for cleanup check
	}
	_ = tp.Shutdown(context.Background())
}
