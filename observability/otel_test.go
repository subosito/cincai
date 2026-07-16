package observability_test

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/subosito/cincai/observability"
)

func TestInit_noopWithoutEndpoint(t *testing.T) {
	t.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", "")
	p, err := observability.Init("cincai-test")
	if err != nil {
		t.Fatal(err)
	}
	if observability.Enabled() {
		t.Fatal("want not enabled")
	}
	observability.Metrics().RecordIngress(context.Background(), "openai-chat-completions", "m", "p", "openai-chat-completions", 200, time.Millisecond)
	_ = observability.Shutdown(context.Background())
	_ = p
}

func TestInit_withEndpoint(t *testing.T) {
	t.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", "http://127.0.0.1:4318")
	t.Setenv("OTEL_SDK_DISABLED", "")
	p, err := observability.Init("cincai-test")
	if err != nil {
		t.Fatal(err)
	}
	if !observability.Enabled() {
		t.Fatal("want enabled")
	}
	observability.Metrics().RecordIngress(context.Background(), "openai-chat-completions", "m", "p", "openai-chat-completions", 200, time.Millisecond)
	_ = observability.Shutdown(context.Background())
	_ = p
}

func TestMain(m *testing.M) {
	_ = observability.Shutdown(context.Background())
	os.Exit(m.Run())
}
