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
	in := anyToInt(usage["input_tokens"])
	out := anyToInt(usage["output_tokens"])
	cacheRead := anyToInt(usage["cache_read_input_tokens"])
	oaUsage := map[string]any{
		"prompt_tokens":     in,
		"completion_tokens": out,
		"total_tokens":      in + out,
	}
	if cacheRead > 0 {
		oaUsage["prompt_tokens_details"] = map[string]any{"cached_tokens": cacheRead}
	}
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
		"usage": oaUsage,
	}
	return json.Marshal(resp)
}

func anyToInt(v any) int {
	switch n := v.(type) {
	case int:
		return n
	case int64:
		return int(n)
	case float64:
		return int(n)
	default:
		return 0
	}
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
