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
//   - none → both false; Anthropic wire drops thinking block
//   - budget_tokens fixed at HybridThinkingBudgetTokens when on (not meta)
//
// Other ladders (GPT, DeepSeek, …): ensure reasoning_effort is set; on
// openai-responses also set reasoning.effort. No SKU changes (ApplyEffort).
//
// Returns the original body when effort is empty or raw is not a JSON object.
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

func expandHybridThinking(wire string, body map[string]any, on bool) {
	// OpenAI-compatible / vLLM / Agnes chat_template_kwargs
	kwargs, _ := body["chat_template_kwargs"].(map[string]any)
	if kwargs == nil {
		kwargs = map[string]any{}
	}
	kwargs["enable_thinking"] = on
	body["chat_template_kwargs"] = kwargs

	// DashScope-style top-level (ignored if unknown)
	body["enable_thinking"] = on

	// Keep a single client-visible effort field for logs / strict proxies.
	body["reasoning_effort"] = mapHybridEffortLabel(on)
	body["effort"] = mapHybridEffortLabel(on)

	// MiniMax / Anthropic-style thinking block (Agnes Anthropic path; MiniMax M3).
	if on {
		th := map[string]any{"type": "enabled"}
		if wire == WireAnthropicMsg {
			th["budget_tokens"] = HybridThinkingBudgetTokens
		}
		body["thinking"] = th
	} else {
		body["thinking"] = map[string]any{"type": "disabled"}
	}

	if wire == WireOpenAIResponses {
		// Responses-shaped thinking off often uses reasoning.effort=none.
		if on {
			setNestedReasoningEffort(body, "on")
		} else {
			setNestedReasoningEffort(body, "none")
		}
	}
}

func mapHybridEffortLabel(on bool) string {
	if on {
		return "on"
	}
	return "none"
}

func expandEffortLadder(wire string, body map[string]any, effort string) {
	// Hunyuan Hy3 documents vendor value "no_think" for direct (non-thinking) mode.
	upstreamEffort := effort
	if effort == "none" {
		upstreamEffort = "no_think"
	}
	body["reasoning_effort"] = upstreamEffort
	body["effort"] = effort // keep client-facing label when possible
	if wire == WireOpenAIResponses {
		// Responses: keep OpenAI-style none (not no_think).
		setNestedReasoningEffort(body, effort)
	}
	// When effort is none/off, also flip hybrid-thinking vendor knobs so models
	// that only honor enable_thinking / thinking.type still turn off.
	if effort == "none" {
		body["enable_thinking"] = false
		kwargs, _ := body["chat_template_kwargs"].(map[string]any)
		if kwargs == nil {
			kwargs = map[string]any{}
		}
		kwargs["enable_thinking"] = false
		kwargs["reasoning_effort"] = "no_think"
		body["chat_template_kwargs"] = kwargs
		body["thinking"] = map[string]any{"type": "disabled"}
	}
}

func setNestedReasoningEffort(body map[string]any, effort string) {
	r, _ := body["reasoning"].(map[string]any)
	if r == nil {
		r = map[string]any{}
	}
	r["effort"] = effort
	body["reasoning"] = r
}
