// Package catalog loads providers.yaml and normalizes capabilities into surfaces.
package catalog

import (
	"os"

	corecatalog "github.com/subosito/cincai/catalog"
	"gopkg.in/yaml.v3"
)

// Load reads providers.yaml, normalizing capabilities and modalities.
func Load(path string) (*corecatalog.Catalog, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var root map[string]any
	if err := yaml.Unmarshal(raw, &root); err != nil {
		return nil, err
	}
	if providers, ok := root["providers"].(map[string]any); ok {
		root["providers"] = normalizeProviders(providers)
	}
	if models, ok := root["models"].(map[string]any); ok {
		root["models"] = normalizeModels(models)
	}
	delete(root, "router")
	delete(root, "billing")
	delete(root, "components")

	out, err := yaml.Marshal(root)
	if err != nil {
		return nil, err
	}
	var doc corecatalog.Document
	if err := yaml.Unmarshal(out, &doc); err != nil {
		return nil, err
	}
	applyWireTranslate(&doc)
	return corecatalog.NewFromDocument(doc)
}