package catalog

import (
	"fmt"
	"strings"

	"gopkg.in/yaml.v3"
)

type hopSKU struct {
	Model  string `yaml:"model"`
	Effort string `yaml:"effort"`
}

func decodeEffortSKUMap(n *yaml.Node) (map[string]string, error) {
	if n == nil || n.Kind == 0 || n.Tag == "!!null" {
		return nil, nil
	}
	switch n.Kind {
	case yaml.MappingNode:
		var m map[string]string
		if err := n.Decode(&m); err != nil {
			return nil, err
		}
		return m, nil
	case yaml.SequenceNode:
		if len(n.Content) == 0 {
			return map[string]string{}, nil
		}
		if n.Content[0] != nil && n.Content[0].Kind == yaml.ScalarNode {
			return nil, errNotEffortSKUMap
		}
		var hops []hopSKU
		if err := n.Decode(&hops); err != nil {
			return nil, err
		}
		out := make(map[string]string, len(hops))
		for _, h := range hops {
			effort := strings.ToLower(strings.TrimSpace(h.Effort))
			sku := strings.TrimSpace(h.Model)
			if effort == "" || sku == "" {
				return nil, fmt.Errorf("hop models entry needs model and effort")
			}
			if _, ok := out[effort]; ok {
				return nil, fmt.Errorf("duplicate hop models effort %q", effort)
			}
			out[effort] = sku
		}
		return out, nil
	default:
		return nil, fmt.Errorf("hop models: expected map or list")
	}
}

var errNotEffortSKUMap = fmt.Errorf("not an effort sku map")

func decodeCompositeIDs(n *yaml.Node) ([]string, error) {
	var ids []string
	if err := n.Decode(&ids); err != nil {
		return nil, err
	}
	return ids, nil
}

// UnmarshalYAML accepts hop models as either an effort→SKU map or a list of
// {model, effort} (prod Cursor authoring).
func (e *PoolEntry) UnmarshalYAML(value *yaml.Node) error {
	var aux struct {
		ProviderRef string    `yaml:"provider_ref"`
		Model       string    `yaml:"model"`
		Surface     string    `yaml:"surface"`
		Adapter     string    `yaml:"adapter"`
		Models      yaml.Node `yaml:"models"`
	}
	if err := value.Decode(&aux); err != nil {
		return err
	}
	e.ProviderRef = aux.ProviderRef
	e.Model = aux.Model
	e.Surface = aux.Surface
	e.Adapter = aux.Adapter
	if aux.Models.Kind == 0 || aux.Models.Tag == "!!null" {
		return nil
	}
	m, err := decodeEffortSKUMap(&aux.Models)
	if err != nil {
		return err
	}
	e.Models = m
	return nil
}

// UnmarshalYAML accepts modality.models as:
//   - []string — composite public ids
//   - map or [{model, effort}, …] — Cursor-style effort SKUs on a shorthand hop
//     (provider_ref + surface on the modality).
func (m *Modality) UnmarshalYAML(value *yaml.Node) error {
	var aux struct {
		Wire        string      `yaml:"wire"`
		Strategy    string      `yaml:"strategy"`
		Providers   []PoolEntry `yaml:"providers"`
		Models      yaml.Node   `yaml:"models"`
		ProviderRef string      `yaml:"provider_ref"`
		Model       string      `yaml:"model"`
		Surface     string      `yaml:"surface"`
		Adapter     string      `yaml:"adapter"`
	}
	if err := value.Decode(&aux); err != nil {
		return err
	}
	m.Wire = aux.Wire
	m.Strategy = aux.Strategy
	m.Providers = aux.Providers

	ref := strings.TrimSpace(aux.ProviderRef)
	if ref != "" || strings.TrimSpace(aux.Model) != "" || strings.TrimSpace(aux.Adapter) != "" {
		entry := PoolEntry{
			ProviderRef: ref,
			Model:       aux.Model,
			Surface:     aux.Surface,
			Adapter:     aux.Adapter,
		}
		if aux.Models.Kind != 0 && aux.Models.Tag != "!!null" {
			skus, err := decodeEffortSKUMap(&aux.Models)
			if err == nil {
				entry.Models = skus
				aux.Models = yaml.Node{}
			} else if err != errNotEffortSKUMap {
				return err
			}
		}
		if len(m.Providers) == 0 {
			m.Providers = []PoolEntry{entry}
		}
	}

	if aux.Models.Kind == 0 || aux.Models.Tag == "!!null" {
		return nil
	}
	if skus, err := decodeEffortSKUMap(&aux.Models); err == nil {
		if len(m.Providers) == 1 && len(m.Providers[0].Models) == 0 {
			m.Providers[0].Models = skus
			return nil
		}
		return fmt.Errorf("modality models: effort SKU list needs a single provider hop")
	} else if err != errNotEffortSKUMap {
		return err
	}
	ids, err := decodeCompositeIDs(&aux.Models)
	if err != nil {
		return err
	}
	m.Models = ids
	return nil
}
