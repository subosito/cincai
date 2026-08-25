package wiretranslate

import (
	"context"
	"encoding/json"
	"io"
	"strings"
	"testing"

	"github.com/subosito/cincai/adaptersdk/handler"
	"github.com/subosito/cincai/internal/wiretranslate/sse"
)

// TestForwardR2ORejectsStatefulContinuation: store:true + previous_response_id
// must 400 before any upstream relay — a stateless chat upstream cannot honor
// server-side turn state.
func TestForwardR2ORejectsStatefulContinuation(t *testing.T) {
	t.Parallel()
	raw := []byte(`{"model":"m","store":true,"previous_response_id":"resp_abc","input":"hi"}`)
	resp, err := forwardR2O(context.Background(), nil, handler.Target{}, raw, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 400 {
		t.Fatalf("status=%d", resp.StatusCode)
	}
	body, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(body), "stateful continuation") {
		t.Fatalf("body=%s", body)
	}
}

func TestForwardR2ARejectsStatefulContinuation(t *testing.T) {
	t.Parallel()
	raw := []byte(`{"model":"m","store":true,"previous_response_id":"resp_abc","input":"hi"}`)
	resp, err := forwardR2A(context.Background(), nil, handler.Target{}, raw, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 400 {
		t.Fatalf("status=%d", resp.StatusCode)
	}
}

// TestForwardR2OAllowsStatelessHistory: store:false with a full item history
// (the mow default) translates normally — the guard must not fire.
func TestForwardR2OAllowsStatelessHistory(t *testing.T) {
	t.Parallel()
	raw := []byte(`{"model":"m","store":false,"input":"hi"}`)
	var req responsesRequest
	if err := decodeJSON(raw, &req); err != nil {
		t.Fatal(err)
	}
	if resp := responsesStatefulGuard(&req); resp != nil {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("guard fired on stateless request: %s", body)
	}
}

// TestR2ORoundTrip: responses request → chat request; fixture chat stream →
// responses stream. Asserts tool-call correlation and cached-token mapping.
func TestR2ORoundTrip(t *testing.T) {
	t.Parallel()
	raw := []byte(`{
		"model": "qwen3.8-max",
		"stream": true,
		"input": [
			{"role": "user", "content": "read foo.txt"},
			{"type": "function_call", "call_id": "call_1", "name": "read", "arguments": "{\"path\":\"foo.txt\"}"},
			{"type": "function_call_output", "call_id": "call_1", "output": "<file contents>"}
		],
		"tools": [{"type": "function", "name": "read", "parameters": {"type": "object"}}]
	}`)
	upstreamBody, err := responsesToChatRequest(raw, "qwen/qwen3.8-max")
	if err != nil {
		t.Fatal(err)
	}
	var chatReq openaiChatRequest
	if err := json.Unmarshal(upstreamBody, &chatReq); err != nil {
		t.Fatal(err)
	}
	if len(chatReq.Messages) != 3 || chatReq.Messages[1].ToolCalls[0].ID != "call_1" || chatReq.Messages[2].ToolCallID != "call_1" {
		t.Fatalf("chat messages=%+v", chatReq.Messages)
	}

	chatStream := "" +
		"data: {\"id\":\"chatcmpl-1\",\"model\":\"qwen3.8-max\",\"choices\":[{\"index\":0,\"delta\":{\"role\":\"assistant\",\"content\":\"\"}}]}\n\n" +
		"data: {\"id\":\"chatcmpl-1\",\"model\":\"qwen3.8-max\",\"choices\":[{\"index\":0,\"delta\":{\"tool_calls\":[{\"index\":0,\"id\":\"call_9\",\"type\":\"function\",\"function\":{\"name\":\"write\",\"arguments\":\"\"}}]}}]}\n\n" +
		"data: {\"id\":\"chatcmpl-1\",\"model\":\"qwen3.8-max\",\"choices\":[{\"index\":0,\"delta\":{\"tool_calls\":[{\"index\":0,\"function\":{\"arguments\":\"{\\\"path\\\":\"}}]}}]}\n\n" +
		"data: {\"id\":\"chatcmpl-1\",\"model\":\"qwen3.8-max\",\"choices\":[{\"index\":0,\"delta\":{\"tool_calls\":[{\"index\":0,\"function\":{\"arguments\":\"\\\"foo.txt\\\"}\"}}]}}]}\n\n" +
		"data: {\"id\":\"chatcmpl-1\",\"model\":\"qwen3.8-max\",\"choices\":[{\"index\":0,\"delta\":{},\"finish_reason\":\"tool_calls\"}]}\n\n" +
		"data: {\"id\":\"chatcmpl-1\",\"model\":\"qwen3.8-max\",\"choices\":[],\"usage\":{\"prompt_tokens\":100,\"completion_tokens\":20,\"prompt_tokens_details\":{\"cached_tokens\":80}}}\n\n" +
		"data: [DONE]\n\n"

	resp, err := translateOpenAIStreamToResponses(strings.NewReader(chatStream), "qwen3.8-max")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	out, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	frames := decodeResponsesFrames(t, string(out))

	var argDeltas []string
	var doneItem map[string]any
	var completed map[string]any
	for _, f := range frames {
		switch f.Event {
		case "response.function_call_arguments.delta":
			argDeltas = append(argDeltas, f.Data["delta"].(string))
		case "response.output_item.done":
			doneItem = f.Data["item"].(map[string]any)
		case "response.completed":
			completed = f.Data["response"].(map[string]any)
		}
	}
	if strings.Join(argDeltas, "") != `{"path":"foo.txt"}` {
		t.Fatalf("arg deltas=%v", argDeltas)
	}
	// call_id correlates with the upstream tool_call id so the client can send
	// a matching function_call_output next turn.
	if doneItem["call_id"] != "call_9" || doneItem["name"] != "write" {
		t.Fatalf("done item=%v", doneItem)
	}
	if doneItem["arguments"] != `{"path":"foo.txt"}` {
		t.Fatalf("assembled arguments=%v", doneItem["arguments"])
	}
	usage := completed["usage"].(map[string]any)
	if usage["input_tokens"] != float64(100) || usage["output_tokens"] != float64(20) {
		t.Fatalf("usage=%v", usage)
	}
	if usage["input_tokens_details"].(map[string]any)["cached_tokens"] != float64(80) {
		t.Fatalf("cached_tokens=%v", usage)
	}
}

// TestR2ARoundTrip: responses request → anthropic request (cache_control
// breakpoints); fixture anthropic stream with thinking + tool_use → responses
// stream (reasoning.delta, function_call_arguments.delta, cached tokens).
func TestR2ARoundTrip(t *testing.T) {
	t.Parallel()
	raw := []byte(`{
		"model": "claude-sonnet-5",
		"stream": true,
		"instructions": "You are Claude Code.",
		"input": [
			{"role": "user", "content": "read foo.txt"},
			{"type": "function_call", "call_id": "call_1", "name": "read", "arguments": "{\"path\":\"foo.txt\"}"},
			{"type": "function_call_output", "call_id": "call_1", "output": "<file contents>"}
		],
		"tools": [{"type": "function", "name": "read", "parameters": {"type": "object"}}],
		"reasoning": {"effort": "medium"}
	}`)
	upstreamBody, err := responsesToAnthropicRequest(raw, "claude-sonnet-5")
	if err != nil {
		t.Fatal(err)
	}
	var anthReq map[string]any
	if err := json.Unmarshal(upstreamBody, &anthReq); err != nil {
		t.Fatal(err)
	}
	// Breakpoints: final system block + final user block (tool history).
	system := anthReq["system"].([]any)
	if cc := system[len(system)-1].(map[string]any)["cache_control"].(map[string]any); cc["type"] != "ephemeral" {
		t.Fatalf("system breakpoint=%v", system)
	}
	msgs := anthReq["messages"].([]any)
	lastUser := msgs[len(msgs)-1].(map[string]any)
	blocks := lastUser["content"].([]any)
	if cc := blocks[len(blocks)-1].(map[string]any)["cache_control"].(map[string]any); cc["type"] != "ephemeral" {
		t.Fatalf("user breakpoint=%v", blocks)
	}
	if anthReq["thinking"].(map[string]any)["budget_tokens"] != float64(2048) {
		t.Fatalf("thinking=%v", anthReq["thinking"])
	}

	anthStream := "" +
		"event: message_start\n" +
		"data: {\"type\":\"message_start\",\"message\":{\"id\":\"msg_1\",\"model\":\"claude-sonnet-5\",\"usage\":{\"input_tokens\":100,\"output_tokens\":1,\"cache_read_input_tokens\":90}}}\n\n" +
		"event: content_block_start\n" +
		"data: {\"type\":\"content_block_start\",\"index\":0,\"content_block\":{\"type\":\"thinking\",\"thinking\":\"\"}}\n\n" +
		"event: content_block_delta\n" +
		"data: {\"type\":\"content_block_delta\",\"index\":0,\"delta\":{\"type\":\"thinking_delta\",\"thinking\":\"Let me \"}}\n\n" +
		"event: content_block_delta\n" +
		"data: {\"type\":\"content_block_delta\",\"index\":0,\"delta\":{\"type\":\"thinking_delta\",\"thinking\":\"think...\"}}\n\n" +
		"event: content_block_stop\n" +
		"data: {\"type\":\"content_block_stop\",\"index\":0}\n\n" +
		"event: content_block_start\n" +
		"data: {\"type\":\"content_block_start\",\"index\":1,\"content_block\":{\"type\":\"tool_use\",\"id\":\"toolu_1\",\"name\":\"write\",\"input\":{}}}\n\n" +
		"event: content_block_delta\n" +
		"data: {\"type\":\"content_block_delta\",\"index\":1,\"delta\":{\"type\":\"input_json_delta\",\"partial_json\":\"{\\\"path\\\":\"}}\n\n" +
		"event: content_block_delta\n" +
		"data: {\"type\":\"content_block_delta\",\"index\":1,\"delta\":{\"type\":\"input_json_delta\",\"partial_json\":\"\\\"foo.txt\\\"}\"}}\n\n" +
		"event: content_block_stop\n" +
		"data: {\"type\":\"content_block_stop\",\"index\":1}\n\n" +
		"event: message_delta\n" +
		"data: {\"type\":\"message_delta\",\"delta\":{\"stop_reason\":\"tool_use\"},\"usage\":{\"output_tokens\":25}}\n\n" +
		"event: message_stop\n" +
		"data: {\"type\":\"message_stop\"}\n\n"

	resp, err := translateAnthropicStreamToResponses(strings.NewReader(anthStream), "claude-sonnet-5")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	out, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	frames := decodeResponsesFrames(t, string(out))

	var reasoning []string
	var argDeltas []string
	var doneItem map[string]any
	var completed map[string]any
	for _, f := range frames {
		switch f.Event {
		case "response.reasoning.delta":
			reasoning = append(reasoning, f.Data["delta"].(string))
		case "response.function_call_arguments.delta":
			argDeltas = append(argDeltas, f.Data["delta"].(string))
		case "response.output_item.done":
			if item := f.Data["item"].(map[string]any); item["type"] == "function_call" {
				doneItem = item
			}
		case "response.completed":
			completed = f.Data["response"].(map[string]any)
		}
	}
	if strings.Join(reasoning, "") != "Let me think..." {
		t.Fatalf("reasoning=%v", reasoning)
	}
	if strings.Join(argDeltas, "") != `{"path":"foo.txt"}` {
		t.Fatalf("arg deltas=%v", argDeltas)
	}
	if doneItem["call_id"] != "toolu_1" || doneItem["name"] != "write" {
		t.Fatalf("done item=%v", doneItem)
	}
	usage := completed["usage"].(map[string]any)
	if usage["input_tokens"] != float64(100) || usage["output_tokens"] != float64(25) {
		t.Fatalf("usage=%v", usage)
	}
	if usage["input_tokens_details"].(map[string]any)["cached_tokens"] != float64(90) {
		t.Fatalf("cached_tokens=%v", usage)
	}
}

// TestR2ANonStreamRoundTrip: anthropic JSON response → responses JSON body.
func TestR2ANonStreamRoundTrip(t *testing.T) {
	t.Parallel()
	anthJSON := []byte(`{
		"id": "msg_1", "model": "claude-sonnet-5", "stop_reason": "tool_use",
		"content": [
			{"type": "text", "text": "Let me read that."},
			{"type": "tool_use", "id": "toolu_1", "name": "read", "input": {"path": "foo.txt"}}
		],
		"usage": {"input_tokens": 50, "output_tokens": 12, "cache_read_input_tokens": 40}
	}`)
	events, err := anthropicNonStreamToEvents(anthJSON)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := encodeResponsesJSON(events, "claude-sonnet-5")
	if err != nil {
		t.Fatal(err)
	}
	var resp map[string]any
	if err := json.Unmarshal(raw, &resp); err != nil {
		t.Fatal(err)
	}
	output := resp["output"].([]any)
	if len(output) != 2 {
		t.Fatalf("output=%v", output)
	}
	msg := output[0].(map[string]any)
	if msg["type"] != "message" {
		t.Fatalf("output[0]=%v", msg)
	}
	if msg["content"].([]any)[0].(map[string]any)["text"] != "Let me read that." {
		t.Fatalf("message=%v", msg)
	}
	fc := output[1].(map[string]any)
	if fc["type"] != "function_call" || fc["call_id"] != "toolu_1" {
		t.Fatalf("function_call=%v", fc)
	}
	var fcArgs map[string]any
	if err := json.Unmarshal([]byte(fc["arguments"].(string)), &fcArgs); err != nil {
		t.Fatalf("arguments=%v", fc["arguments"])
	}
	if fcArgs["path"] != "foo.txt" {
		t.Fatalf("function_call args=%v", fcArgs)
	}
	usage := resp["usage"].(map[string]any)
	if usage["input_tokens_details"].(map[string]any)["cached_tokens"] != float64(40) {
		t.Fatalf("usage=%v", usage)
	}
}

func decodeResponsesFrames(t *testing.T, raw string) []responsesFrame {
	t.Helper()
	var frames []responsesFrame
	err := sse.ReadFrames(strings.NewReader(raw), func(f sse.Frame) error {
		var data map[string]any
		if err := json.Unmarshal(f.Data, &data); err != nil {
			return err
		}
		frames = append(frames, responsesFrame{Event: f.Event, Data: data})
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return frames
}
