package wire

import (
	"testing"

	"github.com/subosito/cincai/catalog"
	"github.com/subosito/cincai/observability"
)

func feed(m *usageMeter, chunks ...string) observability.Usage {
	for _, c := range chunks {
		m.Observe([]byte(c))
	}
	return m.Result()
}

func TestUsageMeterFor(t *testing.T) {
	for _, w := range []string{catalog.WireOpenAIChat, catalog.WireOpenAIResponses, catalog.WireAnthropicMsg} {
		if usageMeterFor(w) == nil {
			t.Fatalf("wire %q: want meter, got nil", w)
		}
	}
	// Media / other wires carry no token usage.
	if usageMeterFor(catalog.WireOpenAIImagesGen) != nil {
		t.Fatal("image wire should have no token meter")
	}
}

func TestOpenAIChat_nonStream(t *testing.T) {
	m := usageMeterFor(catalog.WireOpenAIChat)
	u := feed(m, `{"id":"x","choices":[{"message":{"content":"hi"}}],"usage":{"prompt_tokens":11,"completion_tokens":22,"total_tokens":33}}`)
	if u.InputTokens != 11 || u.OutputTokens != 22 {
		t.Fatalf("usage = %+v, want in=11 out=22", u)
	}
}

func TestOpenAIChat_streamFinalFrame(t *testing.T) {
	m := usageMeterFor(catalog.WireOpenAIChat)
	u := feed(m,
		"data: {\"choices\":[{\"delta\":{\"content\":\"hi\"}}]}\n\n",
		"data: {\"choices\":[],\"usage\":{\"prompt_tokens\":5,\"completion_tokens\":7}}\n\n",
		"data: [DONE]\n\n",
	)
	if u.InputTokens != 5 || u.OutputTokens != 7 {
		t.Fatalf("usage = %+v, want in=5 out=7", u)
	}
}

func TestOpenAIResponsesAPI_inputOutputKeys(t *testing.T) {
	m := usageMeterFor(catalog.WireOpenAIResponses)
	u := feed(m, `{"id":"resp_1","usage":{"input_tokens":40,"output_tokens":9,"total_tokens":49}}`)
	if u.InputTokens != 40 || u.OutputTokens != 9 {
		t.Fatalf("usage = %+v, want in=40 out=9", u)
	}
}

// The r2o/r2a translators emit response.completed with usage nested under
// "response"; the meter must read it (docs/responses-ingress.md §5).
func TestOpenAIResponsesAPI_streamCompletedFrame(t *testing.T) {
	m := usageMeterFor(catalog.WireOpenAIResponses)
	u := feed(m,
		"event: response.created\ndata: {\"type\":\"response.created\",\"response\":{\"id\":\"resp_1\",\"status\":\"in_progress\"}}\n\n",
		"event: response.output_text.delta\ndata: {\"type\":\"response.output_text.delta\",\"output_index\":0,\"content_index\":0,\"delta\":\"hi\"}\n\n",
		"event: response.completed\ndata: {\"type\":\"response.completed\",\"response\":{\"id\":\"resp_1\",\"status\":\"completed\",\"usage\":{\"input_tokens\":100,\"output_tokens\":20,\"input_tokens_details\":{\"cached_tokens\":80}}}}\n\n",
	)
	if u.InputTokens != 100 || u.OutputTokens != 20 || u.CacheReadTokens != 80 {
		t.Fatalf("usage = %+v, want in=100 out=20 cacheRead=80", u)
	}
}

func TestAnthropic_nonStream(t *testing.T) {
	m := usageMeterFor(catalog.WireAnthropicMsg)
	u := feed(m, `{"type":"message","usage":{"input_tokens":100,"output_tokens":25}}`)
	if u.InputTokens != 100 || u.OutputTokens != 25 {
		t.Fatalf("usage = %+v, want in=100 out=25", u)
	}
}

func TestAnthropic_stream(t *testing.T) {
	m := usageMeterFor(catalog.WireAnthropicMsg)
	u := feed(m,
		"event: message_start\ndata: {\"type\":\"message_start\",\"message\":{\"usage\":{\"input_tokens\":100,\"output_tokens\":1}}}\n\n",
		"event: message_delta\ndata: {\"type\":\"message_delta\",\"usage\":{\"output_tokens\":37}}\n\n",
	)
	if u.InputTokens != 100 || u.OutputTokens != 37 {
		t.Fatalf("usage = %+v, want in=100 out=37 (input from message_start, output from message_delta)", u)
	}
}

func TestStreamWithoutUsage_isZero(t *testing.T) {
	m := usageMeterFor(catalog.WireOpenAIChat)
	u := feed(m, "data: {\"choices\":[{\"delta\":{\"content\":\"hi\"}}]}\n\ndata: [DONE]\n\n")
	if !u.Zero() {
		t.Fatalf("usage = %+v, want zero (no include_usage)", u)
	}
}

func TestObserve_splitMidJSON(t *testing.T) {
	// The tee reads arbitrary-sized chunks; usage may split across Observe calls.
	m := usageMeterFor(catalog.WireOpenAIChat)
	full := `{"usage":{"prompt_tokens":8,"completion_tokens":3}}`
	m.Observe([]byte(full[:20]))
	m.Observe([]byte(full[20:]))
	u := m.Result()
	if u.InputTokens != 8 || u.OutputTokens != 3 {
		t.Fatalf("usage = %+v, want in=8 out=3 across split reads", u)
	}
}
