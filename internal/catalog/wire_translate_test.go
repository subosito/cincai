package catalog_test

import (
	"os"
	"path/filepath"
	"testing"

	corecatalog "github.com/subosito/cincai/catalog"

	icatalog "github.com/subosito/cincai/internal/catalog"
)

func TestWireTranslateInjectedAtLoad(t *testing.T) {
	raw := `
providers:
  cc:
    credential_profile: cc
    surfaces:
      chat:
        protocol: openai-chat-completions
        base_url: http://cc/v1
models:
  deepseek-v4-flash-cc:
    modalities:
      anthropic_chat:
        wire: anthropic-messages
        provider_ref: cc
        model: deepseek/deepseek-v4-flash
        surface: chat
`
	tmp := filepath.Join(t.TempDir(), "p.yaml")
	if err := os.WriteFile(tmp, []byte(raw), 0o600); err != nil {
		t.Fatal(err)
	}
	cat, err := icatalog.Load(tmp)
	if err != nil {
		t.Fatal(err)
	}
	plan, err := cat.Resolve("deepseek-v4-flash-cc", corecatalog.WireAnthropicMsg)
	if err != nil {
		t.Fatal(err)
	}
	if plan.Targets[0].Adapter != icatalog.AdapterWireTranslateA2O {
		t.Fatalf("adapter=%q", plan.Targets[0].Adapter)
	}
	if plan.Targets[0].UpstreamModel != "deepseek/deepseek-v4-flash" {
		t.Fatalf("upstream=%q", plan.Targets[0].UpstreamModel)
	}
}

func TestCoreCatalogNoWireTranslate(t *testing.T) {
	raw := `
providers:
  cc:
    credential_profile: cc
    surfaces:
      chat:
        protocol: openai-chat-completions
        base_url: http://cc/v1
models:
  m:
    modalities:
      anthropic_chat:
        wire: anthropic-messages
        providers:
          - provider_ref: cc
            model: x
`
	tmp := filepath.Join(t.TempDir(), "p.yaml")
	if err := os.WriteFile(tmp, []byte(raw), 0o600); err != nil {
		t.Fatal(err)
	}
	cat, err := corecatalog.Load(tmp)
	if err != nil {
		t.Fatal(err)
	}
	_, err = cat.Resolve("m", corecatalog.WireAnthropicMsg)
	if err == nil {
		t.Fatal("expected core catalog to reject wire mismatch without translate")
	}
}
