package catalog

import (
	"strings"

	"github.com/subosito/cincai/internal/catalog/fields"
)

var wireAliases = map[string]string{
	"xai-responses": "openai-responses",
}

var protocolAliases = map[string]string{
	"xai-responses": "openai-responses",
}

func normalizeProviders(in map[string]any) map[string]any {
	out := make(map[string]any, len(in))
	for name, raw := range in {
		entry, ok := raw.(map[string]any)
		if !ok {
			out[name] = raw
			continue
		}
		out[name] = normalizeProviderEntry(entry)
	}
	return out
}

func normalizeProviderEntry(entry map[string]any) map[string]any {
	if out, ok := normalizeCapabilitiesProvider(entry); ok {
		return out
	}
	return entry
}

// normalizeCapabilitiesProvider expands one provider id into multi-surface routing.
// capabilities: chat, anthropic_chat, embed, image_gen|video_gen|speech_gen (generate).
func normalizeCapabilitiesProvider(entry map[string]any) (map[string]any, bool) {
	caps, ok := entry["capabilities"].(map[string]any)
	if !ok || len(caps) == 0 {
		return nil, false
	}
	out := map[string]any{}
	if cp := fields.String(entry["credential_profile"]); cp != "" {
		out["credential_profile"] = cp
	}
	CopyInjectFields(entry, out)
	surfaces := make(map[string]any, len(caps))
	for capName, raw := range caps {
		capEntry, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		surf := map[string]any{}
		if adapter := fields.String(capEntry["adapter"]); adapter != "" {
			surf["adapter"] = adapter
		}
		if protocol := aliasProtocol(fields.String(capEntry["protocol"])); protocol != "" {
			surf["protocol"] = protocol
		}
		if base := fields.FirstNonEmpty(fields.String(capEntry["base_url"]), fields.String(capEntry["url"])); base != "" {
			surf["base_url"] = base
		}
		surfaces[capabilitySurfaceKey(capName)] = surf
	}
	if len(surfaces) == 0 {
		return nil, false
	}
	out["surfaces"] = surfaces
	return out, true
}

func capabilitySurfaceKey(cap string) string {
	return strings.TrimSpace(cap)
}

func normalizeModels(in map[string]any) map[string]any {
	out := make(map[string]any, len(in))
	for name, raw := range in {
		spec, ok := raw.(map[string]any)
		if !ok {
			out[name] = raw
			continue
		}
		out[name] = normalizeModelSpec(spec)
	}
	return out
}

func normalizeModelSpec(spec map[string]any) map[string]any {
	mods, ok := spec["modalities"].(map[string]any)
	if !ok {
		return spec
	}
	outMods := make(map[string]any, len(mods))
	for modName, raw := range mods {
		route, ok := raw.(map[string]any)
		if !ok {
			outMods[modName] = raw
			continue
		}
		target := cincaiModality(modName)
		if target == "" {
			continue
		}
		outMods[target] = normalizeModalityRoute(route, modName, target)
	}
	out := map[string]any{"modalities": outMods}
	return out
}

func cincaiModality(name string) string {
	switch strings.TrimSpace(name) {
	case "chat":
		return "chat"
	case "embed":
		return "embed"
	case "image":
		return "image"
	case "image_gen":
		return "image_gen"
	case "video":
		return "video"
	case "video_gen":
		return "video_gen"
	case "voice":
		return "voice"
	case "speech_gen":
		return "speech_gen"
	case "search_web":
		return "search_web"
	case "search_x":
		return "search_x"
	case "anthropic_chat":
		return "anthropic_chat"
	case "ocr":
		return "ocr"
	default:
		return ""
	}
}

func normalizeModalityRoute(route map[string]any, yamlKey, modality string) map[string]any {
	out := map[string]any{}
	if strat := fields.String(route["strategy"]); strat != "" {
		out["strategy"] = strat
	}
	if wire := normalizeWire(fields.String(route["wire"]), modality); wire != "" {
		out["wire"] = wire
	} else if w := defaultWire(yamlKey); w != "" {
		out["wire"] = w
	}
	if ref := fields.String(route["provider_ref"]); ref != "" {
		entry := map[string]any{"provider_ref": ref}
		if model := fields.String(route["model"]); model != "" {
			entry["model"] = model
		}
		if surface := fields.String(route["surface"]); surface != "" {
			entry["surface"] = surface
		} else if surface := poolSurface(modality); surface != "" {
			entry["surface"] = surface
		}
		if adapter := fields.String(route["adapter"]); adapter != "" {
			entry["adapter"] = adapter
		}
		out["providers"] = []any{normalizePoolEntry(entry)}
		return out
	}
	if pool, ok := route["providers"].([]any); ok {
		entries := make([]any, 0, len(pool))
		for _, item := range pool {
			entry := normalizePoolEntry(item)
			if _, ok := entry["surface"]; !ok {
				if surface := poolSurface(modality); surface != "" {
					entry["surface"] = surface
				}
			}
			entries = append(entries, entry)
		}
		out["providers"] = entries
	}
	return out
}

func normalizePoolEntry(item any) map[string]any {
	switch v := item.(type) {
	case string:
		return map[string]any{"provider_ref": strings.TrimSpace(v)}
	case map[string]any:
		out := map[string]any{}
		if ref := fields.String(v["provider_ref"]); ref != "" {
			out["provider_ref"] = ref
		}
		if model := fields.String(v["model"]); model != "" {
			out["model"] = model
		}
		if surface := fields.String(v["surface"]); surface != "" {
			out["surface"] = surface
		}
		if adapter := fields.String(v["adapter"]); adapter != "" {
			out["adapter"] = adapter
		}
		return out
	default:
		return map[string]any{}
	}
}

func normalizeWire(wire, _ string) string {
	return aliasProtocol(strings.TrimSpace(wire))
}

func aliasProtocol(protocol string) string {
	protocol = strings.TrimSpace(protocol)
	if protocol == "" {
		return ""
	}
	if alias, ok := protocolAliases[protocol]; ok {
		return alias
	}
	if alias, ok := wireAliases[protocol]; ok {
		return alias
	}
	return protocol
}

func poolSurface(modality string) string {
	switch modality {
	case "chat":
		return "chat"
	case "anthropic_chat":
		return "anthropic_chat"
	case "embed":
		return "embed"
	case "image_gen":
		return "image_gen"
	case "video_gen":
		return "video_gen"
	case "speech_gen":
		return "speech_gen"
	case "voice":
		return "voice"
	case "ocr":
		return "ocr"
	default:
		return ""
	}
}

func defaultWire(yamlKey string) string {
	switch strings.TrimSpace(yamlKey) {
	case "voice":
		return "openai-audio-transcriptions"
	case "chat", "anthropic_chat", "image", "video", "search_web", "search_x":
		return "openai-chat-completions"
	case "embed":
		return "openai-embeddings"
	case "image_gen":
		return "openai-images-generations"
	case "video_gen":
		return "openai-videos"
	case "speech_gen":
		return "openai-audio-speech"
	case "ocr":
		return "openai-chat-completions"
	default:
		return ""
	}
}