package wiretranslate

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestResponsesToChatRequest(t *testing.T) {
	t.Parallel()
	raw := []byte(`{
		"model": "qwen3.8-max",
		"stream": true,
		"max_output_tokens": 512,
		"reasoning": {"effort": "high"},
		"input": [
			{"role": "developer", "content": "You are Claude Code."},
			{"role": "user", "content": "read foo.txt"},
			{"type": "function_call", "id": "fc_1", "call_id": "call_1", "name": "read", "arguments": "{\"path\":\"foo.txt\"}"},
			{"type": "function_call_output", "call_id": "call_1", "output": "<file contents>"},
			{"role": "user", "content": [{"type": "input_text", "text": "now summarize"}]}
		],
		"tools": [
			{"type": "function", "name": "read", "description": "read a file", "parameters": {"type": "object", "properties": {"path": {"type": "string"}}}}
		]
	}`)
	out, err := responsesToChatRequest(raw, "qwen/qwen3.8-max")
	if err != nil {
		t.Fatal(err)
	}
	var req openaiChatRequest
	if err := json.Unmarshal(out, &req); err != nil {
		t.Fatal(err)
	}
	if req.Model != "qwen3.8-max" {
		t.Fatalf("model=%q", req.Model)
	}
	if !req.Stream || req.StreamOptions == nil || !req.StreamOptions.IncludeUsage {
		t.Fatalf("stream options=%+v", req.StreamOptions)
	}
	if req.MaxTokens != 512 {
		t.Fatalf("max_tokens=%d", req.MaxTokens)
	}
	if req.ReasoningEffort != "high" {
		t.Fatalf("reasoning_effort=%q", req.ReasoningEffort)
	}
	wantRoles := []string{"system", "user", "assistant", "tool", "user"}
	if len(req.Messages) != len(wantRoles) {
		t.Fatalf("messages=%+v", req.Messages)
	}
	for i, role := range wantRoles {
		if req.Messages[i].Role != role {
			t.Fatalf("messages[%d].Role=%q want %q (%+v)", i, req.Messages[i].Role, role, req.Messages)
		}
	}
	asst := req.Messages[2]
	if len(asst.ToolCalls) != 1 || asst.ToolCalls[0].ID != "call_1" || asst.ToolCalls[0].Function.Name != "read" {
		t.Fatalf("tool_calls=%+v", asst.ToolCalls)
	}
	if asst.ToolCalls[0].Function.Arguments != `{"path":"foo.txt"}` {
		t.Fatalf("arguments=%q", asst.ToolCalls[0].Function.Arguments)
	}
	toolMsg := req.Messages[3]
	if toolMsg.ToolCallID != "call_1" || toolMsg.Content != "<file contents>" {
		t.Fatalf("tool message=%+v", toolMsg)
	}
	if req.Messages[4].Content != "now summarize" {
		t.Fatalf("part-form user content=%+v", req.Messages[4].Content)
	}
	if len(req.Tools) != 1 || req.Tools[0].Function.Name != "read" || req.Tools[0].Function.Parameters["type"] != "object" {
		t.Fatalf("tools=%+v", req.Tools)
	}
}

