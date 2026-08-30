package catalog_test

import (
	"encoding/json"
	"testing"

	"github.com/subosito/cincai/catalog"
)

func TestIsHybridThinkingEfforts(t *testing.T) {
	t.Parallel()
	if !catalog.IsHybridThinkingEfforts([]string{"none", "on"}) {
		t.Fatal("want hybrid")
	}
	if !catalog.IsHybridThinkingEfforts([]string{"on", "off"}) {
		t.Fatal("off alias")
	}
	if catalog.IsHybridThinkingEfforts([]string{"none", "low", "high"}) {
		t.Fatal("ladder is not hybrid")
	}
	if catalog.IsHybridThinkingEfforts([]string{"none"}) {
		t.Fatal("single value")
	}
}

func TestStripEffortHints(t *testing.T) {
	t.Parallel()
	raw := []byte(`{"model":"cs/gemini-3.7-flash","reasoning_effort":"high","effort":"high","reasoning":{"effort":"high"},"messages":[]}`)
	out := catalog.StripEffortHints(raw)
	var body map[string]any
	if err := json.Unmarshal(out, &body); err != nil {
		t.Fatal(err)
	}
	if _, ok := body["reasoning_effort"]; ok {
		t.Fatal("reasoning_effort still set")
	}
	if _, ok := body["effort"]; ok {
		t.Fatal("effort still set")
	}
	if _, ok := body["reasoning"]; ok {
		t.Fatal("reasoning still set")
	}
	if body["model"] != "cs/gemini-3.7-flash" {
		t.Fatalf("model=%v", body["model"])
	}
}

func TestExpandEffortBody_hybridOn(t *testing.T) {
	t.Parallel()
	m := catalog.Model{Efforts: []string{"none", "on"}, DefaultEffort: "none"}
	raw := []byte(`{"model":"agnes-2.5-flash","messages":[{"role":"user","content":"hi"}]}`)
	out, err := catalog.ExpandEffortBody(catalog.WireOpenAIChat, raw, "on", m)
	if err != nil {
		t.Fatal(err)
	}
	var body map[string]any
	if err := json.Unmarshal(out, &body); err != nil {
		t.Fatal(err)
	}
	if body["enable_thinking"] != true {
		t.Fatalf("enable_thinking=%v", body["enable_thinking"])
	}
	kwargs, _ := body["chat_template_kwargs"].(map[string]any)
	if kwargs["enable_thinking"] != true {
		t.Fatalf("kwargs=%v", kwargs)
	}
	// Must not inject hybrid labels into reasoning_effort (vendors reject "on").
	if _, ok := body["reasoning_effort"]; ok {
		t.Fatalf("hybrid path must not set reasoning_effort, got %v", body["reasoning_effort"])
	}
}

func TestExpandEffortBody_hybridNone(t *testing.T) {
	t.Parallel()
	m := catalog.Model{Efforts: []string{"none", "on"}}
	raw := []byte(`{"model":"m","messages":[],"chat_template_kwargs":{"foo":1}}`)
	out, err := catalog.ExpandEffortBody(catalog.WireOpenAIChat, raw, "none", m)
	if err != nil {
		t.Fatal(err)
	}
	var body map[string]any
	_ = json.Unmarshal(out, &body)
	if body["enable_thinking"] != false {
		t.Fatalf("enable_thinking=%v", body["enable_thinking"])
	}
	kwargs, _ := body["chat_template_kwargs"].(map[string]any)
	if kwargs["enable_thinking"] != false || kwargs["foo"] != float64(1) {
		t.Fatalf("kwargs=%v", kwargs)
	}
}

