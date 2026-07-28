package catalog_test

import (
	"testing"

	"github.com/subosito/cincai/catalog"
)

func TestEffortFromBody(t *testing.T) {
	t.Parallel()
	if got := catalog.EffortFromBody([]byte(`{"model":"m","reasoning_effort":"high"}`)); got != "high" {
		t.Fatalf("got %q", got)
	}
	if got := catalog.EffortFromBody([]byte(`{"model":"m","effort":"low"}`)); got != "low" {
		t.Fatalf("got %q", got)
	}
	if got := catalog.EffortFromBody([]byte(`{"reasoning_effort":"medium","effort":"low"}`)); got != "medium" {
		t.Fatalf("got %q", got)
	}
	if got := catalog.EffortFromBody([]byte(`{"model":"m"}`)); got != "" {
		t.Fatalf("got %q", got)
	}
}

func TestApplyEffort_rewritesTierSuffix(t *testing.T) {
	t.Parallel()
	doc := catalog.Document{
		Providers: map[string]catalog.Provider{
			"vendor": {
				CredentialProfile: "vendor",
				Surfaces: map[string]catalog.Surface{
					"chat": {Protocol: "openai-chat-completions", BaseURL: "https://example"},
				},
			},
		},
		Models: map[string]catalog.Model{
			"example-model": {
				Efforts:       []string{"low", "medium", "high"},
				DefaultEffort: "medium",
				Modalities: map[string]catalog.Modality{
					"chat": {
						Wire: catalog.WireOpenAIChat,
						Providers: []catalog.PoolEntry{
							{ProviderRef: "vendor", Surface: "chat", Model: "example-model-medium"},
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
	plan, err := cat.Resolve("example-model", catalog.WireOpenAIChat)
	if err != nil {
		t.Fatal(err)
	}
	// empty → default medium (pool already medium)
	used, err := cat.ApplyEffort("example-model", "", plan.Targets)
	if err != nil {
		t.Fatal(err)
	}
	if used != "medium" || plan.Targets[0].UpstreamModel != "example-model-medium" {
		t.Fatalf("used=%q upstream=%q", used, plan.Targets[0].UpstreamModel)
	}

	plan, _ = cat.Resolve("example-model", catalog.WireOpenAIChat)
	used, err = cat.ApplyEffort("example-model", "HIGH", plan.Targets)
	if err != nil {
		t.Fatal(err)
	}
	if used != "high" || plan.Targets[0].UpstreamModel != "example-model-high" {
		t.Fatalf("used=%q upstream=%q", used, plan.Targets[0].UpstreamModel)
	}

	plan, _ = cat.Resolve("example-model", catalog.WireOpenAIChat)
	if _, err := cat.ApplyEffort("example-model", "banana", plan.Targets); err == nil {
		t.Fatal("want error for unsupported effort")
	}
}

func TestApplyEffort_leavesForeignUpstream(t *testing.T) {
	t.Parallel()
	doc := catalog.Document{
		Providers: map[string]catalog.Provider{
			"alt": {
				CredentialProfile: "alt",
				Surfaces: map[string]catalog.Surface{
					"chat": {Protocol: "openai-chat-completions", BaseURL: "https://example"},
				},
			},
		},
		Models: map[string]catalog.Model{
			"example-model": {
				Efforts:       []string{"low", "medium", "high"},
				DefaultEffort: "medium",
				Modalities: map[string]catalog.Modality{
					"chat": {
						Wire: catalog.WireOpenAIChat,
						Providers: []catalog.PoolEntry{
							{ProviderRef: "alt", Surface: "chat", Model: "vendor/example-model"},
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
	plan, err := cat.Resolve("example-model", catalog.WireOpenAIChat)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := cat.ApplyEffort("example-model", "high", plan.Targets); err != nil {
		t.Fatal(err)
	}
	if plan.Targets[0].UpstreamModel != "vendor/example-model" {
		t.Fatalf("foreign upstream rewritten: %q", plan.Targets[0].UpstreamModel)
	}
}

func TestListModels_includesEfforts(t *testing.T) {
	t.Parallel()
	doc := catalog.Document{
		Providers: map[string]catalog.Provider{
			"vendor": {
				CredentialProfile: "vendor",
				Surfaces: map[string]catalog.Surface{
					"chat": {Protocol: "openai-chat-completions", BaseURL: "https://example"},
				},
			},
		},
		Models: map[string]catalog.Model{
			"example-model": {
				Efforts:       []string{"low", "medium", "high"},
				DefaultEffort: "low",
				Modalities: map[string]catalog.Modality{
					"chat": {
						Wire:      catalog.WireOpenAIChat,
						Providers: []catalog.PoolEntry{{ProviderRef: "vendor", Surface: "chat", Model: "example-model-low"}},
					},
				},
			},
		},
	}
	cat, err := catalog.NewFromDocument(doc)
	if err != nil {
		t.Fatal(err)
	}
	list := cat.ListModels()
	if len(list.Data) != 1 {
		t.Fatalf("data=%+v", list.Data)
	}
	it := list.Data[0]
	if len(it.Efforts) != 3 || it.DefaultEffort != "low" {
		t.Fatalf("item=%+v", it)
	}
}

func TestValidateEffortConfig(t *testing.T) {
	t.Parallel()
	if err := catalog.ValidateEffortConfig("m", catalog.Model{
		DefaultEffort: "high",
		Efforts:       []string{"low", "medium"},
	}); err == nil {
		t.Fatal("want error: default not in efforts")
	}
	if err := catalog.ValidateEffortConfig("m", catalog.Model{
		DefaultEffort: "medium",
		Efforts:       []string{"low", "medium"},
	}); err != nil {
		t.Fatal(err)
	}
	if err := catalog.ValidateEffortConfig("m", catalog.Model{
		DefaultEffort: "medium",
	}); err == nil {
		t.Fatal("want error: default without efforts")
	}
}
