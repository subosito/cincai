package observability_test

import (
	"testing"

	"github.com/subosito/cincai/observability"
)

func TestLoadConfig_disabledWithoutEndpoint(t *testing.T) {
	t.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", "")
	t.Setenv("OTEL_SDK_DISABLED", "")
	cfg := observability.LoadConfig("cincai")
	if cfg.Enabled {
		t.Fatal("want disabled without endpoint")
	}
}

func TestLoadConfig_authTokenHeader(t *testing.T) {
	t.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", "http://collector:4318")
	t.Setenv("OTEL_EXPORTER_AUTH_TOKEN", "secret")
	t.Setenv("OTEL_EXPORTER_OTLP_HEADERS", "")
	cfg := observability.LoadConfig("cincai")
	if !cfg.Enabled {
		t.Fatal("want enabled")
	}
	if cfg.Headers["Authorization"] != "Bearer secret" {
		t.Fatalf("auth header=%q", cfg.Headers["Authorization"])
	}
}

func TestLoadConfig_disabledFlag(t *testing.T) {
	t.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", "http://127.0.0.1:4318")
	t.Setenv("OTEL_SDK_DISABLED", "true")
	cfg := observability.LoadConfig("cincai")
	if cfg.Enabled {
		t.Fatal("want disabled")
	}
}