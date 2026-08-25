package wiretranslate

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/subosito/cincai/adaptersdk/messages"
	"github.com/subosito/cincai/internal/wiretranslate/sse"
)

func parseOpenAIStream(r io.Reader, fn func(messages.StreamEvent) error) error {
	activeTools := make(map[int]bool)
	started := false
	return sse.ReadFrames(r, func(frame sse.Frame) error {
		events, err := parseOpenAIChunk(frame.Data, activeTools, &started)
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

func parseOpenAIChunk(data []byte, activeTools map[int]bool, started *bool) ([]messages.StreamEvent, error) {
	if len(data) == 0 {
		return nil, nil
	}
	if string(data) == "[DONE]" {
		return []messages.StreamEvent{{Kind: messages.KindMessageStop}}, nil
	}
	var chunk struct {
		ID      string `json:"id"`
		Model   string `json:"model"`
		Choices []struct {
			Index int `json:"index"`
			Delta struct {
				Role             string `json:"role"`
				Content          string `json:"content"`
				ReasoningContent string `json:"reasoning_content"`
				Reasoning        string `json:"reasoning"`
				ToolCalls        []struct {
					Index    int    `json:"index"`
					ID       string `json:"id"`
					Type     string `json:"type"`
					Function struct {
						Name      string `json:"name"`
						Arguments string `json:"arguments"`
					} `json:"function"`
				} `json:"tool_calls"`
			} `json:"delta"`
			FinishReason string `json:"finish_reason"`
		} `json:"choices"`
		Usage *struct {
			PromptTokens        int `json:"prompt_tokens"`
			CompletionTokens    int `json:"completion_tokens"`
			PromptTokensDetails *struct {
				CachedTokens int `json:"cached_tokens"`
			} `json:"prompt_tokens_details"`
			// Responses-style (rarely on chat chunks; accept if present).
			InputTokensDetails *struct {
				CachedTokens int `json:"cached_tokens"`
			} `json:"input_tokens_details"`
		} `json:"usage"`
	}
	if err := json.Unmarshal(data, &chunk); err != nil {
		return nil, fmt.Errorf("wire-translate: parse openai chunk: %w", err)
	}

	var out []messages.StreamEvent
	if !*started && chunk.ID != "" {
		*started = true
		out = append(out, messages.StreamEvent{
			Kind:      messages.KindMessageStart,
			MessageID: chunk.ID,
			Model:     chunk.Model,
		})
	}
	for _, choice := range chunk.Choices {
		if rc := strings.TrimSpace(choice.Delta.ReasoningContent); rc != "" {
			out = append(out, messages.StreamEvent{Kind: messages.KindThinkingDelta, Thinking: rc})
		} else if r := strings.TrimSpace(choice.Delta.Reasoning); r != "" {
			out = append(out, messages.StreamEvent{Kind: messages.KindThinkingDelta, Thinking: r})
		}
		if choice.Delta.Content != "" {
			out = append(out, messages.StreamEvent{Kind: messages.KindTextDelta, Text: choice.Delta.Content})
		}
		for _, tc := range choice.Delta.ToolCalls {
			idx := tc.Index
			if tc.ID != "" || tc.Function.Name != "" {
				activeTools[idx] = true
				out = append(out, messages.StreamEvent{
					Kind:      messages.KindToolUseStart,
					ToolIndex: idx,
					ToolID:    tc.ID,
					ToolName:  tc.Function.Name,
				})
			}
			if tc.Function.Arguments != "" {
				out = append(out, messages.StreamEvent{
					Kind:        messages.KindToolInputDelta,
					ToolIndex:   idx,
					PartialJSON: tc.Function.Arguments,
				})
			}
		}
		switch choice.FinishReason {
		case "tool_calls":
			for idx := range activeTools {
				out = append(out, messages.StreamEvent{Kind: messages.KindToolUseStop, ToolIndex: idx})
			}
			clear(activeTools)
		case "stop":
			out = append(out, messages.StreamEvent{Kind: messages.KindMessageStop})
		}
	}
	if chunk.Usage != nil {
		cacheRead := 0
		if chunk.Usage.PromptTokensDetails != nil {
			cacheRead = chunk.Usage.PromptTokensDetails.CachedTokens
		}
		if cacheRead == 0 && chunk.Usage.InputTokensDetails != nil {
			cacheRead = chunk.Usage.InputTokensDetails.CachedTokens
		}
		if chunk.Usage.PromptTokens > 0 || chunk.Usage.CompletionTokens > 0 || cacheRead > 0 {
			out = append(out, messages.StreamEvent{
				Kind:            messages.KindUsage,
				InputTokens:     chunk.Usage.PromptTokens,
				OutputTokens:    chunk.Usage.CompletionTokens,
				CacheReadTokens: cacheRead,
			})
		}
	}
	return out, nil
}

func openAINonStreamToEvents(raw []byte) ([]messages.StreamEvent, error) {
	var resp struct {
		ID      string `json:"id"`
		Model   string `json:"model"`
		Choices []struct {
			Message struct {
				Role      string `json:"role"`
				Content   string `json:"content"`
				ToolCalls []struct {
					ID       string `json:"id"`
					Function struct {
						Name      string `json:"name"`
						Arguments string `json:"arguments"`
					} `json:"function"`
				} `json:"tool_calls"`
			} `json:"message"`
			FinishReason string `json:"finish_reason"`
		} `json:"choices"`
		Usage struct {
			PromptTokens        int `json:"prompt_tokens"`
			CompletionTokens    int `json:"completion_tokens"`
			PromptTokensDetails *struct {
				CachedTokens int `json:"cached_tokens"`
			} `json:"prompt_tokens_details"`
			InputTokensDetails *struct {
				CachedTokens int `json:"cached_tokens"`
			} `json:"input_tokens_details"`
		} `json:"usage"`
	}
	if err := json.Unmarshal(raw, &resp); err != nil {
		return nil, err
	}
	var events []messages.StreamEvent
	events = append(events, messages.StreamEvent{
		Kind:      messages.KindMessageStart,
		MessageID: resp.ID,
		Model:     resp.Model,
	})
	if len(resp.Choices) > 0 {
		msg := resp.Choices[0].Message
		if strings.TrimSpace(msg.Content) != "" {
			events = append(events, messages.StreamEvent{Kind: messages.KindTextDelta, Text: msg.Content})
		}
		for i, tc := range msg.ToolCalls {
			args := strings.TrimSpace(tc.Function.Arguments)
			if args == "" {
				args = "{}"
			}
			events = append(events,
				messages.StreamEvent{Kind: messages.KindToolUseStart, ToolIndex: i, ToolID: tc.ID, ToolName: tc.Function.Name},
				messages.StreamEvent{Kind: messages.KindToolInputDelta, ToolIndex: i, PartialJSON: args},
				messages.StreamEvent{Kind: messages.KindToolUseStop, ToolIndex: i},
			)
		}
		events = append(events, messages.StreamEvent{Kind: messages.KindTelemetry, Message: resp.Choices[0].FinishReason})
	}
	// Usage precedes MessageStop (the KindUsage contract): encoders that
	// assemble the terminal frame on stop read token counts at that moment.
	cacheRead := 0
	if resp.Usage.PromptTokensDetails != nil {
		cacheRead = resp.Usage.PromptTokensDetails.CachedTokens
	}
	if cacheRead == 0 && resp.Usage.InputTokensDetails != nil {
		cacheRead = resp.Usage.InputTokensDetails.CachedTokens
	}
	if resp.Usage.PromptTokens > 0 || resp.Usage.CompletionTokens > 0 || cacheRead > 0 {
		events = append(events, messages.StreamEvent{
			Kind:            messages.KindUsage,
			InputTokens:     resp.Usage.PromptTokens,
			OutputTokens:    resp.Usage.CompletionTokens,
			CacheReadTokens: cacheRead,
		})
	}
	events = append(events, messages.StreamEvent{Kind: messages.KindMessageStop})
	return events, nil
}
