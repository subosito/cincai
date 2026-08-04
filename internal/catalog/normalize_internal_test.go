package catalog

import (
	"strings"
	"testing"
)

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
