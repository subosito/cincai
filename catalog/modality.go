package catalog

import (
	"net/http"
	"strings"
)

// HeaderCatalogModality is an optional ingress hint: which models.<id>.modalities.<key>
// row to use when one wire + catalog model maps to multiple modalities.
// Stripped before upstream relay. Values are operator yaml keys (models.*.modalities.<key>).
const HeaderCatalogModality = "X-Catalog-Modality"

// ModalityHintFromRequest returns an optional catalog modality key for ResolveWithModality.
// Wire-implied defaults apply when the header is absent (e.g. openai-embeddings → embed).
// Dedicated media wires may return "" so Resolve auto-selects the sole modality.
func ModalityHintFromRequest(r *http.Request, wireID string) string {
	switch wireID {
	case WireOpenAIEmbed:
		return "embed"
	case WireOpenAIImagesGen, WireOpenAIAudioSpeech, WireOpenAIAudioTranscriptions, WireOpenAIVideos:
		return ""
	}
	if r == nil {
		return ""
	}
	return strings.TrimSpace(r.Header.Get(HeaderCatalogModality))
}

// StripIngressControlHeaders removes gateway routing hints before upstream relay.
func StripIngressControlHeaders(h http.Header) {
	if h == nil {
		return
	}
	h.Del(HeaderCatalogModality)
}