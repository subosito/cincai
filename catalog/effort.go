package catalog

import (
	"encoding/json"
	"fmt"
	"strings"
)

// EffortFromBody extracts a client effort hint from a chat-style JSON body.
// Prefers top-level reasoning_effort, then nested Responses reasoning.effort,
// then top-level effort. Empty when none are set.
func EffortFromBody(raw []byte) string {
	if len(raw) == 0 {
		return ""
	}
	var body struct {
		ReasoningEffort string `json:"reasoning_effort"`
		Effort          string `json:"effort"`
		Reasoning       *struct {
			Effort string `json:"effort"`
		} `json:"reasoning"`
	}
	if err := json.Unmarshal(raw, &body); err != nil {
		return ""
	}
	if s := strings.TrimSpace(body.ReasoningEffort); s != "" {
		return s
	}
	if body.Reasoning != nil {
		if s := strings.TrimSpace(body.Reasoning.Effort); s != "" {
			return s
		}
	}
	return strings.TrimSpace(body.Effort)
}

// ApplyEffort rewrites targets' UpstreamModel for models that advertise Efforts.
// Pair with ExpandEffortBody to inject vendor body knobs (hybrid thinking, etc.).
//
// Rules:
//   - no Efforts → no-op
//   - effort empty → DefaultEffort (if set); still no-op if both empty
//   - effort must be in Efforts
//   - SKU rewrite only when pool upstream already ends with a listed -{tier}
//   - lean / foreign upstream ids are left alone (body carries effort)
//
// Returns the effort actually used (may be default) and a non-nil error when
// the client requested an unsupported effort.
func (c *Catalog) ApplyEffort(model, effort string, targets []Target) (used string, err error) {
	if c == nil {
		return "", nil
	}
	m, ok := c.doc.Models[model]
	if !ok {
		return "", nil
	}
	return applyEffort(model, m, effort, targets)
}

func applyEffort(publicID string, m Model, effort string, targets []Target) (string, error) {
	if len(m.Efforts) == 0 {
		return "", nil
	}
	effort = strings.ToLower(strings.TrimSpace(effort))
	if effort == "" || effort == "default" || effort == "auto" {
		effort = strings.ToLower(strings.TrimSpace(m.DefaultEffort))
	}
	if effort == "" {
		return "", nil
	}
	if !effortAllowed(m, effort) {
		return "", fmt.Errorf("unsupported effort %q for model (want %s)", effort, strings.Join(normalizedEfforts(m), "|"))
	}
	for i := range targets {
		targets[i].UpstreamModel = rewriteEffortUpstream(publicID, targets[i].UpstreamModel, effort, m.Efforts)
	}
	return effort, nil
}

// rewriteEffortUpstream maps a pool upstream id to the tier for effort.
// Example: example-model-medium + high → example-model-high (SKU-tier models).
//
// Body-only models (OpenAI GPT, DeepSeek, …) use lean ids with no tier suffix:
// leave UpstreamModel unchanged so the client body field (reasoning_effort /
// reasoning.effort) is what the vendor consumes. Never invent publicID-effort.
func rewriteEffortUpstream(publicID, upstream, effort string, efforts []string) string {
	up := strings.TrimSpace(upstream)
	if up == "" {
		up = strings.TrimSpace(publicID)
	}
	lower := strings.ToLower(up)
	for _, e := range efforts {
		e = strings.ToLower(strings.TrimSpace(e))
		if e == "" {
			continue
		}
		suf := "-" + e
		if strings.HasSuffix(lower, suf) {
			return up[:len(up)-len(suf)] + "-" + effort
		}
	}
	return up
}

func effortAllowed(m Model, effort string) bool {
	for _, e := range m.Efforts {
		if strings.EqualFold(strings.TrimSpace(e), effort) {
			return true
		}
	}
	return false
}

func normalizedEfforts(m Model) []string {
	out := make([]string, 0, len(m.Efforts))
	for _, e := range m.Efforts {
		e = strings.ToLower(strings.TrimSpace(e))
		if e != "" {
			out = append(out, e)
		}
	}
	for i := 1; i < len(out); i++ {
		for j := i; j > 0 && out[j] < out[j-1]; j-- {
			out[j], out[j-1] = out[j-1], out[j]
		}
	}
	return out
}

// ValidateEffortConfig checks efforts / default_effort consistency.
func ValidateEffortConfig(modelID string, m Model) error {
	if len(m.Efforts) == 0 {
		if strings.TrimSpace(m.DefaultEffort) != "" {
			return fmt.Errorf("models.%s: default_effort set but efforts is empty", modelID)
		}
		return nil
	}
	for _, e := range m.Efforts {
		if strings.TrimSpace(e) == "" {
			return fmt.Errorf("models.%s: empty entry in efforts", modelID)
		}
	}
	def := strings.TrimSpace(m.DefaultEffort)
	if def != "" && !effortAllowed(m, def) {
		return fmt.Errorf("models.%s: default_effort %q not in efforts", modelID, m.DefaultEffort)
	}
	return nil
}
