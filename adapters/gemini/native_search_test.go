package gemini

import (
	"encoding/json"
	"strings"
	"testing"
)

// fnTool builds a plain function tool (Function is an anonymous struct, so it
// cannot be written as a composite literal from here).
func fnTool(name string) OpenAITool {
	var t OpenAITool
	t.Type = "function"
	t.Function.Name = name
	return t
}

// Gemini has no typed "web_search" tool entry: grounding is a google_search
// sibling in the same tools array. Translating here means a client asks for
// provider search one way and the adapter speaks each vendor's dialect,
// instead of every client learning per-vendor shapes.
func TestNativeWebSearchBecomesGoogleSearch(t *testing.T) {
	tests := []struct {
		name       string
		tools      []OpenAITool
		wantSearch bool
		wantFuncs  int
	}{
		{
			name:       "no tools",
			wantSearch: false,
		},
		{
			name:       "web_search alone",
			tools:      []OpenAITool{{Type: "web_search"}},
			wantSearch: true,
		},
		{
			name:       "openai legacy spelling",
			tools:      []OpenAITool{{Type: "web_search_preview"}},
			wantSearch: true,
		},
		{
			name:       "native gemini spelling passes too",
			tools:      []OpenAITool{{Type: "google_search"}},
			wantSearch: true,
		},
		{
			name:      "function tools untouched",
			tools:     []OpenAITool{fnTool("read")},
			wantFuncs: 1,
		},
		{
			name:       "search alongside function tools",
			tools:      []OpenAITool{fnTool("read"), {Type: "web_search"}},
			wantSearch: true,
			wantFuncs:  1,
		},
		{
			name:       "unrelated provider tool is ignored",
			tools:      []OpenAITool{{Type: "x_search"}},
			wantSearch: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			req := OpenAIRequest{
				Model:    "gemini-3.1-pro",
				Messages: []OpenAIMessage{{Role: "user", Content: "hi"}},
				Tools:    tc.tools,
			}
			out, err := FromOpenAI(req)
			if err != nil {
				t.Fatal(err)
			}
			var gotSearch, gotFuncs int
			for _, g := range out.Tools {
				if g.GoogleSearch != nil {
					gotSearch++
				}
				gotFuncs += len(g.FunctionDeclarations)
			}
			if (gotSearch > 0) != tc.wantSearch {
				t.Fatalf("google_search present=%v want=%v (%+v)", gotSearch > 0, tc.wantSearch, out.Tools)
			}
			if gotFuncs != tc.wantFuncs {
				t.Fatalf("function decls = %d want %d", gotFuncs, tc.wantFuncs)
			}

			// Wire check: google_search must serialize as an empty object, and
			// an absent one must not emit a null field.
			raw, err := json.Marshal(out)
			if err != nil {
				t.Fatal(err)
			}
			if tc.wantSearch && !strings.Contains(string(raw), `"google_search":{}`) {
				t.Fatalf("google_search not serialized: %s", raw)
			}
			if !tc.wantSearch && strings.Contains(string(raw), "google_search") {
				t.Fatalf("google_search leaked when unasked: %s", raw)
			}
		})
	}
}
