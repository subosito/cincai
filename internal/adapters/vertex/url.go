package vertex

import (
	"fmt"
	"net/url"
	"strings"
)

// splitModel splits "google/gemini-3.6-flash" or bare "gemini-3.6-flash" into publisher + name.
func splitModel(model string) (publisher, name string) {
	model = strings.TrimSpace(model)
	if model == "" {
		return "google", ""
	}
	if i := strings.Index(model, "/"); i >= 0 {
		return model[:i], model[i+1:]
	}
	return "google", model
}

// generateContentURL builds:
//
//	{base}/v1/publishers/{publisher}/models/{model}:generateContent
//
// If base already ends with /v1, an extra /v1 is not duplicated beyond Join-style trim.
func generateContentURL(base, model string, stream bool) (string, error) {
	publisher, name := splitModel(model)
	if name == "" {
		return "", fmt.Errorf("vertex: model required")
	}
	base = strings.TrimRight(strings.TrimSpace(base), "/")
	if base == "" {
		return "", fmt.Errorf("vertex: base_url required")
	}
	// Allow base that already includes /v1 or ends without it.
	pathPrefix := ""
	if !strings.HasSuffix(base, "/v1") && !strings.Contains(base, "/v1/") {
		// If base is host root, append /v1; if it already has projects/.../v1 leave as-is.
		if !strings.Contains(base, "/publishers/") {
			pathPrefix = "/v1"
		}
	} else if strings.HasSuffix(base, "/v1") {
		pathPrefix = ""
	}
	action := "generateContent"
	if stream {
		action = "streamGenerateContent"
	}
	u := fmt.Sprintf("%s%s/publishers/%s/models/%s:%s",
		base, pathPrefix, url.PathEscape(publisher), url.PathEscape(name), action)
	// PathEscape escapes / which we don't want in model names with dots only — model names are simple.
	// Re-build without PathEscape for model (Gemini ids are [A-Za-z0-9._-]+).
	u = fmt.Sprintf("%s%s/publishers/%s/models/%s:%s",
		base, pathPrefix, publisher, name, action)
	if stream {
		u += "?alt=sse"
	}
	return u, nil
}
