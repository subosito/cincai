package wiretranslate

import (
	"encoding/json"
	"fmt"
	"strings"
)

func openAIToAnthropicRequest(raw []byte, upstreamModel string) ([]byte, error) {
	var req openaiChatRequest
	if err := json.Unmarshal(raw, &req); err != nil {
		return nil, fmt.Errorf("wire-translate: invalid openai request: %w", err)
	}
	model := strings.TrimSpace(req.Model)
	if model == "" {
		model = upstreamModel
	}
	system, msgs, err := openAIMessagesToAnthropic(req.Messages)
	if err != nil {
		return nil, err
	}
	out := map[string]any{
		"model":      model,
		"max_tokens": req.MaxTokens,
		"stream":     req.Stream,
		"messages":   msgs,
	}
	if system != nil {
		out["system"] = system
	}
	if len(req.Tools) > 0 {
		var tools []map[string]any
		for _, t := range req.Tools {
			name := strings.TrimSpace(t.Function.Name)
			if name == "" {
				continue
			}
			tools = append(tools, map[string]any{
				"name":         name,
				"description":  t.Function.Description,
				"input_schema": t.Function.Parameters,
			})
		}
		if len(tools) > 0 {
			out["tools"] = tools
		}
	}
	if req.MaxTokens == 0 {
		out["max_tokens"] = 4096
	}
	return json.Marshal(out)
}

// openAIMessagesToAnthropic maps OpenAI messages to Anthropic messages + system.
//
// Multiple role=system / developer messages become separate Anthropic system
// text blocks (not joined with newlines). Upstream Anthropic-shaped APIs and
// some auth gates treat system as an ordered list of segments; collapsing them
// into one string can change auth or caching behavior. A single system message
// stays a plain string so simple clients and third-party hosts keep the
// historical shape.
func openAIMessagesToAnthropic(msgs []openaiMessage) (system any, out []map[string]any, err error) {
	var systemParts []string
	for _, m := range msgs {
		role := strings.TrimSpace(m.Role)
		switch role {
		case "system", "developer":
			if s := openAIMessageText(m.Content); s != "" {
				systemParts = append(systemParts, s)
			}
		case "user":
			content, err := openAIUserToAnthropic(m)
			if err != nil {
				return nil, nil, err
			}
			if content != nil {
				out = append(out, map[string]any{"role": "user", "content": content})
			}
		case "assistant":
			content, err := openAIAssistantToAnthropic(m)
			if err != nil {
				return nil, nil, err
			}
			if content != nil {
				msg := map[string]any{"role": "assistant", "content": content}
				if rc := m.ReasoningContent; rc != nil && strings.TrimSpace(*rc) != "" {
					msg["reasoning_content"] = *rc
				}
				out = append(out, msg)
			}
		case "tool":
			out = append(out, map[string]any{
				"role": "user",
				"content": []map[string]any{{
					"type":        "tool_result",
					"tool_use_id": m.ToolCallID,
					"content":     openAIMessageText(m.Content),
				}},
			})
		}
	}
	if len(out) == 0 {
		return nil, nil, fmt.Errorf("wire-translate: no messages")
	}
	return anthropicSystemFromParts(systemParts), out, nil
}

func anthropicSystemFromParts(parts []string) any {
	switch len(parts) {
	case 0:
		return nil
	case 1:
		return parts[0]
	default:
		blocks := make([]map[string]any, 0, len(parts))
		for _, p := range parts {
			blocks = append(blocks, map[string]any{"type": "text", "text": p})
		}
		return blocks
	}
}

func openAIMessageText(content any) string {
	switch c := content.(type) {
	case string:
		return c
	case nil:
		return ""
	default:
		raw, _ := json.Marshal(c)
		return string(raw)
	}
}

func openAIUserToAnthropic(m openaiMessage) (any, error) {
	switch c := m.Content.(type) {
	case string:
		if strings.TrimSpace(c) == "" {
			return nil, nil
		}
		return c, nil
	default:
		text := openAIMessageText(m.Content)
		if strings.TrimSpace(text) == "" {
			return nil, nil
		}
		return text, nil
	}
}

func openAIAssistantToAnthropic(m openaiMessage) (any, error) {
	var blocks []map[string]any
	if text := openAIMessageText(m.Content); strings.TrimSpace(text) != "" {
		blocks = append(blocks, map[string]any{"type": "text", "text": text})
	}
	for _, tc := range m.ToolCalls {
		args := strings.TrimSpace(tc.Function.Arguments)
		if args == "" {
			args = "{}"
		}
		var input any
		_ = json.Unmarshal([]byte(args), &input)
		if input == nil {
			input = map[string]any{}
		}
		blocks = append(blocks, map[string]any{
			"type":  "tool_use",
			"id":    tc.ID,
			"name":  tc.Function.Name,
			"input": input,
		})
	}
	if len(blocks) == 0 {
		return nil, nil
	}
	if len(blocks) == 1 {
		if _, ok := blocks[0]["text"]; ok {
			return blocks[0]["text"], nil
		}
	}
	return blocks, nil
}
