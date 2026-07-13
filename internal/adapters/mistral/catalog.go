package mistral

import (
	"strings"

	icatalog "github.com/subosito/cincai/internal/catalog"
	"github.com/subosito/cincai/internal/catalog/fields"
)

// Surfaces maps Mistral hosts to chat (openai compat) and ocr.
func Surfaces(baseURL string) map[string]any {
	return map[string]any{
		"chat": map[string]any{
			"protocol": "openai-chat-completions",
			"base_url": baseURL,
		},
		"ocr": map[string]any{
			"adapter":  "mistral",
			"base_url": baseURL,
		},
	}
}

// NormalizeProvider expands flat Mistral provider rows.
func NormalizeProvider(entry map[string]any) (map[string]any, bool) {
	baseURL := fields.FirstNonEmpty(fields.String(entry["base_url"]), fields.String(entry["url"]))
	if baseURL == "" || !CompatibleBase(baseURL) {
		return nil, false
	}
	out := providerBase(entry)
	out["surfaces"] = Surfaces(baseURL)
	return out, true
}

func CompatibleBase(baseURL string) bool {
	u := strings.ToLower(strings.TrimSpace(baseURL))
	return strings.Contains(u, "mistral.ai") || strings.Contains(u, "mistral")
}

func providerBase(entry map[string]any) map[string]any {
	out := map[string]any{}
	if cp := fields.String(entry["credential_profile"]); cp != "" {
		out["credential_profile"] = cp
	}
	icatalog.CopyInjectFields(entry, out)
	return out
}
