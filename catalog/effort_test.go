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

func TestApplyEffort_publicIdEndingWithEffortTier(t *testing.T) {
	t.Parallel()
	// Product name ends with an effort token (…-max) but is not a multi-SKU pool.
	// rewriting to …-medium would invent a non-existent model.
	doc := catalog.Document{
		Providers: map[string]catalog.Provider{
			"qwen": {
				CredentialProfile: "qwen",
				Surfaces: map[string]catalog.Surface{
					"chat": {Protocol: "openai-chat-completions", BaseURL: "https://example"},
				},
			},
		},
		Models: map[string]catalog.Model{
			"qwen3.8-max": {
				Modalities: map[string]catalog.Modality{
					"chat": {
						Wire: catalog.WireOpenAIChat,
						Providers: []catalog.PoolEntry{
							{ProviderRef: "qwen", Surface: "chat", Model: "qwen3.8-max"},
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
	if err := cat.MergeMeta(catalog.MetaDocument{
		Models: map[string]catalog.ModelMeta{
			"qwen3.8-max": {
				Efforts:       []string{"minimal", "low", "medium", "high", "xhigh", "max"},
				DefaultEffort: "medium",
				ContextWindow: 1000000,
			},
		},
	}); err != nil {
		t.Fatal(err)
	}
	plan, err := cat.Resolve("qwen3.8-max", catalog.WireOpenAIChat)
	if err != nil {
		t.Fatal(err)
	}
	used, _, err := cat.ApplyEffort("qwen3.8-max", "medium", plan.Targets)
	if err != nil {
		t.Fatal(err)
	}
	if used != "medium" {
		t.Fatalf("used=%q", used)
	}
	if plan.Targets[0].UpstreamModel != "qwen3.8-max" {
		t.Fatalf("must not rewrite qwen3.8-max → …-%s, got %q", used, plan.Targets[0].UpstreamModel)
	}

	// Facet SKUs must keep the same lean upstream (do not rewrite to qwen3.8-medium).
	doc.Models["qwen3.8-max"].Modalities["image"] = catalog.Modality{
		Wire: catalog.WireOpenAIChat,
		Providers: []catalog.PoolEntry{
			{ProviderRef: "qwen", Surface: "chat", Model: "qwen3.8-max"},
		},
	}
	cat, err = catalog.NewFromDocument(doc)
	if err != nil {
		t.Fatal(err)
	}
	if err := cat.MergeMeta(catalog.MetaDocument{
		Models: map[string]catalog.ModelMeta{
			"qwen3.8-max": {
				Efforts:       []string{"minimal", "low", "medium", "high", "xhigh", "max"},
				DefaultEffort: "medium",
				ContextWindow: 1000000,
			},
		},
	}); err != nil {
		t.Fatal(err)
	}
	// Expand facets so Resolve("qwen3.8-max:image") works
	// (NewFromDocument already expands :image from modalities)
	planImg, err := cat.Resolve("qwen3.8-max:image", catalog.WireOpenAIChat)
	if err != nil {
		t.Fatal(err)
	}
	_, nextImg, err := cat.ApplyEffort("qwen3.8-max:image", "medium", planImg.Targets)
	if err != nil {
		t.Fatal(err)
	}
	if len(nextImg) != 1 || nextImg[0].UpstreamModel != "qwen3.8-max" {
		t.Fatalf("facet must not rewrite SKU, got %+v", nextImg)
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
	used, next, err := cat.ApplyEffort("gpt-5.5", "xhigh", plan.Targets)
	if err != nil {
		t.Fatal(err)
	}
	if used != "xhigh" {
		t.Fatalf("used=%q", used)
	}
	if len(next) != 1 || next[0].UpstreamModel != "gpt-5.5" {
		t.Fatalf("body-only model must not rewrite SKU, got %+v", next)
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
	used, next, err := cat.ApplyEffort("example-model", "", plan.Targets)
	if err != nil {
		t.Fatal(err)
	}
	if used != "medium" || len(next) != 1 || next[0].UpstreamModel != "example-model-medium" {
		t.Fatalf("used=%q next=%+v", used, next)
	}

	plan, _ = cat.Resolve("example-model", catalog.WireOpenAIChat)
	used, next, err = cat.ApplyEffort("example-model", "HIGH", plan.Targets)
	if err != nil {
		t.Fatal(err)
	}
	if used != "high" || len(next) != 1 || next[0].UpstreamModel != "example-model-high" {
		t.Fatalf("used=%q next=%+v", used, next)
	}

	plan, _ = cat.Resolve("example-model", catalog.WireOpenAIChat)
	if _, _, err := cat.ApplyEffort("example-model", "banana", plan.Targets); err == nil {
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
	if _, _, err := cat.ApplyEffort("example-model", "high", plan.Targets); err != nil {
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

func TestApplyEffort_cursorSKUTierRewrite(t *testing.T) {
	t.Parallel()
	doc := catalog.Document{
		Providers: map[string]catalog.Provider{
			"cursor": {
				CredentialProfile: "cursor",
				Surfaces: map[string]catalog.Surface{
					"chat": {Protocol: "adapter:cursor", BaseURL: "https://example"},
				},
			},
		},
		Models: map[string]catalog.Model{
			"kimi-k3": {
				Efforts:       []string{"low", "high", "max"},
				DefaultEffort: "max",
				Modalities: map[string]catalog.Modality{
					"chat": {
						Wire: catalog.WireOpenAIChat,
						Providers: []catalog.PoolEntry{
							{
								ProviderRef: "cursor",
								Surface:     "chat",
								Model:       "kimi-k3-max",
								Models: map[string]string{
									"low":  "kimi-k3-low",
									"high": "kimi-k3-high",
									"max":  "kimi-k3-max",
								},
							},
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
	plan, err := cat.Resolve("kimi-k3", catalog.WireOpenAIChat)
	if err != nil {
		t.Fatal(err)
	}
	used, next, err := cat.ApplyEffort("kimi-k3", "high", plan.Targets)
	if err != nil {
		t.Fatal(err)
	}
	if used != "high" {
		t.Fatalf("used=%q", used)
	}
	if len(next) != 1 || next[0].UpstreamModel != "kimi-k3-high" {
		t.Fatalf("upstream=%+v want kimi-k3-high", next)
	}
	used, next, err = cat.ApplyEffort("kimi-k3", "", plan.Targets)
	if err != nil {
		t.Fatal(err)
	}
	if used != "max" || len(next) != 1 || next[0].UpstreamModel != "kimi-k3-max" {
		t.Fatalf("default effort used=%q next=%+v", used, next)
	}
}

func TestApplyEffort_explicitMapBeatsSuffix(t *testing.T) {
	t.Parallel()
	// Map wins even when the pool SKU stem would not suffix-swap to this name.
	doc := catalog.Document{
		Providers: map[string]catalog.Provider{
			"cursor": {
				CredentialProfile: "cursor",
				Surfaces: map[string]catalog.Surface{
					"chat": {Protocol: "adapter:cursor", BaseURL: "https://example"},
				},
			},
		},
		Models: map[string]catalog.Model{
			"glm-5.2": {
				Efforts:       []string{"high", "max"},
				DefaultEffort: "max",
				Modalities: map[string]catalog.Modality{
					"chat": {
						Wire: catalog.WireOpenAIChat,
						Providers: []catalog.PoolEntry{
							{
								ProviderRef: "cursor",
								Surface:     "chat",
								Models: map[string]string{
									"high": "glm-5.2-high",
									"max":  "glm-5.2-max",
								},
							},
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
	if err := catalog.ValidateEffortConfig("glm-5.2", doc.Models["glm-5.2"]); err != nil {
		t.Fatal(err)
	}
	plan, err := cat.Resolve("glm-5.2", catalog.WireOpenAIChat)
	if err != nil {
		t.Fatal(err)
	}
	used, next, err := cat.ApplyEffort("glm-5.2", "high", plan.Targets)
	if err != nil {
		t.Fatal(err)
	}
	if used != "high" || len(next) != 1 || next[0].UpstreamModel != "glm-5.2-high" {
		t.Fatalf("used=%q next=%+v", used, next)
	}
}

func TestValidateEffortConfig_hopModelsKeysMustBeInEfforts(t *testing.T) {
	t.Parallel()
	err := catalog.ValidateEffortConfig("m", catalog.Model{
		Efforts:       []string{"high", "max"},
		DefaultEffort: "max",
		Modalities: map[string]catalog.Modality{
			"chat": {
				Providers: []catalog.PoolEntry{
					{
						ProviderRef: "cursor",
						Models:      map[string]string{"banana": "m-banana"},
					},
				},
			},
		},
	})
	if err == nil {
		t.Fatal("want error: hop models key not in efforts")
	}
}

func TestValidateEffortConfig_modelAndModelsMutuallyExclusive(t *testing.T) {
	t.Parallel()
	err := catalog.ValidateEffortConfig("m", catalog.Model{
		Efforts:       []string{"high", "max"},
		DefaultEffort: "max",
		Modalities: map[string]catalog.Modality{
			"chat": {
				Providers: []catalog.PoolEntry{
					{
						ProviderRef: "cursor",
						Model:       "m-max",
						Models:      map[string]string{"high": "m-high", "max": "m-max"},
					},
				},
			},
		},
	})
	if err == nil {
		t.Fatal("want error: hop model and models are mutually exclusive")
	}
}

func TestValidateEffortConfig_hopModelsMustCoverEfforts(t *testing.T) {
	t.Parallel()
	err := catalog.ValidateEffortConfig("m", catalog.Model{
		Efforts:       []string{"high", "max"},
		DefaultEffort: "max",
		Modalities: map[string]catalog.Modality{
			"chat": {
				Providers: []catalog.PoolEntry{
					{ProviderRef: "cursor", Models: map[string]string{"max": "m-max"}},
				},
			},
		},
	})
	if err == nil {
		t.Fatal("want error: hop models missing high")
	}
}

func TestApplyEffort_defaultEffortIsFirstWhenOmitted(t *testing.T) {
	t.Parallel()
	doc := catalog.Document{
		Providers: map[string]catalog.Provider{
			"cursor": {
				CredentialProfile: "cursor",
				Surfaces: map[string]catalog.Surface{
					"chat": {Protocol: "adapter:cursor", BaseURL: "https://example"},
				},
			},
		},
		Models: map[string]catalog.Model{
			"kimi-k3": {
				Efforts: []string{"low", "high", "max"},
				Modalities: map[string]catalog.Modality{
					"chat": {
						Wire: catalog.WireOpenAIChat,
						Providers: []catalog.PoolEntry{
							{
								ProviderRef: "cursor",
								Surface:     "chat",
								Models: map[string]string{
									"low":  "kimi-k3-low",
									"high": "kimi-k3-high",
									"max":  "kimi-k3-max",
								},
							},
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
	plan, err := cat.Resolve("kimi-k3", catalog.WireOpenAIChat)
	if err != nil {
		t.Fatal(err)
	}
	if plan.Targets[0].UpstreamModel != "kimi-k3-low" {
		t.Fatalf("resolve default sku=%q want kimi-k3-low", plan.Targets[0].UpstreamModel)
	}
	used, next, err := cat.ApplyEffort("kimi-k3", "", plan.Targets)
	if err != nil {
		t.Fatal(err)
	}
	if used != "low" || next[0].UpstreamModel != "kimi-k3-low" {
		t.Fatalf("used=%q next=%+v", used, next)
	}
}

