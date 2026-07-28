package gemini

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/subosito/cincai/adaptersdk/messages"
)

// ErrEmptyCompletion means generateContent returned no visible text and no
// tool calls. Vertex Forward maps this to HTTP 502 so multi-provider failover
// can try the next pool member (a bare Go error is not Retryable).
var ErrEmptyCompletion = errors.New("gemini: empty openai completion")

// ParseGenerateResponse maps a generateContent JSON body to stream events.
func ParseGenerateResponse(raw []byte, model string) ([]messages.StreamEvent, error) {
	var resp GenerateResponse
	if err := json.Unmarshal(raw, &resp); err != nil {
		return nil, fmt.Errorf("gemini: parse response: %w", err)
	}
	if resp.Error != nil && strings.TrimSpace(resp.Error.Message) != "" {
		return nil, fmt.Errorf("gemini: %s", strings.TrimSpace(resp.Error.Message))
	}
	return responseToEvents(resp, model), nil
}

// ParseGenerateStream reads Vertex/Gemini streamGenerateContent (?alt=sse or JSON array).
func ParseGenerateStream(r io.Reader, model string, fn func(messages.StreamEvent) error) error {
	br := bufio.NewReader(io.LimitReader(r, 32<<20))
	peek, err := br.Peek(1)
	if err != nil {
		if err == io.EOF {
			return fmt.Errorf("gemini: empty stream")
		}
		return err
	}

	started := false
	toolSeq := 0
	emit := func(ev messages.StreamEvent) error {
		if ev.Kind == messages.KindMessageStart {
			started = true
		}
		return fn(ev)
	}
	handleChunk := func(chunk []byte) error {
		chunk = bytes.TrimSpace(chunk)
		if len(chunk) == 0 || bytes.Equal(chunk, []byte("[DONE]")) {
			return nil
		}
		// SSE line may be "data: {...}"
		if bytes.HasPrefix(chunk, []byte("data:")) {
			chunk = bytes.TrimSpace(bytes.TrimPrefix(chunk, []byte("data:")))
		}
		if len(chunk) == 0 || bytes.Equal(chunk, []byte("[DONE]")) {
			return nil
		}
		var resp GenerateResponse
		if err := json.Unmarshal(chunk, &resp); err != nil {
			// Some hosts wrap as {"response":{...}} (cloudcode-style).
			var wrap struct {
				Response GenerateResponse `json:"response"`
			}
			if err2 := json.Unmarshal(chunk, &wrap); err2 != nil {
				return fmt.Errorf("gemini: parse stream chunk: %w", err)
			}
			resp = wrap.Response
		}
		for _, ev := range responseToEventsAccum(resp, model, &started, &toolSeq) {
			if err := emit(ev); err != nil {
				return err
			}
		}
		return nil
	}

	if peek[0] == '[' {
		dec := json.NewDecoder(br)
		tok, err := dec.Token()
		if err != nil {
			return err
		}
		if d, ok := tok.(json.Delim); !ok || d != '[' {
			return fmt.Errorf("gemini: expected JSON array stream")
		}
		for dec.More() {
			var raw json.RawMessage
			if err := dec.Decode(&raw); err != nil {
				return err
			}
			if err := handleChunk(raw); err != nil {
				return err
			}
		}
	} else {
		// SSE or single JSON object
		if peek[0] == '{' {
			raw, err := io.ReadAll(br)
			if err != nil {
				return err
			}
			if err := handleChunk(raw); err != nil {
				return err
			}
		} else {
			scanner := bufio.NewScanner(br)
			scanner.Buffer(make([]byte, 0, 64*1024), 16<<20)
			for scanner.Scan() {
				line := scanner.Bytes()
				if err := handleChunk(line); err != nil {
					return err
				}
			}
			if err := scanner.Err(); err != nil {
				return err
			}
		}
	}
	if started {
		return fn(messages.StreamEvent{Kind: messages.KindMessageStop})
	}
	return nil
}

func responseToEvents(resp GenerateResponse, model string) []messages.StreamEvent {
	started := false
	toolSeq := 0
	return responseToEventsAccum(resp, model, &started, &toolSeq)
}

