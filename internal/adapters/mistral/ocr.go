package mistral

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/subosito/cincai/adaptersdk/handler"
)

// OcrHandler translates incoming document/ocr requests to Mistral's /v1/ocr .
// Supports being called via openai-chat style or direct ocr body.
// Public endpoint example: /v1/chat/completions with model mistral-ocr-latest
// (gateway routes based on model/provider and translates body to ocr).
type OcrHandler struct{}

func (h *OcrHandler) Protocol() string { return "mistral-ocr" }

func (h *OcrHandler) Forward(ctx context.Context, client *http.Client, t handler.Target, body io.Reader, hdr http.Header) (*http.Response, error) {
	// Read the incoming body to decide format.
	bodyBytes, err := io.ReadAll(body)
	if err != nil {
		return nil, fmt.Errorf("mistral ocr read body: %w", err)
	}

	// If it's already mistral ocr format (has "document"), use as-is.
	// Otherwise, if it's chat-like with image, translate to ocr document.
	var req map[string]any
	if err := json.Unmarshal(bodyBytes, &req); err != nil {
		return nil, fmt.Errorf("mistral ocr parse: %w", err)
	}

	ocrBody := req
	if msgs, ok := req["messages"].([]any); ok && len(msgs) > 0 {
		// Translate chat with image_url to ocr document request.
		// Assume first user message has image_url content.
		ocrBody = map[string]any{
			"model": req["model"],
		}
		if doc := extractDocumentFromChat(msgs); doc != nil {
			ocrBody["document"] = doc
		} else {
			// fallback
			ocrBody["document"] = map[string]any{
				"type": "image_url",
				"image_url": "data:image/jpeg;base64,",
			}
		}
		copyOcrPassthrough(ocrBody, req)
	}

	targetURL := strings.TrimRight(t.BaseURL, "/") + "/v1/ocr"
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, targetURL, bytes.NewReader(mustJSON(ocrBody)))
	if err != nil {
		return nil, err
	}
	// copy relevant headers, but override auth from target material
	for k, vs := range hdr {
		if strings.ToLower(k) == "authorization" || strings.ToLower(k) == "content-length" {
			continue
		}
		for _, v := range vs {
			httpReq.Header.Add(k, v)
		}
	}
	// inject from t.Material
	if t.Material.Kind == "api_key" && t.Material.APIKey != "" {
		httpReq.Header.Set("Authorization", "Bearer "+t.Material.APIKey)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("mistral ocr forward: %w", err)
	}
	return resp, nil
}

func extractDocumentFromChat(msgs []any) map[string]any {
	for _, m := range msgs {
		msg, ok := m.(map[string]any)
		if !ok || msg["role"] != "user" {
			continue
		}
		content := msg["content"]
		switch c := content.(type) {
		case []any:
			for _, p := range c {
				part, ok := p.(map[string]any)
				if !ok {
					continue
				}
				if iu, ok := part["image_url"].(map[string]any); ok {
					if url, ok := iu["url"].(string); ok {
						return map[string]any{
							"type":       "image_url",
							"image_url":  url,
						}
					}
				}
			}
		}
	}
	return nil
}

func mustJSON(v any) []byte {
	b, _ := json.Marshal(v)
	return b
}
