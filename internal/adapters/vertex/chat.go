package vertex

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/subosito/cincai/adaptersdk/handler"

	"github.com/subosito/cincai/adapters/gemini"
)

// ChatHandler translates OpenAI /v1/chat/completions to Gemini generateContent
// over a generateContent-compatible base_url (zenmux vertex-ai, GCP Vertex, …).
type ChatHandler struct{}

func (h *ChatHandler) Protocol() string { return "vertex" }

func (h *ChatHandler) Forward(ctx context.Context, client *http.Client, t handler.Target, body io.Reader, hdr http.Header) (*http.Response, error) {
	raw, err := io.ReadAll(body)
	if err != nil {
		return nil, err
	}
	var req gemini.OpenAIRequest
	if err := json.Unmarshal(raw, &req); err != nil {
		return nil, fmt.Errorf("vertex: invalid openai request: %w", err)
	}
	gen, err := gemini.FromOpenAI(req)
	if err != nil {
		return nil, err
	}
	model := strings.TrimSpace(req.Model)
	if model == "" {
		model = strings.TrimSpace(t.UpstreamModel)
	}
	events, err := callGenerate(ctx, client, t, model, gen, req.Stream)
	if resp, ok := upstreamHTTPResponse(err); ok {
		return resp, nil
	}
	if err != nil {
		return nil, err
	}

	if req.Stream {
		sseBody, err := gemini.EncodeOpenAISSE(events, model)
		if err != nil {
			return nil, err
		}
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{"text/event-stream"}},
			Body:       io.NopCloser(bytes.NewReader(sseBody)),
		}, nil
	}

	msg, err := gemini.EncodeOpenAICompletion(events, model)
	if err != nil {
		return nil, err
	}
	rawOut, err := json.Marshal(msg)
	if err != nil {
		return nil, err
	}
	return &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(bytes.NewReader(rawOut)),
	}, nil
}
