package catalog

import (
	"sort"

	"github.com/subosito/cincai/ingress/keyring"
)

// ModelListItem is one catalog model for GET /v1/models.
type ModelListItem struct {
	ID      string `json:"id"`
	Object  string `json:"object"`
	Created int64  `json:"created"`
	OwnedBy string `json:"owned_by"`
}

// ModelsListResponse is OpenAI-shaped list envelope.
type ModelsListResponse struct {
	Object string          `json:"object"`
	Data   []ModelListItem `json:"data"`
}

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
		if scopes != nil && !keyring.FilterModels(scopes, id, modelWires(m)) {
			continue
		}
		data = append(data, ModelListItem{
			ID:      id,
			Object:  "model",
			OwnedBy: "cincai",
		})
	}
	return ModelsListResponse{Object: "list", Data: data}
}

func modelWires(m Model) []string {
	seen := make(map[string]bool)
	for _, md := range m.Modalities {
		seen[md.Wire] = true
	}
	out := make([]string, 0, len(seen))
	for w := range seen {
		out = append(out, w)
	}
	return out
}