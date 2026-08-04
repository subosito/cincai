package catalog

import (
	"strings"
	"testing"
)

func TestSearchFacetWarnings(t *testing.T) {
	tests := []struct {
		name   string
		models map[string]Model
		want   int // expected warning count
		substr string
	}{
		{
			name:   "no search modalities",
			models: map[string]Model{"m": {Modalities: map[string]Modality{"chat": {Wire: WireOpenAIChat}}}},
			want:   0,
		},
		{
			name:   "search_web warns",
			models: map[string]Model{"grok-4.3:search": {Modalities: map[string]Modality{"search_web": {Wire: WireOpenAIResponses}}}},
			want:   1,
			substr: "routing alias only",
		},
		{
			name: "search_x warns",
			models: map[string]Model{
				"grok-4.3:search_x": {Modalities: map[string]Modality{"search_x": {Wire: WireOpenAIResponses}}},
			},
			want:   1,
			substr: `tools:[{"type":"web_search"}]`,
		},
		{
			name: "both facets warn",
			models: map[string]Model{
				"grok-4.3:search":   {Modalities: map[string]Modality{"search_web": {Wire: WireOpenAIResponses}}},
				"grok-4.3:search_x": {Modalities: map[string]Modality{"search_x": {Wire: WireOpenAIResponses}}},
			},
			want:   2,
			substr: "unexecuted search call",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := searchFacetWarnings(tt.models)
			if len(got) != tt.want {
				t.Fatalf("got %d warnings, want %d: %v", len(got), tt.want, got)
			}
			for _, w := range got {
				if tt.substr != "" && !strings.Contains(w, tt.substr) {
					t.Fatalf("warning missing %q: %s", tt.substr, w)
				}
			}
		})
	}
}
