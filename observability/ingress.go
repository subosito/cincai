package observability

import (
	"log/slog"
	"time"
)

// RequestLog fields for one ingress request (no secrets).
type RequestLog struct {
	Wire        string `json:"wire"`
	Model       string `json:"model"`
	ProviderRef string `json:"provider_ref"`
	Protocol    string `json:"protocol"`
	Status      int    `json:"status"`
	LatencyMs   int64  `json:"latency_ms"`
	PrincipalID string `json:"principal_id"`
	Usage       *Usage `json:"usage,omitempty"`
}

// Usage is per-request token / unit counts parsed from the upstream response.
// No pricing — measurement only; pricing is a product-layer concern. Zero when
// the upstream did not report usage (unknown, not free).
type Usage struct {
	InputTokens      int    `json:"input_tokens,omitempty"`
	OutputTokens     int    `json:"output_tokens,omitempty"`
	CacheReadTokens  int    `json:"cache_read_tokens,omitempty"`  // prompt-cache hits (Anthropic cache_read, OpenAI cached_tokens)
	CacheWriteTokens int    `json:"cache_write_tokens,omitempty"` // Anthropic cache_creation_input_tokens
	Units            int    `json:"units,omitempty"`              // media wires: images, seconds, characters
	Unit             string `json:"unit,omitempty"`               // "image" | "second" | "character"
}

// Zero reports whether nothing was measured.
func (u Usage) Zero() bool {
	return u.InputTokens == 0 && u.OutputTokens == 0 && u.CacheReadTokens == 0 && u.CacheWriteTokens == 0 && u.Units == 0
}

// SetTestLogger redirects ingress logs (tests only).
func SetTestLogger(l *slog.Logger) {
	ingressLog = l
}

// LogRequest emits exactly one structured line per request (context-free; for tests and direct callers).
func LogRequest(wire, model, providerRef, protocol string, status int, start time.Time, principalID string) {
	RecordIngress(nil, &Recorder{
		Wire: wire, Model: model, ProviderRef: providerRef, Protocol: protocol, PrincipalID: principalID,
	}, status, start)
}
