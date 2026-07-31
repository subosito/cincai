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
	if got := catalog.EffortFromBody([]byte(`{"model":"m","reasoning":{"effort":"xhigh"}}`)); got != "xhigh" {
		t.Fatalf("nested reasoning.effort got %q", got)
	}
	if got := catalog.EffortFromBody([]byte(`{"model":"m"}`)); got != "" {
		t.Fatalf("got %q", got)
	}
}

func TestApplyEffort_bodyOnlyNoSKURewrite(t *testing.T) {
	t.Parallel()
	// OpenAI-style lean id: efforts are advertised but upstream model must not become gpt-5.5-high.
	doc := catalog.Document{
		Providers: map[string]catalog.Provider{
			"codex": {
				CredentialProfile: "codex",
				Surfaces: map[string]catalog.Surface{
					"chat": {Protocol: "openai-chat-completions", BaseURL: "https://example"},
				},
			},
		},
		Models: map[string]catalog.Model{
			"gpt-5.5": {
				Modalities: map[string]catalog.Modality{
					"chat": {
						Wire:      catalog.WireOpenAIChat,
						Providers: []catalog.PoolEntry{{ProviderRef: "codex", Surface: "chat", Model: "gpt-5.5"}},
					},
				},
			},
		},
	}
	cat, err := catalog.NewFromDocument(doc)
	if err != nil {
		t.Fatal(err)
	}
	if err := cat.MergeMeta(catalog.MetaDocument{
		Models: map[string]catalog.ModelMeta{
			"gpt-5.5": {
				Efforts:       []string{"none", "low", "medium", "high", "xhigh"},
				DefaultEffort: "medium",
				ContextWindow: 200000,
			},
		},
	}); err != nil {
		t.Fatal(err)
	}
	plan, err := cat.Resolve("gpt-5.5", catalog.WireOpenAIChat)
	if err != nil {
		t.Fatal(err)
	}
	used, err := cat.ApplyEffort("gpt-5.5", "xhigh", plan.Targets)
	if err != nil {
		t.Fatal(err)
	}
	if used != "xhigh" {
		t.Fatalf("used=%q", used)
	}
	if plan.Targets[0].UpstreamModel != "gpt-5.5" {
		t.Fatalf("body-only model must not rewrite SKU, got %q", plan.Targets[0].UpstreamModel)
	}
	list := cat.ListModels()
	if len(list.Data) != 1 || len(list.Data[0].Efforts) != 5 || list.Data[0].DefaultEffort != "medium" {
		t.Fatalf("list item=%+v", list.Data[0])
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
