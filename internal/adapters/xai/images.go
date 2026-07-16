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
	ar := normalizeXAIAspect(req.AspectRatio, req.Size)
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

// normalizeXAIAspect maps OpenAI-style size (e.g. 1024x1024) and explicit
// aspect_ratio into values accepted by the xAI image API.
func normalizeXAIAspect(aspectRatio, size string) string {
	ar := strings.TrimSpace(aspectRatio)
	if ar == "" {
		ar = strings.TrimSpace(size)
	}
	if ar == "" {
		return "auto"
	}
	switch strings.ToLower(ar) {
	case "auto", "1:1", "16:9", "9:16", "4:3", "3:4", "3:2", "2:3":
		return strings.ToLower(ar)
	// Common OpenAI / DALL·E pixel sizes → nearest aspect.
	case "256x256", "512x512", "1024x1024":
		return "1:1"
	case "1792x1024", "1536x1024", "1344x768":
		return "16:9"
	case "1024x1792", "1024x1536", "768x1344":
		return "9:16"
	default:
		// WxH pixel pair → pick closest supported aspect.
		if w, h, ok := parsePixelSize(ar); ok && w > 0 && h > 0 {
			return closestAspect(w, h)
		}
		return "auto"
	}
}

func parsePixelSize(s string) (w, h int, ok bool) {
	s = strings.ToLower(strings.TrimSpace(s))
	parts := strings.Split(s, "x")
	if len(parts) != 2 {
		return 0, 0, false
	}
	var a, b int
	if _, err := fmt.Sscanf(parts[0], "%d", &a); err != nil {
		return 0, 0, false
	}
	if _, err := fmt.Sscanf(parts[1], "%d", &b); err != nil {
		return 0, 0, false
	}
	return a, b, true
}

func closestAspect(w, h int) string {
	r := float64(w) / float64(h)
	type cand struct {
		name string
		r    float64
	}
	cands := []cand{
		{"1:1", 1}, {"16:9", 16.0 / 9}, {"9:16", 9.0 / 16},
		{"4:3", 4.0 / 3}, {"3:4", 3.0 / 4}, {"3:2", 1.5}, {"2:3", 2.0 / 3},
	}
	best := "auto"
	bestD := 1e9
	for _, c := range cands {
		d := r - c.r
		if d < 0 {
			d = -d
		}
		if d < bestD {
			bestD = d
			best = c.name
		}
	}
	return best
}
