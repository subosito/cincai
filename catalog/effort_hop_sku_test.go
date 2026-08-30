package catalog

import (
	"testing"
)

// Cursor hops after internal/catalog normalize: one PoolEntry with Models.
func TestApplyEffortCursorShorthandHopSKU(t *testing.T) {
	t.Parallel()
	doc := Document{
		Providers: map[string]Provider{
			"cursor": {
				CredentialProfile: "cursor-oauth",
				Surfaces: map[string]Surface{
					"chat": {Adapter: "cursor", BaseURL: "https://api2.cursor.sh"},
				},
			},
		},
		Models: map[string]Model{
			"cs/gemini-3.7-flash": {
				Efforts:       []string{"low", "medium", "high"},
				DefaultEffort: "high",
				Modalities: map[string]Modality{
					"chat": {
						Wire: WireOpenAIChat,
						Providers: []PoolEntry{{
							ProviderRef: "cursor",
							Surface:     "chat",
							Models: map[string]string{
								"low":    "gemini-3.7-flash-low",
								"medium": "gemini-3.7-flash-medium",
								"high":   "gemini-3.7-flash-high",
							},
						}},
					},
				},
			},
		},
	}
	cat, err := NewFromDocument(doc)
	if err != nil {
		t.Fatal(err)
	}
	plan, err := cat.Resolve("cs/gemini-3.7-flash", WireOpenAIChat)
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Targets) != 1 {
		t.Fatalf("targets=%d", len(plan.Targets))
	}
	if got := plan.Targets[0].UpstreamModel; got != "gemini-3.7-flash-high" {
		t.Fatalf("default upstream %q, want gemini-3.7-flash-high", got)
	}
	if len(plan.Targets[0].EffortModels) != 3 {
		t.Fatalf("EffortModels=%v", plan.Targets[0].EffortModels)
	}

	used, next, err := cat.ApplyEffort("cs/gemini-3.7-flash", "high", plan.Targets)
	if err != nil {
		t.Fatal(err)
	}
	if used != "high" {
		t.Fatalf("used=%q", used)
	}
	if got := next[0].UpstreamModel; got != "gemini-3.7-flash-high" {
		t.Fatalf("high sku %q", got)
	}

	used, next, err = cat.ApplyEffort("cs/gemini-3.7-flash", "low", cloneTargets(plan.Targets))
	if err != nil {
		t.Fatal(err)
	}
	if used != "low" || next[0].UpstreamModel != "gemini-3.7-flash-low" {
		t.Fatalf("low used=%q sku=%q", used, next[0].UpstreamModel)
	}
}

func cloneTargets(in []Target) []Target {
	out := make([]Target, len(in))
	copy(out, in)
	return out
}
