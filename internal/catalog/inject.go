package catalog

import (
	"strings"

	"github.com/subosito/cincai/internal/catalog/fields"
)

// CopyInjectFields copies optional inject: map and inject_preset from a provider row.
func CopyInjectFields(entry map[string]any, out map[string]any) {
	if preset := fields.String(entry["inject_preset"]); preset != "" {
		out["inject_preset"] = preset
	}
	raw, ok := entry["inject"].(map[string]any)
	if !ok || len(raw) == 0 {
		return
	}
	spec := make(map[string]string, len(raw))
	for k, v := range raw {
		s, ok := v.(string)
		if !ok {
			continue
		}
		s = strings.TrimSpace(s)
		if s == "" {
			continue
		}
		spec[strings.ToLower(strings.TrimSpace(k))] = s
	}
	if len(spec) > 0 {
		out["inject"] = spec
	}
}