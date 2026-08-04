package catalog

import (
	"fmt"
	"sort"
	"strings"
)

// ValidateRoutes checks every model modality resolves to upstream targets,
// and that groups: entries are well-formed.
func (c *Catalog) ValidateRoutes() error {
	modelIDs := make([]string, 0, len(c.doc.Models))
	for id := range c.doc.Models {
		modelIDs = append(modelIDs, id)
	}
	sort.Strings(modelIDs)

	var errs []string
	for _, modelID := range modelIDs {
		m := c.doc.Models[modelID]
		if err := ValidateEffortConfig(modelID, m); err != nil {
			errs = append(errs, err.Error())
		}
		modNames := make([]string, 0, len(m.Modalities))
		for name := range m.Modalities {
			modNames = append(modNames, name)
		}
		sort.Strings(modNames)
		for _, modName := range modNames {
			mod := m.Modalities[modName]
			if _, err := c.ResolveWithModality(modelID, mod.Wire, modName); err != nil {
				errs = append(errs, fmt.Sprintf("models.%s.modalities.%s (wire %s): %v", modelID, modName, mod.Wire, err))
			}
		}
	}
	errs = append(errs, c.validateGroups()...)
	if len(errs) == 0 {
		return nil
	}
	return fmt.Errorf("catalog routing:\n  %s", strings.Join(errs, "\n  "))
}

func (c *Catalog) validateGroups() []string {
	if len(c.doc.Groups) == 0 {
		return nil
	}
	gids := make([]string, 0, len(c.doc.Groups))
	for id := range c.doc.Groups {
		gids = append(gids, id)
	}
	sort.Strings(gids)
	var errs []string
	for _, gid := range gids {
		if _, ok := c.doc.Models[gid]; ok {
			errs = append(errs, fmt.Sprintf("groups.%s: id collides with models.%s", gid, gid))
		}
		g := c.doc.Groups[gid]
		if len(g.Models) == 0 {
			errs = append(errs, fmt.Sprintf("groups.%s: models list is empty", gid))
			continue
		}
		seen := map[string]bool{}
		for i, mid := range g.Models {
			mid = strings.TrimSpace(mid)
			if mid == "" {
				errs = append(errs, fmt.Sprintf("groups.%s: empty entry at index %d", gid, i))
				continue
			}
			if seen[mid] {
				errs = append(errs, fmt.Sprintf("groups.%s: duplicate member %q", gid, mid))
				continue
			}
			seen[mid] = true
			if _, ok := c.doc.Models[mid]; !ok {
				errs = append(errs, fmt.Sprintf("groups.%s: unknown model %q", gid, mid))
			}
		}
	}
	return errs
}

// ModelCount returns the number of catalog model ids.
func (c *Catalog) ModelCount() int {
	return len(c.doc.Models)
}
