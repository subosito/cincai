package wiretranslate

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/subosito/cincai/adaptersdk/messages"
	"github.com/subosito/cincai/internal/wiretranslate/sse"
)

// responsesParseState tracks output items across Responses SSE events so
// function_call argument deltas correlate with their tool call.
type responsesParseState struct {
	started     bool
	activeTools map[int]bool // output_index → function_call item open
	argsSeen    map[int]bool // output_index → streamed argument deltas
}

func newResponsesParseState() *responsesParseState {
	return &responsesParseState{
		activeTools: map[int]bool{},
		argsSeen:    map[int]bool{},
	}
}

func parseResponsesStream(r io.Reader, fn func(messages.StreamEvent) error) error {
	state := newResponsesParseState()
	return sse.ReadFrames(r, func(frame sse.Frame) error {
		events, err := parseResponsesEvent(frame.Data, state)
		if err != nil {
			return err
		}
		for _, ev := range events {
			if err := fn(ev); err != nil {
				return err
			}
		}
		return nil
	})
}

func parseResponsesEvent(data []byte, state *responsesParseState) ([]messages.StreamEvent, error) {
	if len(data) == 0 {
		return nil, nil
	}
	var base struct {
		Type string `json:"type"`
	}
	if err := json.Unmarshal(data, &base); err != nil {
		return nil, fmt.Errorf("wire-translate: parse responses event: %w", err)
	}
	switch base.Type {
	case "response.created", "response.in_progress":
		return parseResponsesCreated(data, state)
	case "response.output_item.added":
		return parseResponsesItemAdded(data, state)
	case "response.output_item.done":
		return parseResponsesItemDone(data, state)
	case "response.output_text.delta":
		var raw struct {
			Delta string `json:"delta"`
		}
		if err := json.Unmarshal(data, &raw); err != nil {
			return nil, err
		}
		if raw.Delta == "" {
			return nil, nil
		}
		return []messages.StreamEvent{{Kind: messages.KindTextDelta, Text: raw.Delta}}, nil
	case "response.reasoning.delta", "response.reasoning_summary_text.delta":
		var raw struct {
			Delta string `json:"delta"`
		}
		if err := json.Unmarshal(data, &raw); err != nil {
			return nil, err
		}
		if raw.Delta == "" {
			return nil, nil
		}
		return []messages.StreamEvent{{Kind: messages.KindThinkingDelta, Thinking: raw.Delta}}, nil
	case "response.function_call_arguments.delta":
		var raw struct {
			OutputIndex int    `json:"output_index"`
			Delta       string `json:"delta"`
		}
		if err := json.Unmarshal(data, &raw); err != nil {
			return nil, err
		}
		state.argsSeen[raw.OutputIndex] = true
		if raw.Delta == "" {
			return nil, nil
		}
		return []messages.StreamEvent{{
			Kind:        messages.KindToolInputDelta,
			ToolIndex:   raw.OutputIndex,
			PartialJSON: raw.Delta,
		}}, nil
	case "response.completed", "response.incomplete":
		return parseResponsesCompleted(data)
	case "response.failed":
		return parseResponsesFailed(data)
	case "error":
		var raw struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		}
		_ = json.Unmarshal(data, &raw)
		return []messages.StreamEvent{{Kind: messages.KindAPIError, Message: raw.Message, Code: raw.Code}}, nil
	default:
		return nil, nil
	}
}

