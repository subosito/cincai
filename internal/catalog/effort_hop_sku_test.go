package catalog

import (
	"path/filepath"
	"testing"

	corecatalog "github.com/subosito/cincai/catalog"
)

func TestLoadShorthandHopEffortSKU(t *testing.T) {
	t.Parallel()
	cat, err := Load(filepath.Join("testdata", "cursor_effort_hop.yaml"))
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
	if used != "high" || next[0].UpstreamModel != "gemini-3.7-flash-high" {
		t.Fatalf("high used=%q sku=%q", used, next[0].UpstreamModel)
	}

	used, next, err = cat.ApplyEffort("cs/gemini-3.7-flash", "low", append([]corecatalog.Target(nil), plan.Targets...))
	if err != nil {
		t.Fatal(err)
	}
	if used != "low" || next[0].UpstreamModel != "gemini-3.7-flash-low" {
		t.Fatalf("low used=%q sku=%q", used, next[0].UpstreamModel)
	}
}
