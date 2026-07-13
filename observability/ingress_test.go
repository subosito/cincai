package observability_test

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/subosito/cincai/observability"
)

func TestLogRequestFieldsNoSecrets(t *testing.T) {
	var buf bytes.Buffer
	observability.SetTestLogger(slog.New(slog.NewJSONHandler(&buf, nil)))
	observability.LogRequest("openai-chat-completions", "mock-chat", "mock", "openai-chat-completions", 200, time.Now(), "principal-1")
	out := buf.String()
	if !strings.Contains(out, "ingress") {
		t.Fatalf("missing ingress log: %s", out)
	}
	for _, field := range []string{"wire", "model", "provider_ref", "protocol", "status", "latency_ms", "principal_id"} {
		if !strings.Contains(out, field) {
			t.Fatalf("missing field %q in %s", field, out)
		}
	}
	for _, forbidden := range []string{"sk-", "access_token", "refresh_token"} {
		if strings.Contains(out, forbidden) {
			t.Fatalf("secret leaked in log: %s", out)
		}
	}
	var payload map[string]any
	if err := json.Unmarshal([]byte(out), &payload); err != nil {
		t.Fatal(err)
	}
}