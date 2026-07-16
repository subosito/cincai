package catalog_test

import (
	"testing"

	"github.com/subosito/cincai/catalog"
)

func TestDefaultModalityForWire(t *testing.T) {
	if got := catalog.DefaultModalityForWire(catalog.WireOpenAIEmbed); got != "embed" {
		t.Fatalf("got %q want embed", got)
	}
	if got := catalog.DefaultModalityForWire(catalog.WireOpenAIChat); got != "" {
		t.Fatalf("got %q want empty", got)
	}
}

// After expand, chat+search_web under one authoring id become bare + :search.
func TestExpand_ChatAndSearchBecomeDistinctModels(t *testing.T) {
	doc := catalog.Document{
		Providers: map[string]catalog.Provider{
			"vendor-chat": {
				CredentialProfile: "vendor",
				Surfaces: map[string]catalog.Surface{
					"chat": {
						Protocol: "openai-responses",
						BaseURL:  "https://api.example/v1",
					},
				},
			},
		},
		Models: map[string]catalog.Model{
			"m": {
				Modalities: map[string]catalog.Modality{
					"chat":       {Wire: catalog.WireOpenAIResponses, Providers: []catalog.PoolEntry{{ProviderRef: "vendor-chat"}}},
					"search_web": {Wire: catalog.WireOpenAIResponses, Providers: []catalog.PoolEntry{{ProviderRef: "vendor-chat"}}},
				},
			},
		},
	}
	cat, err := catalog.NewFromDocument(doc)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := cat.Resolve("m", catalog.WireOpenAIResponses); err != nil {
		t.Fatalf("bare chat: %v", err)
	}
	if _, err := cat.Resolve("m:search", catalog.WireOpenAIResponses); err != nil {
		t.Fatalf("search facet: %v", err)
	}
}

// chat+image on the same wire expand; bare resolves without a modality hint.
func TestExpand_ChatAndImageNoHeader(t *testing.T) {
	doc := catalog.Document{
		Providers: map[string]catalog.Provider{
			"a": {
				CredentialProfile: "a",
				Surfaces: map[string]catalog.Surface{
					"chat": {Protocol: "openai-chat-completions", BaseURL: "http://a"},
				},
			},
			"b": {
				CredentialProfile: "b",
				Surfaces: map[string]catalog.Surface{
					"chat": {Protocol: "openai-chat-completions", BaseURL: "http://b"},
				},
			},
		},
		Models: map[string]catalog.Model{
			"m": {
				Modalities: map[string]catalog.Modality{
					"chat":  {Wire: catalog.WireOpenAIChat, Providers: []catalog.PoolEntry{{ProviderRef: "a"}}},
					"image": {Wire: catalog.WireOpenAIChat, Providers: []catalog.PoolEntry{{ProviderRef: "b"}}},
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
	if plan.Targets[0].ProviderRef != "a" {
		t.Fatalf("bare want provider a, got %+v", plan.Targets[0])
	}
	plan, err = cat.Resolve("m:image", catalog.WireOpenAIChat)
	if err != nil {
		t.Fatal(err)
	}
	if plan.Targets[0].ProviderRef != "b" {
		t.Fatalf("image facet want provider b, got %+v", plan.Targets[0])
	}
}
