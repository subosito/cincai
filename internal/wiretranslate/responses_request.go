package wiretranslate

import (
	"encoding/json"
	"fmt"
	"strings"
)

// responsesToChatRequest translates a Responses API request (r2o) to an
// OpenAI chat-completions request body.
func responsesToChatRequest(raw []byte, upstreamModel string) ([]byte, error) {
	var req responsesRequest
	if err := json.Unmarshal(raw, &req); err != nil {
		return nil, fmt.Errorf("wire-translate: invalid responses request: %w", err)
	}
	model := strings.TrimSpace(req.Model)
	if model == "" {
		model = upstreamModel
	}
	msgs, err := responsesInputToOpenAI(&req)
	if err != nil {
		return nil, err
	}
	out := openaiChatRequest{
		Model:     model,
		Messages:  msgs,
		Stream:    req.Stream,
		MaxTokens: req.MaxOutputTokens,
	}
	if req.Stream {
		out.StreamOptions = &streamOptions{IncludeUsage: true}
	}
	// Chat hosts (xAI/Qwen/DeepSeek) take the effort knob as reasoning_effort.
	if effort := req.effort(); effort != "" {
		out.ReasoningEffort = effort
	}
	for _, t := range req.Tools {
		name := strings.TrimSpace(t.Name)
		if name == "" || strings.TrimSpace(t.Type) != "function" {
			continue
		}
		out.Tools = append(out.Tools, openaiTool{
			Type: "function",
			Function: struct {
				Name        string         `json:"name"`
				Description string         `json:"description"`
				Parameters  map[string]any `json:"parameters"`
			}{Name: name, Description: t.Description, Parameters: t.Parameters},
		})
	}
	return json.Marshal(out)
}

// responsesInputToOpenAI flattens the input item list to chat messages.
// Consecutive function_call items merge into one assistant message's
// tool_calls — including a preceding assistant message item, so the history
// never holds consecutive same-role messages; function_call_output becomes
// a role:"tool" message correlated by call_id.
func responsesInputToOpenAI(req *responsesRequest) ([]openaiMessage, error) {
	items, err := parseResponsesInput(req.Input)
	if err != nil {
		return nil, fmt.Errorf("wire-translate: invalid responses input: %w", err)
	}
	var out []openaiMessage
	var pending []openaiToolCall
	flush := func() {
		if len(pending) == 0 {
			return
		}
		// An assistant turn with both prose and tool calls arrives as a
		// message item followed by function_call items. Attach the calls to
		// that assistant message: appending a second one produces
		// consecutive same-role messages, which DeepSeek, Qwen and several
		// vLLM/SGLang front ends reject with a 400.
		if n := len(out); n > 0 && out[n-1].Role == "assistant" && len(out[n-1].ToolCalls) == 0 {
			out[n-1].ToolCalls = pending
		} else {
			out = append(out, openaiMessage{Role: "assistant", Content: "", ToolCalls: pending})
		}
		pending = nil
	}
	if s := strings.TrimSpace(req.Instructions); s != "" {
		out = append(out, openaiMessage{Role: "system", Content: s})
	}
	for _, it := range items {
		if role := it.messageRole(); role != "" {
			flush()
			text := it.contentText()
			if strings.TrimSpace(text) == "" {
				continue
			}
			switch role {
			case "system", "developer":
				out = append(out, openaiMessage{Role: "system", Content: text})
			case "user", "assistant":
				out = append(out, openaiMessage{Role: role, Content: text})
			default:
				return nil, fmt.Errorf("wire-translate: unsupported role %q", role)
			}
			continue
		}
		switch strings.TrimSpace(it.Type) {
		case "function_call":
			callID := strings.TrimSpace(it.CallID)
			if callID == "" {
				callID = strings.TrimSpace(it.ID)
			}
			args := strings.TrimSpace(it.Arguments)
			if args == "" {
				args = "{}"
			}
			pending = append(pending, openaiToolCall{
				ID:   callID,
				Type: "function",
				Function: struct {
					Name      string `json:"name"`
					Arguments string `json:"arguments"`
				}{Name: it.Name, Arguments: args},
			})
		case "function_call_output":
			flush()
			out = append(out, openaiMessage{
				Role:       "tool",
				ToolCallID: strings.TrimSpace(it.CallID),
				Content:    it.outputText(),
			})
		default:
			// reasoning items and built-in tool items (web_search_call, …)
			// carry nothing a stateless chat upstream can replay; skip them.
			continue
		}
	}
	flush()
	if len(out) == 0 {
		return nil, fmt.Errorf("wire-translate: no messages")
	}
	return out, nil
}

