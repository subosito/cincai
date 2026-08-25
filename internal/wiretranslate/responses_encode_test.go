package wiretranslate

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/subosito/cincai/adaptersdk/messages"
	"github.com/subosito/cincai/internal/wiretranslate/sse"
)

type responsesFrame struct {
	Event string
	Data  map[string]any
}

// encodeResponsesFixture encodes events and decodes the resulting SSE frames.
func encodeResponsesFixture(t *testing.T, events []messages.StreamEvent, model string) []responsesFrame {
	t.Helper()
	raw, err := encodeResponsesSSE(events, model)
	if err != nil {
		t.Fatal(err)
	}
	var frames []responsesFrame
	err = sse.ReadFrames(strings.NewReader(string(raw)), func(f sse.Frame) error {
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

func frameNames(frames []responsesFrame) []string {
	var names []string
	for _, f := range frames {
		names = append(names, f.Event)
	}
	return names
}

func equalNames(got []string, want []string) bool {
	if len(got) != len(want) {
		return false
	}
	for i := range want {
		if got[i] != want[i] {
			return false
		}
	}
	return true
}

func TestEncodeResponsesSSETextTurn(t *testing.T) {
	t.Parallel()
	frames := encodeResponsesFixture(t, []messages.StreamEvent{
		{Kind: messages.KindMessageStart, MessageID: "resp_1", Model: "grok-4.6"},
		{Kind: messages.KindTextDelta, Text: "Hello"},
		{Kind: messages.KindTextDelta, Text: " world"},
		{Kind: messages.KindUsage, InputTokens: 10, OutputTokens: 2},
		{Kind: messages.KindMessageStop},
	}, "grok-4.6")
	want := []string{
		"response.created",
		"response.output_item.added",
		"response.content_part.added",
		"response.output_text.delta",
		"response.output_text.delta",
		"response.output_item.done",
		"response.completed",
	}
	if !equalNames(frameNames(frames), want) {
		t.Fatalf("frames=%v", frameNames(frames))
	}
	if frames[0].Data["response"].(map[string]any)["id"] != "resp_1" {
		t.Fatalf("created=%v", frames[0].Data)
	}
	if frames[3].Data["delta"] != "Hello" || frames[4].Data["delta"] != " world" {
		t.Fatalf("deltas=%v %v", frames[3].Data, frames[4].Data)
	}
	if frames[3].Data["output_index"] != float64(0) || frames[3].Data["content_index"] != float64(0) {
		t.Fatalf("delta framing=%v", frames[3].Data)
	}
	// output_item.done carries the assembled message item.
	doneItem := frames[5].Data["item"].(map[string]any)
	if doneItem["type"] != "message" || doneItem["role"] != "assistant" {
		t.Fatalf("done item=%v", doneItem)
	}
	content := doneItem["content"].([]any)
	part := content[0].(map[string]any)
	if part["type"] != "output_text" || part["text"] != "Hello world" {
		t.Fatalf("assembled text=%v", part)
	}
	// completed carries usage.
	completed := frames[6].Data["response"].(map[string]any)
	usage := completed["usage"].(map[string]any)
	if usage["input_tokens"] != float64(10) || usage["output_tokens"] != float64(2) {
		t.Fatalf("usage=%v", usage)
	}
	if completed["status"] != "completed" {
		t.Fatalf("status=%v", completed["status"])
	}
}

func TestEncodeResponsesSSEReasoning(t *testing.T) {
	t.Parallel()
	frames := encodeResponsesFixture(t, []messages.StreamEvent{
		{Kind: messages.KindMessageStart, MessageID: "resp_1", Model: "m"},
		{Kind: messages.KindThinkingDelta, Thinking: "Let me "},
		{Kind: messages.KindThinkingDelta, Thinking: "think..."},
		{Kind: messages.KindTextDelta, Text: "answer"},
		{Kind: messages.KindMessageStop},
	}, "m")
	want := []string{
		"response.created",
		"response.output_item.added", // reasoning
		"response.reasoning.delta",
		"response.reasoning.delta",
		"response.output_item.done",  // reasoning
		"response.output_item.added", // message
		"response.content_part.added",
		"response.output_text.delta",
		"response.output_item.done", // message
		"response.completed",
	}
	if !equalNames(frameNames(frames), want) {
		t.Fatalf("frames=%v", frameNames(frames))
	}
	added := frames[1].Data["item"].(map[string]any)
	if added["type"] != "reasoning" {
		t.Fatalf("reasoning item=%v", added)
	}
	if frames[2].Data["delta"] != "Let me " || frames[3].Data["delta"] != "think..." {
		t.Fatalf("reasoning deltas=%v %v", frames[2].Data, frames[3].Data)
	}
	doneItem := frames[4].Data["item"].(map[string]any)
	summary := doneItem["summary"].([]any)
	if len(summary) != 1 || summary[0].(map[string]any)["text"] != "Let me think..." {
		t.Fatalf("reasoning summary=%v", doneItem)
	}
	// Reasoning and message are distinct output items.
	if frames[1].Data["output_index"] != float64(0) || frames[5].Data["output_index"] != float64(1) {
		t.Fatalf("output indices=%v %v", frames[1].Data, frames[5].Data)
	}
}

func TestEncodeResponsesSSEToolCall(t *testing.T) {
	t.Parallel()
	frames := encodeResponsesFixture(t, []messages.StreamEvent{
		{Kind: messages.KindMessageStart, MessageID: "resp_1", Model: "m"},
		{Kind: messages.KindToolUseStart, ToolIndex: 0, ToolID: "call_1", ToolName: "read"},
		{Kind: messages.KindToolInputDelta, ToolIndex: 0, PartialJSON: `{"pa`},
		{Kind: messages.KindToolInputDelta, ToolIndex: 0, PartialJSON: `th":"foo.txt"}`},
		{Kind: messages.KindToolUseStop, ToolIndex: 0},
		{Kind: messages.KindMessageStop},
	}, "m")
	want := []string{
		"response.created",
		"response.output_item.added",
		"response.function_call_arguments.delta",
		"response.function_call_arguments.delta",
		"response.output_item.done",
		"response.completed",
	}
	if !equalNames(frameNames(frames), want) {
		t.Fatalf("frames=%v", frameNames(frames))
	}
	added := frames[1].Data["item"].(map[string]any)
	if added["type"] != "function_call" || added["call_id"] != "call_1" || added["name"] != "read" {
		t.Fatalf("function_call item=%v", added)
	}
	if frames[2].Data["delta"] != `{"pa` || frames[3].Data["delta"] != `th":"foo.txt"}` {
		t.Fatalf("arg deltas=%v %v", frames[2].Data, frames[3].Data)
	}
	if frames[2].Data["item_id"] != added["id"] {
		t.Fatalf("delta item_id=%v want %v", frames[2].Data["item_id"], added["id"])
	}
	doneItem := frames[4].Data["item"].(map[string]any)
	if doneItem["arguments"] != `{"path":"foo.txt"}` {
		t.Fatalf("assembled arguments=%v", doneItem["arguments"])
	}
	if doneItem["call_id"] != "call_1" {
		t.Fatalf("done call_id=%v", doneItem["call_id"])
	}
}

func TestEncodeResponsesSSEUsageCacheRead(t *testing.T) {
	t.Parallel()
	frames := encodeResponsesFixture(t, []messages.StreamEvent{
		{Kind: messages.KindMessageStart, MessageID: "resp_1", Model: "m"},
		{Kind: messages.KindTextDelta, Text: "ok"},
		{Kind: messages.KindUsage, InputTokens: 100, OutputTokens: 5, CacheReadTokens: 80},
		{Kind: messages.KindMessageStop},
	}, "m")
	completed := frames[len(frames)-1].Data["response"].(map[string]any)
	usage := completed["usage"].(map[string]any)
	if usage["input_tokens"] != float64(100) || usage["output_tokens"] != float64(5) {
		t.Fatalf("usage=%v", usage)
	}
	details := usage["input_tokens_details"].(map[string]any)
	if details["cached_tokens"] != float64(80) {
		t.Fatalf("cached_tokens=%v", details)
	}
}

func TestEncodeResponsesSSEIncompleteOnLength(t *testing.T) {
	t.Parallel()
	frames := encodeResponsesFixture(t, []messages.StreamEvent{
		{Kind: messages.KindMessageStart, MessageID: "resp_1", Model: "m"},
		{Kind: messages.KindTextDelta, Text: "truncated"},
		{Kind: messages.KindTelemetry, Message: "max_tokens"},
		{Kind: messages.KindMessageStop},
	}, "m")
	completed := frames[len(frames)-1].Data["response"].(map[string]any)
	if completed["status"] != "incomplete" {
		t.Fatalf("status=%v", completed["status"])
	}
	if completed["incomplete_details"].(map[string]any)["reason"] != "max_output_tokens" {
		t.Fatalf("incomplete_details=%v", completed["incomplete_details"])
	}
}

func TestEncodeResponsesSSEAPIError(t *testing.T) {
	t.Parallel()
	enc := newResponsesStreamEncoder(&strings.Builder{}, "m")
	err := enc.WriteEvent(messages.StreamEvent{Kind: messages.KindAPIError, Message: "boom"})
	if err == nil || !strings.Contains(err.Error(), "boom") {
		t.Fatalf("err=%v", err)
	}
}

func TestEncodeResponsesJSON(t *testing.T) {
	t.Parallel()
	raw, err := encodeResponsesJSON([]messages.StreamEvent{
		{Kind: messages.KindMessageStart, MessageID: "resp_1", Model: "grok-4.6"},
		{Kind: messages.KindThinkingDelta, Thinking: "hmm"},
		{Kind: messages.KindTextDelta, Text: "Hello"},
		{Kind: messages.KindToolUseStart, ToolIndex: 0, ToolID: "call_1", ToolName: "read"},
		{Kind: messages.KindToolInputDelta, ToolIndex: 0, PartialJSON: `{"path":"foo.txt"}`},
		{Kind: messages.KindToolUseStop, ToolIndex: 0},
		{Kind: messages.KindUsage, InputTokens: 10, OutputTokens: 3, CacheReadTokens: 7},
		{Kind: messages.KindMessageStop},
	}, "grok-4.6")
	if err != nil {
		t.Fatal(err)
	}
	var resp map[string]any
	if err := json.Unmarshal(raw, &resp); err != nil {
		t.Fatal(err)
	}
	if resp["id"] != "resp_1" || resp["object"] != "response" || resp["status"] != "completed" {
		t.Fatalf("resp=%v", resp)
	}
	output := resp["output"].([]any)
	if len(output) != 3 {
		t.Fatalf("output=%v", output)
	}
	if output[0].(map[string]any)["type"] != "reasoning" {
		t.Fatalf("output[0]=%v", output[0])
	}
	msg := output[1].(map[string]any)
	if msg["type"] != "message" || msg["role"] != "assistant" {
		t.Fatalf("output[1]=%v", msg)
	}
	part := msg["content"].([]any)[0].(map[string]any)
	if part["type"] != "output_text" || part["text"] != "Hello" {
		t.Fatalf("message part=%v", part)
	}
	fc := output[2].(map[string]any)
	if fc["type"] != "function_call" || fc["call_id"] != "call_1" || fc["name"] != "read" || fc["arguments"] != `{"path":"foo.txt"}` {
		t.Fatalf("function_call=%v", fc)
	}
	usage := resp["usage"].(map[string]any)
	if usage["input_tokens"] != float64(10) || usage["output_tokens"] != float64(3) {
		t.Fatalf("usage=%v", usage)
	}
	if usage["input_tokens_details"].(map[string]any)["cached_tokens"] != float64(7) {
		t.Fatalf("cached=%v", usage)
	}
}
