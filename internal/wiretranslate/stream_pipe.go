package wiretranslate

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/subosito/cincai/adaptersdk/messages"
)

// translateAnthropicStreamToOpenAI pipes Anthropic SSE → OpenAI chat SSE incrementally
// so clients (e.g. mow) receive token deltas live rather than after upstream EOF.
func translateAnthropicStreamToOpenAI(r io.Reader, model string) (*http.Response, error) {
	pr, pw := io.Pipe()
	enc := newOpenAIStreamEncoder(pw, model)
	go func() {
		var err error
		defer func() {
			if err != nil {
				_ = pw.CloseWithError(err)
				return
			}
			if closeErr := enc.Close(); closeErr != nil {
				_ = pw.CloseWithError(closeErr)
				return
			}
			_ = pw.Close()
		}()
		err = parseAnthropicStream(r, func(ev messages.StreamEvent) error {
			return enc.WriteEvent(ev)
		})
	}()
	return openaiSSEResponse(pr), nil
}

// translateOpenAIStreamToAnthropic pipes OpenAI chat SSE → Anthropic SSE incrementally.
func translateOpenAIStreamToAnthropic(r io.Reader, model string) (*http.Response, error) {
	pr, pw := io.Pipe()
	enc := newAnthropicStreamEncoder(pw, model)
	go func() {
		var err error
		defer func() {
			if err != nil {
				_ = pw.CloseWithError(err)
				return
			}
			if closeErr := enc.Close(); closeErr != nil {
				_ = pw.CloseWithError(closeErr)
				return
			}
			_ = pw.Close()
		}()
		err = parseOpenAIStream(r, func(ev messages.StreamEvent) error {
			return enc.WriteEvent(ev)
		})
	}()
	return sseResponse(pr), nil
}

// --- OpenAI chat.completion.chunk encoder (incremental) ---

type openAIStreamEncoder struct {
	w           io.Writer
	chunkID     string
	created     int64
	model       string
	roleSent    bool
	wrote       bool
	finish      string
	toolStarted map[int]bool
}

func newOpenAIStreamEncoder(w io.Writer, model string) *openAIStreamEncoder {
	return &openAIStreamEncoder{
		w:           w,
		chunkID:     "chatcmpl-wiretranslate",
		created:     time.Now().Unix(),
		model:       strings.TrimSpace(model),
		finish:      "stop",
		toolStarted: map[int]bool{},
	}
}

