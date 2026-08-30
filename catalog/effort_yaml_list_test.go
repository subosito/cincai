package catalog

import (
	"os"
	"path/filepath"
	"testing"
)

func TestApplyEffortYAMLListHopSKU(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "providers.yaml")
	src := []byte(`
providers:
  cursor:
    credential_profile: cursor-oauth
    surfaces:
      chat:
        adapter: cursor
        base_url: https://api2.cursor.sh
models:
  cs/claude-sonnet-5:
    efforts: [low, medium, high]
    default_effort: medium
    modalities:
      chat:
        wire: openai-chat-completions
        provider_ref: cursor
        models:
          - model: claude-sonnet-5-low
            effort: low
          - model: claude-sonnet-5-medium
            effort: medium
          - model: claude-sonnet-5-high
            effort: high
        surface: chat
`)
	if err := os.WriteFile(path, src, 0o644); err != nil {
		t.Fatal(err)
	}
	cat, err := Load(path)
	if err != nil {
		t.Fatalf("Load list hops: %v", err)
	}
	plan, err := cat.Resolve("cs/claude-sonnet-5", WireOpenAIChat)
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Targets) != 1 {
		t.Fatalf("targets=%d", len(plan.Targets))
	}
	if got := plan.Targets[0].UpstreamModel; got != "claude-sonnet-5-medium" {
		t.Fatalf("default upstream %q, want claude-sonnet-5-medium", got)
	}
	if n := len(plan.Targets[0].EffortModels); n != 3 {
		t.Fatalf("EffortModels=%v (want 3; YAML list hops must normalize to a map)", plan.Targets[0].EffortModels)
	}

	used, next, err := cat.ApplyEffort("cs/claude-sonnet-5", "high", cloneTargets(plan.Targets))
	if err != nil {
		t.Fatal(err)
	}
	if used != "high" {
		t.Fatalf("used=%q", used)
	}
	if got := next[0].UpstreamModel; got != "claude-sonnet-5-high" {
		t.Fatalf("high sku %q, want claude-sonnet-5-high", got)
	}
}
