package wiretranslate

import (
	"encoding/json"
	"testing"
)

func TestAnthropicToOpenAIRequest(t *testing.T) {
	raw := []byte(`{
		"model": "deepseek-v4-flash-cc",
		"max_tokens": 64,
		"stream": true,
		"system": "You are helpful.",
		"messages": [{"role":"user","content":"hi"}]
	}`)
	out, err := anthropicToOpenAIRequest(raw, "deepseek/deepseek-v4-flash")
	if err != nil {
		t.Fatal(err)
	}
	var req openaiChatRequest
	if err := json.Unmarshal(out, &req); err != nil {
		t.Fatal(err)
	}
	if req.Model != "deepseek-v4-flash-cc" {
		t.Fatalf("model=%q", req.Model)
	}
	if !req.Stream || req.StreamOptions == nil || !req.StreamOptions.IncludeUsage {
		t.Fatalf("stream options=%+v", req.StreamOptions)
	}
	if len(req.Messages) != 2 || req.Messages[0].Role != "system" || req.Messages[1].Role != "user" {
		t.Fatalf("messages=%+v", req.Messages)
	}
}

func TestAnthropicToOpenAIRequestSystemRoleMessage(t *testing.T) {
	raw := []byte(`{
		"model": "qwen3.7-plus",
		"max_tokens": 64,
		"messages": [
			{"role":"system","content":"You are a helpful assistant."},
			{"role":"system","content":[{"type":"text","text":"Block form.","cache_control":{"type":"ephemeral"}}]},
			{"role":"user","content":"hi"}
		]
	}`)
	out, err := anthropicToOpenAIRequest(raw, "qwen/qwen3.7-plus")
	if err != nil {
		t.Fatal(err)
	}
	var req openaiChatRequest
	if err := json.Unmarshal(out, &req); err != nil {
		t.Fatal(err)
	}
	if len(req.Messages) != 3 {
		t.Fatalf("messages=%+v", req.Messages)
	}
	if req.Messages[0].Role != "system" || req.Messages[0].Content != "You are a helpful assistant." {
		t.Fatalf("system[0]=%+v", req.Messages[0])
	}
	if req.Messages[1].Role != "system" || req.Messages[1].Content != "Block form." {
		t.Fatalf("system[1]=%+v", req.Messages[1])
	}
	if req.Messages[2].Role != "user" {
		t.Fatalf("user=%+v", req.Messages[2])
	}
}

func TestOpenAIToAnthropicRequest(t *testing.T) {
	raw := []byte(`{
		"model": "claude-sonnet",
		"max_tokens": 32,
		"stream": false,
		"messages": [
			{"role":"system","content":"sys"},
			{"role":"user","content":"hello"}
		]
	}`)
	out, err := openAIToAnthropicRequest(raw, "claude-sonnet-4-6")
	if err != nil {
		t.Fatal(err)
	}
	var req map[string]any
	if err := json.Unmarshal(out, &req); err != nil {
		t.Fatal(err)
	}
	if req["system"] != "sys" {
		t.Fatalf("system=%v", req["system"])
	}
	msgs, ok := req["messages"].([]any)
	if !ok || len(msgs) != 1 {
		t.Fatalf("messages=%v", req["messages"])
	}
}

// Multiple OpenAI system messages must become separate Anthropic system text
// blocks (order-preserving), not one joined string.
func TestOpenAIToAnthropicRequest_SystemBlocksFromMultipleSystemMessages(t *testing.T) {
	raw := []byte(`{
		"model": "example-model",
		"max_tokens": 32,
		"stream": false,
		"messages": [
			{"role":"system","content":"You are a helpful assistant."},
			{"role":"system","content":"Follow project conventions."},
			{"role":"user","content":"hi"}
		]
	}`)
	out, err := openAIToAnthropicRequest(raw, "example-model")
	if err != nil {
		t.Fatal(err)
	}
	var req map[string]any
	if err := json.Unmarshal(out, &req); err != nil {
		t.Fatal(err)
	}
	blocks, ok := req["system"].([]any)
	if !ok || len(blocks) != 2 {
		t.Fatalf("system want 2 blocks, got %T %v", req["system"], req["system"])
	}
	b0, _ := blocks[0].(map[string]any)
	b1, _ := blocks[1].(map[string]any)
	if b0["type"] != "text" || b0["text"] != "You are a helpful assistant." {
		t.Fatalf("system[0]=%v", blocks[0])
	}
	if b1["type"] != "text" || b1["text"] != "Follow project conventions." {
		t.Fatalf("system[1]=%v", blocks[1])
	}
	if s, ok := req["system"].(string); ok {
		t.Fatalf("system must not be a joined string: %q", s)
	}
}