func (e *openAIStreamEncoder) writeChunk(choices []map[string]any) error {
	payload := map[string]any{
		"id":      e.chunkID,
		"object":  "chat.completion.chunk",
		"created": e.created,
		"model":   e.model,
		"choices": choices,
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	e.wrote = true
	_, err = fmt.Fprintf(e.w, "data: %s\n\n", raw)
	return err
}

func (e *openAIStreamEncoder) ensureRole() error {
	if e.roleSent {
		return nil
	}
	e.roleSent = true
	return e.writeChunk([]map[string]any{{
		"index": 0,
		"delta": map[string]any{"role": "assistant", "content": ""},
	}})
}

func (e *openAIStreamEncoder) WriteEvent(ev messages.StreamEvent) error {
	switch ev.Kind {
	case messages.KindMessageStart:
		if s := strings.TrimSpace(ev.Model); s != "" {
			e.model = s
		}
		if s := strings.TrimSpace(ev.MessageID); s != "" {
			e.chunkID = s
		}
	case messages.KindTextDelta:
		if err := e.ensureRole(); err != nil {
			return err
		}
		if ev.Text == "" {
			return nil
		}
		return e.writeChunk([]map[string]any{{
			"index": 0,
			"delta": map[string]any{"content": ev.Text},
		}})
	case messages.KindToolUseStart:
		if err := e.ensureRole(); err != nil {
			return err
		}
		idx := ev.ToolIndex
		e.toolStarted[idx] = true
		e.finish = "tool_calls"
		return e.writeChunk([]map[string]any{{
			"index": 0,
			"delta": map[string]any{
				"tool_calls": []map[string]any{{
					"index": idx,
					"id":    ev.ToolID,
					"type":  "function",
					"function": map[string]any{
						"name":      ev.ToolName,
						"arguments": "",
					},
				}},
			},
		}})
	case messages.KindToolInputDelta:
		if ev.PartialJSON == "" {
			return nil
		}
		return e.writeChunk([]map[string]any{{
			"index": 0,
			"delta": map[string]any{
				"tool_calls": []map[string]any{{
					"index": ev.ToolIndex,
					"function": map[string]any{
						"arguments": ev.PartialJSON,
					},
				}},
			},
		}})
	case messages.KindTelemetry:
		if s := strings.TrimSpace(ev.Message); s != "" {
			e.finish = mapOpenAIFinish(s)
		}
	case messages.KindMessageStop:
		if err := e.ensureRole(); err != nil {
			return err
		}
		return e.writeChunk([]map[string]any{{
			"index":         0,
			"delta":         map[string]any{},
			"finish_reason": e.finish,
		}})
	case messages.KindAPIError:
		if ev.Message != "" {
			return fmt.Errorf("wire-translate: upstream: %s", ev.Message)
		}
	}
	return nil
}

func (e *openAIStreamEncoder) Close() error {
	if !e.wrote {
		if err := e.ensureRole(); err != nil {
			return err
		}
		if err := e.writeChunk([]map[string]any{{
			"index":         0,
			"delta":         map[string]any{},
			"finish_reason": e.finish,
		}}); err != nil {
			return err
		}
	}
	_, err := fmt.Fprintf(e.w, "data: [DONE]\n\n")
	return err
}

// encodeOpenAISSE keeps the batch API for non-stream callers/tests.
func encodeOpenAISSE(events []messages.StreamEvent, model string) ([]byte, error) {
	var buf strings.Builder
	enc := newOpenAIStreamEncoder(&stringWriter{&buf}, model)
	for _, ev := range events {
		if err := enc.WriteEvent(ev); err != nil {
			return nil, err
		}
	}
	if err := enc.Close(); err != nil {
		return nil, err
	}
	if buf.Len() == 0 {
		return nil, fmt.Errorf("wire-translate: no openai stream events")
	}
	return []byte(buf.String()), nil
}

// --- Anthropic SSE encoder (incremental) ---

type anthropicStreamEncoder struct {
	w           io.Writer
	model       string
	msgID       string
	msgStarted  bool
	textStarted bool
	textIndex   int
	toolBlocks  map[int]bool
	stopReason  string
	outputTok   int
	wrote       bool
}

func newAnthropicStreamEncoder(w io.Writer, model string) *anthropicStreamEncoder {
	return &anthropicStreamEncoder{
		w:          w,
		model:      strings.TrimSpace(model),
		toolBlocks: map[int]bool{},
		stopReason: "end_turn",
		textIndex:  0,
	}
}

func (e *anthropicStreamEncoder) write(event string, payload any) error {
	raw, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	e.wrote = true
	_, err = fmt.Fprintf(e.w, "event: %s\ndata: %s\n\n", event, raw)
	return err
}

func (e *anthropicStreamEncoder) ensureMessageStart() error {
	if e.msgStarted {
		return nil
	}
	e.msgStarted = true
	id := e.msgID
	if id == "" {
		id = "msg_wiretranslate"
	}
	model := e.model
	if model == "" {
		model = "unknown"
	}
	return e.write("message_start", map[string]any{
		"type": "message_start",
		"message": map[string]any{
			"id":    id,
			"type":  "message",
			"role":  "assistant",
			"model": model,
			"usage": map[string]any{"input_tokens": 0, "output_tokens": 0},
		},
	})
}

func (e *anthropicStreamEncoder) WriteEvent(ev messages.StreamEvent) error {
	switch ev.Kind {
	case messages.KindMessageStart:
		if s := strings.TrimSpace(ev.MessageID); s != "" {
			e.msgID = s
		}
		if s := strings.TrimSpace(ev.Model); s != "" {
			e.model = s
		}
		return e.ensureMessageStart()
	case messages.KindTextDelta:
		if err := e.ensureMessageStart(); err != nil {
			return err
		}
		if !e.textStarted {
			e.textStarted = true
			if err := e.write("content_block_start", map[string]any{
				"type":  "content_block_start",
				"index": e.textIndex,
				"content_block": map[string]any{
					"type": "text",
					"text": "",
				},
			}); err != nil {
				return err
			}
		}
		if ev.Text == "" {
			return nil
		}
		return e.write("content_block_delta", map[string]any{
			"type":  "content_block_delta",
			"index": e.textIndex,
			"delta": map[string]any{
				"type": "text_delta",
				"text": ev.Text,
			},
		})
	case messages.KindToolUseStart:
		if err := e.ensureMessageStart(); err != nil {
			return err
		}
		idx := ev.ToolIndex
		if e.toolBlocks[idx] {
			return nil
		}
		e.toolBlocks[idx] = true
		e.stopReason = "tool_use"
		return e.write("content_block_start", map[string]any{
			"type":  "content_block_start",
			"index": idx,
			"content_block": map[string]any{
				"type":  "tool_use",
				"id":    ev.ToolID,
				"name":  ev.ToolName,
				"input": map[string]any{},
			},
		})
	case messages.KindToolInputDelta:
		return e.write("content_block_delta", map[string]any{
			"type":  "content_block_delta",
			"index": ev.ToolIndex,
			"delta": map[string]any{
				"type":         "input_json_delta",
				"partial_json": ev.PartialJSON,
			},
		})
	case messages.KindToolUseStop:
		return e.write("content_block_stop", map[string]any{
			"type":  "content_block_stop",
			"index": ev.ToolIndex,
		})
	case messages.KindTelemetry:
		if s := strings.TrimSpace(ev.Message); s != "" {
			e.stopReason = s
		}
	case messages.KindUsage:
		e.outputTok = ev.OutputTokens
	case messages.KindMessageStop:
		if err := e.ensureMessageStart(); err != nil {
			return err
		}
		if e.textStarted {
			if err := e.write("content_block_stop", map[string]any{
				"type":  "content_block_stop",
				"index": e.textIndex,
			}); err != nil {
				return err
			}
		}
		if err := e.write("message_delta", map[string]any{
			"type":  "message_delta",
			"delta": map[string]any{"stop_reason": e.stopReason, "stop_sequence": nil},
			"usage": map[string]any{"output_tokens": e.outputTok},
		}); err != nil {
			return err
		}
		return e.write("message_stop", map[string]any{"type": "message_stop"})
	case messages.KindAPIError:
		if ev.Message != "" {
			return fmt.Errorf("wire-translate: upstream: %s", ev.Message)
		}
	}
	return nil
}

func (e *anthropicStreamEncoder) Close() error {
	if !e.wrote {
		if err := e.ensureMessageStart(); err != nil {
			return err
		}
		return e.WriteEvent(messages.StreamEvent{Kind: messages.KindMessageStop})
	}
	return nil
}

// encodeAnthropicSSE batch path for tests / non-pipe callers.
func encodeAnthropicSSE(events []messages.StreamEvent, model string) ([]byte, error) {
	var buf strings.Builder
	enc := newAnthropicStreamEncoder(&stringWriter{&buf}, model)
	for _, ev := range events {
		if err := enc.WriteEvent(ev); err != nil {
			return nil, err
		}
	}
	if err := enc.Close(); err != nil {
		return nil, err
	}
	if buf.Len() == 0 {
		return nil, fmt.Errorf("wire-translate: no anthropic stream events")
	}
	return []byte(buf.String()), nil
}

type stringWriter struct{ b *strings.Builder }

func (w *stringWriter) Write(p []byte) (int, error) { return w.b.Write(p) }
