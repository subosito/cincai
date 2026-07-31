package catalog

import (
	"fmt"
	"os"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

// ModelPricing is optional cost metadata advertised on GET /v1/models.
// Operator estimate for this catalog id — not SaaS invoicing.
//
// Token models use input/output per million tokens. Media models use
// per_unit + unit (image, second, character, hour, …).
type ModelPricing struct {
	Currency          string  `yaml:"currency,omitempty" json:"currency,omitempty"`
	InputPerMTok      float64 `yaml:"input_per_mtok,omitempty" json:"input_per_mtok,omitempty"`
	OutputPerMTok     float64 `yaml:"output_per_mtok,omitempty" json:"output_per_mtok,omitempty"`
	CacheReadPerMTok  float64 `yaml:"cache_read_per_mtok,omitempty" json:"cache_read_per_mtok,omitempty"`
	CacheWritePerMTok float64 `yaml:"cache_write_per_mtok,omitempty" json:"cache_write_per_mtok,omitempty"`
	// PerUnit + Unit cover non-token workloads (image gen, video, speech, ASR).
	PerUnit float64 `yaml:"per_unit,omitempty" json:"per_unit,omitempty"`
	Unit    string  `yaml:"unit,omitempty" json:"unit,omitempty"`
}

// ModelMeta is optional product metadata for one public catalog model id.
// Context/pricing and effort lists live here (not providers.yaml) so operators
// can refresh product facts without touching routing pools.
type ModelMeta struct {
	ContextWindow   int64         `yaml:"context_window,omitempty" json:"context_window,omitempty"`
	MaxOutputTokens int64         `yaml:"max_output_tokens,omitempty" json:"max_output_tokens,omitempty"`
	// Efforts lists supported reasoning-effort values for clients (GET /v1/models).
	// Body-only models (OpenAI GPT, DeepSeek) advertise here without SKU rewrite;
	// tiered SKUs (Gemini …-low) still rewrite when the pool model has a tier suffix.
	Efforts       []string      `yaml:"efforts,omitempty" json:"efforts,omitempty"`
	DefaultEffort string        `yaml:"default_effort,omitempty" json:"default_effort,omitempty"`
	Pricing       *ModelPricing `yaml:"pricing,omitempty" json:"pricing,omitempty"`
}

// MetaBilling is optional document-level currency / version for audits.
type MetaBilling struct {
	Currency string `yaml:"currency,omitempty"`
	Version  string `yaml:"version,omitempty"`
}

// MetaDocument is models.meta.yaml root.
type MetaDocument struct {
	Billing MetaBilling           `yaml:"billing,omitempty"`
	Models  map[string]ModelMeta  `yaml:"models"`
}

// LoadMetaFile reads models.meta.yaml. Empty path is a no-op (nil doc).
func LoadMetaFile(path string) (*MetaDocument, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return nil, nil
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var doc MetaDocument
	if err := yaml.Unmarshal(raw, &doc); err != nil {
		return nil, err
	}
	return &doc, nil
}

// MergeMetaFile loads path and merges into this catalog. Empty path is a no-op.
func (c *Catalog) MergeMetaFile(path string) error {
	doc, err := LoadMetaFile(path)
	if err != nil {
		return err
	}
	if doc == nil {
		return nil
	}
	return c.MergeMeta(*doc)
}

// MergeMeta attaches optional context/pricing/effort metadata keyed by public model id.
// Keys must name a catalog model or the base id of expanded :facet clones.
// Facet ids (base:image) inherit base meta when not listed explicitly.
// Missing meta for a model is fine — only listed keys are advertised.
//
// Efforts from meta are also copied onto the live Model so ApplyEffort and
// expand inherit the same lists without re-reading providers.yaml.
func (c *Catalog) MergeMeta(doc MetaDocument) error {
	if c == nil {
		return fmt.Errorf("catalog meta: nil catalog")
	}
	if len(doc.Models) == 0 {
		return nil
	}

	currency := strings.TrimSpace(doc.Billing.Currency)
	if currency == "" {
		currency = "USD"
	}

	// Validate every meta key references something in the catalog.
	keys := make([]string, 0, len(doc.Models))
	for id := range doc.Models {
		keys = append(keys, id)
	}
	sort.Strings(keys)
	for _, id := range keys {
		if !c.metaKeyKnown(id) {
			return fmt.Errorf("models.meta: unknown model %q (not in catalog)", id)
		}
		if err := validateModelMeta(id, doc.Models[id]); err != nil {
			return err
		}
	}

	resolved := make(map[string]ModelMeta, len(c.doc.Models))
	for modelID := range c.doc.Models {
		m, ok := lookupModelMeta(doc.Models, modelID)
		if !ok {
			continue
		}
		m = withCurrency(m, currency)
		resolved[modelID] = m
		if len(m.Efforts) > 0 {
			mod := c.doc.Models[modelID]
			mod.Efforts = append([]string(nil), m.Efforts...)
			mod.DefaultEffort = strings.TrimSpace(m.DefaultEffort)
			c.doc.Models[modelID] = mod
		}
	}
	c.meta = resolved
	c.metaVersion = strings.TrimSpace(doc.Billing.Version)
	return nil
}

func (c *Catalog) metaKeyKnown(id string) bool {
	if _, ok := c.doc.Models[id]; ok {
		return true
	}
	prefix := id + FacetSeparator
	for mid := range c.doc.Models {
		if strings.HasPrefix(mid, prefix) {
			return true
		}
	}
	return false
}

// lookupModelMeta: exact id, else base id when modelID is base:facet.
func lookupModelMeta(meta map[string]ModelMeta, modelID string) (ModelMeta, bool) {
	if m, ok := meta[modelID]; ok {
		return m, true
	}
	if i := strings.LastIndex(modelID, FacetSeparator); i > 0 {
		base := modelID[:i]
		if m, ok := meta[base]; ok {
			return m, true
		}
	}
	return ModelMeta{}, false
}

func withCurrency(m ModelMeta, currency string) ModelMeta {
	if m.Pricing == nil {
		return m
	}
	p := *m.Pricing
	if strings.TrimSpace(p.Currency) == "" {
		p.Currency = currency
	}
	m.Pricing = &p
	return m
}

func validateModelMeta(id string, m ModelMeta) error {
	if m.ContextWindow < 0 {
		return fmt.Errorf("models.meta %q: context_window must be >= 0", id)
	}
	if m.MaxOutputTokens < 0 {
		return fmt.Errorf("models.meta %q: max_output_tokens must be >= 0", id)
	}
	if err := ValidateEffortConfig(id, Model{
		Efforts:       m.Efforts,
		DefaultEffort: m.DefaultEffort,
	}); err != nil {
		// ValidateEffortConfig prefixes with models.<id> — map to models.meta wording.
		return fmt.Errorf("models.meta: %w", err)
	}
	hasEffort := len(m.Efforts) > 0
	if m.Pricing == nil {
		if m.ContextWindow == 0 && m.MaxOutputTokens == 0 && !hasEffort {
			return fmt.Errorf("models.meta %q: empty entry (need context_window, max_output_tokens, efforts, and/or pricing)", id)
		}
		return nil
	}
	p := m.Pricing
	// Non-nil pricing may be free (all-zero token rates). Only reject negatives
	// and unit/per_unit mismatches.
	if p.InputPerMTok < 0 || p.OutputPerMTok < 0 ||
		p.CacheReadPerMTok < 0 || p.CacheWritePerMTok < 0 || p.PerUnit < 0 {
		return fmt.Errorf("models.meta %q: pricing rates must be >= 0", id)
	}
	unit := strings.TrimSpace(p.Unit)
	if p.PerUnit != 0 && unit == "" {
		return fmt.Errorf("models.meta %q: pricing.unit required when per_unit is set", id)
	}
	if unit != "" && p.PerUnit == 0 &&
		p.InputPerMTok == 0 && p.OutputPerMTok == 0 &&
		p.CacheReadPerMTok == 0 && p.CacheWritePerMTok == 0 {
		// unit-only with zero per_unit is free media — ok
	}
	return nil
}

// MetaVersion returns billing.version from the last merged meta file (if any).
func (c *Catalog) MetaVersion() string {
	if c == nil {
		return ""
	}
	return c.metaVersion
}

// MetaFor returns resolved meta for a public catalog model id.
func (c *Catalog) MetaFor(modelID string) (ModelMeta, bool) {
	if c == nil || c.meta == nil {
		return ModelMeta{}, false
	}
	m, ok := c.meta[modelID]
	return m, ok
}
