package catalog

import (
	"fmt"
	"log/slog"
	"sort"
	"strings"
)

// ValidateRoutes checks every model modality resolves to upstream targets.
func (c *Catalog) ValidateRoutes() error {
	// Non-fatal: deployed catalogs may declare search facets, which keep
	// loading fine but mislead operators into expecting search behavior.
	for _, w := range searchFacetWarnings(c.doc.Models) {
		slog.Warn(w)
	}
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
	if len(errs) == 0 {
		return nil
	}
	return fmt.Errorf("catalog routing:\n  %s", strings.Join(errs, "\n  "))
}

// ModelCount returns the number of catalog model ids.
func (c *Catalog) ModelCount() int {
	return len(c.doc.Models)
}

// searchFacetWarnings builds operator warnings for search_web/search_x
// modalities. The facet only picks a route: provider-executed search is
// enabled by the client declaring the provider's search tool in the request
// body (e.g. Responses tools:[{"type":"web_search"}]), which works on the
// bare model id too. A request to the faceted id without that tool yields an
// unexecuted search call. One warning per faceted model id, sorted.
func searchFacetWarnings(models map[string]Model) []string {
	var warns []string
	for _, modelID := range sortedKeys(models) {
		for _, modName := range []string{"search_web", "search_x"} {
			if _, ok := models[modelID].Modalities[modName]; !ok {
				continue
			}
			warns = append(warns, fmt.Sprintf(
				"catalog: %s declares modality %q — a routing alias only; provider-executed search requires the client to declare the provider's search tool in the request body (e.g. Responses tools:[{\"type\":\"web_search\"}]); without it, a request to %s produces an unexecuted search call",
				modelID, modName, modelID))
		}
	}
	return warns
}

func sortedKeys(m map[string]Model) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
