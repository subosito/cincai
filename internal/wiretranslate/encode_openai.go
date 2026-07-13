package wiretranslate

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/subosito/cincai/adaptersdk/messages"
)

func encodeOpenAISSE(events []messages.StreamEvent, model string) ([]byte, error) {
	var buf bytes.Buffer
	chunkID := "chatcmpl-wiretranslate"
	created := time.Now().Unix()
	msgModel := strings.TrimSpace(model)
	var (
		roleSent    bool
		textStarted bool
		finish      = "stop"
	)

	writeChunk := func(choices []map[string]any) error {
		payload := map[string]any{
			"id":      chunkID,
			"object":  "chat.completion.chunk",
			"created": created,
			"model":   msgModel,
			"choices": choices,
		}
		raw, err := json.Marshal(payload)
		if err != nil {
			return err
		}
		_, err = fmt.Fprintf(&buf, "data: %s\n\n", raw)
		return err
	}

	for _, ev := range events {
		switch ev.Kind {
		case messages.KindMessageStart:
			if strings.TrimSpace(ev.Model) != "" {
				msgModel = ev.Model
			}
			if strings.TrimSpace(ev.MessageID) != "" {
				chunkID = ev.MessageID
			}
		case messages.KindTextDelta:
			if !roleSent {
				roleSent = true
				if err := writeChunk([]map[string]any{{
					"index": 0,
					"delta": map[string]any{"role": "assistant", "content": ""},
				}}); err != nil {
					return nil, err
				}
			}
			textStarted = true
			if err := writeChunk([]map[string]any{{
				"index": 0,
				"delta": map[string]any{"content": ev.Text},
			}}); err != nil {
				return nil, err
			}
		case messages.KindTelemetry:
			if s := strings.TrimSpace(ev.Message); s != "" {
				finish = mapOpenAIFinish(s)
			}
		case messages.KindMessageStop:
			if !roleSent {
				if err := writeChunk([]map[string]any{{
					"index": 0,
					"delta": map[string]any{"role": "assistant", "content": ""},
				}}); err != nil {
					return nil, err
				}
				roleSent = true
			}
			if err := writeChunk([]map[string]any{{
				"index":         0,
				"delta":         map[string]any{},
				"finish_reason": finish,
			}}); err != nil {
				return nil, err
			}
		}
	}
	if buf.Len() == 0 {
		return nil, fmt.Errorf("wire-translate: no openai stream events")
	}
	if !textStarted {
		if err := writeChunk([]map[string]any{{
			"index": 0,
			"delta": map[string]any{"role": "assistant", "content": ""},
		}}); err != nil {
			return nil, err
		}
	}
	_, _ = fmt.Fprintf(&buf, "data: [DONE]\n\n")
	return buf.Bytes(), nil
}

func encodeOpenAIJSON(events []messages.StreamEvent, model string) ([]byte, error) {
	msg, err := buildAnthropicMessage(events, model)
	if err != nil {
		return nil, err
	}
	content := ""
	var toolCalls []map[string]any
	if blocks, ok := msg["content"].([]map[string]any); ok {
		for _, b := range blocks {
			switch b["type"] {
			case "text":
				content += fmt.Sprint(b["text"])
			case "tool_use":
				args, _ := json.Marshal(b["input"])
				toolCalls = append(toolCalls, map[string]any{
					"id":   b["id"],
					"type": "function",
					"function": map[string]any{
						"name":      b["name"],
						"arguments": string(args),
					},
				})
			}
		}
	}
	finish := "stop"
	if sr, ok := msg["stop_reason"].(string); ok && sr == "tool_use" {
		finish = "tool_calls"
	}
	usage, _ := msg["usage"].(map[string]any)
	resp := map[string]any{
		"id":      msg["id"],
		"object":  "chat.completion",
		"created": time.Now().Unix(),
		"model":   msg["model"],
		"choices": []map[string]any{{
			"index": 0,
			"message": map[string]any{
				"role":       "assistant",
				"content":    content,
				"tool_calls": toolCalls,
			},
			"finish_reason": finish,
		}},
		"usage": map[string]any{
			"prompt_tokens":     usage["input_tokens"],
			"completion_tokens": usage["output_tokens"],
			"total_tokens":      sumTokens(usage),
		},
	}
	return json.Marshal(resp)
}

func sumTokens(usage map[string]any) int {
	in, _ := usage["input_tokens"].(int)
	out, _ := usage["output_tokens"].(int)
	return in + out
}

func mapOpenAIFinish(stop string) string {
	switch strings.TrimSpace(stop) {
	case "tool_use":
		return "tool_calls"
	case "max_tokens":
		return "length"
	default:
		return "stop"
	}
}