package catalog_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/subosito/cincai/catalog"
	icatalog "github.com/subosito/cincai/internal/catalog"
)

func TestProviderProxyPlumbedToTarget(t *testing.T) {
	t.Parallel()
	raw := `
providers:
  vendor:
    credential_profile: vendor-api
    proxy: http://127.0.0.1:8080
    capabilities:
      chat:
        protocol: openai-chat-completions
        base_url: https://api.example.com
models:
  example-model:
    modalities:
      chat:
        wire: openai-chat-completions
        provider_ref: vendor
        model: example-model-medium
        surface: chat
`
	// Use product Load path (normalize capabilities → surfaces).
	tmp := filepath.Join(t.TempDir(), "p.yaml")
	if err := os.WriteFile(tmp, []byte(raw), 0o600); err != nil {
		t.Fatal(err)
	}
	cat, err := icatalog.Load(tmp)
	if err != nil {
		t.Fatal(err)
	}
	plan, err := cat.Resolve("example-model", catalog.WireOpenAIChat)
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Targets) != 1 {
		t.Fatalf("targets=%d", len(plan.Targets))
	}
	if got := plan.Targets[0].Proxy; got != "http://127.0.0.1:8080" {
		t.Fatalf("Proxy=%q", got)
	}
	if plan.Targets[0].UpstreamModel != "example-model-medium" {
		t.Fatalf("UpstreamModel=%q", plan.Targets[0].UpstreamModel)
	}
}
