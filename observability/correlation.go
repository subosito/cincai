package observability

import (
	"context"
	"strings"
)

type correlationKey struct{}

const headerCorrelationID = "X-Correlation-Id"

// WithCorrelationID stores correlation_id on ctx (from X-Correlation-Id).
func WithCorrelationID(ctx context.Context, correlationID string) context.Context {
	correlationID = strings.TrimSpace(correlationID)
	if correlationID == "" {
		return ctx
	}
	return context.WithValue(ctx, correlationKey{}, correlationID)
}

// CorrelationIDFromContext reads correlation_id from ctx.
func CorrelationIDFromContext(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	v, _ := ctx.Value(correlationKey{}).(string)
	return strings.TrimSpace(v)
}