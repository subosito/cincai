package inject

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/subosito/cincai/credential/store"
)

// Spec is a flat header → template map from providers.yaml inject:.
type Spec map[string]string

// Route is catalog inject config for one upstream hop.
type Route struct {
	Spec   Spec
	Preset string
}

// AdapterDefault is the adapter fallback when catalog omits inject and inject_preset.
type AdapterDefault struct {
	Spec   Spec   // adapter fallback header templates
	Preset string // only bearer or x-api-key
}

// ApplyRoute strips client auth and applies catalog inject (map wins over preset).
func ApplyRoute(m store.Material, r *http.Request, route Route, adapterDefault AdapterDefault) error {
	StripClient(r)
	if len(route.Spec) > 0 {
		return ApplySpec(m, r, route.Spec)
	}
	if preset := strings.TrimSpace(route.Preset); preset != "" {
		return applyCatalogPreset(m, r, preset)
	}
	if len(adapterDefault.Spec) > 0 {
		return ApplySpec(m, r, adapterDefault.Spec)
	}
	return applyCatalogPreset(m, r, adapterDefault.Preset)
}

// applyCatalogPreset applies yaml inject_preset — only bearer and x-api-key.
// Any other header shape belongs in inject: map or adapter Default.Spec.
func applyCatalogPreset(m store.Material, r *http.Request, preset string) error {
	preset = strings.ToLower(strings.TrimSpace(preset))
	switch m.Kind {
	case store.KindAPIKey:
		key := strings.TrimSpace(m.APIKey)
		switch preset {
		case "", "bearer":
			r.Header.Set("Authorization", "Bearer "+key)
		case "x-api-key":
			r.Header.Set("x-api-key", key)
		default:
			return fmt.Errorf("unknown inject_preset %q for api_key (use inject: map, or bearer | x-api-key)", preset)
		}
	case store.KindOAuth:
		switch preset {
		case "", "bearer":
			r.Header.Set("Authorization", "Bearer "+strings.TrimSpace(m.AccessToken))
		case "x-api-key":
			return fmt.Errorf("inject_preset x-api-key does not apply to oauth (use inject: map)")
		default:
			return fmt.Errorf("unknown inject_preset %q for oauth (use inject: map, or bearer)", preset)
		}
	}
	return nil
}

// ApplySpec substitutes ${key}, ${access}, ${accountId}, ${projectId} into spec headers.
func ApplySpec(m store.Material, r *http.Request, spec Spec) error {
	if len(spec) == 0 {
		return nil
	}
	key, access, accountID, projectID := materialValues(m)
	for name, tmpl := range spec {
		val, err := substitute(tmpl, key, access, accountID, projectID)
		if err != nil {
			return fmt.Errorf("inject %q: %w", name, err)
		}
		if val == "" {
			return fmt.Errorf("inject %q: empty value after substitution", name)
		}
		r.Header.Set(canonicalHeader(name), val)
	}
	return nil
}

func materialValues(m store.Material) (key, access, accountID, projectID string) {
	switch m.Kind {
	case store.KindAPIKey:
		key = strings.TrimSpace(m.APIKey)
	case store.KindOAuth:
		access = strings.TrimSpace(m.AccessToken)
		// Many catalogs write inject maps with ${key} for a single Bearer-shaped secret
		// (same pattern as imported access tokens stored as api_key). Alias access → key
		// so OAuth rows work without a second inject template.
		key = access
		accountID = m.Extra("account_id")
		projectID = m.Extra("project_id")
	}
	return key, access, accountID, projectID
}

func substitute(tmpl, key, access, accountID, projectID string) (string, error) {
	out := tmpl
	if strings.Contains(out, "${key}") {
		if key == "" {
			return "", fmt.Errorf("missing ${key}")
		}
		out = strings.ReplaceAll(out, "${key}", key)
	}
	if strings.Contains(out, "${access}") {
		if access == "" {
			return "", fmt.Errorf("missing ${access}")
		}
		out = strings.ReplaceAll(out, "${access}", access)
	}
	if strings.Contains(out, "${accountId}") {
		if accountID == "" {
			return "", fmt.Errorf("missing ${accountId}")
		}
		out = strings.ReplaceAll(out, "${accountId}", accountID)
	}
	if strings.Contains(out, "${projectId}") {
		if projectID == "" {
			return "", fmt.Errorf("missing ${projectId}")
		}
		out = strings.ReplaceAll(out, "${projectId}", projectID)
	}
	if strings.Contains(out, "${") {
		return "", fmt.Errorf("unsupported placeholder in %q", tmpl)
	}
	return out, nil
}

func canonicalHeader(name string) string {
	lower := strings.ToLower(strings.TrimSpace(name))
	switch lower {
	case "authorization":
		return "Authorization"
	case "x-api-key":
		return "X-Api-Key"
	default:
		return lower
	}
}