// TestResponsesToChatRequestMergesToolCallsIntoAssistantMessage: an
// assistant turn with both prose and a tool call arrives as a message item
// followed by function_call items. Appending a separate
// {role:"assistant", tool_calls:[...]} message would produce consecutive
// same-role messages, which DeepSeek, Qwen and several vLLM/SGLang front
// ends reject with a 400 — the calls merge into the prose message instead.
func TestResponsesToChatRequestMergesToolCallsIntoAssistantMessage(t *testing.T) {
	t.Parallel()
	raw := []byte(`{
		"model": "m",
		"input": [
			{"role": "user", "content": "read foo.txt"},
			{"type": "message", "role": "assistant", "content": [{"type": "output_text", "text": "Let me read it."}]},
			{"type": "function_call", "call_id": "call_1", "name": "read", "arguments": "{\"path\":\"foo.txt\"}"},
			{"type": "function_call_output", "call_id": "call_1", "output": "contents"}
		]
	}`)
	out, err := responsesToChatRequest(raw, "m")
	if err != nil {
		t.Fatal(err)
	}
	var req openaiChatRequest
	if err := json.Unmarshal(out, &req); err != nil {
		t.Fatal(err)
	}
	wantRoles := []string{"user", "assistant", "tool"}
	if len(req.Messages) != len(wantRoles) {
		t.Fatalf("messages=%+v", req.Messages)
	}
	for i, role := range wantRoles {
		if req.Messages[i].Role != role {
			t.Fatalf("messages[%d].Role=%q want %q (%+v)", i, req.Messages[i].Role, role, req.Messages)
		}
	}
	asst := req.Messages[1]
	if asst.Content != "Let me read it." {
		t.Fatalf("assistant content=%+v", asst.Content)
	}
	if len(asst.ToolCalls) != 1 || asst.ToolCalls[0].ID != "call_1" {
		t.Fatalf("assistant tool_calls=%+v", asst.ToolCalls)
	}
}

func TestResponsesToChatRequestMergesConsecutiveFunctionCalls(t *testing.T) {
	t.Parallel()
	raw := []byte(`{
		"model": "m",
		"input": [
			{"role": "user", "content": "read both"},
			{"type": "function_call", "call_id": "call_1", "name": "read", "arguments": "{\"path\":\"a\"}"},
			{"type": "function_call", "call_id": "call_2", "name": "read", "arguments": "{\"path\":\"b\"}"},
			{"type": "function_call_output", "call_id": "call_1", "output": "A"},
			{"type": "function_call_output", "call_id": "call_2", "output": "B"}
		]
	}`)
	out, err := responsesToChatRequest(raw, "m")
	if err != nil {
		t.Fatal(err)
	}
	var req openaiChatRequest
	if err := json.Unmarshal(out, &req); err != nil {
		t.Fatal(err)
	}
	wantRoles := []string{"user", "assistant", "tool", "tool"}
	if len(req.Messages) != len(wantRoles) {
		t.Fatalf("messages=%+v", req.Messages)
	}
	for i, role := range wantRoles {
		if req.Messages[i].Role != role {
			t.Fatalf("messages[%d].Role=%q want %q", i, req.Messages[i].Role, role)
		}
	}
	if len(req.Messages[1].ToolCalls) != 2 {
		t.Fatalf("merged tool_calls=%+v", req.Messages[1].ToolCalls)
	}
	if req.Messages[2].ToolCallID != "call_1" || req.Messages[3].ToolCallID != "call_2" {
		t.Fatalf("tool correlation=%+v", req.Messages)
	}
}

// TestResponsesToChatRequestToolOutputContentParts: the Responses API permits
// function_call_output.output as an array of content parts (current SDKs emit
// it for non-text tool results). A typed string field fails the whole request
// decode with a 400; the flattener must produce the part text.
func TestResponsesToChatRequestToolOutputContentParts(t *testing.T) {
	t.Parallel()
	raw := []byte(`{
		"model": "m",
		"input": [
			{"role": "user", "content": "read foo.txt"},
			{"type": "function_call", "call_id": "call_1", "name": "read", "arguments": "{\"path\":\"foo.txt\"}"},
			{"type": "function_call_output", "call_id": "call_1", "output": [{"type": "input_text", "text": "line one"}, {"type": "input_text", "text": "line two"}]}
		]
	}`)
	out, err := responsesToChatRequest(raw, "m")
	if err != nil {
		t.Fatal(err)
	}
	var req openaiChatRequest
	if err := json.Unmarshal(out, &req); err != nil {
		t.Fatal(err)
	}
	toolMsg := req.Messages[2]
	if toolMsg.Role != "tool" || toolMsg.ToolCallID != "call_1" {
		t.Fatalf("tool message=%+v", toolMsg)
	}
	if toolMsg.Content != "line one\nline two" {
		t.Fatalf("flattened output=%+v", toolMsg.Content)
	}
}

