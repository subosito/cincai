package catalog_test

import (
	"net/http"
	"testing"

	"github.com/subosito/cincai/catalog"
)

func TestModalityHintFromRequest(t *testing.T) {
	req, _ := http.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	req.Header.Set(catalog.HeaderCatalogModality, "image")
	if got := catalog.ModalityHintFromRequest(req, catalog.WireOpenAIChat); got != "image" {
		t.Fatalf("got %q want image", got)
	}
	if got := catalog.ModalityHintFromRequest(nil, catalog.WireOpenAIEmbed); got != "embed" {
		t.Fatalf("embed wire got %q", got)
	}
	if got := catalog.ModalityHintFromRequest(req, catalog.WireOpenAIImagesGen); got != "" {
		t.Fatalf("images wire got %q want empty", got)
	}
}

func TestPickModalityDefaultsChatOverSearchSiblings(t *testing.T) {
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
	plan, err := cat.Resolve("m", catalog.WireOpenAIResponses)
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Targets) != 1 {
		t.Fatalf("default chat targets=%d", len(plan.Targets))
	}
	plan, err = cat.ResolveWithModality("m", catalog.WireOpenAIResponses, "search_web")
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Targets) != 1 {
		t.Fatalf("search_web targets=%d", len(plan.Targets))
	}
}

func TestResolveRequiresHintForNonSearchAmbiguity(t *testing.T) {
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
	if _, err := cat.Resolve("m", catalog.WireOpenAIChat); err == nil {
		t.Fatal("want error for chat+image without X-Catalog-Modality")
	}
	plan, err := cat.ResolveWithModality("m", catalog.WireOpenAIChat, "image")
	if err != nil {
		t.Fatal(err)
	}
	if plan.Targets[0].ProviderRef != "b" {
		t.Fatalf("target=%+v", plan.Targets[0])
	}
}