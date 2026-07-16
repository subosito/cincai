package observability

import (
	"context"
	"net/http"
	"strings"
)

// Optional host attribution headers. Labels only — never used for routing
// (path + body model). Public OpenAI-compatible clients do not need them.
//
// Default wire names are X-Cincai-*. Gateways may accept additional aliases
// per slot via AttributionConfig / StashHostAttributionWith. All configured
// aliases are stripped before upstream so provider APIs never see them.
// X-Correlation-Id stays for engine ingress logs.
const (
	HeaderActor     = "X-Cincai-Actor"     // who: service, bot, app, tenant client
	HeaderSession   = "X-Cincai-Session"   // conversation / session id
	HeaderComponent = "X-Cincai-Component" // feature bucket: turn.chat, tool.generate_image, …
)

// AttributionConfig lists HTTP header aliases per semantic slot.
// For each slot, the first non-empty value among aliases wins.
// Empty slices fall back to the X-Cincai-* defaults.
type AttributionConfig struct {
	Actor     []string // header names; first non-empty wins
	Session   []string
	Component []string
}

// DefaultAttributionConfig returns the stock X-Cincai-* header names.
func DefaultAttributionConfig() AttributionConfig {
	return AttributionConfig{
		Actor:     []string{HeaderActor},
		Session:   []string{HeaderSession},
		Component: []string{HeaderComponent},
	}
}

// withDefaults fills empty slots with X-Cincai-* defaults and trims names.
func (c AttributionConfig) withDefaults() AttributionConfig {
	d := DefaultAttributionConfig()
	c.Actor = normalizeAliases(c.Actor, d.Actor)
	c.Session = normalizeAliases(c.Session, d.Session)
	c.Component = normalizeAliases(c.Component, d.Component)
	return c
}

func normalizeAliases(in, fallback []string) []string {
	var out []string
	seen := map[string]bool{}
	for _, raw := range in {
		name := strings.TrimSpace(raw)
		if name == "" || seen[strings.ToLower(name)] {
			continue
		}
		seen[strings.ToLower(name)] = true
		out = append(out, name)
	}
	if len(out) == 0 {
		return append([]string(nil), fallback...)
	}
	return out
}

// HostAttribution is optional request metadata for usage sinks / host logs.
// Fields are product-agnostic; wire header names are configured separately.
type HostAttribution struct {
	Actor         string
	Session       string
	CorrelationID string
	Component     string
	Path          string
	Method        string
}

type hostAttrKey struct{}

// StashHostAttribution records optional host headers into context, then strips
// the default X-Cincai-* aliases so they are not forwarded upstream.
// X-Correlation-Id is left for ingress logging / OTel correlation.
//
// Wire via gateway WrapDataHandler (or equivalent) in host binaries.
func StashHostAttribution(next http.Handler) http.Handler {
	return StashHostAttributionWith(DefaultAttributionConfig())(next)
}

// StashHostAttributionWith is like StashHostAttribution but accepts alias lists
// per slot. First non-empty alias wins; every configured alias is stripped.
//
// Example — prefer a host-specific actor header, fall back to the default:
//
//	cfg := observability.AttributionConfig{
//		Actor: []string{"X-Host-Actor", observability.HeaderActor},
//		// Session / Component empty → X-Cincai-* defaults
//	}
//	h := observability.StashHostAttributionWith(cfg)(inner)
func StashHostAttributionWith(cfg AttributionConfig) func(http.Handler) http.Handler {
	cfg = cfg.withDefaults()
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r == nil {
				next.ServeHTTP(w, r)
				return
			}
			a := HostAttribution{
				Actor:         firstHeader(r.Header, cfg.Actor),
				Session:       firstHeader(r.Header, cfg.Session),
				CorrelationID: strings.TrimSpace(r.Header.Get(headerCorrelationID)),
				Component:     firstHeader(r.Header, cfg.Component),
				Path:          r.URL.Path,
				Method:        r.Method,
			}
			ctx := context.WithValue(r.Context(), hostAttrKey{}, a)
			stripHeaders(r.Header, cfg.Actor)
			stripHeaders(r.Header, cfg.Session)
			stripHeaders(r.Header, cfg.Component)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

func firstHeader(h http.Header, names []string) string {
	for _, name := range names {
		if v := strings.TrimSpace(h.Get(name)); v != "" {
			return v
		}
	}
	return ""
}

func stripHeaders(h http.Header, names []string) {
	for _, name := range names {
		h.Del(name)
	}
}

// HostAttributionFrom returns stashed labels, or zero if none.
func HostAttributionFrom(ctx context.Context) HostAttribution {
	if ctx == nil {
		return HostAttribution{}
	}
	a, _ := ctx.Value(hostAttrKey{}).(HostAttribution)
	return a
}
