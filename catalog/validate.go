package catalog

import (
	"fmt"
	"sort"
	"strings"
)

// ValidateRoutes checks every model modality resolves to upstream targets.
func (c *Catalog) ValidateRoutes() error {
	modelIDs := make([]string, 0, len(c.doc.Models))
	for id := range c.doc.Models {
		modelIDs = append(modelIDs, id)
	}
	sort.Strings(modelIDs)

	var errs []string
	for _, modelID := range modelIDs {
		m := c.doc.Models[modelID]
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
	if len(errs) == 0 {
		return nil
	}
	return fmt.Errorf("catalog routing:\n  %s", strings.Join(errs, "\n  "))
}

// ModelCount returns the number of catalog model ids.
func (c *Catalog) ModelCount() int {
	return len(c.doc.Models)
}
