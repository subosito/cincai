package catalog_test

import (
	"strings"
	"testing"

	"github.com/subosito/cincai/catalog"
)

func chainDoc() catalog.Document {
	return catalog.Document{
		Providers: map[string]catalog.Provider{
			"codex": {
				CredentialProfile: "codex",
				Surfaces: map[string]catalog.Surface{
					"chat": {Protocol: "openai-chat-completions", BaseURL: "https://codex.example"},
				},
			},
			"ag": {
				CredentialProfile: "ag",
				Surfaces: map[string]catalog.Surface{
					"chat": {Protocol: "openai-chat-completions", BaseURL: "https://ag.example"},
				},
			},
			"qwen": {
				CredentialProfile: "qwen",
				Surfaces: map[string]catalog.Surface{
					"chat": {Protocol: "openai-chat-completions", BaseURL: "https://qwen.example"},
				},
			},
		},
		Models: map[string]catalog.Model{
			"gpt-luna": {
				Modalities: map[string]catalog.Modality{
					"chat": {
						Wire: catalog.WireOpenAIChat,
						Providers: []catalog.PoolEntry{
							{ProviderRef: "codex", Surface: "chat", Model: "gpt-luna"},
						},
					},
				},
			},
			"gemini-flash": {
				Modalities: map[string]catalog.Modality{
					"chat": {
						Wire: catalog.WireOpenAIChat,
						Providers: []catalog.PoolEntry{
							{ProviderRef: "ag", Surface: "chat", Model: "gemini-flash-medium"},
						},
					},
				},
			},
			"qwen-plus": {
				Modalities: map[string]catalog.Modality{
					"chat": {
						Wire: catalog.WireOpenAIChat,
						Strategy: catalog.StrategyFailover,
						Providers: []catalog.PoolEntry{
							{ProviderRef: "qwen", Surface: "chat", Model: "qwen-plus"},
							{ProviderRef: "ag", Surface: "chat", Model: "qwen/plus"},
						},
					},
				},
			},
			"agent-cheap": {
				Modalities: map[string]catalog.Modality{
					"chat": {
						Wire:     catalog.WireOpenAIChat,
						Strategy: catalog.StrategyFailover,
						Models:   []string{"gpt-luna", "gemini-flash", "qwen-plus"},
					},
				},
			},
		},
	}
}

func TestResolve_modelChainFlattensHops(t *testing.T) {
	t.Parallel()
	cat, err := catalog.NewFromDocument(chainDoc())
	if err != nil {
		t.Fatal(err)
	}
	plan, err := cat.Resolve("agent-cheap", catalog.WireOpenAIChat)
	if err != nil {
		t.Fatal(err)
	}
	if plan.Strategy != catalog.StrategyFailover {
		t.Fatalf("strategy=%q", plan.Strategy)
	}
	wantModels := []string{"gpt-luna", "gemini-flash", "qwen-plus"}
	if strings.Join(plan.Models, ",") != strings.Join(wantModels, ",") {
		t.Fatalf("plan.Models=%v want %v", plan.Models, wantModels)
	}
	// luna(1) + gemini(1) + qwen failover(2) = 4 targets
	if len(plan.Targets) != 4 {
		t.Fatalf("targets=%d %+v", len(plan.Targets), plan.Targets)
	}
	if plan.Targets[0].Model != "gpt-luna" || plan.Targets[0].ProviderRef != "codex" {
		t.Fatalf("t0=%+v", plan.Targets[0])
	}
	if plan.Targets[1].Model != "gemini-flash" || plan.Targets[1].UpstreamModel != "gemini-flash-medium" {
		t.Fatalf("t1=%+v", plan.Targets[1])
	}
	if plan.Targets[2].ProviderRef != "qwen" || plan.Targets[3].ProviderRef != "ag" {
		t.Fatalf("qwen pool order: %s %s", plan.Targets[2].ProviderRef, plan.Targets[3].ProviderRef)
	}
}

func TestResolve_modelChainXorProviders(t *testing.T) {
	t.Parallel()
	doc := chainDoc()
	doc.Models["bad"] = catalog.Model{
		Modalities: map[string]catalog.Modality{
			"chat": {
				Wire: catalog.WireOpenAIChat,
				Providers: []catalog.PoolEntry{
					{ProviderRef: "codex", Surface: "chat", Model: "x"},
				},
				Models: []string{"gpt-luna"},
			},
		},
	}
	cat, err := catalog.NewFromDocument(doc)
	if err != nil {
		t.Fatal(err)
	}
	_, err = cat.Resolve("bad", catalog.WireOpenAIChat)
	if err == nil || !strings.Contains(err.Error(), "not both") {
		t.Fatalf("want xor error, got %v", err)
	}
}

func TestResolve_modelChainCycle(t *testing.T) {
	t.Parallel()
	doc := chainDoc()
	doc.Models["loop-a"] = catalog.Model{
		Modalities: map[string]catalog.Modality{
			"chat": {Wire: catalog.WireOpenAIChat, Models: []string{"loop-b"}},
		},
	}
	doc.Models["loop-b"] = catalog.Model{
		Modalities: map[string]catalog.Modality{
			"chat": {Wire: catalog.WireOpenAIChat, Models: []string{"loop-a"}},
		},
	}
	cat, err := catalog.NewFromDocument(doc)
	if err != nil {
		t.Fatal(err)
	}
	_, err = cat.Resolve("loop-a", catalog.WireOpenAIChat)
	if err == nil || !strings.Contains(err.Error(), "cycle") {
		t.Fatalf("want cycle error, got %v", err)
	}
}

func TestResolve_modelChainUnknownHop(t *testing.T) {
	t.Parallel()
	doc := chainDoc()
	doc.Models["orphan"] = catalog.Model{
		Modalities: map[string]catalog.Modality{
			"chat": {Wire: catalog.WireOpenAIChat, Models: []string{"no-such-model"}},
		},
	}
	cat, err := catalog.NewFromDocument(doc)
	if err != nil {
		t.Fatal(err)
	}
	_, err = cat.Resolve("orphan", catalog.WireOpenAIChat)
	if err == nil || !strings.Contains(err.Error(), "unknown model") {
		t.Fatalf("want unknown hop, got %v", err)
	}
}

func TestListModels_includesModelChain(t *testing.T) {
	t.Parallel()
	cat, err := catalog.NewFromDocument(chainDoc())
	if err != nil {
		t.Fatal(err)
	}
	list := cat.ListModels()
	var found bool
	for _, it := range list.Data {
		if it.ID != "agent-cheap" {
			continue
		}
		found = true
		if strings.Join(it.Models, ",") != "gpt-luna,gemini-flash,qwen-plus" {
			t.Fatalf("list models=%v", it.Models)
		}
	}
	if !found {
		t.Fatal("agent-cheap missing from list")
	}
	// leaf omits models field (empty)
	for _, it := range list.Data {
		if it.ID == "gpt-luna" && len(it.Models) != 0 {
			t.Fatalf("leaf should omit models, got %v", it.Models)
		}
	}
}

func TestValidateRoutes_modelChain(t *testing.T) {
	t.Parallel()
	cat, err := catalog.NewFromDocument(chainDoc())
	if err != nil {
		t.Fatal(err)
	}
	if err := cat.ValidateRoutes(); err != nil {
		t.Fatal(err)
	}
}
