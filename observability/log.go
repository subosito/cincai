package observability

import (
	"context"
	"log/slog"

	"go.opentelemetry.io/otel/trace"
)

type traceContextHandler struct {
	inner slog.Handler
}

func newTraceContextHandler(inner slog.Handler) *traceContextHandler {
	return &traceContextHandler{inner: inner}
}

func (h *traceContextHandler) Enabled(ctx context.Context, level slog.Level) bool {
	return h.inner.Enabled(ctx, level)
}

func (h *traceContextHandler) Handle(ctx context.Context, r slog.Record) error {
	sc := trace.SpanFromContext(ctx).SpanContext()
	if sc.IsValid() {
		r.AddAttrs(
			slog.String("trace_id", sc.TraceID().String()),
			slog.String("span_id", sc.SpanID().String()),
		)
	}
	if corr := CorrelationIDFromContext(ctx); corr != "" {
		r.AddAttrs(slog.String("correlation_id", corr))
	}
	return h.inner.Handle(ctx, r)
}

func (h *traceContextHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return newTraceContextHandler(h.inner.WithAttrs(attrs))
}

func (h *traceContextHandler) WithGroup(name string) slog.Handler {
	return newTraceContextHandler(h.inner.WithGroup(name))
}

// LogInfo logs with trace correlation when ctx carries a span.
func LogInfo(ctx context.Context, msg string, args ...any) {
	slog.InfoContext(ctx, msg, args...)
}