func parseResponsesCreated(data []byte, state *responsesParseState) ([]messages.StreamEvent, error) {
	var raw struct {
		Response struct {
			ID    string `json:"id"`
			Model string `json:"model"`
		} `json:"response"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, err
	}
	if state.started {
		return nil, nil
	}
	state.started = true
	return []messages.StreamEvent{{
		Kind:      messages.KindMessageStart,
		MessageID: raw.Response.ID,
		Model:     raw.Response.Model,
	}}, nil
}

func parseResponsesItemAdded(data []byte, state *responsesParseState) ([]messages.StreamEvent, error) {
	var raw struct {
		OutputIndex int `json:"output_index"`
		Item        struct {
			Type      string `json:"type"`
			ID        string `json:"id"`
			CallID    string `json:"call_id"`
			Name      string `json:"name"`
			Arguments string `json:"arguments"`
		} `json:"item"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, err
	}
	if raw.Item.Type != "function_call" {
		return nil, nil
	}
	state.activeTools[raw.OutputIndex] = true
	toolID := strings.TrimSpace(raw.Item.CallID)
	if toolID == "" {
		toolID = raw.Item.ID
	}
	out := []messages.StreamEvent{{
		Kind:      messages.KindToolUseStart,
		ToolIndex: raw.OutputIndex,
		ToolID:    toolID,
		ToolName:  raw.Item.Name,
	}}
	// Some hosts deliver the whole arguments blob on the added item.
	if args := strings.TrimSpace(raw.Item.Arguments); args != "" {
		state.argsSeen[raw.OutputIndex] = true
		out = append(out, messages.StreamEvent{
			Kind:        messages.KindToolInputDelta,
			ToolIndex:   raw.OutputIndex,
			PartialJSON: args,
		})
	}
	return out, nil
}

func parseResponsesItemDone(data []byte, state *responsesParseState) ([]messages.StreamEvent, error) {
	var raw struct {
		OutputIndex int `json:"output_index"`
		Item        struct {
			Type      string `json:"type"`
			Arguments string `json:"arguments"`
		} `json:"item"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, err
	}
	if raw.Item.Type != "function_call" || !state.activeTools[raw.OutputIndex] {
		return nil, nil
	}
	delete(state.activeTools, raw.OutputIndex)
	var out []messages.StreamEvent
	// Hosts that skip argument deltas still carry the assembled item here.
	if args := strings.TrimSpace(raw.Item.Arguments); args != "" && !state.argsSeen[raw.OutputIndex] {
		out = append(out, messages.StreamEvent{
			Kind:        messages.KindToolInputDelta,
			ToolIndex:   raw.OutputIndex,
			PartialJSON: args,
		})
	}
	return append(out, messages.StreamEvent{Kind: messages.KindToolUseStop, ToolIndex: raw.OutputIndex}), nil
}

func parseResponsesCompleted(data []byte) ([]messages.StreamEvent, error) {
	var raw struct {
		Response struct {
			Usage *struct {
				InputTokens        int `json:"input_tokens"`
				OutputTokens       int `json:"output_tokens"`
				InputTokensDetails *struct {
					CachedTokens int `json:"cached_tokens"`
				} `json:"input_tokens_details"`
			} `json:"usage"`
		} `json:"response"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, err
	}
	// Usage precedes MessageStop so encoders that assemble the terminal frame
	// on stop (chat completed, responses completed) see the token counts.
	var out []messages.StreamEvent
	if u := raw.Response.Usage; u != nil {
		cacheRead := 0
		if u.InputTokensDetails != nil {
			cacheRead = u.InputTokensDetails.CachedTokens
		}
		if u.InputTokens > 0 || u.OutputTokens > 0 || cacheRead > 0 {
			out = append(out, messages.StreamEvent{
				Kind:            messages.KindUsage,
				InputTokens:     u.InputTokens,
				OutputTokens:    u.OutputTokens,
				CacheReadTokens: cacheRead,
			})
		}
	}
	return append(out, messages.StreamEvent{Kind: messages.KindMessageStop}), nil
}

func parseResponsesFailed(data []byte) ([]messages.StreamEvent, error) {
	var raw struct {
		Response struct {
			Error *struct {
				Code    string `json:"code"`
				Message string `json:"message"`
			} `json:"error"`
		} `json:"response"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, err
	}
	if raw.Response.Error == nil {
		return []messages.StreamEvent{{Kind: messages.KindAPIError, Message: "response failed"}}, nil
	}
	return []messages.StreamEvent{{
		Kind:    messages.KindAPIError,
		Message: raw.Response.Error.Message,
		Code:    raw.Response.Error.Code,
	}}, nil
}
