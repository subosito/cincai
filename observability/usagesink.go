package observability

import "context"

// UsageEvent is one finalized ingress request handed to a registered UsageSink
// for persistence / aggregation. Measurement only — no pricing.
type UsageEvent struct {
	Wire        string
	Model       string
	ProviderRef string
	Protocol    string
	PrincipalID string
	Status      int
	LatencyMs   int64
	Usage       Usage
}

// UsageSink receives every finalized ingress request. Embedders that persist usage
// register one; core defaults to none, so the log line stays the only
// output. Implementations must not block the request path — buffer / write async.
type UsageSink interface {
	RecordUsage(ctx context.Context, ev UsageEvent)
}

var usageSink UsageSink

// SetUsageSink registers the process-wide usage sink (nil disables).
func SetUsageSink(s UsageSink) { usageSink = s }
