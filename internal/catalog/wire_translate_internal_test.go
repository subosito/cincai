package catalog

import (
	"os"
	"path/filepath"
	"testing"

	corecatalog "github.com/subosito/cincai/catalog"
	"gopkg.in/yaml.v3"
)

func TestApplyWireTranslateMutatesPool(t *testing.T) {
	doc := corecatalog.Document{
		Providers: map[string]corecatalog.Provider{
			"cc": {
				CredentialProfile: "cc",
				Surfaces: map[string]corecatalog.Surface{
					"chat": {
						Protocol: "openai-chat-completions",
						BaseURL:  "http://cc/v1",
					},
				},
			},
		},
		Models: map[string]corecatalog.Model{
			"m": {
				Modalities: map[string]corecatalog.Modality{
					"anthropic_chat": {
						Wire: corecatalog.WireAnthropicMsg,
						Providers: []corecatalog.PoolEntry{{
							ProviderRef: "cc",
							Surface:     "chat",
							Model:       "upstream/model",
						}},
					},
				},
			},
		},
	}
	applyWireTranslate(&doc)
	entry := doc.Models["m"].Modalities["anthropic_chat"].Providers[0]
	if entry.Surface != wireTranslateSurfacePrefix+AdapterWireTranslateA2O {
		t.Fatalf("surface=%q", entry.Surface)
	}
	if _, ok := doc.Providers["cc"].Surfaces[wireTranslateSurfacePrefix+AdapterWireTranslateA2O]; !ok {
		t.Fatal("injected surface missing")
	}
	cat, err := corecatalog.NewFromDocument(doc)
	if err != nil {
		t.Fatal(err)
	}
	plan, err := cat.Resolve("m", corecatalog.WireAnthropicMsg)
	if err != nil {
		t.Fatal(err)
	}
	if plan.Targets[0].Adapter != AdapterWireTranslateA2O {
		t.Fatalf("adapter=%q", plan.Targets[0].Adapter)
	}
}

func TestDocBeforeWireTranslate(t *testing.T) {
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
        provider_ref: cc
        model: upstream/model
        surface: chat
`
	var root map[string]any
	if err := yaml.Unmarshal([]byte(raw), &root); err != nil {
		t.Fatal(err)
	}
	root["providers"] = normalizeProviders(root["providers"].(map[string]any))
	root["models"] = normalizeModels(root["models"].(map[string]any))
	out, err := yaml.Marshal(root)
	if err != nil {
		t.Fatal(err)
	}
	var doc corecatalog.Document
	if err := yaml.Unmarshal(out, &doc); err != nil {
		t.Fatal(err)
	}
	mod := doc.Models["m"].Modalities["anthropic_chat"]
	if mod.Wire != corecatalog.WireAnthropicMsg {
		t.Fatalf("wire=%q", mod.Wire)
	}
	if mod.Providers[0].Surface != "chat" {
		t.Fatalf("surface=%q", mod.Providers[0].Surface)
	}
	applyWireTranslate(&doc)
	mod = doc.Models["m"].Modalities["anthropic_chat"]
	if mod.Providers[0].Surface != wireTranslateSurfacePrefix+AdapterWireTranslateA2O {
		t.Fatalf("after apply surface=%q", mod.Providers[0].Surface)
	}
}

func TestLoadWireTranslate(t *testing.T) {
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
        provider_ref: cc
        model: upstream/model
        surface: chat
`
	tmp := filepath.Join(t.TempDir(), "p.yaml")
	if err := os.WriteFile(tmp, []byte(raw), 0o600); err != nil {
		t.Fatal(err)
	}
	cat, err := Load(tmp)
	if err != nil {
		t.Fatal(err)
	}
	plan, err := cat.Resolve("m", corecatalog.WireAnthropicMsg)
	if err != nil {
		t.Fatal(err)
	}
	if plan.Targets[0].Adapter != AdapterWireTranslateA2O {
		t.Fatalf("adapter=%q", plan.Targets[0].Adapter)
	}
}