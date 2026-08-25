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
	normalized, err := normalizeModels(root["models"].(map[string]any))
	if err != nil {
		t.Fatal(err)
	}
	root["models"] = normalized
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

func TestApplyWireTranslateResponsesIngress(t *testing.T) {
	for _, tc := range []struct {
		name             string
		upstreamProtocol string
		wantAdapter      string
	}{
		{"chat upstream", "openai-chat-completions", AdapterWireTranslateR2O},
		{"compat chat upstream", "openai-compat-chat", AdapterWireTranslateR2O},
		{"anthropic upstream", "anthropic-messages", AdapterWireTranslateR2A},
	} {
		t.Run(tc.name, func(t *testing.T) {
			doc := corecatalog.Document{
				Providers: map[string]corecatalog.Provider{
					"up": {
						CredentialProfile: "up",
						Surfaces: map[string]corecatalog.Surface{
							"chat": {Protocol: tc.upstreamProtocol, BaseURL: "http://up/v1"},
						},
					},
				},
				Models: map[string]corecatalog.Model{
					"m": {
						Modalities: map[string]corecatalog.Modality{
							"chat": {
								Wire: corecatalog.WireOpenAIResponses,
								Providers: []corecatalog.PoolEntry{{
									ProviderRef: "up",
									Surface:     "chat",
									Model:       "upstream/model",
								}},
							},
						},
					},
				},
			}
			applyWireTranslate(&doc)
			entry := doc.Models["m"].Modalities["chat"].Providers[0]
			if entry.Surface != wireTranslateSurfacePrefix+tc.wantAdapter {
				t.Fatalf("surface=%q", entry.Surface)
			}
			cat, err := corecatalog.NewFromDocument(doc)
			if err != nil {
				t.Fatal(err)
			}
			plan, err := cat.Resolve("m", corecatalog.WireOpenAIResponses)
			if err != nil {
				t.Fatal(err)
			}
			if plan.Targets[0].Adapter != tc.wantAdapter {
				t.Fatalf("adapter=%q want %q", plan.Targets[0].Adapter, tc.wantAdapter)
			}
		})
	}
}

// A responses-native upstream (protocol openai-responses) stays passthrough.
func TestApplyWireTranslateResponsesPassthroughUntouched(t *testing.T) {
	doc := corecatalog.Document{
		Providers: map[string]corecatalog.Provider{
			"xai": {
				CredentialProfile: "xai",
				Surfaces: map[string]corecatalog.Surface{
					"responses": {Protocol: "openai-responses", BaseURL: "https://api.x.ai/v1"},
				},
			},
		},
		Models: map[string]corecatalog.Model{
			"m": {
				Modalities: map[string]corecatalog.Modality{
					"chat": {
						Wire: corecatalog.WireOpenAIResponses,
						Providers: []corecatalog.PoolEntry{{
							ProviderRef: "xai",
							Surface:     "responses",
							Model:       "grok-4.6",
						}},
					},
				},
			},
		},
	}
	applyWireTranslate(&doc)
	entry := doc.Models["m"].Modalities["chat"].Providers[0]
	if entry.Surface != "responses" {
		t.Fatalf("surface=%q — passthrough surface must not be rewritten", entry.Surface)
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
