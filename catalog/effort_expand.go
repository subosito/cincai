package catalog

import (
	"encoding/json"
	"fmt"
	"strings"
)

// HybridThinkingBudgetTokens is the fixed Anthropic-style thinking budget when
// hybrid effort "on" is mapped to thinking:{type:enabled}. Not catalog config —
// Agnes docs suggest 2048 for ordinary coding tasks.
const HybridThinkingBudgetTokens = 2048

// IsHybridThinkingEfforts reports whether the efforts list is the hybrid
// thinking switch [none, on] (aliases: off→none). Used to map a single client
// effort value onto enable_thinking rather than a multi-step ladder.
func IsHybridThinkingEfforts(efforts []string) bool {
	set := map[string]bool{}
	for _, e := range efforts {
		e = strings.ToLower(strings.TrimSpace(e))
		switch e {
		case "", "default", "auto":
			continue
		case "off":
			e = "none"
		}
		set[e] = true
	}
	return len(set) == 2 && set["none"] && set["on"]
}

// ExpandEffortBody rewrites the ingress JSON so upstream sees vendor knobs for
// the resolved effort. Call after ApplyEffort with the effort actually used.
//
// Hybrid efforts [none, on] (Agnes, Qwen-style):
//   - on  → enable_thinking + chat_template_kwargs.enable_thinking
//   - none → both false
//   - never write reasoning_effort=on (enum validators reject it)
//
// Other ladders (GPT, DeepSeek, …): inject reasoning_effort (OpenAI) or
// output_config.effort (Anthropic). Never rewrite none→no_think on the main
// reasoning_effort field.
func ExpandEffortBody(wire string, raw []byte, effort string, m Model) ([]byte, error) {
	effort = strings.ToLower(strings.TrimSpace(effort))
	if effort == "" || len(raw) == 0 || len(m.Efforts) == 0 {
		return raw, nil
	}
	var body map[string]any
	if err := json.Unmarshal(raw, &body); err != nil {
		return nil, fmt.Errorf("effort expand: body: %w", err)
	}
	if body == nil {
		return raw, nil
	}

	if IsHybridThinkingEfforts(m.Efforts) {
		expandHybridThinking(wire, body, effort == "on" || effort == "true")
	} else {
		expandEffortLadder(wire, body, effort)
	}
	out, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("effort expand: marshal: %w", err)
	}
	return out, nil
}

// StripEffortHints removes client effort fields from a chat-style JSON body.
// Use on hops that already selected an upstream SKU via models[effort] so the
// vendor does not apply a second, independent effort mapping.
func StripEffortHints(raw []byte) []byte {
	if len(raw) == 0 {
		return raw
	}
	var body map[string]any
	if err := json.Unmarshal(raw, &body); err != nil || body == nil {
		return raw
	}
	delete(body, "reasoning_effort")
	delete(body, "effort")
	if r, ok := body["reasoning"].(map[string]any); ok {
		delete(r, "effort")
		if len(r) == 0 {
			delete(body, "reasoning")
		} else {
			body["reasoning"] = r
		}
	}
	out, err := json.Marshal(body)
	if err != nil {
		return raw
	}
	return out
}

func expandHybridThinking(wire string, body map[string]any, on bool) {
	// Never set reasoning_effort to hybrid labels "on"/"none" — many hosts
	// validate a fixed enum (Qwen 3.8, etc.) and return 400.
	delete(body, "reasoning_effort")
	delete(body, "effort")
	if r, ok := body["reasoning"].(map[string]any); ok {
		delete(r, "effort")
		if len(r) == 0 {
			delete(body, "reasoning")
		} else {
			body["reasoning"] = r
		}
	}

	kwargs, _ := body["chat_template_kwargs"].(map[string]any)
	if kwargs == nil {
		kwargs = map[string]any{}
	}
	kwargs["enable_thinking"] = on
	body["chat_template_kwargs"] = kwargs
	body["enable_thinking"] = on

	switch wire {
	case WireAnthropicMsg:
		if on {
			body["thinking"] = map[string]any{
				"type":          "enabled",
				"budget_tokens": HybridThinkingBudgetTokens,
			}
		} else {
			body["thinking"] = map[string]any{"type": "disabled"}
		}
	case WireOpenAIChat:
		// MiniMax uses thinking.type; Qwen/Agnes use enable_thinking.
		if on {
			body["thinking"] = map[string]any{"type": "enabled"}
		} else {
			body["thinking"] = map[string]any{"type": "disabled"}
		}
	case WireOpenAIResponses:
		if on {
			setNestedReasoningEffort(body, "high")
		} else {
			setNestedReasoningEffort(body, "none")
		}
	}
}

func expandEffortLadder(wire string, body map[string]any, effort string) {
	// Anthropic rejects top-level "effort". OpenAI uses reasoning_effort.
	// Never map none→no_think on reasoning_effort (DeepSeek/Qwen/MiMo reject it).
	delete(body, "effort")

	switch wire {
	case WireAnthropicMsg:
		delete(body, "reasoning_effort")
		oc, _ := body["output_config"].(map[string]any)
		if oc == nil {
			oc = map[string]any{}
		}
		oc["effort"] = effort
		body["output_config"] = oc
	case WireOpenAIResponses:
		body["reasoning_effort"] = effort
		setNestedReasoningEffort(body, effort)
	default:
		body["reasoning_effort"] = effort
	}
	// Ladder only: reasoning_effort / output_config.effort. Do not inject
	// enable_thinking or thinking companions — those are hybrid-only. Qwen
	// 3.8 rejects enable_thinking=false ("restricted to True") even when
	// reasoning_effort=none is a valid enum value.
}

func setNestedReasoningEffort(body map[string]any, effort string) {
	r, _ := body["reasoning"].(map[string]any)
	if r == nil {
		r = map[string]any{}
	}
	r["effort"] = effort
	body["reasoning"] = r
}
