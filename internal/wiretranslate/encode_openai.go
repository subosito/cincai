package wiretranslate

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/subosito/cincai/adaptersdk/messages"
)

// encodeOpenAISSE is defined in stream_pipe.go (incremental encoder batch wrapper).

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
