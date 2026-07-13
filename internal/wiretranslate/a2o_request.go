package wiretranslate

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/subosito/cincai/adaptersdk/messages"
)

type anthropicRequest struct {
	Model     string          `json:"model"`
	MaxTokens int             `json:"max_tokens"`
	Stream    bool            `json:"stream"`
	System    any             `json:"system,omitempty"`
	Messages  json.RawMessage `json:"messages"`
	Tools     []anthropicTool `json:"tools,omitempty"`
}

type anthropicTool struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	InputSchema map[string]any `json:"input_schema"`
}

type openaiChatRequest struct {
	Model         string          `json:"model"`
	Messages      []openaiMessage `json:"messages"`
	Tools         []openaiTool    `json:"tools,omitempty"`
	Stream        bool            `json:"stream,omitempty"`
	StreamOptions *streamOptions  `json:"stream_options,omitempty"`
	MaxTokens     int             `json:"max_tokens,omitempty"`
}

type openaiMessage struct {
	Role             string           `json:"role"`
	Content          any              `json:"content,omitempty"`
	ReasoningContent *string          `json:"reasoning_content,omitempty"`
	ToolCalls        []openaiToolCall `json:"tool_calls,omitempty"`
	ToolCallID       string           `json:"tool_call_id,omitempty"`
}

type openaiToolCall struct {
	ID       string `json:"id"`
	Type     string `json:"type"`
	Function struct {
		Name      string `json:"name"`
		Arguments string `json:"arguments"`
	} `json:"function"`
}

type openaiTool struct {
	Type     string `json:"type"`
	Function struct {
		Name        string         `json:"name"`
		Description string         `json:"description"`
		Parameters  map[string]any `json:"parameters"`
	} `json:"function"`
}

type streamOptions struct {
	IncludeUsage bool `json:"include_usage"`
}

func anthropicToOpenAIRequest(raw []byte, upstreamModel string) ([]byte, error) {
	var req anthropicRequest
	if err := json.Unmarshal(raw, &req); err != nil {
		return nil, fmt.Errorf("wire-translate: invalid anthropic request: %w", err)
	}
	model := strings.TrimSpace(req.Model)
	if model == "" {
		model = upstreamModel
	}
	msgs, err := anthropicMessagesToOpenAI(req.System, req.Messages)
	if err != nil {
		return nil, err
	}
	out := openaiChatRequest{
		Model:     model,
		Messages:  msgs,
		MaxTokens: req.MaxTokens,
		Stream:    req.Stream,
	}
	if req.Stream {
		out.StreamOptions = &streamOptions{IncludeUsage: true}
	}
	for _, t := range req.Tools {
		name := strings.TrimSpace(t.Name)
		if name == "" {
			continue
		}
		out.Tools = append(out.Tools, openaiTool{
			Type: "function",
			Function: struct {
				Name        string         `json:"name"`
				Description string         `json:"description"`
				Parameters  map[string]any `json:"parameters"`
			}{Name: name, Description: t.Description, Parameters: t.InputSchema},
		})
	}
	return json.Marshal(out)
}

func anthropicMessagesToOpenAI(system any, raw json.RawMessage) ([]openaiMessage, error) {
	msgs, err := messages.ParseMessages(raw)
	if err != nil {
		return nil, err
	}
	var out []openaiMessage
	if s := extractSystemText(system); strings.TrimSpace(s) != "" {
		out = append(out, openaiMessage{Role: "system", Content: s})
	}
	for _, m := range msgs {
		parts, err := anthropicMessageToOpenAI(m)
		if err != nil {
			return nil, err
		}
		if messageHadToolCalls(m) {
			rc := strings.TrimSpace(m.ReasoningContent)
			for i := range parts {
				if parts[i].Role != "assistant" {
					continue
				}
				parts[i].ReasoningContent = &rc
				if len(parts[i].ToolCalls) > 0 && parts[i].Content == nil {
					parts[i].Content = ""
				}
			}
		} else if rc := strings.TrimSpace(m.ReasoningContent); rc != "" {
			for i := range parts {
				if parts[i].Role == "assistant" {
					parts[i].ReasoningContent = &rc
				}
			}
		}
		out = append(out, parts...)
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("wire-translate: no messages")
	}
	return out, nil
}

