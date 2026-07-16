package catalog

import (
	"fmt"
	"sort"
	"strings"
)

// FacetSeparator separates base model id from a same-wire facet after expand
// (e.g. grok-4.3:image). Chosen over "-" because hyphens already appear in
// model names (deepseek-v4-flash, claude-sonnet-4-6).
const FacetSeparator = ":"

// ExpandWireCollisions rewrites models so that no model id has two modalities
// on the same wire. Nested multi-modality authoring stays valid: operators can
// write chat+image+search under one id; load expands non-primary rows to
// base:facet public ids (e.g. grok-4.3:image). Clients then use standard
// path + model only.
//
// Primary for a colliding wire group: prefer "chat", then "anthropic_chat",
// else the first name sorted. Facet tokens default to the modality key;
// search_web becomes "search" for a shorter public id.
//
// Modalities on distinct wires remain on the original id (path already
// disambiguates). Explicit models that would clash with an expanded id error.
func ExpandWireCollisions(doc *Document) error {
	if doc == nil || len(doc.Models) == 0 {
		return nil
	}

	// Snapshot ids so we only expand authoring models, not ones we just added.
	baseIDs := make([]string, 0, len(doc.Models))
	for id := range doc.Models {
		baseIDs = append(baseIDs, id)
	}
	sort.Strings(baseIDs)

	toAdd := make(map[string]Model)

	for _, id := range baseIDs {
		model := doc.Models[id]
		if len(model.Modalities) <= 1 {
			continue
		}

		byWire := map[string][]string{}
		for modName, mod := range model.Modalities {
			w := strings.TrimSpace(mod.Wire)
			byWire[w] = append(byWire[w], modName)
		}

		kept := make(map[string]Modality, len(model.Modalities))
		for wire, names := range byWire {
			sort.Strings(names)
			if len(names) <= 1 {
				for _, n := range names {
					kept[n] = model.Modalities[n]
				}
				continue
			}

			primary := pickPrimaryModality(names)
			kept[primary] = model.Modalities[primary]

			for _, n := range names {
				if n == primary {
					continue
				}
				facet := FacetFromModality(n)
				flatID := id + FacetSeparator + facet
				if err := assertExpandTargetFree(doc, toAdd, flatID, id, n); err != nil {
					return err
				}
				toAdd[flatID] = Model{
					Modalities: map[string]Modality{
						n: model.Modalities[n],
					},
				}
			}
			_ = wire // wire used for grouping only
		}

		model.Modalities = kept
		doc.Models[id] = model
	}

	for id, m := range toAdd {
		doc.Models[id] = m
	}
	return nil
}

func assertExpandTargetFree(doc *Document, pending map[string]Model, flatID, fromID, modName string) error {
	if _, ok := doc.Models[flatID]; ok {
		return fmt.Errorf("catalog expand: model %q already exists (cannot expand %q modality %q)", flatID, fromID, modName)
	}
	if _, ok := pending[flatID]; ok {
		return fmt.Errorf("catalog expand: duplicate expanded id %q (from %q modality %q)", flatID, fromID, modName)
	}
	return nil
}

// FacetFromModality maps a modalities.<key> name to the public :facet token.
func FacetFromModality(modKey string) string {
	switch strings.TrimSpace(modKey) {
	case "search_web":
		return "search"
	default:
		return strings.TrimSpace(modKey)
	}
}

func pickPrimaryModality(names []string) string {
	for _, prefer := range []string{"chat", "anthropic_chat"} {
		for _, n := range names {
			if n == prefer {
				return n
			}
		}
	}
	if len(names) == 0 {
		return ""
	}
	// names are sorted by caller
	return names[0]
}
