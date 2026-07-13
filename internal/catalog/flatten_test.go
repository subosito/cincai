package catalog_test

import (
	"os"
	"path/filepath"
	"testing"

	icatalog "github.com/subosito/cincai/internal/catalog"
)

func TestLoadFlattenedDevCatalog(t *testing.T) {
	path := filepath.Join("..", "..", "config", "providers.yaml.example")
	if _, err := os.Stat(path); err != nil {
		t.Skip("providers.yaml.example missing")
	}
	cat, err := icatalog.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	plan, err := cat.Resolve("eleven_v3", "openai-audio-speech")
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Targets) == 0 {
		t.Fatal("expected targets")
	}
	if plan.Targets[0].Adapter != "elevenlabs" {
		t.Fatalf("adapter=%q want elevenlabs", plan.Targets[0].Adapter)
	}
	if plan.Targets[0].UpstreamModel != "eleven_v3" {
		t.Fatalf("model=%q", plan.Targets[0].UpstreamModel)
	}
}