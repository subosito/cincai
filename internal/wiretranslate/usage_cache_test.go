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
