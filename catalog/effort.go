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
//   - effort empty / default / auto → DefaultEffort, else first Efforts entry
//   - effort must be in Efforts
//   - if a hop has Models, UpstreamModel is models[effort] (error if missing)
//   - else SKU rewrite only when the pool upstream already ends with a listed -{tier}
//   - lean / foreign upstream ids are left alone (body carries effort)
//
// Returns the effort actually used (may be default), the (mutated) target
// list, and a non-nil error when the client requested an unsupported effort.
func (c *Catalog) ApplyEffort(model, effort string, targets []Target) (used string, next []Target, err error) {
	if c == nil {
		return "", targets, nil
	}
	m, ok := c.doc.Models[model]
	if !ok {
		return "", targets, nil
	}
	return applyEffort(model, m, effort, targets)
}

func applyEffort(publicID string, m Model, effort string, targets []Target) (string, []Target, error) {
	if len(m.Efforts) == 0 {
		return "", targets, nil
	}
	effort = strings.ToLower(strings.TrimSpace(effort))
	if effort == "" || effort == "default" || effort == "auto" {
		effort = resolvedDefaultEffort(m)
	}
	if effort == "" {
		return "", targets, nil
	}
	if !effortAllowed(m, effort) {
		return "", nil, fmt.Errorf("unsupported effort %q for model (want %s)", effort, strings.Join(normalizedEfforts(m), "|"))
	}
	for i := range targets {
		if len(targets[i].EffortModels) > 0 {
			sku, ok := lookupHopEffortModel(targets[i].EffortModels, effort)
			if !ok {
				return "", nil, fmt.Errorf("no hop models[%q] for model", effort)
			}
			targets[i].UpstreamModel = sku
			continue
		}
		targets[i].UpstreamModel = rewriteEffortUpstream(publicID, targets[i].UpstreamModel, effort, m.Efforts)
	}
	return effort, targets, nil
}

func resolvedDefaultEffort(m Model) string {
	if def := strings.ToLower(strings.TrimSpace(m.DefaultEffort)); def != "" {
		return def
	}
	if len(m.Efforts) == 0 {
		return ""
	}
	return strings.ToLower(strings.TrimSpace(m.Efforts[0]))
}

// rewriteEffortUpstream maps a pool upstream id to the tier for effort.
// Example: example-model-medium + high → example-model-high (SKU-tier models).
//
// Body-only models (OpenAI GPT, DeepSeek, Qwen, …) use lean ids with no tier
// suffix: leave UpstreamModel unchanged so the client body field
// (reasoning_effort / reasoning.effort) is what the vendor consumes.
//
// Never invent publicID-effort. Also never rewrite when upstream equals the
// public id — product names can end with an effort-looking token (e.g.
// qwen3.8-max) without being a multi-SKU effort pool (gemini-…-low).
func rewriteEffortUpstream(publicID, upstream, effort string, efforts []string) string {
	up := strings.TrimSpace(upstream)
	pub := strings.TrimSpace(publicID)
	// Facets (qwen3.8-max:image) share the base product id; strip before compare.
	base := pub
	if i := strings.Index(base, ":"); i >= 0 {
		base = base[:i]
	}
	if up == "" {
		up = base
		if up == "" {
			up = pub
		}
	}
	// Lean models: public base id is the whole SKU (e.g. qwen3.8-max ends with an
	// effort-looking token but is not a multi-SKU pool like gemini-…-low).
	if strings.EqualFold(up, pub) || strings.EqualFold(up, base) {
		return up
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

func lookupHopEffortModel(models map[string]string, effort string) (string, bool) {
	if len(models) == 0 {
		return "", false
	}
	for k, v := range models {
		if !strings.EqualFold(strings.TrimSpace(k), effort) {
			continue
		}
		sku := strings.TrimSpace(v)
		return sku, sku != ""
	}
	return "", false
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
	return validateEffortHops(modelID, m)
}

// validateEffortHops checks PoolEntry.Models.
// When models is set, model is forbidden (SKU is models[default_effort]).
// Keys must be advertised efforts; every advertised effort must have a SKU.
func validateEffortHops(modelID string, m Model) error {
	for _, mod := range m.Modalities {
		for _, e := range mod.Providers {
			if len(e.Models) == 0 {
				continue
			}
			if strings.TrimSpace(e.Model) != "" {
				return fmt.Errorf("models.%s: hop model and models are mutually exclusive", modelID)
			}
			seen := map[string]string{}
			for k, v := range e.Models {
				key := strings.ToLower(strings.TrimSpace(k))
				sku := strings.TrimSpace(v)
				if key == "" || sku == "" {
					return fmt.Errorf("models.%s: empty hop models entry", modelID)
				}
				if !effortAllowed(m, key) {
					return fmt.Errorf("models.%s: hop models key %q not in efforts", modelID, k)
				}
				if prev, ok := seen[key]; ok && prev != sku {
					return fmt.Errorf("models.%s: duplicate hop models key %q", modelID, k)
				}
				seen[key] = sku
			}
			for _, want := range normalizedEfforts(m) {
				if _, ok := seen[want]; !ok {
					return fmt.Errorf("models.%s: hop models missing key %q", modelID, want)
				}
			}
		}
	}
	return nil
}
