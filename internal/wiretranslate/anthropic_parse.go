package wiretranslate

import (
	"encoding/json"
	"fmt"
	"io"

	"github.com/subosito/cincai/adaptersdk/messages"
	"github.com/subosito/cincai/internal/wiretranslate/sse"
)

func parseAnthropicStream(r io.Reader, fn func(messages.StreamEvent) error) error {
	return sse.ReadFrames(r, func(frame sse.Frame) error {
		events, err := parseAnthropicFrame(frame.Event, frame.Data)
		if err != nil {
			return err
		}
		for _, ev := range events {
			if err := fn(ev); err != nil {
				return err
			}
		}
		return nil
	})
}

func parseAnthropicFrame(eventName string, data []byte) ([]messages.StreamEvent, error) {
	if len(data) == 0 {
		return nil, nil
	}
	var base struct {
		Type string `json:"type"`
	}
	if err := json.Unmarshal(data, &base); err != nil {
		return nil, fmt.Errorf("wire-translate: parse anthropic frame: %w", err)
	}
	typ := base.Type
	if typ == "" && eventName == "error" {
		typ = "error"
	}
	switch typ {
	case "ping":
		return []messages.StreamEvent{{Kind: messages.KindPing}}, nil
	case "message_start":
		var raw struct {
			Message struct {
				ID    string `json:"id"`
				Model string `json:"model"`
			} `json:"message"`
		}
		if err := json.Unmarshal(data, &raw); err != nil {
			return nil, err
		}
		return []messages.StreamEvent{{
			Kind:      messages.KindMessageStart,
			MessageID: raw.Message.ID,
			Model:     raw.Message.Model,
		}}, nil
	case "message_stop":
		return []messages.StreamEvent{{Kind: messages.KindMessageStop}}, nil
	case "message_delta":
		var raw struct {
			Usage struct {
				InputTokens  int `json:"input_tokens"`
				OutputTokens int `json:"output_tokens"`
			} `json:"usage"`
			Delta struct {
				StopReason string `json:"stop_reason"`
			} `json:"delta"`
		}
		if err := json.Unmarshal(data, &raw); err != nil {
			return nil, err
		}
		out := []messages.StreamEvent{{Kind: messages.KindTelemetry, Message: raw.Delta.StopReason}}
		if raw.Usage.InputTokens > 0 || raw.Usage.OutputTokens > 0 {
			out = append(out, messages.StreamEvent{
				Kind:         messages.KindUsage,
				InputTokens:  raw.Usage.InputTokens,
				OutputTokens: raw.Usage.OutputTokens,
			})
		}
		return out, nil
	case "content_block_start":
		var raw struct {
			Index        int `json:"index"`
			ContentBlock struct {
				Type string `json:"type"`
				ID   string `json:"id"`
				Name string `json:"name"`
			} `json:"content_block"`
		}
		if err := json.Unmarshal(data, &raw); err != nil {
			return nil, err
		}
		if raw.ContentBlock.Type == "tool_use" {
			return []messages.StreamEvent{{
				Kind:      messages.KindToolUseStart,
				ToolIndex: raw.Index,
				ToolID:    raw.ContentBlock.ID,
				ToolName:  raw.ContentBlock.Name,
			}}, nil
		}
		return nil, nil
	case "content_block_stop":
		var raw struct {
			Index int `json:"index"`
		}
		if err := json.Unmarshal(data, &raw); err != nil {
			return nil, err
		}
		return []messages.StreamEvent{{Kind: messages.KindToolUseStop, ToolIndex: raw.Index}}, nil
	case "content_block_delta":
		return parseAnthropicContentDelta(data)
	case "error":
		var raw struct {
			Error struct {
				Type    string `json:"type"`
				Message string `json:"message"`
			} `json:"error"`
		}
		_ = json.Unmarshal(data, &raw)
		return []messages.StreamEvent{{Kind: messages.KindAPIError, Message: raw.Error.Message, Code: raw.Error.Type}}, nil
	default:
		return nil, nil
	}
}

func parseAnthropicContentDelta(data []byte) ([]messages.StreamEvent, error) {
	var raw struct {
		Index int `json:"index"`
		Delta struct {
			Type        string `json:"type"`
			Text        string `json:"text"`
			PartialJSON string `json:"partial_json"`
		} `json:"delta"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, err
	}
	switch raw.Delta.Type {
	case "text_delta":
		return []messages.StreamEvent{{Kind: messages.KindTextDelta, Text: raw.Delta.Text}}, nil
	case "input_json_delta":
		return []messages.StreamEvent{{
			Kind:        messages.KindToolInputDelta,
			ToolIndex:   raw.Index,
			PartialJSON: raw.Delta.PartialJSON,
		}}, nil
	default:
		return nil, nil
	}
}
