package wiretranslate

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/subosito/cincai/adaptersdk/messages"
)

func TestOpenAINonStreamToEvents_cacheRead(t *testing.T) {
	t.Parallel()
	raw := []byte(`{
	  "id":"chatcmpl-1","model":"gpt-test",
	  "choices":[{"message":{"role":"assistant","content":"hi"},"finish_reason":"stop"}],
	  "usage":{"prompt_tokens":100,"completion_tokens":5,"prompt_tokens_details":{"cached_tokens":80}}
	}`)
	evs, err := openAINonStreamToEvents(raw)
	if err != nil {
		t.Fatal(err)
	}
	var cache int
	for _, ev := range evs {
		if ev.Kind == messages.KindUsage {
			cache = ev.CacheReadTokens
		}
	}
	if cache != 80 {
		t.Fatalf("CacheReadTokens=%d want 80", cache)
	}
}

func TestEncodeOpenAIJSON_cacheRead(t *testing.T) {
	t.Parallel()
	b, err := encodeOpenAIJSON([]messages.StreamEvent{
		{Kind: messages.KindMessageStart, MessageID: "m1", Model: "claude"},
		{Kind: messages.KindTextDelta, Text: "ok"},
		{Kind: messages.KindUsage, InputTokens: 50, OutputTokens: 3, CacheReadTokens: 40},
		{Kind: messages.KindMessageStop},
	}, "claude")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(b), `"cached_tokens":40`) && !strings.Contains(string(b), `"cached_tokens": 40`) {
		t.Fatalf("missing cached_tokens: %s", b)
	}
}

func TestBuildAnthropicMessage_cache(t *testing.T) {
	t.Parallel()
	msg, err := buildAnthropicMessage([]messages.StreamEvent{
		{Kind: messages.KindMessageStart, MessageID: "m1", Model: "claude"},
		{Kind: messages.KindTextDelta, Text: "ok"},
		{Kind: messages.KindUsage, InputTokens: 50, OutputTokens: 3, CacheReadTokens: 40, CacheWriteTokens: 10},
	}, "claude")
	if err != nil {
		t.Fatal(err)
	}
	usage, _ := msg["usage"].(map[string]any)
	if usage["cache_read_input_tokens"] != 40 {
		t.Fatalf("usage=%v", usage)
	}
	if usage["cache_creation_input_tokens"] != 10 {
		t.Fatalf("usage=%v", usage)
	}
}

func TestAnthropicNonStreamToEvents_cache(t *testing.T) {
	t.Parallel()
	raw := []byte(`{
	  "id":"msg_1","model":"claude","stop_reason":"end_turn",
	  "content":[{"type":"text","text":"hi"}],
	  "usage":{"input_tokens":20,"output_tokens":2,"cache_read_input_tokens":15,"cache_creation_input_tokens":5}
	}`)
	evs, err := anthropicNonStreamToEvents(raw)
	if err != nil {
		t.Fatal(err)
	}
	var read, write int
	for _, ev := range evs {
		if ev.Kind == messages.KindUsage {
			read, write = ev.CacheReadTokens, ev.CacheWriteTokens
		}
	}
	if read != 15 || write != 5 {
		t.Fatalf("cache read=%d write=%d", read, write)
	}
}

// TestParseAnthropicFrame_totalInputTokens: Anthropic's input_tokens excludes
// cached tokens; the KindUsage contract is the total prompt, so the parser
// folds cache_read + cache_creation in once.
func TestParseAnthropicFrame_totalInputTokens(t *testing.T) {
	t.Parallel()
	data := []byte(`{"type":"message_start","message":{"id":"msg_1","model":"claude","usage":{"input_tokens":100,"output_tokens":1,"cache_read_input_tokens":4000,"cache_creation_input_tokens":200}}}`)
	evs, err := parseAnthropicFrame("message_start", data)
	if err != nil {
		t.Fatal(err)
	}
	var usage *messages.StreamEvent
	for i := range evs {
		if evs[i].Kind == messages.KindUsage {
			usage = &evs[i]
		}
	}
	if usage == nil {
		t.Fatalf("events=%+v", evs)
	}
	if usage.InputTokens != 4300 {
		t.Fatalf("InputTokens=%d want 4300 (100 + 4000 + 200)", usage.InputTokens)
	}
	if usage.CacheReadTokens != 4000 || usage.CacheWriteTokens != 200 {
		t.Fatalf("cache read=%d write=%d", usage.CacheReadTokens, usage.CacheWriteTokens)
	}
	if usage.CacheReadTokens+usage.CacheWriteTokens > usage.InputTokens {
		t.Fatalf("invariant violated: cached %d + %d > input %d", usage.CacheReadTokens, usage.CacheWriteTokens, usage.InputTokens)
	}
}

// TestEncodeOpenAIJSON_promptIncludesCache: the o2a path — Anthropic usage
// normalized at the parser must surface on the OpenAI wire as the full
// prompt_tokens (OpenAI's prompt_tokens includes cached tokens).
func TestEncodeOpenAIJSON_promptIncludesCache(t *testing.T) {
	t.Parallel()
	b, err := encodeOpenAIJSON([]messages.StreamEvent{
		{Kind: messages.KindMessageStart, MessageID: "m1", Model: "claude"},
		{Kind: messages.KindTextDelta, Text: "ok"},
		{Kind: messages.KindUsage, InputTokens: 4300, OutputTokens: 3, CacheReadTokens: 4000, CacheWriteTokens: 200},
		{Kind: messages.KindMessageStop},
	}, "claude")
	if err != nil {
		t.Fatal(err)
	}
	var resp struct {
		Usage struct {
			PromptTokens int `json:"prompt_tokens"`
		} `json:"usage"`
	}
	if err := json.Unmarshal(b, &resp); err != nil {
		t.Fatal(err)
	}
	if resp.Usage.PromptTokens != 4300 {
		t.Fatalf("prompt_tokens=%d want 4300: %s", resp.Usage.PromptTokens, b)
	}
}