// TestResponsesToAnthropicRequestToolOutputObject: an error-shaped object
// output keeps its raw JSON form so the tool result round-trips instead of
// failing the request decode.
func TestResponsesToAnthropicRequestToolOutputObject(t *testing.T) {
	t.Parallel()
	raw := []byte(`{
		"model": "m",
		"input": [
			{"role": "user", "content": "read foo.txt"},
			{"type": "function_call", "call_id": "call_1", "name": "read", "arguments": "{\"path\":\"foo.txt\"}"},
			{"type": "function_call_output", "call_id": "call_1", "output": {"error": "permission denied"}}
		]
	}`)
	out, err := responsesToAnthropicRequest(raw, "m")
	if err != nil {
		t.Fatal(err)
	}
	var req map[string]any
	if err := json.Unmarshal(out, &req); err != nil {
		t.Fatal(err)
	}
	msgs := req["messages"].([]any)
	last := msgs[len(msgs)-1].(map[string]any)
	result := last["content"].([]any)[0].(map[string]any)
	if result["type"] != "tool_result" {
		t.Fatalf("tool_result=%v", result)
	}
	var errPayload map[string]any
	content, ok := result["content"].(string)
	if !ok {
		t.Fatalf("tool_result content=%v", result["content"])
	}
	if err := json.Unmarshal([]byte(content), &errPayload); err != nil || errPayload["error"] != "permission denied" {
		t.Fatalf("tool_result content=%q err=%v", content, err)
	}
}

func TestResponsesToChatRequestStringInput(t *testing.T) {
	t.Parallel()
	out, err := responsesToChatRequest([]byte(`{"model":"m","input":"hi"}`), "m")
	if err != nil {
		t.Fatal(err)
	}
	var req openaiChatRequest
	if err := json.Unmarshal(out, &req); err != nil {
		t.Fatal(err)
	}
	if len(req.Messages) != 1 || req.Messages[0].Role != "user" || req.Messages[0].Content != "hi" {
		t.Fatalf("messages=%+v", req.Messages)
	}
}

