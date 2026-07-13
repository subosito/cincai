package elevenlabs

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/subosito/cincai/adaptersdk/handler"
	"github.com/subosito/cincai/credential/inject"
	"github.com/subosito/cincai/observability"
)

// SpeechHandler translates OpenAI /v1/audio/speech to ElevenLabs text-to-speech.
type SpeechHandler struct{}

func (h *SpeechHandler) Protocol() string { return "elevenlabs-tts" }

type openaiSpeechReq struct {
	Input          string `json:"input"`
	Voice          string `json:"voice"`
	ResponseFormat string `json:"response_format"`
}

func (h *SpeechHandler) Forward(ctx context.Context, client *http.Client, t handler.Target, body io.Reader, hdr http.Header) (*http.Response, error) {
	raw, err := io.ReadAll(body)
	if err != nil {
		return nil, err
	}
	var req openaiSpeechReq
	if err := json.Unmarshal(raw, &req); err != nil {
		return nil, fmt.Errorf("elevenlabs-tts: invalid json: %w", err)
	}
	text := strings.TrimSpace(req.Input)
	if text == "" {
		return nil, fmt.Errorf("elevenlabs-tts: input is required")
	}
	voiceID := strings.TrimSpace(req.Voice)
	if voiceID == "" {
		return nil, fmt.Errorf("elevenlabs-tts: voice is required")
	}

	base := strings.TrimRight(t.BaseURL, "/")
	targetURL := base + "/v1/text-to-speech/" + voiceID
	u, err := url.Parse(targetURL)
	if err != nil {
		return nil, err
	}
	q := u.Query()
	q.Set("output_format", elevenlabsOutputFormat(req.ResponseFormat))
	u.RawQuery = q.Encode()

	upBody, err := json.Marshal(map[string]string{
		"text":     text,
		"model_id": strings.TrimSpace(t.UpstreamModel),
	})
	if err != nil {
		return nil, err
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, u.String(), strings.NewReader(string(upBody)))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Accept", elevenlabsAccept(req.ResponseFormat))
	inject.CopyHeaders(httpReq, hdr)
	if err := inject.ApplyRoute(t.Material, httpReq, inject.Route{
		Spec:   t.Inject,
		Preset: t.InjectPreset,
	}, inject.AdapterDefault{Spec: DefaultInject}); err != nil {
		return nil, err
	}
	return observability.HTTPDo(ctx, client, httpReq)
}

func elevenlabsAccept(openaiFormat string) string {
	switch strings.ToLower(strings.TrimSpace(openaiFormat)) {
	case "mp3", "aac", "flac":
		return "audio/mpeg"
	case "wav", "pcm":
		return "audio/wav"
	case "opus", "":
		return "audio/ogg"
	default:
		return "audio/ogg"
	}
}

func elevenlabsOutputFormat(openaiFormat string) string {
	switch strings.ToLower(strings.TrimSpace(openaiFormat)) {
	case "mp3":
		return "mp3_44100_128"
	case "opus":
		return "opus_48000_64"
	case "aac":
		return "mp3_44100_128"
	case "flac":
		return "mp3_44100_128"
	case "wav", "pcm":
		return "pcm_44100"
	default:
		return "opus_48000_64"
	}
}