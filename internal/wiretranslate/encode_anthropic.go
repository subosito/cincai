package wiretranslate

import (
	"encoding/json"
	"strings"

	"github.com/subosito/cincai/adaptersdk/messages"
)

// encodeAnthropicSSE is defined in stream_pipe.go (incremental encoder batch wrapper).

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
