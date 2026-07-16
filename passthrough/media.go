package passthrough

import (
	"context"
	"io"
	"net/http"

	"github.com/subosito/cincai/adaptersdk/handler"
	"github.com/subosito/cincai/upstream"
)

// ImageHandler relays image requests without transformation.
type ImageHandler struct {
	ProtocolName string
	WireID       string
}

func (h *ImageHandler) Protocol() string { return h.ProtocolName }

func (h *ImageHandler) Forward(ctx context.Context, client *http.Client, t handler.Target, ingressPath string, body io.Reader, hdr http.Header) (*http.Response, error) {
	path := upstream.ImageUpstreamPath(t.BaseURL, ingressPath)
	return relay(ctx, client, t, http.MethodPost, path, body, hdr)
}

// SpeechHandler relays TTS requests without transformation.
type SpeechHandler struct {
	ProtocolName string
	WireID       string
}

func (h *SpeechHandler) Protocol() string { return h.ProtocolName }

func (h *SpeechHandler) Forward(ctx context.Context, client *http.Client, t handler.Target, body io.Reader, hdr http.Header) (*http.Response, error) {
	return relay(ctx, client, t, http.MethodPost, "/v1/audio/speech", body, hdr)
}

// TranscriptionHandler relays STT requests without transformation.
type TranscriptionHandler struct {
	ProtocolName string
	WireID       string
}

func (h *TranscriptionHandler) Protocol() string { return h.ProtocolName }

func (h *TranscriptionHandler) Forward(ctx context.Context, client *http.Client, t handler.Target, body io.Reader, hdr http.Header) (*http.Response, error) {
	return relay(ctx, client, t, http.MethodPost, "/v1/audio/transcriptions", body, hdr)
}

// VideoHandler relays video submit/poll without transformation.
type VideoHandler struct {
	ProtocolName string
	WireID       string
}

func (h *VideoHandler) Protocol() string { return h.ProtocolName }

func (h *VideoHandler) Forward(ctx context.Context, client *http.Client, t handler.Target, ingressPath string, body io.Reader, hdr http.Header) (*http.Response, error) {
	method := http.MethodPost
	if body == nil {
		method = http.MethodGet
	}
	path := upstream.VideoUpstreamPath(t.BaseURL, ingressPath)
	return relay(ctx, client, t, method, path, body, hdr)
}
