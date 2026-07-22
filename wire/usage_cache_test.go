package wire

import (
	"bytes"
	"compress/gzip"
	"testing"

	"github.com/subosito/cincai/catalog"
)

func TestAnthropicCacheTokens_nonStream(t *testing.T) {
	m := usageMeterFor(catalog.WireAnthropicMsg)
	u := feed(m, `{"usage":{"input_tokens":100,"output_tokens":25,"cache_read_input_tokens":5000,"cache_creation_input_tokens":200}}`)
	if u.InputTokens != 100 || u.OutputTokens != 25 || u.CacheReadTokens != 5000 || u.CacheWriteTokens != 200 {
		t.Fatalf("usage = %+v, want in=100 out=25 cr=5000 cw=200", u)
	}
}

func TestOpenAICachedTokens(t *testing.T) {
	m := usageMeterFor(catalog.WireOpenAIChat)
	u := feed(m, `{"usage":{"prompt_tokens":80,"completion_tokens":10,"prompt_tokens_details":{"cached_tokens":4000}}}`)
	if u.InputTokens != 80 || u.OutputTokens != 10 || u.CacheReadTokens != 4000 {
		t.Fatalf("usage = %+v, want in=80 out=10 cr=4000", u)
	}
}

// xAI / OpenAI Responses API: usage.input_tokens_details.cached_tokens
func TestResponsesInputTokensDetailsCached(t *testing.T) {
	m := usageMeterFor(catalog.WireOpenAIResponses)
	u := feed(m, `{"usage":{"input_tokens":2227,"input_tokens_details":{"cached_tokens":2176},"output_tokens":27,"output_tokens_details":{"reasoning_tokens":26},"total_tokens":2254}}`)
	if u.InputTokens != 2227 || u.OutputTokens != 27 || u.CacheReadTokens != 2176 {
		t.Fatalf("usage = %+v, want in=2227 out=27 cr=2176", u)
	}
}

func TestResponsesStreamNestedUsageCached(t *testing.T) {
	m := usageMeterFor(catalog.WireOpenAIResponses)
	// final event often nests usage under response
	u := feed(m, `{"type":"response.completed","response":{"usage":{"input_tokens":1000,"input_tokens_details":{"cached_tokens":800},"output_tokens":40}}}`)
	if u.InputTokens != 1000 || u.OutputTokens != 40 || u.CacheReadTokens != 800 {
		t.Fatalf("usage = %+v, want in=1000 out=40 cr=800", u)
	}
}

func TestGzipSSE_inflatesAndParses(t *testing.T) {
	sse := `event: message_start
data: {"type":"message_start","message":{"usage":{"input_tokens":568,"cache_read_input_tokens":4000,"cache_creation_input_tokens":120,"output_tokens":1}}}

event: message_delta
data: {"type":"message_delta","usage":{"output_tokens":16}}

`
	var buf bytes.Buffer
	zw := gzip.NewWriter(&buf)
	_, _ = zw.Write([]byte(sse))
	_ = zw.Close()

	m := usageMeterFor(catalog.WireAnthropicMsg)
	m.encoding = "gzip"
	m.Observe(buf.Bytes())
	u := m.Result()
	if u.InputTokens != 568 || u.OutputTokens != 16 || u.CacheReadTokens != 4000 || u.CacheWriteTokens != 120 {
		t.Fatalf("gzip SSE usage = %+v, want in=568 out=16 cr=4000 cw=120", u)
	}
}
