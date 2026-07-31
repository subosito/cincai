package catalog

import (
	"sort"
	"strings"

	"github.com/subosito/cincai/ingress/keyring"
)

// ModelListItem is one catalog model for GET /v1/models.
//
// Wire / Wires are cincai extensions beyond the minimal OpenAI model object so
// clients (e.g. mow) can pick the correct client protocol without guessing from
// the model id string.
type ModelListItem struct {
	ID      string `json:"id"`
	Object  string `json:"object"`
	Created int64  `json:"created"`
	OwnedBy string `json:"owned_by"`
	// Wire is the preferred chat/agent wire for this model (if any).
	Wire string `json:"wire,omitempty"`
	// Wires lists every catalog wire this model is registered on.
	Wires []string `json:"wires,omitempty"`
	// Efforts lists allowed reasoning-effort values when the catalog advertises them.
	// Clients should use this instead of a static none|low|medium|high list.
	Efforts []string `json:"efforts,omitempty"`
	// DefaultEffort is the catalog default when the client omits effort.
	DefaultEffort string `json:"default_effort,omitempty"`
	// Facet is the capability token from modality keys after expand: "chat" for
	// the primary agent row, or "search" / "image" / … for non-chat clones.
	// Derived from modalities only — never by parsing ":" in the model id
	// (colons may be part of legitimate provider model ids). Clients (e.g. mow
	// ACP) should offer only facet=="chat" (or empty on plain OpenAI catalogs).
	Facet string `json:"facet,omitempty"`
	// ContextWindow is total context budget clients may assume (optional meta).
	ContextWindow int64 `json:"context_window,omitempty"`
	// MaxOutputTokens is an optional generation cap hint from model meta.
	MaxOutputTokens int64 `json:"max_output_tokens,omitempty"`
	// Pricing is optional operator cost estimate from models.meta.yaml.
	Pricing *ModelPricing `json:"pricing,omitempty"`
}

// ModelsListResponse is OpenAI-shaped list envelope.
type ModelsListResponse struct {
	Object string          `json:"object"`
	Data   []ModelListItem `json:"data"`
}

// listModels reports Created as the time this catalog was loaded. Cincai routes
// models, it does not publish them, so it has no per-model creation date — but
// the OpenAI model schema types created as a required int (openai-python rejects
// the payload without it), so omitting it breaks strict clients. Loaded-at is
// stable for the process and at least true about this gateway, where the zero
// value claimed 1970-01-01.

// ListModels returns all catalog models (no scope filter).
func (c *Catalog) ListModels() ModelsListResponse {
	return c.listModels(nil)
}

// ListModelsFor returns models visible to gateway key scopes.
func (c *Catalog) ListModelsFor(scopes []string) ModelsListResponse {
	return c.listModels(scopes)
}

func (c *Catalog) listModels(scopes []string) ModelsListResponse {
	ids := make([]string, 0, len(c.doc.Models))
	for id := range c.doc.Models {
		ids = append(ids, id)
	}
	sort.Strings(ids)

	data := make([]ModelListItem, 0, len(ids))
	for _, id := range ids {
		m := c.doc.Models[id]
		wires := modelWires(m)
		if scopes != nil && !keyring.FilterModels(scopes, id, wires) {
			continue
		}
		sort.Strings(wires)
		item := ModelListItem{
			ID:      id,
			Object:  "model",
			Created: c.loadedAt,
			OwnedBy: "cincai",
			Wire:    preferredWireForModel(m, wires),
			Wires:   wires,
			Facet:   listFacet(m),
		}
		if len(m.Efforts) > 0 {
			item.Efforts = append([]string(nil), m.Efforts...)
		}
		if def := strings.TrimSpace(m.DefaultEffort); def != "" {
			item.DefaultEffort = def
		}
		if meta, ok := c.MetaFor(id); ok {
			if meta.ContextWindow > 0 {
				item.ContextWindow = meta.ContextWindow
			}
			if meta.MaxOutputTokens > 0 {
				item.MaxOutputTokens = meta.MaxOutputTokens
			}
			if meta.Pricing != nil {
				p := *meta.Pricing
				item.Pricing = &p
			}
		}
		data = append(data, item)
	}
	return ModelsListResponse{Object: "list", Data: data}
}

// listFacet derives the capability facet for GET /v1/models from modality keys
// (post ExpandWireCollisions). Do not parse ":" from the model id — some
// providers use colon inside the id itself, unrelated to cincai facets.
//
// Primary agent rows → "chat"; expanded same-wire clones → search/image/….
func listFacet(m Model) string {
	keys := make([]string, 0, len(m.Modalities))
	for k := range m.Modalities {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	switch len(keys) {
	case 0:
		return "chat"
	case 1:
		if keys[0] == "chat" || keys[0] == "anthropic_chat" {
			return "chat"
		}
		return FacetFromModality(keys[0])
	default:
		for _, prefer := range []string{"chat", "anthropic_chat"} {
			if _, ok := m.Modalities[prefer]; ok {
				return "chat"
			}
		}
		return FacetFromModality(keys[0])
	}
}

func modelWires(m Model) []string {
	seen := make(map[string]bool)
	for _, md := range m.Modalities {
		if md.Wire == "" {
			continue
		}
		seen[md.Wire] = true
	}
	out := make([]string, 0, len(seen))
	for w := range seen {
		out = append(out, w)
	}
	return out
}

// preferredWireForModel is the wire advertised as GET /v1/models `wire`.
// Prefer the authoring modality named "chat" (or "anthropic_chat") so dual-wire
// models keep their native protocol as default — e.g. Claude → anthropic-messages,
// Grok → openai-responses — while openai-chat-completions stays available as a
// secondary path for clients that only speak that wire. Fall back to PreferredChatWire.
func preferredWireForModel(m Model, wires []string) string {
	for _, key := range []string{"chat", "anthropic_chat"} {
		if md, ok := m.Modalities[key]; ok {
			if w := strings.TrimSpace(md.Wire); w != "" {
				return w
			}
		}
	}
	return PreferredChatWire(wires)
}

// PreferredChatWire picks a chat wire when modality authoring did not name a
// primary "chat" row. Prefer OpenAI chat completions, then Anthropic Messages,
// then OpenAI Responses; otherwise the first sorted wire.
func PreferredChatWire(wires []string) string {
	if len(wires) == 0 {
		return ""
	}
	set := make(map[string]bool, len(wires))
	for _, w := range wires {
		set[w] = true
	}
	for _, pref := range []string{WireOpenAIChat, WireAnthropicMsg, WireOpenAIResponses} {
		if set[pref] {
			return pref
		}
	}
	sorted := append([]string(nil), wires...)
	sort.Strings(sorted)
	return sorted[0]
}
