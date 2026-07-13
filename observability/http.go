package observability

import (
	"context"
	"net/http"
	"time"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	oteltrace "go.opentelemetry.io/otel/trace"
)

// HTTPDo performs an outbound HTTP request with an upstream span and metrics.
func HTTPDo(ctx context.Context, client *http.Client, req *http.Request) (*http.Response, error) {
	if client == nil {
		client = http.DefaultClient
	}
	host := ""
	if req.URL != nil {
		host = req.URL.Host
	}
	ctx, span := StartSpan(ctx, "upstream.http",
		oteltrace.WithAttributes(
			attribute.String("http.method", req.Method),
			attribute.String("server.address", host),
		),
	)
	defer span.End()
	InjectHTTP(ctx, req.Header)
	start := time.Now()
	resp, err := client.Do(req.WithContext(ctx))
	status := 0
	if resp != nil {
		status = resp.StatusCode
		span.SetAttributes(attribute.Int("http.status_code", status))
	}
	Metrics().RecordUpstream(ctx, host, status, time.Since(start))
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return nil, err
	}
	if status >= 500 {
		span.SetStatus(codes.Error, http.StatusText(status))
	}
	return resp, nil
}