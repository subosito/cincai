package mistral

import (
	"strings"
	"testing"
)

func TestInlineOcrTables_ReplacesMarkdownLinks(t *testing.T) {
	md := "Intro\n\n[tbl-0](tbl-0.markdown)\n\nOutro"
	tables := []any{
		map[string]any{
			"id":      "tbl-0.markdown",
			"format":  "markdown",
			"content": "| A | B |\n| --- | --- |\n| 1 | 2 |",
		},
	}
	got := inlineOcrTables(md, tables)
	want := "Intro\n\n| A | B |\n| --- | --- |\n| 1 | 2 |\n\nOutro"
	if got != want {
		t.Fatalf("got:\n%s\n\nwant:\n%s", got, want)
	}
}

func TestInlineOcrTables_AppendsUnreferenced(t *testing.T) {
	md := "Only text"
	tables := []any{
		map[string]any{"id": "tbl-1.html", "content": "<table><tr><td>x</td></tr></table>"},
	}
	got := inlineOcrTables(md, tables)
	want := "Only text\n\n<table><tr><td>x</td></tr></table>"
	if got != want {
		t.Fatalf("got:\n%s\n\nwant:\n%s", got, want)
	}
}

func TestRenderOcrPage_HeaderFooterAndTables(t *testing.T) {
	page := map[string]any{
		"header":   "Page header",
		"markdown": "Body [tbl-2](tbl-2.markdown)",
		"footer":   "Page footer",
		"tables": []any{
			map[string]any{"id": "tbl-2.markdown", "content": "| x |"},
		},
	}
	got := renderOcrPage(page)
	want := "Page header\n\nBody | x |\n\nPage footer"
	if got != want {
		t.Fatalf("got:\n%s\n\nwant:\n%s", got, want)
	}
}

func TestExtractOcrMarkdown_MultiPage(t *testing.T) {
	data := map[string]any{
		"pages": []any{
			map[string]any{"markdown": "page one"},
			map[string]any{"markdown": "page two"},
		},
	}
	got := extractOcrMarkdown(data)
	if !strings.Contains(got, "<!-- page 1 -->") || !strings.Contains(got, "page two") {
		t.Fatalf("unexpected multi-page output: %q", got)
	}
}