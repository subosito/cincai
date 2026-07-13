package observability_test

import (
	"context"
	"net/http"
	"testing"

	"github.com/subosito/cincai/observability"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
)

func TestExtractHTTP_roundTrip(t *testing.T) {
	tp := sdktrace.NewTracerProvider()
	otel.SetTracerProvider(tp)
	otel.SetTextMapPropagator(propagation.TraceContext{})
	t.Cleanup(func() { _ = tp.Shutdown(context.Background()) })

	ctx, span := tp.Tracer("test").Start(context.Background(), "parent")
	h := http.Header{}
	observability.InjectHTTP(ctx, h)
	span.End()

	child := observability.ExtractHTTP(context.Background(), h)
	_, childSpan := tp.Tracer("test").Start(child, "child")
	if !childSpan.SpanContext().IsValid() {
		t.Fatal("want valid child span")
	}
	childSpan.End()
}