func TestResponsesToAnthropicRequestCacheBreakpoints(t *testing.T) {
	t.Parallel()
	raw := []byte(`{
		"model": "claude-sonnet-5",
		"instructions": "You are Claude Code.",
		"input": [
			{"role": "developer", "content": "Follow project conventions."},
			{"role": "user", "content": "read foo.txt"},
			{"type": "function_call", "id": "fc_1", "call_id": "call_1", "name": "read", "arguments": "{\"path\":\"foo.txt\"}"},
			{"type": "function_call_output", "call_id": "call_1", "output": "<file contents>"}
		],
		"tools": [
			{"type": "function", "name": "read", "description": "read a file", "parameters": {"type": "object"}},
			{"type": "function", "name": "write", "description": "write a file", "parameters": {"type": "object"}}
		],
		"reasoning": {"effort": "medium"}
	}`)
	out, err := responsesToAnthropicRequest(raw, "claude-sonnet-5")
	if err != nil {
		t.Fatal(err)
	}
	var req map[string]any
	if err := json.Unmarshal(out, &req); err != nil {
		t.Fatal(err)
	}

	// Breakpoint (a): final system block.
	system, ok := req["system"].([]any)
	if !ok || len(system) != 2 {
		t.Fatalf("system=%v", req["system"])
	}
	sys0, _ := system[0].(map[string]any)
	sys1, _ := system[1].(map[string]any)
	if sys0["text"] != "You are Claude Code." || sys1["text"] != "Follow project conventions." {
		t.Fatalf("system blocks=%v", system)
	}
	if _, ok := sys0["cache_control"]; ok {
		t.Fatalf("non-final system block must not be marked: %v", sys0)
	}
	cc, _ := sys1["cache_control"].(map[string]any)
	if cc["type"] != "ephemeral" {
		t.Fatalf("final system block cache_control=%v", sys1["cache_control"])
	}

	// Breakpoint (a): final tool definition only.
	tools, ok := req["tools"].([]any)
	if !ok || len(tools) != 2 {
		t.Fatalf("tools=%v", req["tools"])
	}
	tool0, _ := tools[0].(map[string]any)
	tool1, _ := tools[1].(map[string]any)
	if _, ok := tool0["cache_control"]; ok {
		t.Fatalf("non-final tool must not be marked: %v", tool0)
	}
	cc, _ = tool1["cache_control"].(map[string]any)
	if cc["type"] != "ephemeral" {
		t.Fatalf("final tool cache_control=%v", tool1["cache_control"])
	}
	if tool0["name"] != "read" || tool0["input_schema"] == nil {
		t.Fatalf("tool shape=%v", tool0)
	}

	// Messages: user text, assistant tool_use, user tool_result.
	msgs, ok := req["messages"].([]any)
	if !ok || len(msgs) != 3 {
		t.Fatalf("messages=%v", req["messages"])
	}
	asst, _ := msgs[1].(map[string]any)
	if asst["role"] != "assistant" {
		t.Fatalf("msgs[1]=%v", asst)
	}
	asstBlocks, _ := asst["content"].([]any)
	toolUse, _ := asstBlocks[0].(map[string]any)
	if toolUse["type"] != "tool_use" || toolUse["id"] != "call_1" || toolUse["name"] != "read" {
		t.Fatalf("tool_use=%v", toolUse)
	}
	input, _ := toolUse["input"].(map[string]any)
	if input["path"] != "foo.txt" {
		t.Fatalf("tool_use input=%v", toolUse["input"])
	}
	last, _ := msgs[2].(map[string]any)
	if last["role"] != "user" {
		t.Fatalf("msgs[2]=%v", last)
	}
	lastBlocks, _ := last["content"].([]any)
	result, _ := lastBlocks[0].(map[string]any)
	if result["type"] != "tool_result" || result["tool_use_id"] != "call_1" || result["content"] != "<file contents>" {
		t.Fatalf("tool_result=%v", result)
	}

	// Breakpoint (b): final block of the last user message (tool history).
	cc, _ = result["cache_control"].(map[string]any)
	if cc["type"] != "ephemeral" {
		t.Fatalf("final user block cache_control=%v", result["cache_control"])
	}

	// reasoning.effort → thinking budget.
	thinking, _ := req["thinking"].(map[string]any)
	if thinking["type"] != "enabled" || thinking["budget_tokens"] != float64(2048) {
		t.Fatalf("thinking=%v", req["thinking"])
	}
}

func TestResponsesToAnthropicRequestNoToolHistory(t *testing.T) {
	t.Parallel()
	raw := []byte(`{
		"model": "claude-sonnet-5",
		"input": [
			{"role": "developer", "content": "sys"},
			{"role": "user", "content": "hello"}
		]
	}`)
	out, err := responsesToAnthropicRequest(raw, "claude-sonnet-5")
	if err != nil {
		t.Fatal(err)
	}
	var req map[string]any
	if err := json.Unmarshal(out, &req); err != nil {
		t.Fatal(err)
	}
	system, _ := req["system"].([]any)
	if len(system) != 1 {
		t.Fatalf("system=%v", req["system"])
	}
	sys0, _ := system[0].(map[string]any)
	if cc, _ := sys0["cache_control"].(map[string]any); cc["type"] != "ephemeral" {
		t.Fatalf("system cache_control=%v", sys0["cache_control"])
	}
	msgs, _ := req["messages"].([]any)
	user, _ := msgs[0].(map[string]any)
	blocks, _ := user["content"].([]any)
	block0, _ := blocks[0].(map[string]any)
	if _, ok := block0["cache_control"]; ok {
		t.Fatalf("user block must not be marked without tool history: %v", block0)
	}
	if _, ok := req["thinking"]; ok {
		t.Fatalf("thinking must be absent without effort: %v", req["thinking"])
	}
	if req["max_tokens"] != float64(4096) {
		t.Fatalf("max_tokens=%v", req["max_tokens"])
	}
}

