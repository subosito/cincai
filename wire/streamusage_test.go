package wire

import (
	"encoding/json"
	"testing"

	"github.com/subosito/cincai/catalog"
)

func TestInjectIncludeUsage_streamingAdds(t *testing.T) {
	out, injected := injectIncludeUsage(catalog.WireOpenAIChat, []byte(`{"model":"m","stream":true,"messages":[]}`))
	if !injected {
		t.Fatal("want injected=true for streaming request without include_usage")
	}
	var m map[string]json.RawMessage
	_ = json.Unmarshal(out, &m)
	var opts struct {
		IncludeUsage bool `json:"include_usage"`
	}
	_ = json.Unmarshal(m["stream_options"], &opts)
	if !opts.IncludeUsage {
		t.Fatalf("include_usage not set: %s", out)
	}
}

func TestInjectIncludeUsage_nonStreamSkips(t *testing.T) {
	if _, injected := injectIncludeUsage(catalog.WireOpenAIChat, []byte(`{"model":"m","messages":[]}`)); injected {
		t.Fatal("non-stream request should not inject")
	}
}

func TestInjectIncludeUsage_respectsClientSet(t *testing.T) {
	raw := []byte(`{"model":"m","stream":true,"stream_options":{"include_usage":false}}`)
	out, injected := injectIncludeUsage(catalog.WireOpenAIChat, raw)
	if injected {
		t.Fatal("client set include_usage — must not inject or strip")
	}
	if string(out) != string(raw) {
		t.Fatalf("body must be unchanged, got %s", out)
	}
}

func TestInjectIncludeUsage_mergesExistingStreamOptions(t *testing.T) {
	out, injected := injectIncludeUsage(catalog.WireOpenAIChat, []byte(`{"model":"m","stream":true,"stream_options":{"foo":1}}`))
	if !injected {
		t.Fatal("want injected")
	}
	var m map[string]json.RawMessage
	_ = json.Unmarshal(out, &m)
	var opts map[string]json.RawMessage
	_ = json.Unmarshal(m["stream_options"], &opts)
	if _, ok := opts["foo"]; !ok {
		t.Fatalf("existing stream_options.foo dropped: %s", out)
	}
	if string(opts["include_usage"]) != "true" {
		t.Fatalf("include_usage not merged: %s", out)
	}
}

func TestInjectIncludeUsage_nonOpenAIWireSkips(t *testing.T) {
	if _, injected := injectIncludeUsage(catalog.WireAnthropicMsg, []byte(`{"model":"m","stream":true}`)); injected {
		t.Fatal("anthropic always streams usage — no injection")
	}
}

func TestIsUsageOnlyDataLine(t *testing.T) {
	strip := [][]byte{
		[]byte(`data: {"choices":[],"usage":{"prompt_tokens":5,"completion_tokens":7}}`),
		[]byte(`data:{"choices":[],"usage":{"total_tokens":3}}`),
	}
	for _, l := range strip {
		if !isUsageOnlyDataLine(l) {
			t.Fatalf("want strip: %s", l)
		}
	}
	keep := [][]byte{
		[]byte(`data: {"choices":[{"delta":{"content":"hi"}}]}`), // content frame
		[]byte(`data: [DONE]`), // terminator
		[]byte(`data: {"choices":[{"finish_reason":"stop"}],"usage":{}}`), // has choices
		[]byte(`event: message_start`),                                    // not a data line
		[]byte(``),
	}
	for _, l := range keep {
		if isUsageOnlyDataLine(l) {
			t.Fatalf("must not strip: %s", l)
		}
	}
}
