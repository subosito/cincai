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
	if body["reasoning_effort"] != "on" {
		t.Fatalf("reasoning_effort=%v", body["reasoning_effort"])
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
	// ladder must not set hybrid-only kwargs
	if _, ok := body["chat_template_kwargs"]; ok {
		t.Fatalf("unexpected kwargs: %v", body)
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
