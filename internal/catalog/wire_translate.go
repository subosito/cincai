package catalog

import (
	corecatalog "github.com/subosito/cincai/catalog"
)

// Wire-translate adapter names (catalog passthrough does not inject these).
const (
	AdapterWireTranslateA2O = "wire-translate-a2o"
	AdapterWireTranslateO2A = "wire-translate-o2a"
)

const wireTranslateSurfacePrefix = "__cincai_"

// applyWireTranslate injects adapter surfaces when ingress wire ≠ upstream protocol.
// internal/catalog.Load runs this before NewFromDocument; bare catalog.Load does not.
func applyWireTranslate(doc *corecatalog.Document) {
	for modelName, model := range doc.Models {
		for modName, mod := range model.Modalities {
			wire := mod.Wire
			for i, entry := range mod.Providers {
				prov, ok := doc.Providers[entry.ProviderRef]
				if !ok {
					continue
				}
				surfKey := entry.Surface
				s, ok := prov.Surfaces[surfKey]
				if !ok {
					if fb, ok := prov.Surfaces["chat"]; ok {
						s = fb
						surfKey = "chat"
					} else {
						continue
					}
				}
				if s.Adapter != "" {
					continue
				}
				if protocolMatchesWire(s.Protocol, wire) {
					continue
				}
				adapter, ok := wireTranslateAdapter(s.Protocol, wire)
				if !ok {
					continue
				}
				injKey := wireTranslateSurfacePrefix + adapter
				if _, exists := prov.Surfaces[injKey]; !exists {
					if prov.Surfaces == nil {
						prov.Surfaces = make(map[string]corecatalog.Surface)
					}
					prov.Surfaces[injKey] = corecatalog.Surface{
						Adapter:  adapter,
						Protocol: s.Protocol,
						BaseURL:  s.BaseURL,
					}
					doc.Providers[entry.ProviderRef] = prov
				}
				entry.Surface = injKey
				mod.Providers[i] = entry
			}
			model.Modalities[modName] = mod
		}
		doc.Models[modelName] = model
	}
}

func wireTranslateAdapter(upstreamProtocol, ingressWire string) (string, bool) {
	switch ingressWire {
	case corecatalog.WireAnthropicMsg:
		if upstreamProtocol == "openai-chat-completions" || upstreamProtocol == "openai-compat-chat" {
			return AdapterWireTranslateA2O, true
		}
	case corecatalog.WireOpenAIChat:
		if upstreamProtocol == "anthropic-messages" {
			return AdapterWireTranslateO2A, true
		}
	}
	return "", false
}

func protocolMatchesWire(protocol, wire string) bool {
	switch wire {
	case corecatalog.WireOpenAIChat:
		return protocol == "openai-chat-completions" || protocol == "openai-compat-chat"
	case corecatalog.WireAnthropicMsg:
		return protocol == "anthropic-messages"
	case corecatalog.WireOpenAIEmbed:
		return protocol == "openai-embeddings"
	case corecatalog.WireOpenAIResponses:
		return protocol == "openai-responses"
	case corecatalog.WireOpenAIImagesGen:
		return protocol == "openai-images"
	case corecatalog.WireOpenAIAudioSpeech:
		return protocol == "openai-tts"
	case corecatalog.WireOpenAIAudioTranscriptions:
		return protocol == "openai-transcriptions"
	case corecatalog.WireOpenAIVideos:
		return protocol == "openai-videos"
	default:
		return false
	}
}
