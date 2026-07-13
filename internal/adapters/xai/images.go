package xai

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/subosito/cincai/adaptersdk/handler"
	"github.com/subosito/cincai/observability"
	"github.com/subosito/cincai/upstream"

	"github.com/subosito/cincai/adaptersdk/upstreamauth"
)

// ImageHandler translates OpenAI /v1/images/* to xAI image API.
type ImageHandler struct{}

func (h *ImageHandler) Protocol() string { return "xai-images" }

type openAIImageGenerateReq struct {
	Model       string `json:"model"`
	Prompt      string `json:"prompt"`
	N           int    `json:"n"`
	AspectRatio string `json:"aspect_ratio,omitempty"`
	Resolution  string `json:"resolution,omitempty"`
	Size        string `json:"size,omitempty"`
}

type openAIImageEditReq struct {
	Model  string `json:"model"`
	Prompt string `json:"prompt"`
	Image  string `json:"image"`
}

func (h *ImageHandler) Forward(ctx context.Context, client *http.Client, t handler.Target, ingressPath string, body io.Reader, hdr http.Header) (*http.Response, error) {
	raw, err := io.ReadAll(body)
	if err != nil {
		return nil, err
	}
	model := strings.TrimSpace(t.UpstreamModel)
	if model == "" {
		return nil, fmt.Errorf("xai-images: upstream model required")
	}

	upstreamBody, err := buildXAIImageBody(ingressPath, raw, model)
	if err != nil {
		return nil, err
	}

	targetURL := upstream.ImageUpstreamPath(t.BaseURL, ingressPath)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, targetURL, bytes.NewReader(upstreamBody))
	if err != nil {
		return nil, err
	}
	if err := upstreamauth.ApplyTranslated(t, req, hdr, upstreamauth.BearerDefault()); err != nil {
		return nil, err
	}
	return observability.HTTPDo(ctx, client, req)
}

func buildXAIImageBody(ingressPath string, raw []byte, model string) ([]byte, error) {
	switch ingressPath {
	case "/v1/images/edits":
		return buildXAIEditBody(raw, model)
	default:
		return buildXAIGenerateBody(raw, model)
	}
}

func buildXAIGenerateBody(raw []byte, model string) ([]byte, error) {
	var req openAIImageGenerateReq
	if err := json.Unmarshal(raw, &req); err != nil {
		return nil, fmt.Errorf("xai-images: invalid json: %w", err)
	}
	prompt := strings.TrimSpace(req.Prompt)
	if prompt == "" {
		return nil, fmt.Errorf("xai-images: prompt is required")
	}
	resolution := strings.ToLower(strings.TrimSpace(req.Resolution))
	if resolution != "1k" && resolution != "2k" {
		resolution = "2k"
	}
	n := req.N
	if n <= 0 {
		n = 1
	}
	if n > 4 {
		n = 4
	}
	ar := strings.TrimSpace(req.AspectRatio)
	if ar == "" {
		ar = strings.TrimSpace(req.Size)
	}
	if ar == "" {
		ar = "auto"
	}
	switch ar {
	case "auto", "1:1", "16:9", "9:16", "4:3", "3:4", "3:2", "2:3":
	default:
		return nil, fmt.Errorf("xai-images: invalid aspect_ratio %q", ar)
	}
	body := map[string]any{
		"model":           model,
		"prompt":          prompt,
		"resolution":      resolution,
		"n":               n,
		"response_format": "b64_json",
		"aspect_ratio":    ar,
	}
	return json.Marshal(body)
}

func buildXAIEditBody(raw []byte, model string) ([]byte, error) {
	var req openAIImageEditReq
	if err := json.Unmarshal(raw, &req); err != nil {
		return nil, fmt.Errorf("xai-images: invalid json: %w", err)
	}
	prompt := strings.TrimSpace(req.Prompt)
	if prompt == "" {
		return nil, fmt.Errorf("xai-images: prompt is required")
	}
	image := strings.TrimSpace(req.Image)
	if image == "" {
		return nil, fmt.Errorf("xai-images: image is required")
	}
	body := map[string]any{
		"model":           model,
		"prompt":          prompt,
		"resolution":      "2k",
		"n":               1,
		"response_format": "b64_json",
		"image": map[string]any{
			"type": "image_url",
			"url":  imageDataURL(image),
		},
	}
	return json.Marshal(body)
}

func imageDataURL(raw string) string {
	raw = strings.TrimSpace(raw)
	if strings.HasPrefix(raw, "data:") {
		return raw
	}
	return "data:image/png;base64," + raw
}