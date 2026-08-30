package catalog_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/subosito/cincai/catalog"
	icatalog "github.com/subosito/cincai/internal/catalog"
)

func TestAllowModelsPlumbedToTarget(t *testing.T) {
	t.Parallel()
	raw := `
providers:
  vendor:
    credential_profile: vendor-api
    allow_models:
      - sku-a
      - sku-b
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
        model: sku-a
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
	plan, err := cat.Resolve("example-model", catalog.WireOpenAIChat)
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Targets) != 1 {
		t.Fatalf("targets=%d", len(plan.Targets))
	}
	got := plan.Targets[0].AllowModels
	if len(got) != 2 || got[0] != "sku-a" || got[1] != "sku-b" {
		t.Fatalf("AllowModels=%v", got)
	}
}

func TestUpstreamAllowed(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		target catalog.Target
		want   bool
	}{
		{
			name:   "empty allowlist allows any",
			target: catalog.Target{Model: "m", UpstreamModel: "anything"},
			want:   true,
		},
		{
			name:   "listed upstream allowed",
			target: catalog.Target{UpstreamModel: "sku-a", AllowModels: []string{"sku-a", "sku-b"}},
			want:   true,
		},
		{
			name:   "unlisted upstream rejected",
			target: catalog.Target{UpstreamModel: "sku-x", AllowModels: []string{"sku-a"}},
			want:   false,
		},
		{
			name:   "trim upstream before compare",
			target: catalog.Target{UpstreamModel: " sku-a ", AllowModels: []string{"sku-a"}},
			want:   true,
		},
		{
			name:   "empty upstream falls back to catalog model id",
			target: catalog.Target{Model: "public-id", AllowModels: []string{"public-id"}},
			want:   true,
		},
		{
			name:   "case sensitive",
			target: catalog.Target{UpstreamModel: "SKU-A", AllowModels: []string{"sku-a"}},
			want:   false,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := tc.target.UpstreamAllowed(); got != tc.want {
				t.Fatalf("UpstreamAllowed()=%v want %v (check=%q)", got, tc.want, tc.target.CheckUpstreamModel())
			}
		})
	}
}

func TestAllowModelsAfterEffortRewrite(t *testing.T) {
	t.Parallel()
	doc := catalog.Document{
		Providers: map[string]catalog.Provider{
			"vendor": {
				CredentialProfile: "vendor-api",
				AllowModels:       []string{"sku-high"},
				Surfaces: map[string]catalog.Surface{
					"chat": {Protocol: "openai-chat-completions", BaseURL: "https://api.example.com"},
				},
			},
		},
		Models: map[string]catalog.Model{
			"example-model": {
				Efforts:       []string{"low", "high"},
				DefaultEffort: "low",
				Modalities: map[string]catalog.Modality{
					"chat": {
						Wire: catalog.WireOpenAIChat,
						Providers: []catalog.PoolEntry{{
							ProviderRef: "vendor",
							Surface:     "chat",
							Models:      map[string]string{"low": "sku-low", "high": "sku-high"},
						}},
					},
				},
			},
		},
	}
	cat, err := catalog.NewFromDocument(doc)
	if err != nil {
		t.Fatal(err)
	}
	plan, err := cat.Resolve("example-model", catalog.WireOpenAIChat)
	if err != nil {
		t.Fatal(err)
	}
	used, next, err := cat.ApplyEffort("example-model", "high", plan.Targets)
	if err != nil {
		t.Fatal(err)
	}
	if used != "high" {
		t.Fatalf("used=%q", used)
	}
	if len(next) != 1 || next[0].UpstreamModel != "sku-high" {
		t.Fatalf("upstream=%q", next[0].UpstreamModel)
	}
	if !next[0].UpstreamAllowed() {
		t.Fatal("sku-high should be allowed after effort rewrite")
	}
	_, nextLow, err := cat.ApplyEffort("example-model", "low", plan.Targets)
	if err != nil {
		t.Fatal(err)
	}
	if nextLow[0].UpstreamAllowed() {
		t.Fatalf("sku-low should be rejected by allowlist, got upstream=%q", nextLow[0].UpstreamModel)
	}
}
