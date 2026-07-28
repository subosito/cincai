package gemini

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestEncodeOpenAICompletion_toolCalls(t *testing.T) {
	t.Parallel()
	raw := []byte(`{
	  "candidates":[{
	    "content":{"role":"model","parts":[
	      {"functionCall":{"name":"recall","args":{"query":"pinjaman"},"id":"c1"}}
	    ]},
	    "finishReason":"STOP"
	  }],
	  "usageMetadata":{"promptTokenCount":10,"candidatesTokenCount":3}
	}`)
	events, err := ParseGenerateResponse(raw, "gemini-test")
	if err != nil {
		t.Fatal(err)
	}
	out, err := EncodeOpenAICompletion(events, "gemini-test")
	if err != nil {
		t.Fatal(err)
	}
	b, _ := json.Marshal(out)
	if !strings.Contains(string(b), `"tool_calls"`) || !strings.Contains(string(b), `"recall"`) {
		t.Fatalf("want tool_calls in %s", b)
	}
	choices := out["choices"].([]map[string]any)
	fr := choices[0]["finish_reason"]
	if fr != "tool_calls" {
		t.Fatalf("finish_reason=%v", fr)
	}
}

func TestEncodeOpenAICompletion_text(t *testing.T) {
	t.Parallel()
	raw := []byte(`{
	  "candidates":[{"content":{"parts":[{"text":"hello"}]},"finishReason":"STOP"}],
	  "usageMetadata":{"promptTokenCount":1,"candidatesTokenCount":1}
	}`)
	events, err := ParseGenerateResponse(raw, "m")
	if err != nil {
		t.Fatal(err)
	}
	out, err := EncodeOpenAICompletion(events, "m")
	if err != nil {
		t.Fatal(err)
	}
	msg := out["choices"].([]map[string]any)[0]["message"].(map[string]any)
	if msg["content"] != "hello" {
		t.Fatalf("%+v", msg)
	}
}

func TestEncodeOpenAISSE_toolCalls(t *testing.T) {
	t.Parallel()
	raw := []byte(`{
	  "candidates":[{
	    "content":{"parts":[{"functionCall":{"name":"bash","args":{"command":"pwd"},"id":"x"}}]},
	    "finishReason":"STOP"
	  }]
	}`)
	events, err := ParseGenerateResponse(raw, "m")
	if err != nil {
		t.Fatal(err)
	}
	sse, err := EncodeOpenAISSE(events, "m")
	if err != nil {
		t.Fatal(err)
	}
	s := string(sse)
	if !strings.Contains(s, "tool_calls") || !strings.Contains(s, "bash") {
		t.Fatalf("sse=%s", s)
	}
	if !strings.Contains(s, "[DONE]") {
		t.Fatal("missing DONE")
	}
}
