package xai

import (
	"strings"

	icatalog "github.com/subosito/cincai/internal/catalog"
	"github.com/subosito/cincai/internal/catalog/fields"
)

// Surfaces maps xAI hosts to chat passthrough and responses ingress.
func Surfaces(baseURL string) map[string]any {
	return map[string]any{
		"chat": map[string]any{
			"protocol": "openai-chat-completions",
			"base_url": baseURL,
		},
		"responses": map[string]any{
			"protocol": "openai-responses",
			"base_url": baseURL,
		},
	}
}

// ImageSurfaces returns image-gen routing for xAI-backed models.
func ImageSurfaces(baseURL string) map[string]any {
	return map[string]any{
		"image_gen": map[string]any{
			"adapter":  "xai",
			"base_url": baseURL,
		},
	}
}

// NormalizeProvider expands flat xAI provider rows.
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
	return strings.Contains(u, "api.x.ai")
}

func providerBase(entry map[string]any) map[string]any {
	out := map[string]any{}
	if cp := fields.String(entry["credential_profile"]); cp != "" {
		out["credential_profile"] = cp
	}
	icatalog.CopyInjectFields(entry, out)
	return out
}