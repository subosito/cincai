package inject

import (
	"net/http"
	"strings"
	"sync"

	"github.com/subosito/cincai/credential/store"
)

// OAuthPresetFunc applies extra OAuth headers after Bearer is set.
type OAuthPresetFunc func(r *http.Request)

var (
	oauthPresetMu sync.RWMutex
	oauthPresets  = map[string]OAuthPresetFunc{}
)

// RegisterOAuthPreset registers extension OAuth header shaping (integrator binary).
func RegisterOAuthPreset(name string, fn OAuthPresetFunc) {
	key := strings.ToLower(strings.TrimSpace(name))
	if key == "" || fn == nil {
		return
	}
	oauthPresetMu.Lock()
	defer oauthPresetMu.Unlock()
	oauthPresets[key] = fn
}

// CopyHeaders copies ingress headers onto an outbound request.
func CopyHeaders(dst *http.Request, src http.Header) {
	for k, vals := range src {
		for _, v := range vals {
			dst.Header.Add(k, v)
		}
	}
}

// clientDeniedRequestHeaders are provider-agnostic client headers that must never reach an
// upstream: client auth (would leak the gateway key or override the injected credential),
// client session/proxy headers, hop-by-hop headers, and client User-Agent.
//
// User-Agent matters: Cloudflare (and similar edges) fingerprint browser signatures. Forwarding
// e.g. Python-urllib/* from the gateway client yields CF 1010 while the same credential works
// with Go's default UA. Leave UA unset so net/http sends Go-http-client (or set one in an
// adapter). A header only one provider treats as sensitive is stripped by that provider's
// adapter (see e.g. adaptersdk/upstreamauth.ApplyTranslated), not here.
var clientDeniedRequestHeaders = []string{
	"Authorization",
	"X-Api-Key", // canonicalises x-api-key too
	"Cookie",
	"Proxy-Authorization",
	"User-Agent",
	// Hop-by-hop (RFC 7230) — must not be relayed on the outbound hop.
	"Connection",
	"Keep-Alive",
	"Proxy-Connection",
	"TE",
	"Trailer",
	"Transfer-Encoding",
	"Upgrade",
}

// StripClient removes client-controlled headers that must not be forwarded upstream.
func StripClient(r *http.Request) {
	for _, h := range clientDeniedRequestHeaders {
		r.Header.Del(h)
	}
}

// Apply writes upstream auth headers from material.
// Catalog yaml should use ApplyRoute; inject_preset there is limited to bearer | x-api-key.
func Apply(m store.Material, r *http.Request, preset string) {
	StripClient(r)
	applyPreset(m, r, preset)
}

func applyPreset(m store.Material, r *http.Request, preset string) {
	preset = strings.ToLower(strings.TrimSpace(preset))
	switch m.Kind {
	case store.KindAPIKey:
		applyAPIKey(m.APIKey, r, preset)
	case store.KindOAuth:
		r.Header.Set("Authorization", "Bearer "+strings.TrimSpace(m.AccessToken))
		applyOAuthPreset(preset, r)
	}
}

func applyAPIKey(key string, r *http.Request, preset string) {
	key = strings.TrimSpace(key)
	switch strings.ToLower(strings.TrimSpace(preset)) {
	case "", "bearer":
		r.Header.Set("Authorization", "Bearer "+key)
	case "x-api-key":
		r.Header.Set("x-api-key", key)
	default:
		r.Header.Set("Authorization", "Bearer "+key)
	}
}

func applyOAuthPreset(preset string, r *http.Request) {
	if preset == "" {
		return
	}
	oauthPresetMu.RLock()
	fn := oauthPresets[preset]
	oauthPresetMu.RUnlock()
	if fn != nil {
		fn(r)
	}
}
