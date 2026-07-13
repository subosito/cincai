package mistral

import (
	"fmt"
	"regexp"
	"strings"
)

// ocrPassthroughKeys are forwarded from chat completion bodies to Mistral /v1/ocr.
var ocrPassthroughKeys = []string{
	"pages",
	"include_image_base64",
	"image_limit",
	"image_min_size",
	"table_format",
	"extract_header",
	"extract_footer",
	"bbox_annotation_format",
	"document_annotation_format",
	"document_annotation_prompt",
	"confidence_scores_granularity",
}

var ocrTableLinkRE = regexp.MustCompile(`!?\[[^\]]*\]\(([^)]+)\)`)

func copyOcrPassthrough(dst, src map[string]any) {
	for _, key := range ocrPassthroughKeys {
		if v, ok := src[key]; ok {
			dst[key] = v
		}
	}
}

func extractOcrMarkdown(ocrData map[string]any) string {
	pages, ok := ocrData["pages"].([]any)
	if !ok || len(pages) == 0 {
		return ""
	}
	var b strings.Builder
	for i, p := range pages {
		page, ok := p.(map[string]any)
		if !ok {
			continue
		}
		md := renderOcrPage(page)
		if strings.TrimSpace(md) == "" {
			continue
		}
		if b.Len() > 0 {
			b.WriteString("\n\n")
		}
		if len(pages) > 1 {
			b.WriteString(fmt.Sprintf("<!-- page %d -->\n", i+1))
		}
		b.WriteString(md)
	}
	return strings.TrimSpace(b.String())
}

func renderOcrPage(page map[string]any) string {
	var parts []string
	if header := ocrTextField(page["header"]); header != "" {
		parts = append(parts, header)
	}
	md, _ := page["markdown"].(string)
	md = inlineOcrTables(md, page["tables"])
	if strings.TrimSpace(md) != "" {
		parts = append(parts, md)
	}
	if footer := ocrTextField(page["footer"]); footer != "" {
		parts = append(parts, footer)
	}
	return strings.TrimSpace(strings.Join(parts, "\n\n"))
}

func ocrTextField(v any) string {
	switch t := v.(type) {
	case string:
		return strings.TrimSpace(t)
	case nil:
		return ""
	default:
		return strings.TrimSpace(fmt.Sprint(t))
	}
}

func inlineOcrTables(md string, rawTables any) string {
	tables, ok := rawTables.([]any)
	if !ok || len(tables) == 0 {
		return md
	}
	byID := make(map[string]string, len(tables))
	var ordered []string
	for _, raw := range tables {
		tbl, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		id := strings.TrimSpace(ocrTableID(tbl))
		content := strings.TrimSpace(ocrTableContent(tbl))
		if id == "" || content == "" {
			continue
		}
		byID[id] = content
		ordered = append(ordered, id)
	}
	if len(byID) == 0 {
		return md
	}

	referenced := make(map[string]bool)
	if strings.TrimSpace(md) != "" {
		md = ocrTableLinkRE.ReplaceAllStringFunc(md, func(match string) string {
			sub := ocrTableLinkRE.FindStringSubmatch(match)
			if len(sub) < 2 {
				return match
			}
			id := strings.TrimSpace(sub[1])
			if content, ok := byID[id]; ok {
				referenced[id] = true
				return content
			}
			return match
		})
	}

	var extras []string
	for _, id := range ordered {
		if referenced[id] {
			continue
		}
		extras = append(extras, byID[id])
	}
	if len(extras) == 0 {
		return md
	}
	extraBlock := strings.Join(extras, "\n\n")
	if strings.TrimSpace(md) == "" {
		return extraBlock
	}
	return md + "\n\n" + extraBlock
}

func ocrTableID(tbl map[string]any) string {
	if id, ok := tbl["id"].(string); ok && strings.TrimSpace(id) != "" {
		return id
	}
	return strings.TrimSpace(fmt.Sprint(tbl["id"]))
}

func ocrTableContent(tbl map[string]any) string {
	if content, ok := tbl["content"].(string); ok {
		return content
	}
	if content, ok := tbl["markdown"].(string); ok {
		return content
	}
	if content, ok := tbl["html"].(string); ok {
		return content
	}
	return ""
}