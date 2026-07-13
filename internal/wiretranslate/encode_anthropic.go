package wiretranslate

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/subosito/cincai/adaptersdk/messages"
)

func encodeAnthropicSSE(events []messages.StreamEvent, model string) ([]byte, error) {
	var buf bytes.Buffer
	var (
		msgID       string
		msgModel    string
		textStarted bool
		textIndex   int
		toolBlocks  = make(map[int]bool)
		outputTok   int
		stopReason  = "end_turn"
	)

	write := func(event string, payload any) error {
		raw, err := json.Marshal(payload)
		if err != nil {
			return err
		}
		if _, err := fmt.Fprintf(&buf, "event: %s\ndata: %s\n\n", event, raw); err != nil {
			return err
		}
		return nil
	}

	for _, ev := range events {
		switch ev.Kind {
		case messages.KindMessageStart:
			msgID = ev.MessageID
			msgModel = ev.Model
			if strings.TrimSpace(msgModel) == "" {
				msgModel = model
			}
			if err := write("message_start", map[string]any{
				"type": "message_start",
				"message": map[string]any{
					"id":    fallbackMsgID(msgID),
					"type":  "message",
					"role":  "assistant",
					"model": msgModel,
					"usage": map[string]any{"input_tokens": 0, "output_tokens": 0},
				},
			}); err != nil {
				return nil, err
			}
		case messages.KindTextDelta:
			if !textStarted {
				textStarted = true
				if err := write("content_block_start", map[string]any{
					"type":  "content_block_start",
					"index": textIndex,
					"content_block": map[string]any{
						"type": "text",
						"text": "",
					},
				}); err != nil {
					return nil, err
				}
			}
			if err := write("content_block_delta", map[string]any{
				"type":  "content_block_delta",
				"index": textIndex,
				"delta": map[string]any{
					"type": "text_delta",
					"text": ev.Text,
				},
			}); err != nil {
				return nil, err
			}
		case messages.KindToolUseStart:
			idx := ev.ToolIndex
			if !toolBlocks[idx] {
				toolBlocks[idx] = true
				if err := write("content_block_start", map[string]any{
					"type":  "content_block_start",
					"index": idx,
					"content_block": map[string]any{
						"type":  "tool_use",
						"id":    ev.ToolID,
						"name":  ev.ToolName,
						"input": map[string]any{},
					},
				}); err != nil {
					return nil, err
				}
			}
		case messages.KindToolInputDelta:
			if err := write("content_block_delta", map[string]any{
				"type":  "content_block_delta",
				"index": ev.ToolIndex,
				"delta": map[string]any{
					"type":         "input_json_delta",
					"partial_json": ev.PartialJSON,
				},
			}); err != nil {
				return nil, err
			}
		case messages.KindToolUseStop:
			if err := write("content_block_stop", map[string]any{
				"type":  "content_block_stop",
				"index": ev.ToolIndex,
			}); err != nil {
				return nil, err
			}
		case messages.KindTelemetry:
			if s := strings.TrimSpace(ev.Message); s != "" {
				stopReason = mapStopReason(s)
			}
		case messages.KindUsage:
			outputTok = ev.OutputTokens
		case messages.KindMessageStop:
			if textStarted {
				if err := write("content_block_stop", map[string]any{
					"type":  "content_block_stop",
					"index": textIndex,
				}); err != nil {
					return nil, err
				}
			}
			if err := write("message_delta", map[string]any{
				"type": "message_delta",
				"delta": map[string]any{
					"stop_reason":   stopReason,
					"stop_sequence": nil,
				},
				"usage": map[string]any{
					"output_tokens": outputTok,
				},
			}); err != nil {
				return nil, err
			}
			if err := write("message_stop", map[string]any{"type": "message_stop"}); err != nil {
				return nil, err
			}
		}
	}
	if buf.Len() == 0 {
		return nil, fmt.Errorf("wire-translate: no anthropic events")
	}
	return buf.Bytes(), nil
}

func encodeAnthropicJSON(events []messages.StreamEvent, model string) ([]byte, error) {
	msg, err := buildAnthropicMessage(events, model)
	if err != nil {
		return nil, err
	}
	return json.Marshal(msg)
}

func buildAnthropicMessage(events []messages.StreamEvent, model string) (map[string]any, error) {
	var (
		msgID    string
		msgModel string
		blocks   []map[string]any
		textBuf  strings.Builder
		inputTok int
		outTok   int
		stop     = "end_turn"
	)
	flushText := func() {
		if textBuf.Len() == 0 {
			return
		}
		blocks = append(blocks, map[string]any{"type": "text", "text": textBuf.String()})
		textBuf.Reset()
	}
	for _, ev := range events {
		switch ev.Kind {
		case messages.KindMessageStart:
			msgID = ev.MessageID
			msgModel = ev.Model
		case messages.KindTextDelta:
			textBuf.WriteString(ev.Text)
		case messages.KindToolUseStart:
			flushText()
			block := map[string]any{
				"type":  "tool_use",
				"id":    ev.ToolID,
				"name":  ev.ToolName,
				"input": map[string]any{},
			}
			blocks = append(blocks, block)
		case messages.KindToolInputDelta:
			if len(blocks) == 0 {
				continue
			}
			last := blocks[len(blocks)-1]
			if last["type"] != "tool_use" {
				continue
			}
			var input map[string]any
			_ = json.Unmarshal([]byte(ev.PartialJSON), &input)
			last["input"] = input
		case messages.KindTelemetry:
			if s := strings.TrimSpace(ev.Message); s != "" {
				stop = mapStopReason(s)
			}
		case messages.KindUsage:
			inputTok = ev.InputTokens
			outTok = ev.OutputTokens
		}
	}
	flushText()
	if strings.TrimSpace(msgModel) == "" {
		msgModel = model
	}
	if len(blocks) == 0 {
		// Reasoning models can burn the whole max_tokens budget before emitting
		// content; return a valid empty message instead of failing the request.
		blocks = []map[string]any{}
	}
	return map[string]any{
		"id":            fallbackMsgID(msgID),
		"type":          "message",
		"role":          "assistant",
		"model":         msgModel,
		"content":       blocks,
		"stop_reason":   stop,
		"stop_sequence": nil,
		"usage": map[string]any{
			"input_tokens":  inputTok,
			"output_tokens": outTok,
		},
	}, nil
}

func fallbackMsgID(id string) string {
	if strings.TrimSpace(id) != "" {
		return id
	}
	return "msg_wiretranslate"
}

func mapStopReason(reason string) string {
	switch strings.TrimSpace(reason) {
	case "tool_calls":
		return "tool_use"
	case "stop", "end_turn":
		return "end_turn"
	case "length", "max_tokens":
		return "max_tokens"
	default:
		if reason == "" {
			return "end_turn"
		}
		return reason
	}
}