func TestResponsesToAnthropicRequestMergesToolResultsIntoOneUserMessage(t *testing.T) {
	t.Parallel()
	raw := []byte(`{
		"model": "m",
		"input": [
			{"role": "user", "content": "read both"},
			{"type": "function_call", "call_id": "call_1", "name": "read", "arguments": "{\"path\":\"a\"}"},
			{"type": "function_call", "call_id": "call_2", "name": "read", "arguments": "{\"path\":\"b\"}"},
			{"type": "function_call_output", "call_id": "call_1", "output": "A"},
			{"type": "function_call_output", "call_id": "call_2", "output": "B"}
		]
	}`)
	out, err := responsesToAnthropicRequest(raw, "m")
	if err != nil {
		t.Fatal(err)
	}
	var req map[string]any
	if err := json.Unmarshal(out, &req); err != nil {
		t.Fatal(err)
	}
	msgs, ok := req["messages"].([]any)
	if !ok || len(msgs) != 3 {
		t.Fatalf("messages=%v", req["messages"])
	}
	for i, want := range []string{"user", "assistant", "user"} {
		m, _ := msgs[i].(map[string]any)
		if m["role"] != want {
			t.Fatalf("msgs[%d].role=%v want %q", i, m["role"], want)
		}
	}
	asst, _ := msgs[1].(map[string]any)
	if blocks, _ := asst["content"].([]any); len(blocks) != 2 {
		t.Fatalf("assistant tool_use blocks=%v", asst["content"])
	}
	last, _ := msgs[2].(map[string]any)
	blocks, _ := last["content"].([]any)
	if len(blocks) != 2 {
		t.Fatalf("user tool_result blocks=%v", last["content"])
	}
	// Only the final block of the merged user message carries the breakpoint.
	b0, _ := blocks[0].(map[string]any)
	b1, _ := blocks[1].(map[string]any)
	if _, ok := b0["cache_control"]; ok {
		t.Fatalf("non-final user block marked: %v", b0)
	}
	if cc, _ := b1["cache_control"].(map[string]any); cc["type"] != "ephemeral" {
		t.Fatalf("final user block cache_control=%v", b1["cache_control"])
	}
}

// TestResponsesToAnthropicRequestRollingCacheBreakpoints: a single moving
// tail breakpoint limits cache reads to the system+tools prefix — turn N+1
// finds no marker at turn N's position and re-processes the accumulated
// tool body at full price. Two rolling breakpoints keep a marker on the
// previous user turn, which is a guaranteed prefix hit.
func TestResponsesToAnthropicRequestRollingCacheBreakpoints(t *testing.T) {
	t.Parallel()
	raw := []byte(`{
		"model": "m",
		"input": [
			{"role": "user", "content": "read a"},
			{"type": "function_call", "call_id": "call_1", "name": "read", "arguments": "{\"path\":\"a\"}"},
			{"type": "function_call_output", "call_id": "call_1", "output": "A"},
			{"role": "user", "content": "read b"},
			{"type": "function_call", "call_id": "call_2", "name": "read", "arguments": "{\"path\":\"b\"}"},
			{"type": "function_call_output", "call_id": "call_2", "output": "B"}
		]
	}`)
	out, err := responsesToAnthropicRequest(raw, "m")
	if err != nil {
		t.Fatal(err)
	}
	var req map[string]any
	if err := json.Unmarshal(out, &req); err != nil {
		t.Fatal(err)
	}
	msgs := req["messages"].([]any)
	// user("read a"), assistant(tool_use), user(tool_result A + "read b"),
	// assistant(tool_use), user(tool_result B).
	if len(msgs) != 5 {
		t.Fatalf("messages=%v", req["messages"])
	}
	marked := func(i int) bool {
		m, _ := msgs[i].(map[string]any)
		blocks, _ := m["content"].([]any)
		if len(blocks) == 0 {
			return false
		}
		last, _ := blocks[len(blocks)-1].(map[string]any)
		_, ok := last["cache_control"]
		return ok
	}
	if !marked(4) {
		t.Fatalf("last user turn unmarked: %v", msgs[4])
	}
	if !marked(2) {
		t.Fatalf("previous user turn lost its breakpoint: %v", msgs[2])
	}
	if marked(0) {
		t.Fatalf("third-oldest user turn must age out of the rolling window: %v", msgs[0])
	}
}

