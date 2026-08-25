package wiretranslate

import (
	"strings"
	"testing"

	"github.com/subosito/cincai/adaptersdk/messages"
)

// parseResponsesFixture runs the stream parser over an SSE fixture and
// collects the events.
func parseResponsesFixture(t *testing.T, fixture string) []messages.StreamEvent {
	t.Helper()
	var out []messages.StreamEvent
	err := parseResponsesStream(strings.NewReader(fixture), func(ev messages.StreamEvent) error {
		out = append(out, ev)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return out
}

func TestParseResponsesStream(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name    string
		fixture string
		check   func(t *testing.T, evs []messages.StreamEvent)
	}{
		{
			name: "created",
			fixture: "event: response.created\n" +
				"data: {\"type\":\"response.created\",\"response\":{\"id\":\"resp_1\",\"model\":\"grok-4.6\",\"status\":\"in_progress\"}}\n\n",
			check: func(t *testing.T, evs []messages.StreamEvent) {
				if len(evs) != 1 || evs[0].Kind != messages.KindMessageStart {
					t.Fatalf("events=%+v", evs)
				}
				if evs[0].MessageID != "resp_1" || evs[0].Model != "grok-4.6" {
					t.Fatalf("start=%+v", evs[0])
				}
			},
		},
		{
			name: "text delta",
			fixture: "event: response.output_text.delta\n" +
				"data: {\"type\":\"response.output_text.delta\",\"output_index\":0,\"content_index\":0,\"delta\":\"Hello\"}\n\n",
			check: func(t *testing.T, evs []messages.StreamEvent) {
				if len(evs) != 1 || evs[0].Kind != messages.KindTextDelta || evs[0].Text != "Hello" {
					t.Fatalf("events=%+v", evs)
				}
			},
		},
		{
			name: "reasoning delta",
			fixture: "event: response.reasoning.delta\n" +
				"data: {\"type\":\"response.reasoning.delta\",\"output_index\":0,\"content_index\":0,\"delta\":\"Let me think\"}\n\n",
			check: func(t *testing.T, evs []messages.StreamEvent) {
				if len(evs) != 1 || evs[0].Kind != messages.KindThinkingDelta || evs[0].Thinking != "Let me think" {
					t.Fatalf("events=%+v", evs)
				}
			},
		},
		{
			name: "reasoning summary delta",
			fixture: "event: response.reasoning_summary_text.delta\n" +
				"data: {\"type\":\"response.reasoning_summary_text.delta\",\"output_index\":0,\"summary_index\":0,\"delta\":\"summary\"}\n\n",
			check: func(t *testing.T, evs []messages.StreamEvent) {
				if len(evs) != 1 || evs[0].Kind != messages.KindThinkingDelta || evs[0].Thinking != "summary" {
					t.Fatalf("events=%+v", evs)
				}
			},
		},
		{
			name: "function_call added",
			fixture: "event: response.output_item.added\n" +
				"data: {\"type\":\"response.output_item.added\",\"output_index\":1,\"item\":{\"type\":\"function_call\",\"id\":\"fc_1\",\"call_id\":\"call_1\",\"name\":\"read\",\"arguments\":\"\"}}\n\n",
			check: func(t *testing.T, evs []messages.StreamEvent) {
				if len(evs) != 1 || evs[0].Kind != messages.KindToolUseStart {
					t.Fatalf("events=%+v", evs)
				}
				if evs[0].ToolIndex != 1 || evs[0].ToolID != "call_1" || evs[0].ToolName != "read" {
					t.Fatalf("start=%+v", evs[0])
				}
			},
		},
		{
			name: "function_call arguments delta",
			fixture: "event: response.output_item.added\n" +
				"data: {\"type\":\"response.output_item.added\",\"output_index\":1,\"item\":{\"type\":\"function_call\",\"id\":\"fc_1\",\"call_id\":\"call_1\",\"name\":\"read\",\"arguments\":\"\"}}\n\n" +
				"event: response.function_call_arguments.delta\n" +
				"data: {\"type\":\"response.function_call_arguments.delta\",\"output_index\":1,\"item_id\":\"fc_1\",\"delta\":\"{\\\"pa\"}\n\n",
			check: func(t *testing.T, evs []messages.StreamEvent) {
				if len(evs) != 2 || evs[1].Kind != messages.KindToolInputDelta {
					t.Fatalf("events=%+v", evs)
				}
				if evs[1].ToolIndex != 1 || evs[1].PartialJSON != `{"pa` {
					t.Fatalf("delta=%+v", evs[1])
				}
			},
		},
		{
			name: "function_call done without streamed deltas carries assembled arguments",
			fixture: "event: response.output_item.added\n" +
				"data: {\"type\":\"response.output_item.added\",\"output_index\":1,\"item\":{\"type\":\"function_call\",\"id\":\"fc_1\",\"call_id\":\"call_1\",\"name\":\"read\",\"arguments\":\"\"}}\n\n" +
				"event: response.output_item.done\n" +
				"data: {\"type\":\"response.output_item.done\",\"output_index\":1,\"item\":{\"type\":\"function_call\",\"id\":\"fc_1\",\"call_id\":\"call_1\",\"name\":\"read\",\"arguments\":\"{\\\"path\\\":\\\"foo.txt\\\"}\"}}\n\n",
			check: func(t *testing.T, evs []messages.StreamEvent) {
				if len(evs) != 3 {
					t.Fatalf("events=%+v", evs)
				}
				if evs[1].Kind != messages.KindToolInputDelta || evs[1].PartialJSON != `{"path":"foo.txt"}` {
					t.Fatalf("assembled delta=%+v", evs[1])
				}
				if evs[2].Kind != messages.KindToolUseStop || evs[2].ToolIndex != 1 {
					t.Fatalf("stop=%+v", evs[2])
				}
			},
		},
		{
			name: "function_call done after streamed deltas emits stop only",
			fixture: "event: response.output_item.added\n" +
				"data: {\"type\":\"response.output_item.added\",\"output_index\":1,\"item\":{\"type\":\"function_call\",\"id\":\"fc_1\",\"call_id\":\"call_1\",\"name\":\"read\",\"arguments\":\"\"}}\n\n" +
				"event: response.function_call_arguments.delta\n" +
				"data: {\"type\":\"response.function_call_arguments.delta\",\"output_index\":1,\"item_id\":\"fc_1\",\"delta\":\"{}\"}\n\n" +
				"event: response.output_item.done\n" +
				"data: {\"type\":\"response.output_item.done\",\"output_index\":1,\"item\":{\"type\":\"function_call\",\"id\":\"fc_1\",\"call_id\":\"call_1\",\"name\":\"read\",\"arguments\":\"{}\"}}\n\n",
			check: func(t *testing.T, evs []messages.StreamEvent) {
				if len(evs) != 3 || evs[2].Kind != messages.KindToolUseStop {
					t.Fatalf("events=%+v", evs)
				}
			},
		},
		{
			name: "message item added is not a tool start",
			fixture: "event: response.output_item.added\n" +
				"data: {\"type\":\"response.output_item.added\",\"output_index\":0,\"item\":{\"type\":\"message\",\"role\":\"assistant\",\"content\":[]}}\n\n",
			check: func(t *testing.T, evs []messages.StreamEvent) {
				if len(evs) != 0 {
					t.Fatalf("events=%+v", evs)
				}
			},
		},
		{
			name: "completed usage with cached tokens",
			fixture: "event: response.completed\n" +
				"data: {\"type\":\"response.completed\",\"response\":{\"id\":\"resp_1\",\"status\":\"completed\",\"usage\":{\"input_tokens\":100,\"output_tokens\":50,\"input_tokens_details\":{\"cached_tokens\":80}}}}\n\n",
			check: func(t *testing.T, evs []messages.StreamEvent) {
				if len(evs) != 2 {
					t.Fatalf("events=%+v", evs)
				}
				u := evs[0]
				if u.Kind != messages.KindUsage || u.InputTokens != 100 || u.OutputTokens != 50 || u.CacheReadTokens != 80 {
					t.Fatalf("usage=%+v", u)
				}
				if evs[1].Kind != messages.KindMessageStop {
					t.Fatalf("stop=%+v", evs[1])
				}
			},
		},
		{
			name: "completed with top-level usage",
			fixture: "event: response.completed\n" +
				"data: {\"type\":\"response.completed\",\"response\":{\"id\":\"resp_1\",\"status\":\"completed\"},\"usage\":{\"input_tokens\":42,\"output_tokens\":7,\"input_tokens_details\":{\"cached_tokens\":30}}}\n\n",
			check: func(t *testing.T, evs []messages.StreamEvent) {
				// Hosts emit usage inside the response object or at the top
				// level; missing the fallback reports zero tokens.
				if len(evs) != 2 {
					t.Fatalf("events=%+v", evs)
				}
				u := evs[0]
				if u.Kind != messages.KindUsage || u.InputTokens != 42 || u.OutputTokens != 7 || u.CacheReadTokens != 30 {
					t.Fatalf("usage=%+v", u)
				}
				if evs[1].Kind != messages.KindMessageStop {
					t.Fatalf("stop=%+v", evs[1])
				}
			},
		},
		{
			name: "failed",
			fixture: "event: response.failed\n" +
				"data: {\"type\":\"response.failed\",\"response\":{\"id\":\"resp_1\",\"status\":\"failed\",\"error\":{\"code\":\"server_error\",\"message\":\"boom\"}}}\n\n",
			check: func(t *testing.T, evs []messages.StreamEvent) {
				if len(evs) != 1 || evs[0].Kind != messages.KindAPIError {
					t.Fatalf("events=%+v", evs)
				}
				if evs[0].Message != "boom" || evs[0].Code != "server_error" {
					t.Fatalf("error=%+v", evs[0])
				}
			},
		},
		{
			name: "error event",
			fixture: "event: error\n" +
				"data: {\"type\":\"error\",\"code\":\"rate_limit\",\"message\":\"slow down\"}\n\n",
			check: func(t *testing.T, evs []messages.StreamEvent) {
				if len(evs) != 1 || evs[0].Kind != messages.KindAPIError || evs[0].Message != "slow down" {
					t.Fatalf("events=%+v", evs)
				}
			},
		},
		{
			name: "unknown event ignored",
			fixture: "event: response.content_part.added\n" +
				"data: {\"type\":\"response.content_part.added\",\"output_index\":0,\"content_index\":0,\"part\":{\"type\":\"output_text\",\"text\":\"\"}}\n\n",
			check: func(t *testing.T, evs []messages.StreamEvent) {
				if len(evs) != 0 {
					t.Fatalf("events=%+v", evs)
				}
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			tc.check(t, parseResponsesFixture(t, tc.fixture))
		})
	}
}

func TestParseResponsesStreamFullTurn(t *testing.T) {
	t.Parallel()
	fixture := "" +
		"event: response.created\n" +
		"data: {\"type\":\"response.created\",\"response\":{\"id\":\"resp_1\",\"model\":\"grok-4.6\"}}\n\n" +
		"event: response.output_item.added\n" +
		"data: {\"type\":\"response.output_item.added\",\"output_index\":0,\"item\":{\"type\":\"message\",\"role\":\"assistant\",\"content\":[]}}\n\n" +
		"event: response.output_text.delta\n" +
		"data: {\"type\":\"response.output_text.delta\",\"output_index\":0,\"content_index\":0,\"delta\":\"Hello\"}\n\n" +
		"event: response.output_text.delta\n" +
		"data: {\"type\":\"response.output_text.delta\",\"output_index\":0,\"content_index\":0,\"delta\":\" world\"}\n\n" +
		"event: response.completed\n" +
		"data: {\"type\":\"response.completed\",\"response\":{\"id\":\"resp_1\",\"usage\":{\"input_tokens\":10,\"output_tokens\":2}}}\n\n"
	evs := parseResponsesFixture(t, fixture)
	var kinds []messages.Kind
	var text strings.Builder
	for _, ev := range evs {
		kinds = append(kinds, ev.Kind)
		text.WriteString(ev.Text)
	}
	if len(kinds) != 5 {
		t.Fatalf("kinds=%v", kinds)
	}
	if kinds[0] != messages.KindMessageStart || kinds[3] != messages.KindUsage || kinds[4] != messages.KindMessageStop {
		t.Fatalf("kinds=%v", kinds)
	}
	if text.String() != "Hello world" {
		t.Fatalf("text=%q", text.String())
	}
}