// TestEncodeResponsesSSEUsageDetailsAlwaysEmitted: OpenAI always sends
// input_tokens_details; SDK models with a non-optional field fail to decode
// when it is absent, so emit it even with zero cached tokens.
func TestEncodeResponsesSSEUsageDetailsAlwaysEmitted(t *testing.T) {
	t.Parallel()
	frames := encodeResponsesFixture(t, []messages.StreamEvent{
		{Kind: messages.KindMessageStart, MessageID: "resp_1", Model: "m"},
		{Kind: messages.KindTextDelta, Text: "ok"},
		{Kind: messages.KindUsage, InputTokens: 10, OutputTokens: 2},
		{Kind: messages.KindMessageStop},
	}, "m")
	completed := frames[len(frames)-1].Data["response"].(map[string]any)
	usage := completed["usage"].(map[string]any)
	details, ok := usage["input_tokens_details"].(map[string]any)
	if !ok {
		t.Fatalf("input_tokens_details missing: %v", usage)
	}
	if details["cached_tokens"] != float64(0) {
		t.Fatalf("cached_tokens=%v", details)
	}
}

// TestNonStreamToEventsUsageBeforeStop: the KindUsage contract requires
// usage to precede MessageStop — encoders that assemble the terminal frame
// on stop (chat completed, responses completed) read token counts at that
// moment, so usage appended after stop encodes zero tokens on the live SSE
// path.
func TestNonStreamToEventsUsageBeforeStop(t *testing.T) {
	t.Parallel()
	assertUsageBeforeStop := func(t *testing.T, evs []messages.StreamEvent) {
		t.Helper()
		usageIdx, stopIdx := -1, -1
		for i, ev := range evs {
			switch ev.Kind {
			case messages.KindUsage:
				usageIdx = i
			case messages.KindMessageStop:
				stopIdx = i
			}
		}
		if usageIdx < 0 || stopIdx < 0 {
			t.Fatalf("events=%+v", evs)
		}
		if usageIdx > stopIdx {
			t.Fatalf("usage (index %d) after message stop (index %d)", usageIdx, stopIdx)
		}
	}

	t.Run("anthropic", func(t *testing.T) {
		t.Parallel()
		evs, err := anthropicNonStreamToEvents([]byte(`{
		  "id":"msg_1","model":"claude","stop_reason":"end_turn",
		  "content":[{"type":"text","text":"hi"}],
		  "usage":{"input_tokens":20,"output_tokens":2}
		}`))
		if err != nil {
			t.Fatal(err)
		}
		assertUsageBeforeStop(t, evs)
	})
	t.Run("openai", func(t *testing.T) {
		t.Parallel()
		evs, err := openAINonStreamToEvents([]byte(`{
		  "id":"chatcmpl-1","model":"gpt-test",
		  "choices":[{"message":{"role":"assistant","content":"hi"},"finish_reason":"stop"}],
		  "usage":{"prompt_tokens":20,"completion_tokens":2}
		}`))
		if err != nil {
			t.Fatal(err)
		}
		assertUsageBeforeStop(t, evs)
	})
}

// TestAnthropicNonStreamToEventsToolIndex: tool_use blocks must carry their
// content-block index — with per-index argument buffering, parallel calls
// sharing index 0 collide and overwrite each other.
func TestAnthropicNonStreamToEventsToolIndex(t *testing.T) {
	t.Parallel()
	raw := []byte(`{
	  "id":"msg_1","model":"claude","stop_reason":"tool_use",
	  "content":[
	    {"type":"text","text":"reading both"},
	    {"type":"tool_use","id":"toolu_1","name":"read","input":{"path":"a"}},
	    {"type":"tool_use","id":"toolu_2","name":"read","input":{"path":"b"}}
	  ],
	  "usage":{"input_tokens":10,"output_tokens":5}
	}`)
	evs, err := anthropicNonStreamToEvents(raw)
	if err != nil {
		t.Fatal(err)
	}
	var starts []messages.StreamEvent
	for _, ev := range evs {
		if ev.Kind == messages.KindToolUseStart {
			starts = append(starts, ev)
		}
	}
	if len(starts) != 2 {
		t.Fatalf("tool starts=%+v", starts)
	}
	if starts[0].ToolIndex != 1 || starts[1].ToolIndex != 2 {
		t.Fatalf("tool indices=%d,%d want 1,2", starts[0].ToolIndex, starts[1].ToolIndex)
	}
	if starts[0].ToolID != "toolu_1" || starts[1].ToolID != "toolu_2" {
		t.Fatalf("tool ids=%+v", starts)
	}
}

func TestParseOpenAIChunk_cacheRead(t *testing.T) {
	t.Parallel()
	active := map[int]bool{}
	started := false
	data := []byte(`{"id":"c1","model":"m","choices":[],"usage":{"prompt_tokens":10,"completion_tokens":1,"prompt_tokens_details":{"cached_tokens":7}}}`)
	evs, err := parseOpenAIChunk(data, active, &started)
	if err != nil {
		t.Fatal(err)
	}
	var cache int
	for _, ev := range evs {
		if ev.Kind == messages.KindUsage {
			cache = ev.CacheReadTokens
		}
	}
	if cache != 7 {
		t.Fatalf("cache=%d; events=%+v", cache, evs)
	}
	// round-trip sanity via JSON
	_ = json.RawMessage(data)
}
