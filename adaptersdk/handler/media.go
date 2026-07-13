package handler

import (
	"context"
	"io"
	"net/http"
)

// Image handles image generation.
type Image interface {
	Protocol() string
	Forward(ctx context.Context, client *http.Client, t Target, ingressPath string, body io.Reader, hdr http.Header) (*http.Response, error)
}

// Speech handles text-to-speech.
type Speech interface {
	Protocol() string
	Forward(ctx context.Context, client *http.Client, t Target, body io.Reader, hdr http.Header) (*http.Response, error)
}

// Transcription handles speech-to-text (Whisper / OpenAI-compat STT).
type Transcription interface {
	Protocol() string
	Forward(ctx context.Context, client *http.Client, t Target, body io.Reader, hdr http.Header) (*http.Response, error)
}

// Video handles video generation.
type Video interface {
	Protocol() string
	Forward(ctx context.Context, client *http.Client, t Target, ingressPath string, body io.Reader, hdr http.Header) (*http.Response, error)
}