func extractSystemText(system any) string {
	switch v := system.(type) {
	case string:
		return v
	case []any:
		var parts []string
		for _, item := range v {
			block, ok := item.(map[string]any)
			if !ok {
				continue
			}
			if strings.TrimSpace(fmt.Sprint(block["type"])) != "text" {
				continue
			}
			if t := strings.TrimSpace(fmt.Sprint(block["text"])); t != "" {
				parts = append(parts, t)
			}
		}
		return strings.Join(parts, "\n")
	default:
		return ""
	}
}

func anthropicMessageToOpenAI(m messages.APIMessage) ([]openaiMessage, error) {
	switch strings.TrimSpace(m.Role) {
	case "user":
		return userToOpenAI(m.Content)
	case "assistant":
		return assistantToOpenAI(m.Content)
	case "system":
		// Not part of the Anthropic spec, but some clients (e.g. Claude Code)
		// put system prompts in the messages array; map to an OpenAI system message.
		if s := extractSystemText(normalizeSystemContent(m.Content)); strings.TrimSpace(s) != "" {
			return []openaiMessage{{Role: "system", Content: s}}, nil
		}
		return nil, nil
	default:
		return nil, fmt.Errorf("wire-translate: unsupported role %q", m.Role)
	}
}

// normalizeSystemContent adapts message content (string or content blocks) to
// the shape extractSystemText expects.
func normalizeSystemContent(content any) any {
	raw, err := json.Marshal(content)
	if err != nil {
		return content
	}
	var generic any
	if err := json.Unmarshal(raw, &generic); err != nil {
		return content
	}
	return generic
}

func userToOpenAI(content any) ([]openaiMessage, error) {
	switch c := content.(type) {
	case string:
		if strings.TrimSpace(c) == "" {
			return nil, nil
		}
		return []openaiMessage{{Role: "user", Content: c}}, nil
	default:
		raw, err := json.Marshal(content)
		if err != nil {
			return nil, err
		}
		var blocks []messages.ContentBlock
		if err := json.Unmarshal(raw, &blocks); err != nil {
			return nil, err
		}
		return blocksToOpenAI(blocks)
	}
}

func assistantToOpenAI(content any) ([]openaiMessage, error) {
	switch c := content.(type) {
	case string:
		if strings.TrimSpace(c) == "" {
			return nil, nil
		}
		return []openaiMessage{{Role: "assistant", Content: c}}, nil
	default:
		raw, err := json.Marshal(content)
		if err != nil {
			return nil, err
		}
		var blocks []messages.ContentBlock
		if err := json.Unmarshal(raw, &blocks); err != nil {
			return nil, err
		}
		var textParts []string
		var toolCalls []openaiToolCall
		for _, b := range blocks {
			switch b.Type {
			case "text":
				if t := strings.TrimSpace(b.Text); t != "" {
					textParts = append(textParts, t)
				}
			case "tool_use":
				args := strings.TrimSpace(string(b.Input))
				if args == "" {
					args = "{}"
				}
				toolCalls = append(toolCalls, openaiToolCall{
					ID:   b.ID,
					Type: "function",
					Function: struct {
						Name      string `json:"name"`
						Arguments string `json:"arguments"`
					}{Name: b.Name, Arguments: args},
				})
			}
		}
		if len(toolCalls) == 0 && len(textParts) == 0 {
			return nil, nil
		}
		msg := openaiMessage{Role: "assistant", ToolCalls: toolCalls}
		if len(textParts) > 0 {
			msg.Content = strings.Join(textParts, "\n")
		}
		return []openaiMessage{msg}, nil
	}
}

func blocksToOpenAI(blocks []messages.ContentBlock) ([]openaiMessage, error) {
	var out []openaiMessage
	for _, b := range blocks {
		switch b.Type {
		case "tool_result":
			out = append(out, openaiMessage{
				Role:       "tool",
				ToolCallID: b.ToolUseID,
				Content:    b.Content,
			})
		case "text":
			if t := strings.TrimSpace(b.Text); t != "" {
				out = append(out, openaiMessage{Role: "user", Content: t})
			}
		}
	}
	return out, nil
}

func messageHadToolCalls(m messages.APIMessage) bool {
	if strings.TrimSpace(m.Role) != "assistant" {
		return false
	}
	raw, err := json.Marshal(m.Content)
	if err != nil {
		return false
	}
	var blocks []messages.ContentBlock
	if err := json.Unmarshal(raw, &blocks); err != nil {
		return false
	}
	for _, b := range blocks {
		if b.Type == "tool_use" {
			return true
		}
	}
	return false
}
