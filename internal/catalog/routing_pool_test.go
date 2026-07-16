package catalog_test

import (
	"os"
	"path/filepath"
	"testing"

	corecatalog "github.com/subosito/cincai/catalog"

	icatalog "github.com/subosito/cincai/internal/catalog"
)

// Pool syntax lives in docs/routing.md — not the minimal providers.yaml.example.
func TestModelPoolFailover(t *testing.T) {
	raw := `
providers:
  zhipu:
    credential_profile: zhipu-api
    surfaces:
      chat:
        protocol: openai-chat-completions
        base_url: https://api.z.ai/api/paas/v4
  openrouter:
    credential_profile: openrouter-api
    surfaces:
      chat:
        protocol: openai-chat-completions
        base_url: https://openrouter.ai/api/v1
models:
  glm-5.2:
    modalities:
      chat:
        wire: openai-chat-completions
        strategy: failover
        providers:
          - provider_ref: zhipu
            model: glm-5.2
          - provider_ref: openrouter
            model: z-ai/glm-5.2
`
	tmp := filepath.Join(t.TempDir(), "p.yaml")
	if err := os.WriteFile(tmp, []byte(raw), 0o600); err != nil {
		t.Fatal(err)
	}
	cat, err := icatalog.Load(tmp)
	if err != nil {
		t.Fatal(err)
	}
	plan, err := cat.Resolve("glm-5.2", corecatalog.WireOpenAIChat)
	if err != nil {
		t.Fatalf("glm-5.2: %v", err)
	}
	if plan.Strategy != corecatalog.StrategyFailover {
		t.Fatalf("strategy=%q, want failover", plan.Strategy)
	}
	if len(plan.Targets) != 2 {
		t.Fatalf("targets=%d, want 2", len(plan.Targets))
	}
	wantModel := map[string]string{
		"zhipu":      "glm-5.2",
		"openrouter": "z-ai/glm-5.2",
	}
	for _, tgt := range plan.Targets {
		want, ok := wantModel[tgt.ProviderRef]
		if !ok {
			t.Fatalf("unexpected provider_ref %q in pool", tgt.ProviderRef)
		}
		if tgt.UpstreamModel != want {
			t.Fatalf("%s upstream model=%q, want %q", tgt.ProviderRef, tgt.UpstreamModel, want)
		}
	}
}
