package upstream

import (
	"net/url"
	"strings"
)

// JoinURL appends an ingress wire path to a provider base_url.
// When base_url already ends with /v1 and path starts with /v1/, the duplicate
// segment is dropped so both catalog styles work:
//   - https://host/v1            + /v1/chat/completions → …/v1/chat/completions
//   - https://api.example.com    + /v1/chat/completions → …/v1/chat/completions
func JoinURL(baseURL, path string) string {
	base := strings.TrimRight(baseURL, "/")
	if path == "" || path == "/" {
		return base
	}
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	if strings.HasSuffix(base, "/v1") && strings.HasPrefix(path, "/v1/") {
		path = strings.TrimPrefix(path, "/v1")
	}
	return base + path
}

// ImageUpstreamPath maps OpenAI ingress /v1/images/* to provider base_url layout.
// Maps OpenAI image ingress to provider base_url layout (base …/v1/images + /generations).
func ImageUpstreamPath(baseURL, ingressPath string) string {
	base := strings.TrimRight(baseURL, "/")
	switch ingressPath {
	case "/v1/images/generations":
		if strings.HasSuffix(base, "/images") {
			return base + "/generations"
		}
		return JoinURL(baseURL, ingressPath)
	case "/v1/images/edits":
		if strings.HasSuffix(base, "/images") {
			return base + "/edits"
		}
		return JoinURL(baseURL, ingressPath)
	default:
		return JoinURL(baseURL, ingressPath)
	}
}

// VideoUpstreamPath maps OpenAI ingress /v1/videos/* to provider base_url layout.
// Maps OpenAI video ingress to provider base_url layout (base …/v1/videos + /generations or /{id}).
func VideoUpstreamPath(baseURL, ingressPath string) string {
	base := strings.TrimRight(baseURL, "/")
	switch {
	case ingressPath == "/v1/videos/generations":
		if strings.HasSuffix(base, "/videos") {
			return base + "/generations"
		}
		return JoinURL(baseURL, ingressPath)
	case ingressPath == "/v1/videos/edits":
		if strings.HasSuffix(base, "/videos") {
			return base + "/edits"
		}
		return JoinURL(baseURL, ingressPath)
	case ingressPath == "/v1/videos/extensions":
		if strings.HasSuffix(base, "/videos") {
			return base + "/extensions"
		}
		return JoinURL(baseURL, ingressPath)
	case strings.HasPrefix(ingressPath, "/v1/videos/"):
		// id is client-controlled and arrives already percent-decoded (r.URL.Path).
		// Re-escape it so an embedded '?' or '/' cannot inject query params or extra
		// path segments into the upstream request when net/http re-parses the URL.
		id := url.PathEscape(strings.TrimPrefix(ingressPath, "/v1/videos/"))
		if strings.HasSuffix(base, "/videos") {
			return base + "/" + id
		}
		return JoinURL(baseURL, "/v1/videos/"+id)
	default:
		return JoinURL(baseURL, ingressPath)
	}
}
