package upstream

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

// An upstream response must not set cookies on the gateway origin; useful headers
// (rate limits, request id) pass through.
func TestWriteUpstreamHeadersStripsSensitive(t *testing.T) {
	resp := &http.Response{Header: http.Header{}}
	resp.Header.Set("Set-Cookie", "sess=leak")
	resp.Header.Set("X-Ratelimit-Remaining", "42")
	resp.Header.Set("X-Request-Id", "req-123")
	resp.Header.Set("Content-Type", "application/json")

	rec := httptest.NewRecorder()
	writeUpstreamHeaders(rec, resp)
	got := rec.Header()

	if v := got.Get("Set-Cookie"); v != "" {
		t.Errorf("Set-Cookie leaked to client: %q", v)
	}
	for _, h := range []string{"X-Ratelimit-Remaining", "X-Request-Id", "Content-Type"} {
		if got.Get(h) == "" {
			t.Errorf("useful header %q was wrongly stripped", h)
		}
	}
}