// TestMarkLastUserBlocksSkipsMalformedMessages: a user message without
// block-form content is skipped, not fatal — one odd message must not
// disable caching for the whole conversation.
func TestMarkLastUserBlocksSkipsMalformedMessages(t *testing.T) {
	t.Parallel()
	msgs := []map[string]any{
		{"role": "user", "content": []map[string]any{{"type": "text", "text": "first"}}},
		{"role": "user", "content": "not-blocks"},
		{"role": "user", "content": []map[string]any{{"type": "text", "text": "last"}}},
	}
	markLastUserBlocks(msgs, 2)
	first := msgs[0]["content"].([]map[string]any)
	last := msgs[2]["content"].([]map[string]any)
	if _, ok := last[0]["cache_control"]; !ok {
		t.Fatalf("last user block unmarked: %v", last)
	}
	if _, ok := first[0]["cache_control"]; !ok {
		t.Fatalf("walk aborted at the malformed message: %v", first)
	}
}

func TestResponsesToAnthropicRequestThinkingBudgetLadder(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		effort string
		budget int
	}{
		{"low", 1024},
		{"medium", 2048},
		{"high", 8192},
		{"none", 0},
	} {
		raw := []byte(`{"model":"m","input":"hi","reasoning":{"effort":"` + tc.effort + `"}}`)
		out, err := responsesToAnthropicRequest(raw, "m")
		if err != nil {
			t.Fatal(err)
		}
		var req map[string]any
		if err := json.Unmarshal(out, &req); err != nil {
			t.Fatal(err)
		}
		thinking, _ := req["thinking"].(map[string]any)
		if tc.budget == 0 {
			if thinking["type"] != "disabled" {
				t.Fatalf("effort=%q thinking=%v", tc.effort, thinking)
			}
			continue
		}
		if thinking["type"] != "enabled" || thinking["budget_tokens"] != float64(tc.budget) {
			t.Fatalf("effort=%q thinking=%v want budget %d", tc.effort, thinking, tc.budget)
		}
		if mt := req["max_tokens"].(float64); int(mt) <= tc.budget {
			t.Fatalf("effort=%q max_tokens=%v must exceed budget %d", tc.effort, mt, tc.budget)
		}
	}
}

func TestResponsesToChatRequestSkipsReasoningItems(t *testing.T) {
	t.Parallel()
	raw := []byte(`{
		"model": "m",
		"input": [
			{"role": "user", "content": "hi"},
			{"type": "reasoning", "id": "rs_1", "summary": []},
			{"type": "message", "role": "assistant", "content": [{"type": "output_text", "text": "hello"}]}
		]
	}`)
	out, err := responsesToChatRequest(raw, "m")
	if err != nil {
		t.Fatal(err)
	}
	var req openaiChatRequest
	if err := json.Unmarshal(out, &req); err != nil {
		t.Fatal(err)
	}
	if len(req.Messages) != 2 || req.Messages[0].Role != "user" || req.Messages[1].Role != "assistant" {
		t.Fatalf("messages=%+v", req.Messages)
	}
	if req.Messages[1].Content != "hello" {
		t.Fatalf("assistant content=%+v", req.Messages[1].Content)
	}
	if strings.Contains(string(out), "rs_1") {
		t.Fatalf("reasoning item leaked into chat request: %s", out)
	}
}
