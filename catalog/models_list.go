package catalog

import (
	"sort"

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
		data = append(data, ModelListItem{
			ID:      id,
			Object:  "model",
			Created: c.loadedAt,
			OwnedBy: "cincai",
			Wire:    PreferredChatWire(wires),
			Wires:   wires,
		})
	}
	return ModelsListResponse{Object: "list", Data: data}
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

// PreferredChatWire picks the best client wire for an agent/chat harness.
// Prefer OpenAI chat completions (mow's primary path), then Anthropic Messages,
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
