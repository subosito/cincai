package catalog_test

import (
	"testing"

	"github.com/subosito/cincai/catalog"
)

func TestLoadProviderInjectMap(t *testing.T) {
	doc := catalog.Document{
		Providers: map[string]catalog.Provider{
			"acme": {
				CredentialProfile: "acme-oauth",
				Inject: map[string]string{
					"authorization": "Bearer ${access}",
					"x-account-id":  "${accountId}",
				},
				Surfaces: map[string]catalog.Surface{
					"default": {Protocol: "openai-chat-completions", BaseURL: "https://example.com"},
				},
			},
		},
		Models: map[string]catalog.Model{
			"m": {
				Modalities: map[string]catalog.Modality{
					"chat": {
						Wire: catalog.WireOpenAIChat,
						Providers: []catalog.PoolEntry{
							{ProviderRef: "acme", Model: "gpt"},
						},
					},
				},
			},
		},
	}
	cat, err := catalog.NewFromDocument(doc)
	if err != nil {
		t.Fatal(err)
	}
	plan, err := cat.Resolve("m", catalog.WireOpenAIChat)
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Targets) != 1 {
		t.Fatalf("targets=%d", len(plan.Targets))
	}
	tgt := plan.Targets[0]
	if got := tgt.Inject["authorization"]; got != "Bearer ${access}" {
		t.Fatalf("inject authorization=%q", got)
	}
	if got := tgt.Inject["x-account-id"]; got != "${accountId}" {
		t.Fatalf("inject account=%q", got)
	}
}