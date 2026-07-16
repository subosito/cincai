package compose

import (
	"fmt"
	"sort"
	"strings"

	"github.com/subosito/cincai/adaptersdk"
	"github.com/subosito/cincai/passthrough"
)

// DefaultAdapters returns the stock core adapter (passthrough relay only).
func DefaultAdapters() []adaptersdk.Adapter {
	return []adaptersdk.Adapter{passthrough.New()}
}

// FromConfig filters available adapters by cincai.yaml adapters.enable.
// An enable entry that names no available adapter is an error: silently dropping
// it would let a typo pass startup and then fail on every request it routes.
func FromConfig(enable []string, available []adaptersdk.Adapter) (*adaptersdk.Registry, error) {
	if len(available) == 0 {
		available = DefaultAdapters()
	}
	have := make(map[string]bool, len(available))
	for _, a := range available {
		have[strings.ToLower(a.Name())] = true
	}
	want := make(map[string]bool, len(enable))
	var unknown []string
	for _, name := range enable {
		key := strings.ToLower(strings.TrimSpace(name))
		if key == "" {
			continue
		}
		if !have[key] {
			unknown = append(unknown, strings.TrimSpace(name))
			continue
		}
		want[key] = true
	}
	if len(unknown) > 0 {
		return nil, fmt.Errorf("unknown adapter(s) in adapters.enable: %s (available: %s)",
			strings.Join(unknown, ", "), strings.Join(adapterNames(available), ", "))
	}
	reg := adaptersdk.NewRegistry()
	for _, a := range available {
		if !want[strings.ToLower(a.Name())] {
			continue
		}
		if err := a.Register(reg); err != nil {
			return nil, fmt.Errorf("register %s: %w", a.Name(), err)
		}
	}
	if registryEmpty(reg) {
		return nil, fmt.Errorf("no adapters enabled; check adapters.enable")
	}
	return reg, nil
}

func adapterNames(available []adaptersdk.Adapter) []string {
	out := make([]string, 0, len(available))
	for _, a := range available {
		out = append(out, a.Name())
	}
	sort.Strings(out)
	return out
}

func registryEmpty(reg *adaptersdk.Registry) bool {
	return len(reg.ChatHandlers) == 0 && len(reg.EmbedHandlers) == 0 &&
		len(reg.ImageHandlers) == 0 && len(reg.SpeechHandlers) == 0 && len(reg.VideoHandlers) == 0 &&
		len(reg.ImageAdapters) == 0 && len(reg.SpeechAdapters) == 0 && len(reg.EmbedAdapters) == 0 &&
		len(reg.VideoAdapters) == 0 && len(reg.ChatAdapters) == 0
}
