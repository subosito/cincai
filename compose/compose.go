package compose

import (
	"fmt"
	"strings"

	"github.com/subosito/cincai/adaptersdk"
	"github.com/subosito/cincai/passthrough"
)

// DefaultAdapters returns the stock core adapter (passthrough relay only).
func DefaultAdapters() []adaptersdk.Adapter {
	return []adaptersdk.Adapter{passthrough.New()}
}

// FromConfig filters available adapters by cincai.yaml adapters.enable.
func FromConfig(enable []string, available []adaptersdk.Adapter) (*adaptersdk.Registry, error) {
	if len(available) == 0 {
		available = DefaultAdapters()
	}
	want := make(map[string]bool, len(enable))
	for _, name := range enable {
		want[strings.ToLower(strings.TrimSpace(name))] = true
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

func registryEmpty(reg *adaptersdk.Registry) bool {
	return len(reg.ChatHandlers) == 0 && len(reg.EmbedHandlers) == 0 &&
		len(reg.ImageHandlers) == 0 && len(reg.SpeechHandlers) == 0 && len(reg.VideoHandlers) == 0 &&
		len(reg.ImageAdapters) == 0 && len(reg.SpeechAdapters) == 0 && len(reg.EmbedAdapters) == 0 &&
		len(reg.VideoAdapters) == 0 && len(reg.ChatAdapters) == 0
}