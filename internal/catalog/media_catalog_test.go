package catalog_test

import (
	"os"
	"path/filepath"
	"testing"

	corecatalog "github.com/subosito/cincai/catalog"

	icatalog "github.com/subosito/cincai/internal/catalog"
)

func TestXAIImageCatalog(t *testing.T) {
	path := filepath.Join("..", "..", "config", "providers.yaml.example")
	if _, err := os.Stat(path); err != nil {
		t.Skip("providers.yaml.example missing")
	}
	cat, err := icatalog.Load(path)
	if err != nil {
		t.Fatal(err)
	}

	plan, err := cat.Resolve("grok-imagine-image-quality", corecatalog.WireOpenAIImagesGen)
	if err != nil {
		t.Fatalf("grok imagine: %v", err)
	}
	if plan.Targets[0].Adapter != "xai" {
		t.Fatalf("adapter=%q", plan.Targets[0].Adapter)
	}
	if plan.Targets[0].ProviderRef != "xai" {
		t.Fatalf("provider=%q", plan.Targets[0].ProviderRef)
	}
}

func TestElevenLabsSpeechCatalog(t *testing.T) {
	path := filepath.Join("..", "..", "config", "providers.yaml.example")
	cat, err := icatalog.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	plan, err := cat.Resolve("eleven_v3", corecatalog.WireOpenAIAudioSpeech)
	if err != nil {
		t.Fatalf("eleven_v3 speech: %v", err)
	}
	if plan.Targets[0].Adapter != "elevenlabs" {
		t.Fatalf("adapter=%q", plan.Targets[0].Adapter)
	}
	if plan.Targets[0].ProviderRef != "elevenlabs" {
		t.Fatalf("provider=%q", plan.Targets[0].ProviderRef)
	}
	if plan.Targets[0].UpstreamModel != "eleven_v3" {
		t.Fatalf("upstream_model=%q", plan.Targets[0].UpstreamModel)
	}
}

func TestXAIVideoCatalog(t *testing.T) {
	path := filepath.Join("..", "..", "config", "providers.yaml.example")
	cat, err := icatalog.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	plan, err := cat.Resolve("grok-imagine-video", corecatalog.WireOpenAIVideos)
	if err != nil {
		t.Fatalf("grok video: %v", err)
	}
	if plan.Targets[0].Adapter != "" {
		t.Fatalf("adapter=%q want passthrough", plan.Targets[0].Adapter)
	}
	if plan.Targets[0].Protocol != "openai-videos" {
		t.Fatalf("protocol=%q", plan.Targets[0].Protocol)
	}
	if plan.Targets[0].ProviderRef != "xai" {
		t.Fatalf("provider=%q", plan.Targets[0].ProviderRef)
	}
	if plan.Targets[0].BaseURL != "https://api.x.ai/v1/videos" {
		t.Fatalf("base_url=%q", plan.Targets[0].BaseURL)
	}
}
