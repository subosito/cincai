package wiretranslate

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/subosito/cincai/adaptersdk"
	"github.com/subosito/cincai/adaptersdk/handler"
	"github.com/subosito/cincai/adaptersdk/messages"
	"github.com/subosito/cincai/adaptersdk/upstreamauth"
	cincaicatalog "github.com/subosito/cincai/internal/catalog"
	"github.com/subosito/cincai/observability"
	"github.com/subosito/cincai/upstream"
)

// Adapter registers wire-translate chat handlers.
type Adapter struct{}

func New() *Adapter { return &Adapter{} }

func (a *Adapter) Name() string { return "wire-translate" }

func (a *Adapter) Register(reg *adaptersdk.Registry) error {
	adaptersdk.RegisterChatAdapter(reg, cincaicatalog.AdapterWireTranslateA2O, &Handler{Name: cincaicatalog.AdapterWireTranslateA2O})
	adaptersdk.RegisterChatAdapter(reg, cincaicatalog.AdapterWireTranslateO2A, &Handler{Name: cincaicatalog.AdapterWireTranslateO2A})
	return nil
}

// Handler converts between OpenAI chat and Anthropic messages on the ingress wire.
type Handler struct {
	Name string
}

func (h *Handler) Protocol() string { return h.Name }

func (h *Handler) Forward(ctx context.Context, client *http.Client, t handler.Target, body io.Reader, hdr http.Header) (*http.Response, error) {
	raw, err := io.ReadAll(body)
	if err != nil {
		return nil, err
	}
	switch h.Name {
	case cincaicatalog.AdapterWireTranslateA2O:
		return forwardA2O(ctx, client, t, raw, hdr)
	case cincaicatalog.AdapterWireTranslateO2A:
		return forwardO2A(ctx, client, t, raw, hdr)
	default:
		return nil, fmt.Errorf("wire-translate: unknown handler %q", h.Name)
	}
}

func forwardA2O(ctx context.Context, client *http.Client, t handler.Target, raw []byte, hdr http.Header) (*http.Response, error) {
	var ingress anthropicRequest
	if err := decodeJSON(raw, &ingress); err != nil {
		return nil, err
	}
	upstreamBody, err := anthropicToOpenAIRequest(raw, t.UpstreamModel)
	if err != nil {
		return nil, err
	}
	resp, err := relayPOST(ctx, client, t, "/v1/chat/completions", bytes.NewReader(upstreamBody), hdr)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return passthroughError(resp)
	}
	model := strings.TrimSpace(ingress.Model)
	if model == "" {
		model = t.UpstreamModel
	}
	if ingress.Stream {
		return translateOpenAIStreamToAnthropic(resp.Body, model)
	}
	outRaw, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	events, err := openAINonStreamToEvents(outRaw)
	if err != nil {
		return nil, err
	}
	body, err := encodeAnthropicJSON(events, model)
	if err != nil {
		return nil, err
	}
	return jsonResponse(body), nil
}

func forwardO2A(ctx context.Context, client *http.Client, t handler.Target, raw []byte, hdr http.Header) (*http.Response, error) {
	var ingress openaiChatRequest
	if err := decodeJSON(raw, &ingress); err != nil {
		return nil, err
	}
	upstreamBody, err := openAIToAnthropicRequest(raw, t.UpstreamModel)
	if err != nil {
		return nil, err
	}
	resp, err := relayPOST(ctx, client, t, "/v1/messages", bytes.NewReader(upstreamBody), hdr)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return passthroughError(resp)
	}
	model := strings.TrimSpace(ingress.Model)
	if model == "" {
		model = t.UpstreamModel
	}
	if ingress.Stream {
		return translateAnthropicStreamToOpenAI(resp.Body, model)
	}
	outRaw, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	events, err := anthropicNonStreamToEvents(outRaw)
	if err != nil {
		return nil, err
	}
	body, err := encodeOpenAIJSON(events, model)
	if err != nil {
		return nil, err
	}
	return jsonResponse(body), nil
}

func relayPOST(ctx context.Context, client *http.Client, t handler.Target, path string, body io.Reader, hdr http.Header) (*http.Response, error) {
	url := upstream.JoinURL(t.BaseURL, path)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, body)
	if err != nil {
		return nil, err
	}
	if err := upstreamauth.Apply(t, req, hdr, upstreamauth.BearerDefault()); err != nil {
		return nil, err
	}
	return observability.HTTPDo(ctx, client, req)
}

// Stream translate implementations live in stream_pipe.go (incremental io.Pipe).

func anthropicNonStreamToEvents(raw []byte) ([]messages.StreamEvent, error) {
	var msg struct {
		ID      string `json:"id"`
		Model   string `json:"model"`
		Content []struct {
			Type  string          `json:"type"`
			Text  string          `json:"text"`
			ID    string          `json:"id"`
			Name  string          `json:"name"`
			Input json.RawMessage `json:"input"`
		} `json:"content"`
		StopReason string `json:"stop_reason"`
		Usage      struct {
			InputTokens  int `json:"input_tokens"`
			OutputTokens int `json:"output_tokens"`
		} `json:"usage"`
	}
	if err := json.Unmarshal(raw, &msg); err != nil {
		return nil, err
	}
	var events []messages.StreamEvent
	events = append(events, messages.StreamEvent{Kind: messages.KindMessageStart, MessageID: msg.ID, Model: msg.Model})
	for _, b := range msg.Content {
		switch b.Type {
		case "text":
			if strings.TrimSpace(b.Text) != "" {
				events = append(events, messages.StreamEvent{Kind: messages.KindTextDelta, Text: b.Text})
			}
		case "tool_use":
			args := strings.TrimSpace(string(b.Input))
			if args == "" {
				args = "{}"
			}
			events = append(events,
				messages.StreamEvent{Kind: messages.KindToolUseStart, ToolID: b.ID, ToolName: b.Name},
				messages.StreamEvent{Kind: messages.KindToolInputDelta, PartialJSON: args},
				messages.StreamEvent{Kind: messages.KindToolUseStop},
			)
		}
	}
	events = append(events, messages.StreamEvent{Kind: messages.KindTelemetry, Message: msg.StopReason})
	events = append(events, messages.StreamEvent{Kind: messages.KindMessageStop})
	if msg.Usage.InputTokens > 0 || msg.Usage.OutputTokens > 0 {
		events = append(events, messages.StreamEvent{
			Kind:         messages.KindUsage,
			InputTokens:  msg.Usage.InputTokens,
			OutputTokens: msg.Usage.OutputTokens,
		})
	}
	return events, nil
}

func decodeJSON(raw []byte, v any) error {
	if err := json.Unmarshal(raw, v); err != nil {
		return fmt.Errorf("wire-translate: invalid request: %w", err)
	}
	return nil
}

func passthroughError(resp *http.Response) (*http.Response, error) {
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	return &http.Response{
		StatusCode: resp.StatusCode,
		Header:     resp.Header.Clone(),
		Body:       io.NopCloser(bytes.NewReader(body)),
	}, nil
}

func jsonResponse(body []byte) *http.Response {
	return &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(bytes.NewReader(body)),
	}
}

func sseResponse(body io.Reader) *http.Response {
	return &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"text/event-stream"}},
		Body:       io.NopCloser(body),
	}
}

func openaiSSEResponse(body io.Reader) *http.Response {
	return &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"text/event-stream; charset=utf-8"}},
		Body:       io.NopCloser(body),
	}
}