// responsesToAnthropicRequest translates a Responses API request (r2a) to an
// Anthropic messages request body, injecting cache_control breakpoints at the
// canonical positions (docs/responses-ingress.md §1):
//
//   - final tool definition and final system block (the static prefix), and
//   - the final block of the last two user messages when the conversation
//     has tool history (two rolling breakpoints, so the previous turn's
//     position still carries a marker on the next turn).
//
// Without these markers Anthropic re-processes the full prefix every turn and
// reports zero cache_read tokens.
func responsesToAnthropicRequest(raw []byte, upstreamModel string) ([]byte, error) {
	var req responsesRequest
	if err := json.Unmarshal(raw, &req); err != nil {
		return nil, fmt.Errorf("wire-translate: invalid responses request: %w", err)
	}
	model := strings.TrimSpace(req.Model)
	if model == "" {
		model = upstreamModel
	}
	system, msgs, toolHistory, err := responsesInputToAnthropic(&req)
	if err != nil {
		return nil, err
	}
	maxTokens := req.MaxOutputTokens
	if maxTokens == 0 {
		maxTokens = 4096
	}
	// Breakpoint (b): two rolling user-turn breakpoints when tool history
	// exists. Mark before building out: the blocks are reference types, but
	// a future refactor that deep-copies msgs into out would otherwise drop
	// the markers silently.
	if toolHistory {
		markLastUserBlocks(msgs, 2)
	}
	out := map[string]any{
		"model":      model,
		"max_tokens": maxTokens,
		"stream":     req.Stream,
		"messages":   msgs,
	}
	var tools []map[string]any
	for _, t := range req.Tools {
		name := strings.TrimSpace(t.Name)
		if name == "" || strings.TrimSpace(t.Type) != "function" {
			continue
		}
		tools = append(tools, map[string]any{
			"name":         name,
			"description":  t.Description,
			"input_schema": t.Parameters,
		})
	}
	// Breakpoint (a): end of system + tools.
	if len(tools) > 0 {
		tools[len(tools)-1]["cache_control"] = map[string]any{"type": "ephemeral"}
		out["tools"] = tools
	}
	if len(system) > 0 {
		system[len(system)-1]["cache_control"] = map[string]any{"type": "ephemeral"}
		out["system"] = system
	}
	if effort := req.effort(); effort != "" {
		if budget := thinkingBudgetTokens(effort); budget > 0 {
			out["thinking"] = map[string]any{"type": "enabled", "budget_tokens": budget}
			if maxTokens <= budget {
				out["max_tokens"] = budget + 4096
			}
		} else {
			out["thinking"] = map[string]any{"type": "disabled"}
		}
	}
	return json.Marshal(out)
}

// responsesInputToAnthropic flattens the input item list to Anthropic system
// blocks + role-alternating messages. Content is always block form so
// cache_control breakpoints can attach.
func responsesInputToAnthropic(req *responsesRequest) (system []map[string]any, msgs []map[string]any, toolHistory bool, err error) {
	items, err := parseResponsesInput(req.Input)
	if err != nil {
		return nil, nil, false, fmt.Errorf("wire-translate: invalid responses input: %w", err)
	}
	appendBlock := func(role string, block map[string]any) {
		if n := len(msgs); n > 0 && msgs[n-1]["role"] == role {
			msgs[n-1]["content"] = append(msgs[n-1]["content"].([]map[string]any), block)
			return
		}
		msgs = append(msgs, map[string]any{"role": role, "content": []map[string]any{block}})
	}
	if s := strings.TrimSpace(req.Instructions); s != "" {
		system = append(system, map[string]any{"type": "text", "text": s})
	}
	for _, it := range items {
		if role := it.messageRole(); role != "" {
			text := it.contentText()
			if strings.TrimSpace(text) == "" {
				continue
			}
			switch role {
			case "system", "developer":
				system = append(system, map[string]any{"type": "text", "text": text})
			case "user":
				appendBlock("user", map[string]any{"type": "text", "text": text})
			case "assistant":
				appendBlock("assistant", map[string]any{"type": "text", "text": text})
			default:
				return nil, nil, false, fmt.Errorf("wire-translate: unsupported role %q", role)
			}
			continue
		}
		switch strings.TrimSpace(it.Type) {
		case "function_call":
			toolHistory = true
			callID := strings.TrimSpace(it.CallID)
			if callID == "" {
				callID = strings.TrimSpace(it.ID)
			}
			var input any
			if args := strings.TrimSpace(it.Arguments); args != "" {
				_ = json.Unmarshal([]byte(args), &input)
			}
			if input == nil {
				input = map[string]any{}
			}
			appendBlock("assistant", map[string]any{
				"type":  "tool_use",
				"id":    callID,
				"name":  it.Name,
				"input": input,
			})
		case "function_call_output":
			toolHistory = true
			appendBlock("user", map[string]any{
				"type":        "tool_result",
				"tool_use_id": strings.TrimSpace(it.CallID),
				"content":     it.outputText(),
			})
		default:
			// See responsesInputToOpenAI: non-replayable items are skipped.
			continue
		}
	}
	if len(msgs) == 0 {
		return nil, nil, false, fmt.Errorf("wire-translate: no messages")
	}
	return system, msgs, toolHistory, nil
}

// markLastUserBlocks sets cache_control on the final content block of the
// last n user messages. Two rolling breakpoints keep a marker at the
// previous turn's position: Anthropic reads the longest prefix match among
// the breakpoints in the current request, so a single moving tail
// breakpoint would limit cache reads to the system+tools prefix and
// re-process the accumulated tool-use/tool-result body at full price every
// turn. A user message without block-form content is skipped rather than
// aborting the walk, so one odd message cannot disable caching entirely.
func markLastUserBlocks(msgs []map[string]any, n int) {
	marked := 0
	for i := len(msgs) - 1; i >= 0 && marked < n; i-- {
		if msgs[i]["role"] != "user" {
			continue
		}
		blocks, ok := msgs[i]["content"].([]map[string]any)
		if !ok || len(blocks) == 0 {
			continue
		}
		blocks[len(blocks)-1]["cache_control"] = map[string]any{"type": "ephemeral"}
		marked++
	}
}

// thinkingBudgetTokens maps a Responses reasoning effort to an Anthropic
// thinking budget. "none"/"off" disable thinking (0); unknown ladder labels
// land on the medium budget.
func thinkingBudgetTokens(effort string) int {
	switch strings.ToLower(strings.TrimSpace(effort)) {
	case "none", "off":
		return 0
	case "minimal", "low":
		return 1024
	case "high":
		return 8192
	case "xhigh":
		return 16384
	case "max":
		return 32768
	default: // medium and unrecognized labels
		return 2048
	}
}
