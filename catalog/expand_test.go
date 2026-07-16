package catalog_test

import (
	"testing"

	"github.com/subosito/cincai/catalog"
)

func TestExpandWireCollisions_GrokStyle(t *testing.T) {
	doc := catalog.Document{
		Providers: map[string]catalog.Provider{
			"xai": {
				CredentialProfile: "xai-oauth",
				Surfaces: map[string]catalog.Surface{
					"chat":  {Protocol: "openai-responses", BaseURL: "https://api.x.ai/v1"},
					"image": {Protocol: "openai-responses", BaseURL: "https://api.x.ai/v1"},
				},
			},
		},
		Models: map[string]catalog.Model{
			"grok-4.3": {
				Modalities: map[string]catalog.Modality{
					"chat": {
						Wire:      catalog.WireOpenAIResponses,
						Providers: []catalog.PoolEntry{{ProviderRef: "xai", Surface: "chat"}},
					},
					"image": {
						Wire:      catalog.WireOpenAIResponses,
						Providers: []catalog.PoolEntry{{ProviderRef: "xai", Surface: "image"}},
					},
					"search_web": {
						Wire:      catalog.WireOpenAIResponses,
						Providers: []catalog.PoolEntry{{ProviderRef: "xai", Surface: "chat"}},
					},
					"search_x": {
						Wire:      catalog.WireOpenAIResponses,
						Providers: []catalog.PoolEntry{{ProviderRef: "xai", Surface: "chat"}},
					},
				},
			},
		},
	}
	cat, err := catalog.NewFromDocument(doc)
	if err != nil {
		t.Fatal(err)
	}

	// Bare id = chat only; no header needed.
	if _, err := cat.Resolve("grok-4.3", catalog.WireOpenAIResponses); err != nil {
		t.Fatalf("bare resolve: %v", err)
	}
	// Facets are public model ids.
	for _, id := range []string{"grok-4.3:image", "grok-4.3:search", "grok-4.3:search_x"} {
		if _, err := cat.Resolve(id, catalog.WireOpenAIResponses); err != nil {
			t.Fatalf("resolve %s: %v", id, err)
		}
	}
	// search_web facet is shortened to :search
	if _, err := cat.Resolve("grok-4.3:search_web", catalog.WireOpenAIResponses); err == nil {
		t.Fatal("expected search_web to expand as :search, not :search_web")
	}
}

func TestExpandWireCollisions_DifferentWiresStayNested(t *testing.T) {
	doc := catalog.Document{
		Providers: map[string]catalog.Provider{
			"p": {
				CredentialProfile: "p",
				Surfaces: map[string]catalog.Surface{
					"chat":  {Protocol: "openai-chat-completions", BaseURL: "http://a"},
					"embed": {Protocol: "openai-embeddings", BaseURL: "http://a"},
				},
			},
		},
		Models: map[string]catalog.Model{
			"m": {
				Modalities: map[string]catalog.Modality{
					"chat":  {Wire: catalog.WireOpenAIChat, Providers: []catalog.PoolEntry{{ProviderRef: "p"}}},
					"embed": {Wire: catalog.WireOpenAIEmbed, Providers: []catalog.PoolEntry{{ProviderRef: "p"}}},
				},
			},
		},
	}
	cat, err := catalog.NewFromDocument(doc)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := cat.Resolve("m", catalog.WireOpenAIChat); err != nil {
		t.Fatal(err)
	}
	if _, err := cat.Resolve("m", catalog.WireOpenAIEmbed); err != nil {
		t.Fatal(err)
	}
	// No facet ids created when wires differ.
	if _, err := cat.Resolve("m:embed", catalog.WireOpenAIEmbed); err == nil {
		t.Fatal("did not expect m:embed when wires already disambiguate")
	}
}

func TestExpandWireCollisions_ClashError(t *testing.T) {
	doc := catalog.Document{
		Providers: map[string]catalog.Provider{
			"p": {
				CredentialProfile: "p",
				Surfaces: map[string]catalog.Surface{
					"chat": {Protocol: "openai-chat-completions", BaseURL: "http://a"},
				},
			},
		},
		Models: map[string]catalog.Model{
			"m": {
				Modalities: map[string]catalog.Modality{
					"chat":  {Wire: catalog.WireOpenAIChat, Providers: []catalog.PoolEntry{{ProviderRef: "p"}}},
					"image": {Wire: catalog.WireOpenAIChat, Providers: []catalog.PoolEntry{{ProviderRef: "p"}}},
				},
			},
			"m:image": {
				Modalities: map[string]catalog.Modality{
					"image": {Wire: catalog.WireOpenAIChat, Providers: []catalog.PoolEntry{{ProviderRef: "p"}}},
				},
			},
		},
	}
	if _, err := catalog.NewFromDocument(doc); err == nil {
		t.Fatal("want clash error for pre-existing m:image")
	}
}

func TestFacetFromModality(t *testing.T) {
	if got := catalog.FacetFromModality("search_web"); got != "search" {
		t.Fatalf("got %q", got)
	}
	if got := catalog.FacetFromModality("image"); got != "image" {
		t.Fatalf("got %q", got)
	}
}
