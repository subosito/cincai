package observability

import (
	"context"
	"net/http"

	"go.opentelemetry.io/otel"
)

type headerCarrier http.Header

func (c headerCarrier) Get(key string) string {
	return http.Header(c).Get(key)
}

func (c headerCarrier) Set(key, val string) {
	http.Header(c).Set(key, val)
}

func (c headerCarrier) Keys() []string {
	keys := make([]string, 0, len(c))
	for k := range c {
		keys = append(keys, k)
	}
	return keys
}

// ExtractHTTP continues a trace from inbound HTTP headers (W3C tracecontext).
func ExtractHTTP(ctx context.Context, h http.Header) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	if h == nil {
		return ctx
	}
	return otel.GetTextMapPropagator().Extract(ctx, headerCarrier(h))
}

// InjectHTTP writes W3C trace context into outbound HTTP headers.
func InjectHTTP(ctx context.Context, h http.Header) {
	if h == nil {
		return
	}
	otel.GetTextMapPropagator().Inject(ctx, headerCarrier(h))
}