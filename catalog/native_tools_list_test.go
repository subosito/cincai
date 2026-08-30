package catalog_test

import (
	"encoding/json"
	"testing"

	"github.com/subosito/cincai/catalog"
)

// Provider-executed tool capability belongs to the model, and the gateway
// already knows it. Publishing it on /v1/models is what lets clients stop
// repeating the same list in their own config — and stops a client declaring
// a tool the model cannot actually run.
func TestModelsListPublishesNativeTools(t *testing.T) {
	doc := catalog.Document{
		Providers: map[string]catalog.Provider{
			"p": {Surfaces: map[string]catalog.Surface{
				"chat": {Protocol: "openai-responses", BaseURL: "https://example.invalid/v1"},
			}},
		},
		Models: map[string]catalog.Model{
			"searcher": {
				NativeTools: []map[string]any{{"type": "web_search"}, {"type": "x_search"}},
				Modalities: map[string]catalog.Modality{
					"chat": {Wire: catalog.WireOpenAIResponses, Providers: []catalog.PoolEntry{{ProviderRef: "p"}}},
				},
			},
			"plain": {
				Modalities: map[string]catalog.Modality{
					"chat": {Wire: catalog.WireOpenAIResponses, Providers: []catalog.PoolEntry{{ProviderRef: "p"}}},
				},
			},
		},
	}
	cat, err := catalog.NewFromDocument(doc)
	if err != nil {
		t.Fatal(err)
	}

	byID := map[string]catalog.ModelListItem{}
	for _, item := range cat.ListModels().Data {
		byID[item.ID] = item
	}

	got, ok := byID["searcher"]
	if !ok {
		t.Fatalf("searcher missing from list: %v", byID)
	}
	if len(got.NativeTools) != 2 {
		t.Fatalf("native_tools not published: %#v", got.NativeTools)
	}
	if got.NativeTools[0]["type"] != "web_search" {
		t.Fatalf("declaration mangled: %#v", got.NativeTools[0])
	}

	if !got.SupportsNativeTool("web_search") {
		t.Fatalf("expected SupportsNativeTool(web_search) to be true")
	}
	if !got.SupportsNativeTool("x_search") {
		t.Fatalf("expected SupportsNativeTool(x_search) to be true")
	}
	if got.SupportsNativeTool("other") {
		t.Fatalf("expected SupportsNativeTool(other) to be false")
	}

	// A model without the capability must not advertise one, and the key must
	// be absent rather than null so plain clients see nothing at all.
	plain, ok := byID["plain"]
	if !ok {
		t.Fatal("plain missing from list")
	}
	if plain.SupportsNativeTool("web_search") {
		t.Fatalf("plain model should not support web_search")
	}
	if len(plain.NativeTools) != 0 {
		t.Fatalf("plain model claims tools: %#v", plain.NativeTools)
	}
	raw, err := json.Marshal(plain)
	if err != nil {
		t.Fatal(err)
	}
	if string(raw) != "" && contains(string(raw), "native_tools") {
		t.Fatalf("empty native_tools serialized: %s", raw)
	}
}

func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