func responseToEventsAccum(resp GenerateResponse, model string, started *bool, toolSeq *int) []messages.StreamEvent {
	var out []messages.StreamEvent
	if !*started {
		*started = true
		msgModel := strings.TrimSpace(resp.ModelVersion)
		if msgModel == "" {
			msgModel = model
		}
		out = append(out, messages.StreamEvent{
			Kind:      messages.KindMessageStart,
			MessageID: strings.TrimSpace(resp.ResponseID),
			Model:     msgModel,
		})
	}
	finish := ""
	for _, cand := range resp.Candidates {
		for _, part := range cand.Content.Parts {
			if t := strings.TrimSpace(part.Text); t != "" {
				out = append(out, messages.StreamEvent{Kind: messages.KindTextDelta, Text: t})
			}
			if part.FunctionCall != nil && strings.TrimSpace(part.FunctionCall.Name) != "" {
				idx := *toolSeq
				*toolSeq++
				argsRaw, _ := json.Marshal(part.FunctionCall.Args)
				if len(argsRaw) == 0 || string(argsRaw) == "null" {
					argsRaw = []byte("{}")
				}
				// Vertex does not round-trip functionCall.id; use name as
				// OpenAI tool_call id (stable enough for single-turn pairing).
				toolID := strings.TrimSpace(part.FunctionCall.Name)
				out = append(out,
					messages.StreamEvent{
						Kind:             messages.KindToolUseStart,
						ToolIndex:        idx,
						ToolID:           toolID,
						ToolName:         part.FunctionCall.Name,
						ThoughtSignature: strings.TrimSpace(part.ThoughtSignature),
					},
					messages.StreamEvent{
						Kind:        messages.KindToolInputDelta,
						ToolIndex:   idx,
						PartialJSON: string(argsRaw),
					},
					messages.StreamEvent{Kind: messages.KindToolUseStop, ToolIndex: idx},
				)
				finish = "tool_use"
			}
		}
		if fr := strings.TrimSpace(cand.FinishReason); fr != "" {
			if finish == "" {
				finish = fr
			}
		}
	}
	if resp.UsageMetadata.PromptTokenCount > 0 || resp.UsageMetadata.CandidatesTokenCount > 0 {
		out = append(out, messages.StreamEvent{
			Kind:         messages.KindUsage,
			InputTokens:  resp.UsageMetadata.PromptTokenCount,
			OutputTokens: resp.UsageMetadata.CandidatesTokenCount,
		})
	}
	if finish != "" {
		out = append(out, messages.StreamEvent{Kind: messages.KindTelemetry, Message: finish})
	}
	return out
}

// EncodeOpenAICompletion builds a non-stream chat.completion object from events.
func EncodeOpenAICompletion(events []messages.StreamEvent, model string) (map[string]any, error) {
	var text strings.Builder
	chunkID := "chatcmpl-gemini"
	msgModel := strings.TrimSpace(model)
	finish := "stop"
	var promptTok, completionTok int
	type pendingTool struct {
		id, name, args, thoughtSig string
	}
	tools := map[int]*pendingTool{}
	var order []int

	for _, ev := range events {
		switch ev.Kind {
		case messages.KindMessageStart:
			if strings.TrimSpace(ev.Model) != "" {
				msgModel = ev.Model
			}
			if strings.TrimSpace(ev.MessageID) != "" {
				chunkID = ev.MessageID
			}
		case messages.KindTextDelta:
			text.WriteString(ev.Text)
		case messages.KindToolUseStart:
			if _, ok := tools[ev.ToolIndex]; !ok {
				order = append(order, ev.ToolIndex)
			}
			tools[ev.ToolIndex] = &pendingTool{
				id:         ev.ToolID,
				name:       ev.ToolName,
				thoughtSig: strings.TrimSpace(ev.ThoughtSignature),
			}
		case messages.KindToolInputDelta:
			if p := tools[ev.ToolIndex]; p != nil {
				p.args += ev.PartialJSON
			}
		case messages.KindUsage:
			promptTok = ev.InputTokens
			completionTok = ev.OutputTokens
		case messages.KindTelemetry:
			if strings.TrimSpace(ev.Message) != "" {
				finish = mapFinishReason(ev.Message)
			}
		}
	}
	if strings.TrimSpace(msgModel) == "" {
		msgModel = model
	}

	msg := map[string]any{"role": "assistant"}
	content := text.String()
	var toolCalls []map[string]any
	for _, idx := range order {
		p := tools[idx]
		if p == nil || strings.TrimSpace(p.name) == "" {
			continue
		}
		args := strings.TrimSpace(p.args)
		if args == "" {
			args = "{}"
		}
		id := strings.TrimSpace(p.id)
		if id == "" {
			id = p.name
		}
		tc := map[string]any{
			"id":   id,
			"type": "function",
			"function": map[string]any{
				"name":      p.name,
				"arguments": args,
			},
		}
		if p.thoughtSig != "" {
			tc["thought_signature"] = p.thoughtSig
		}
		toolCalls = append(toolCalls, tc)
	}
	if len(toolCalls) > 0 {
		msg["tool_calls"] = toolCalls
		msg["content"] = content // may be ""
		finish = "tool_calls"
	} else {
		if content == "" {
			return nil, ErrEmptyCompletion
		}
		msg["content"] = content
	}

	return map[string]any{
		"id":      chunkID,
		"object":  "chat.completion",
		"created": time.Now().Unix(),
		"model":   msgModel,
		"choices": []map[string]any{{
			"index":         0,
			"message":       msg,
			"finish_reason": finish,
		}},
		"usage": map[string]any{
			"prompt_tokens":     promptTok,
			"completion_tokens": completionTok,
			"total_tokens":      promptTok + completionTok,
		},
	}, nil
}

