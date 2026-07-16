package mistral

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/subosito/cincai/adaptersdk/handler"
)

// ChatHandler for Mistral.
// For regular chat models, passthrough to Mistral's OpenAI compat /v1/chat/completions.
// For ocr models (e.g. mistral-ocr-latest), translate the chat request (with image/document) to Mistral /v1/ocr .
type ChatHandler struct{}

func (h *ChatHandler) Protocol() string { return "openai-chat-completions" }

func (h *ChatHandler) Forward(ctx context.Context, client *http.Client, t handler.Target, body io.Reader, hdr http.Header) (*http.Response, error) {
	bodyBytes, err := io.ReadAll(body)
	if err != nil {
		return nil, fmt.Errorf("mistral chat read: %w", err)
	}

	var req map[string]any
	json.Unmarshal(bodyBytes, &req) // ignore err for now

	model := ""
	if m, ok := req["model"].(string); ok {
		model = strings.ToLower(m)
	}

	targetURL := strings.TrimRight(t.BaseURL, "/") + "/v1/chat/completions"

	if strings.Contains(model, "ocr") || strings.Contains(model, "document") {
		// Translate to OCR
		ocrReq := translateChatToOcr(req)
		ocrBody, _ := json.Marshal(ocrReq)
		httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(t.BaseURL, "/")+"/v1/ocr", bytes.NewReader(ocrBody))
		if err != nil {
			return nil, err
		}
		// copy headers except auth
		for k, vs := range hdr {
			lk := strings.ToLower(k)
			if lk == "authorization" || lk == "content-length" {
				continue
			}
			for _, v := range vs {
				httpReq.Header.Add(k, v)
			}
		}
		if t.Material.Kind == "api_key" && t.Material.APIKey != "" {
			httpReq.Header.Set("Authorization", "Bearer "+t.Material.APIKey)
		}
		httpReq.Header.Set("Content-Type", "application/json")

		resp, err := client.Do(httpReq)
		if err != nil {
			return nil, fmt.Errorf("mistral ocr: %w", err)
		}
		// Translate ocr response back to chat completion format
		return translateOcrRespToChat(resp), nil
	}

	// Normal chat passthrough to mistral /v1/chat/completions
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, targetURL, bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, err
	}
	for k, vs := range hdr {
		if strings.ToLower(k) == "authorization" {
			continue
		}
		for _, v := range vs {
			httpReq.Header.Add(k, v)
		}
	}
	if t.Material.Kind == "api_key" && t.Material.APIKey != "" {
		httpReq.Header.Set("Authorization", "Bearer "+t.Material.APIKey)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	return client.Do(httpReq)
}

func translateChatToOcr(chatReq map[string]any) map[string]any {
	ocrReq := map[string]any{
		"model": chatReq["model"],
	}
	// Extract document from messages content image_url
	if msgs, ok := chatReq["messages"].([]any); ok {
		for _, m := range msgs {
			if msg, ok := m.(map[string]any); ok {
				if content, ok := msg["content"].([]any); ok {
					for _, c := range content {
						if part, ok := c.(map[string]any); ok {
							if iu, ok := part["image_url"].(map[string]any); ok {
								if url, ok := iu["url"].(string); ok {
									docType := "image_url"
									if strings.Contains(url, "pdf") || strings.HasPrefix(url, "data:application/pdf") {
										docType = "document_url"
									}
									ocrReq["document"] = map[string]any{
										"type":  docType,
										docType: url,
									}
									break
								}
							}
						}
					}
				}
			}
		}
	}
	copyOcrPassthrough(ocrReq, chatReq)
	return ocrReq
}

// chatCompletionID mints an OpenAI-shaped id for the completion synthesized from
// Mistral's /v1/ocr response, which carries no id of its own to reuse.
func chatCompletionID() string {
	b := make([]byte, 12)
	// crypto/rand.Read never returns an error and always fills b.
	_, _ = rand.Read(b)
	return "chatcmpl-" + hex.EncodeToString(b)
}

func translateOcrRespToChat(ocrResp *http.Response) *http.Response {
	body, _ := io.ReadAll(ocrResp.Body)
	ocrResp.Body.Close()

	// Upstream errors: relay body unchanged (avoid bogus empty completions).
	if ocrResp.StatusCode < 200 || ocrResp.StatusCode >= 300 {
		ocrResp.Body = io.NopCloser(bytes.NewReader(body))
		ocrResp.ContentLength = int64(len(body))
		return ocrResp
	}

	var ocrData map[string]any
	json.Unmarshal(body, &ocrData)

	msg := map[string]any{
		"role":    "assistant",
		"content": extractOcrMarkdown(ocrData),
	}
	if conf := extractOcrConfidence(ocrData); conf != nil {
		msg["ocr_confidence"] = conf
	}
	chatResp := map[string]any{
		"id":      chatCompletionID(),
		"object":  "chat.completion",
		"created": time.Now().Unix(),
		"model":   ocrData["model"],
		"choices": []map[string]any{
			{
				"index":         0,
				"message":       msg,
				"finish_reason": "stop",
			},
		},
	}

	newBody, _ := json.Marshal(chatResp)
	ocrResp.StatusCode = http.StatusOK
	ocrResp.Body = io.NopCloser(bytes.NewReader(newBody))
	ocrResp.ContentLength = int64(len(newBody))
	// Replace upstream headers: Mistral /v1/ocr Content-Length no longer matches
	// the synthesized chat completion and breaks Go's HTTP client (IncompleteRead).
	ocrResp.Header = make(http.Header)
	ocrResp.Header.Set("Content-Type", "application/json")
	ocrResp.Header.Set("Content-Length", strconv.Itoa(len(newBody)))
	return ocrResp
}
