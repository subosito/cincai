package adaptersdk

import "github.com/subosito/cincai/adaptersdk/handler"

// Adapter is a vendor implementation (passthrough relay or translate).
type Adapter interface {
	Name() string
	Register(reg *Registry) error
}

// Registry holds passthrough protocol handlers and translate adapters.
type Registry struct {
	ChatHandlers          map[string]handler.Chat
	EmbedHandlers         map[string]handler.Embed
	ImageHandlers         map[string]handler.Image
	SpeechHandlers        map[string]handler.Speech
	TranscriptionHandlers map[string]handler.Transcription
	VideoHandlers         map[string]handler.Video
	ImageAdapters         map[string]handler.Image
	SpeechAdapters        map[string]handler.Speech
	EmbedAdapters         map[string]handler.Embed
	VideoAdapters         map[string]handler.Video
	ChatAdapters          map[string]handler.Chat
}

func NewRegistry() *Registry {
	return &Registry{
		ChatHandlers:          make(map[string]handler.Chat),
		EmbedHandlers:         make(map[string]handler.Embed),
		ImageHandlers:         make(map[string]handler.Image),
		SpeechHandlers:        make(map[string]handler.Speech),
		TranscriptionHandlers: make(map[string]handler.Transcription),
		VideoHandlers:         make(map[string]handler.Video),
		ImageAdapters:         make(map[string]handler.Image),
		SpeechAdapters:        make(map[string]handler.Speech),
		EmbedAdapters:         make(map[string]handler.Embed),
		VideoAdapters:         make(map[string]handler.Video),
		ChatAdapters:          make(map[string]handler.Chat),
	}
}

// RegisterChat adds a passthrough chat handler keyed by protocol.
func RegisterChat(reg *Registry, h handler.Chat) {
	reg.ChatHandlers[h.Protocol()] = h
}

// RegisterEmbed adds a passthrough embed handler keyed by protocol.
func RegisterEmbed(reg *Registry, h handler.Embed) {
	reg.EmbedHandlers[h.Protocol()] = h
}

// RegisterImage adds a passthrough image handler keyed by protocol.
func RegisterImage(reg *Registry, h handler.Image) {
	reg.ImageHandlers[h.Protocol()] = h
}

// RegisterSpeech adds a passthrough speech handler keyed by protocol.
func RegisterSpeech(reg *Registry, h handler.Speech) {
	reg.SpeechHandlers[h.Protocol()] = h
}

// RegisterTranscription adds a passthrough STT handler keyed by protocol.
func RegisterTranscription(reg *Registry, h handler.Transcription) {
	reg.TranscriptionHandlers[h.Protocol()] = h
}

// RegisterVideo adds a passthrough video handler keyed by protocol.
func RegisterVideo(reg *Registry, h handler.Video) {
	reg.VideoHandlers[h.Protocol()] = h
}

// RegisterImageAdapter adds a translate image adapter keyed by adapter name.
func RegisterImageAdapter(reg *Registry, name string, h handler.Image) {
	reg.ImageAdapters[name] = h
}

// RegisterSpeechAdapter adds a translate speech adapter keyed by adapter name.
func RegisterSpeechAdapter(reg *Registry, name string, h handler.Speech) {
	reg.SpeechAdapters[name] = h
}

// RegisterEmbedAdapter adds a translate embed adapter keyed by adapter name.
func RegisterEmbedAdapter(reg *Registry, name string, h handler.Embed) {
	reg.EmbedAdapters[name] = h
}

// RegisterVideoAdapter adds a translate video adapter keyed by adapter name.
func RegisterVideoAdapter(reg *Registry, name string, h handler.Video) {
	reg.VideoAdapters[name] = h
}

// RegisterChatAdapter adds a translate chat adapter keyed by adapter name.
func RegisterChatAdapter(reg *Registry, name string, h handler.Chat) {
	reg.ChatAdapters[name] = h
}

// RegisteredProtocols returns the passthrough protocol ids registered on reg,
// i.e. the values a catalog surface may name in `protocol:`.
func RegisteredProtocols(reg *Registry) map[string]bool {
	out := map[string]bool{}
	if reg == nil {
		return out
	}
	for k := range reg.ChatHandlers {
		out[k] = true
	}
	for k := range reg.EmbedHandlers {
		out[k] = true
	}
	for k := range reg.ImageHandlers {
		out[k] = true
	}
	for k := range reg.SpeechHandlers {
		out[k] = true
	}
	for k := range reg.TranscriptionHandlers {
		out[k] = true
	}
	for k := range reg.VideoHandlers {
		out[k] = true
	}
	return out
}

// RegisteredAdapters returns the translate adapter names registered on reg,
// i.e. the values a catalog surface or pool entry may name in `adapter:`.
// These are handler keys, not Adapter.Name(): the wire-translate adapter
// registers as wire-translate-a2o and wire-translate-o2a.
func RegisteredAdapters(reg *Registry) map[string]bool {
	out := map[string]bool{}
	if reg == nil {
		return out
	}
	for k := range reg.ChatAdapters {
		out[k] = true
	}
	for k := range reg.EmbedAdapters {
		out[k] = true
	}
	for k := range reg.ImageAdapters {
		out[k] = true
	}
	for k := range reg.SpeechAdapters {
		out[k] = true
	}
	for k := range reg.VideoAdapters {
		out[k] = true
	}
	return out
}
