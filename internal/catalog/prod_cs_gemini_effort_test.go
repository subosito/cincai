package catalog

import (
	"testing"

	corecatalog "github.com/subosito/cincai/catalog"
)

// Live /app/chacha catalog: cs/gemini-3.7-flash must send Cursor SKUs, not the
// public id (Cursor maps an unknown/public id to gemini-3.7-flash-medium).
func TestProdCsGeminiEffortSKUs(t *testing.T) {
	t.Parallel()
	cat, err := Load("/app/chacha/config/providers.yaml")
	if err != nil {
		t.Fatal(err)
	}
	plan, err := cat.Resolve("cs/gemini-3.7-flash", corecatalog.WireOpenAIChat)
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Targets) != 1 {
		t.Fatalf("targets=%d", len(plan.Targets))
	}
	if plan.Targets[0].ProviderRef != "cursor" {
		t.Fatalf("provider=%q", plan.Targets[0].ProviderRef)
	}
	if got := plan.Targets[0].UpstreamModel; got != "gemini-3.7-flash-high" {
		t.Fatalf("default upstream %q (Cursor will show this SKU)", got)
	}
	if len(plan.Targets[0].EffortModels) != 3 {
		t.Fatalf("EffortModels=%v — hop map missing, ApplyEffort cannot remap", plan.Targets[0].EffortModels)
	}

	for effort, want := range map[string]string{
		"low":    "gemini-3.7-flash-low",
		"medium": "gemini-3.7-flash-medium",
		"high":   "gemini-3.7-flash-high",
	} {
		used, next, err := cat.ApplyEffort("cs/gemini-3.7-flash", effort, append([]corecatalog.Target(nil), plan.Targets...))
		if err != nil {
			t.Fatalf("%s: %v", effort, err)
		}
		if used != effort || next[0].UpstreamModel != want {
			t.Fatalf("%s used=%q sku=%q want %q", effort, used, next[0].UpstreamModel, want)
		}
	}
}
