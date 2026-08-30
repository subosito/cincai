package catalog

import "strings"

// CheckUpstreamModel returns the upstream id used for allowlist checks.
// Prefers UpstreamModel; falls back to the catalog model id when empty.
func (t Target) CheckUpstreamModel() string {
	if s := strings.TrimSpace(t.UpstreamModel); s != "" {
		return s
	}
	return strings.TrimSpace(t.Model)
}

// UpstreamAllowed reports whether the target's upstream model is permitted.
// Empty AllowModels means all models are allowed.
func (t Target) UpstreamAllowed() bool {
	if len(t.AllowModels) == 0 {
		return true
	}
	model := t.CheckUpstreamModel()
	for _, allowed := range t.AllowModels {
		if strings.TrimSpace(allowed) == model {
			return true
		}
	}
	return false
}
