package catalog

import (
	"strings"
	"testing"
)

func TestNormalizeRoute_keepsHopEffortModels(t *testing.T) {
	out := normalizeModalityRoute(map[string]any{
		"wire":         "openai-chat-completions",
		"provider_ref": "cursor",
		"model":        "kimi-k3-max",
		"models": map[string]any{
			"high": "kimi-k3-high",
			"max":  "kimi-k3-max",
		},
	}, "chat", "chat")
	pool, ok := out["providers"].([]any)
	if !ok || len(pool) != 1 {
		t.Fatalf("providers=%v", out["providers"])
	}
	entry, ok := pool[0].(map[string]any)
	if !ok {
		t.Fatalf("entry=%T", pool[0])
	}
	skus, ok := entry["models"].(map[string]any)
	if !ok || skus["high"] != "kimi-k3-high" || skus["max"] != "kimi-k3-max" {
		t.Fatalf("hop models=%v", entry["models"])
	}
	if entry["model"] != "kimi-k3-max" {
		t.Fatalf("model=%v", entry["model"])
	}
}

func TestNormalizeRoute_hopEffortModelsList(t *testing.T) {
	out := normalizeModalityRoute(map[string]any{
		"wire":         "openai-chat-completions",
		"provider_ref": "cursor",
		"models": []any{
			map[string]any{"model": "gemini-3.7-flash-low", "effort": "low"},
			map[string]any{"model": "gemini-3.7-flash-medium", "effort": "medium"},
			map[string]any{"model": "gemini-3.7-flash-high", "effort": "high"},
		},
		"surface": "chat",
	}, "chat", "chat")
	pool, ok := out["providers"].([]any)
	if !ok || len(pool) != 1 {
		t.Fatalf("providers=%v", out["providers"])
	}
	entry := pool[0].(map[string]any)
	skus, ok := entry["models"].(map[string]any)
	if !ok || skus["low"] != "gemini-3.7-flash-low" || skus["high"] != "gemini-3.7-flash-high" {
		t.Fatalf("hop models=%v", entry["models"])
	}
	if _, ok := out["models"]; ok {
		t.Fatalf("must not treat SKU list as composite models: %v", out["models"])
	}
}

// Removed search facet keys must fail catalog load loudly (they once selected
// a route without enabling search); provider search is a client-declared tool
// on the bare model id now.
func TestNormalizeModelSpecRemovedSearchKeys(t *testing.T) {
	for _, key := range []string{"search_web", "search_x"} {
		_, err := normalizeModelSpec(map[string]any{
			"modalities": map[string]any{
				"chat": map[string]any{"provider_ref": "xai"},
				key:    map[string]any{"provider_ref": "xai"},
			},
		})
		if err == nil {
			t.Fatalf("%s: want load error, got nil", key)
		}
		for _, want := range []string{key, "modality removed", "bare model id"} {
			if !strings.Contains(err.Error(), want) {
				t.Fatalf("%s: error %q missing %q", key, err, want)
			}
		}
	}
}