// EncodeOpenAISSE builds OpenAI chat.completion.chunk SSE from events.
func EncodeOpenAISSE(events []messages.StreamEvent, model string) ([]byte, error) {
	var buf bytes.Buffer
	chunkID := "chatcmpl-gemini"
	created := time.Now().Unix()
	msgModel := strings.TrimSpace(model)
	finish := "stop"
	roleSent := false
	sawTool := false
	sawText := false

	writeChunk := func(choices []map[string]any, usage map[string]any) error {
		payload := map[string]any{
			"id":      chunkID,
			"object":  "chat.completion.chunk",
			"created": created,
			"model":   msgModel,
			"choices": choices,
		}
		if usage != nil {
			payload["usage"] = usage
		}
		raw, err := json.Marshal(payload)
		if err != nil {
			return err
		}
		_, err = fmt.Fprintf(&buf, "data: %s\n\n", raw)
		return err
	}

	ensureRole := func() error {
		if roleSent {
			return nil
		}
		roleSent = true
		return writeChunk([]map[string]any{{
			"index": 0,
			"delta": map[string]any{"role": "assistant", "content": ""},
		}}, nil)
	}

	var promptTok, completionTok int
	for _, ev := range events {
		switch ev.Kind {
		case messages.KindMessageStart:
			if strings.TrimSpace(ev.Model) != "" {
				msgModel = ev.Model
			}
			if strings.TrimSpace(ev.MessageID) != "" {
				chunkID = ev.MessageID
			}
		case messages.KindTextDelta:
			if err := ensureRole(); err != nil {
				return nil, err
			}
			sawText = true
			if err := writeChunk([]map[string]any{{
				"index": 0,
				"delta": map[string]any{"content": ev.Text},
			}}, nil); err != nil {
				return nil, err
			}
		case messages.KindToolUseStart:
			if err := ensureRole(); err != nil {
				return nil, err
			}
			sawTool = true
			finish = "tool_calls"
			id := strings.TrimSpace(ev.ToolID)
			if id == "" {
				id = ev.ToolName
			}
			tc := map[string]any{
				"index": ev.ToolIndex,
				"id":    id,
				"type":  "function",
				"function": map[string]any{
					"name":      ev.ToolName,
					"arguments": "",
				},
			}
			if sig := strings.TrimSpace(ev.ThoughtSignature); sig != "" {
				tc["thought_signature"] = sig
			}
			if err := writeChunk([]map[string]any{{
				"index": 0,
				"delta": map[string]any{"tool_calls": []map[string]any{tc}},
			}}, nil); err != nil {
				return nil, err
			}
		case messages.KindToolInputDelta:
			if err := writeChunk([]map[string]any{{
				"index": 0,
				"delta": map[string]any{
					"tool_calls": []map[string]any{{
						"index": ev.ToolIndex,
						"function": map[string]any{
							"arguments": ev.PartialJSON,
						},
					}},
				},
			}}, nil); err != nil {
				return nil, err
			}
		case messages.KindUsage:
			promptTok = ev.InputTokens
			completionTok = ev.OutputTokens
		case messages.KindTelemetry:
			if strings.TrimSpace(ev.Message) != "" {
				finish = mapFinishReason(ev.Message)
			}
		case messages.KindMessageStop:
			// finish chunk below
		}
	}

	if sawTool {
		finish = "tool_calls"
	}
	if !sawText && !sawTool {
		return nil, ErrEmptyCompletion
	}
	if !roleSent {
		if err := ensureRole(); err != nil {
			return nil, err
		}
	}
	var usage map[string]any
	if promptTok > 0 || completionTok > 0 {
		usage = map[string]any{
			"prompt_tokens":     promptTok,
			"completion_tokens": completionTok,
			"total_tokens":      promptTok + completionTok,
		}
	}
	if err := writeChunk([]map[string]any{{
		"index":         0,
		"delta":         map[string]any{},
		"finish_reason": finish,
	}}, usage); err != nil {
		return nil, err
	}
	_, _ = buf.WriteString("data: [DONE]\n\n")
	return buf.Bytes(), nil
}

func mapFinishReason(s string) string {
	switch strings.ToUpper(strings.TrimSpace(s)) {
	case "STOP", "END_TURN", "stop", "end_turn":
		return "stop"
	case "MAX_TOKENS", "max_tokens", "LENGTH":
		return "length"
	case "TOOL_USE", "tool_use", "TOOL_CALLS":
		return "tool_calls"
	default:
		if strings.EqualFold(s, "STOP") {
			return "stop"
		}
		return "stop"
	}
}
