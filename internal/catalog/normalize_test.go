package catalog_test

import (
	"os"
	"path/filepath"
	"testing"

	corecatalog "github.com/subosito/cincai/catalog"

	icatalog "github.com/subosito/cincai/internal/catalog"
)

func TestStarterCatalogDeepseek(t *testing.T) {
	path := filepath.Join("..", "..", "config", "providers.yaml.example")
	if _, err := os.Stat(path); err != nil {
		t.Skip("providers.yaml.example missing")
	}
	cat, err := icatalog.Load(path)
	if err != nil {
		t.Fatal(err)
	}

	chat, err := cat.Resolve("deepseek-v4-flash", corecatalog.WireOpenAIChat)
	if err != nil {
		t.Fatalf("deepseek: %v", err)
	}
	if chat.Targets[0].Protocol != "openai-chat-completions" {
		t.Fatalf("deepseek protocol=%q", chat.Targets[0].Protocol)
	}
	if chat.Targets[0].ProviderRef != "deepseek" {
		t.Fatalf("deepseek provider=%q", chat.Targets[0].ProviderRef)
	}
}

func TestGrokImageUnderstand_keepsOpenAIResponsesWire(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "providers.yaml")
	const yamlDoc = `
providers:
  xai:
    credential_profile: xai
    surfaces:
      responses:
        protocol: openai-responses
        base_url: https://api.x.ai/v1
models:
  grok-4.3:
    modalities:
      chat:
        wire: openai-responses
        provider_ref: xai
      image:
        wire: openai-responses
        provider_ref: xai
`
	if err := os.WriteFile(path, []byte(yamlDoc), 0o644); err != nil {
		t.Fatal(err)
	}
	cat, err := icatalog.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	plan, err := cat.ResolveWithModality("grok-4.3", corecatalog.WireOpenAIResponses, "image")
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Targets) == 0 || plan.Targets[0].ProviderRef != "xai" {
		t.Fatalf("targets=%+v", plan.Targets)
	}
}