func TestExpandEffortBody_hybridAnthropicBudget(t *testing.T) {
	t.Parallel()
	m := catalog.Model{Efforts: []string{"none", "on"}}
	raw := []byte(`{"model":"m","messages":[]}`)
	out, err := catalog.ExpandEffortBody(catalog.WireAnthropicMsg, raw, "on", m)
	if err != nil {
		t.Fatal(err)
	}
	var body map[string]any
	_ = json.Unmarshal(out, &body)
	th, _ := body["thinking"].(map[string]any)
	if th["type"] != "enabled" {
		t.Fatalf("thinking=%v", th)
	}
	if th["budget_tokens"] != float64(catalog.HybridThinkingBudgetTokens) {
		t.Fatalf("budget=%v", th["budget_tokens"])
	}
}

func TestExpandEffortBody_ladderInject(t *testing.T) {
	t.Parallel()
	m := catalog.Model{Efforts: []string{"none", "low", "medium", "high", "xhigh"}}
	raw := []byte(`{"model":"gpt-5.5","messages":[]}`)
	out, err := catalog.ExpandEffortBody(catalog.WireOpenAIChat, raw, "xhigh", m)
	if err != nil {
		t.Fatal(err)
	}
	var body map[string]any
	_ = json.Unmarshal(out, &body)
	if body["reasoning_effort"] != "xhigh" {
		t.Fatalf("%v", body["reasoning_effort"])
	}
	if _, ok := body["effort"]; ok {
		t.Fatalf("must not set top-level effort (Anthropic rejects it): %v", body)
	}
	// ladder must not set hybrid-only kwargs
	if _, ok := body["chat_template_kwargs"]; ok {
		t.Fatalf("unexpected kwargs: %v", body)
	}
}

func TestExpandEffortBody_ladderNoneKeepsNoneNotNoThink(t *testing.T) {
	t.Parallel()
	m := catalog.Model{Efforts: []string{"none", "low", "high", "max"}}
	raw := []byte(`{"model":"deepseek-v4-pro","messages":[]}`)
	out, err := catalog.ExpandEffortBody(catalog.WireOpenAIChat, raw, "none", m)
	if err != nil {
		t.Fatal(err)
	}
	var body map[string]any
	_ = json.Unmarshal(out, &body)
	if body["reasoning_effort"] != "none" {
		t.Fatalf("want reasoning_effort=none, got %v", body["reasoning_effort"])
	}
	// ladder must not inject hybrid companions (Qwen 3.8 rejects enable_thinking=false)
	if _, ok := body["enable_thinking"]; ok {
		t.Fatalf("ladder must not set enable_thinking: %v", body)
	}
	if _, ok := body["chat_template_kwargs"]; ok {
		t.Fatalf("ladder must not set chat_template_kwargs: %v", body)
	}
	if _, ok := body["thinking"]; ok {
		t.Fatalf("ladder must not set thinking: %v", body)
	}
}

func TestExpandEffortBody_anthropicUsesOutputConfig(t *testing.T) {
	t.Parallel()
	m := catalog.Model{Efforts: []string{"low", "medium", "high", "xhigh", "max"}}
	raw := []byte(`{"model":"claude-sonnet-5","messages":[]}`)
	out, err := catalog.ExpandEffortBody(catalog.WireAnthropicMsg, raw, "max", m)
	if err != nil {
		t.Fatal(err)
	}
	var body map[string]any
	_ = json.Unmarshal(out, &body)
	if _, ok := body["effort"]; ok {
		t.Fatalf("top-level effort not allowed: %v", body)
	}
	oc, _ := body["output_config"].(map[string]any)
	if oc["effort"] != "max" {
		t.Fatalf("output_config=%v", oc)
	}
}

func TestExpandEffortBody_responsesLadder(t *testing.T) {
	t.Parallel()
	m := catalog.Model{Efforts: []string{"none", "low", "high", "max"}}
	raw := []byte(`{"model":"deepseek-v4-flash","input":"hi"}`)
	out, err := catalog.ExpandEffortBody(catalog.WireOpenAIResponses, raw, "max", m)
	if err != nil {
		t.Fatal(err)
	}
	var body map[string]any
	_ = json.Unmarshal(out, &body)
	r, _ := body["reasoning"].(map[string]any)
	if r["effort"] != "max" {
		t.Fatalf("reasoning=%v", r)
	}
}
