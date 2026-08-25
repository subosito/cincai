package wiretranslate

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/subosito/cincai/adaptersdk/messages"
)

// --- Responses SSE encoder (incremental) ---

// responsesStreamEncoder re-encodes StreamEvents as Responses API SSE events.
// It tracks output_index / content_index so item framing (output_item.added →
// deltas → output_item.done) stays consistent across text, reasoning, and
// function_call items.
type responsesStreamEncoder struct {
	w       io.Writer
	model   string
	respID  string
	created int64

	createdSent bool
	wrote       bool

	outputIndex int
	itemSeq     int
	itemKind    string // "" | "message" | "reasoning" | "function_call"
	itemID      string
	toolCallID  string
	toolName    string
	toolOutput  map[int]int    // upstream tool index → responses output_index
	toolItemIDs map[int]string // upstream tool index → function_call item id

	textBuf  strings.Builder
	thinkBuf strings.Builder
	argsBuf  strings.Builder

	doneItems []any
	status    string

	inputTok   int
	outputTok  int
	cacheRead  int
	cacheWrite int
}

func newResponsesStreamEncoder(w io.Writer, model string) *responsesStreamEncoder {
	return &responsesStreamEncoder{
		w:           w,
		model:       strings.TrimSpace(model),
		created:     time.Now().Unix(),
		outputIndex: -1,
		toolOutput:  map[int]int{},
		toolItemIDs: map[int]string{},
		status:      "completed",
	}
}

func (e *responsesStreamEncoder) write(event string, payload any) error {
	raw, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	e.wrote = true
	_, err = fmt.Fprintf(e.w, "event: %s\ndata: %s\n\n", event, raw)
	return err
}

func (e *responsesStreamEncoder) responseID() string {
	if strings.TrimSpace(e.respID) == "" {
		e.respID = "resp_wiretranslate"
	}
	return e.respID
}

func (e *responsesStreamEncoder) nextItemID(prefix string) string {
	e.itemSeq++
	base := strings.TrimPrefix(e.responseID(), "resp_")
	if base == "" {
		base = "wiretranslate"
	}
	return fmt.Sprintf("%s_%s_%d", prefix, base, e.itemSeq)
}

func (e *responsesStreamEncoder) ensureCreated() error {
	if e.createdSent {
		return nil
	}
	e.createdSent = true
	return e.write("response.created", map[string]any{
		"type": "response.created",
		"response": map[string]any{
			"id":         e.responseID(),
			"object":     "response",
			"created_at": e.created,
			"model":      e.model,
			"status":     "in_progress",
			"output":     []any{},
		},
	})
}

// closeItem seals the open output item, emitting output_item.done with the
// assembled item and recording it for the response.completed output list.
func (e *responsesStreamEncoder) closeItem() error {
	if e.itemKind == "" {
		return nil
	}
	var item map[string]any
	switch e.itemKind {
	case "message":
		item = map[string]any{
			"type":   "message",
			"id":     e.itemID,
			"role":   "assistant",
			"status": "completed",
			"content": []map[string]any{{
				"type": "output_text",
				"text": e.textBuf.String(),
			}},
		}
	case "reasoning":
		summary := []map[string]any{}
		if t := e.thinkBuf.String(); t != "" {
			summary = append(summary, map[string]any{"type": "summary_text", "text": t})
		}
		item = map[string]any{
			"type":    "reasoning",
			"id":      e.itemID,
			"summary": summary,
		}
	case "function_call":
		args := strings.TrimSpace(e.argsBuf.String())
		if args == "" {
			args = "{}"
		}
		item = map[string]any{
			"type":      "function_call",
			"id":        e.itemID,
			"call_id":   e.toolCallID,
			"name":      e.toolName,
			"arguments": args,
			"status":    "completed",
		}
	}
	e.doneItems = append(e.doneItems, item)
	e.itemKind = ""
	e.textBuf.Reset()
	e.thinkBuf.Reset()
	e.argsBuf.Reset()
	return e.write("response.output_item.done", map[string]any{
		"type":         "response.output_item.done",
		"output_index": e.outputIndex,
		"item":         item,
	})
}

func (e *responsesStreamEncoder) openMessageItem() error {
	if e.itemKind == "message" {
		return nil
	}
	if err := e.closeItem(); err != nil {
		return err
	}
	e.outputIndex++
	e.itemKind = "message"
	e.itemID = e.nextItemID("msg")
	if err := e.write("response.output_item.added", map[string]any{
		"type":         "response.output_item.added",
		"output_index": e.outputIndex,
		"item": map[string]any{
			"type":    "message",
			"id":      e.itemID,
			"role":    "assistant",
			"status":  "in_progress",
			"content": []any{},
		},
	}); err != nil {
		return err
	}
	return e.write("response.content_part.added", map[string]any{
		"type":          "response.content_part.added",
		"output_index":  e.outputIndex,
		"content_index": 0,
		"part":          map[string]any{"type": "output_text", "text": ""},
	})
}

func (e *responsesStreamEncoder) openReasoningItem() error {
	if e.itemKind == "reasoning" {
		return nil
	}
	if err := e.closeItem(); err != nil {
		return err
	}
	e.outputIndex++
	e.itemKind = "reasoning"
	e.itemID = e.nextItemID("rs")
	return e.write("response.output_item.added", map[string]any{
		"type":         "response.output_item.added",
		"output_index": e.outputIndex,
		"item": map[string]any{
			"type":    "reasoning",
			"id":      e.itemID,
			"summary": []any{},
		},
	})
}

func (e *responsesStreamEncoder) WriteEvent(ev messages.StreamEvent) error {
	switch ev.Kind {
	case messages.KindMessageStart:
		if s := strings.TrimSpace(ev.MessageID); s != "" {
			e.respID = s
		}
		if s := strings.TrimSpace(ev.Model); s != "" {
			e.model = s
		}
		return e.ensureCreated()
	case messages.KindTextDelta:
		if err := e.ensureCreated(); err != nil {
			return err
		}
		if err := e.openMessageItem(); err != nil {
			return err
		}
		if ev.Text == "" {
			return nil
		}
		e.textBuf.WriteString(ev.Text)
		return e.write("response.output_text.delta", map[string]any{
			"type":          "response.output_text.delta",
			"output_index":  e.outputIndex,
			"content_index": 0,
			"delta":         ev.Text,
		})
	case messages.KindThinkingDelta:
		if err := e.ensureCreated(); err != nil {
			return err
		}
		if err := e.openReasoningItem(); err != nil {
			return err
		}
		if ev.Thinking == "" {
			return nil
		}
		e.thinkBuf.WriteString(ev.Thinking)
		return e.write("response.reasoning.delta", map[string]any{
			"type":          "response.reasoning.delta",
			"output_index":  e.outputIndex,
			"content_index": 0,
			"delta":         ev.Thinking,
		})
	case messages.KindToolUseStart:
		if err := e.ensureCreated(); err != nil {
			return err
		}
		if err := e.closeItem(); err != nil {
			return err
		}
		e.outputIndex++
		e.itemKind = "function_call"
		e.toolCallID = strings.TrimSpace(ev.ToolID)
		if e.toolCallID == "" {
			e.toolCallID = e.nextItemID("call")
		}
		e.itemID = "fc_" + e.toolCallID
		e.toolName = ev.ToolName
		e.toolOutput[ev.ToolIndex] = e.outputIndex
		e.toolItemIDs[ev.ToolIndex] = e.itemID
		return e.write("response.output_item.added", map[string]any{
			"type":         "response.output_item.added",
			"output_index": e.outputIndex,
			"item": map[string]any{
				"type":      "function_call",
				"id":        e.itemID,
				"call_id":   e.toolCallID,
				"name":      e.toolName,
				"arguments": "",
				"status":    "in_progress",
			},
		})
	case messages.KindToolInputDelta:
		if ev.PartialJSON == "" {
			return nil
		}
		outIdx, ok := e.toolOutput[ev.ToolIndex]
		if !ok {
			if e.itemKind != "function_call" {
				return nil
			}
			outIdx = e.outputIndex
		}
		if e.itemKind == "function_call" && outIdx == e.outputIndex {
			e.argsBuf.WriteString(ev.PartialJSON)
		}
		return e.write("response.function_call_arguments.delta", map[string]any{
			"type":         "response.function_call_arguments.delta",
			"output_index": outIdx,
			"item_id":      e.toolItemIDs[ev.ToolIndex],
			"delta":        ev.PartialJSON,
		})
	case messages.KindToolUseStop:
		// Anthropic fires block stops for text blocks too; only close when the
		// stop targets the open function_call item.
		if e.itemKind == "function_call" && e.toolOutput[ev.ToolIndex] == e.outputIndex {
			return e.closeItem()
		}
		return nil
	case messages.KindTelemetry:
		switch strings.TrimSpace(ev.Message) {
		case "length", "max_tokens":
			e.status = "incomplete"
		}
	case messages.KindUsage:
		if ev.InputTokens > 0 {
			e.inputTok = ev.InputTokens
		}
		if ev.OutputTokens > 0 {
			e.outputTok = ev.OutputTokens
		}
		if ev.CacheReadTokens > 0 {
			e.cacheRead = ev.CacheReadTokens
		}
		if ev.CacheWriteTokens > 0 {
			e.cacheWrite = ev.CacheWriteTokens
		}
	case messages.KindMessageStop:
		if err := e.ensureCreated(); err != nil {
			return err
		}
		if err := e.closeItem(); err != nil {
			return err
		}
		return e.write("response.completed", map[string]any{
			"type":     "response.completed",
			"response": e.responseObject(),
		})
	case messages.KindAPIError:
		if ev.Message != "" {
			return fmt.Errorf("wire-translate: upstream: %s", ev.Message)
		}
	}
	return nil
}

// responseObject assembles the full Responses object carried by
// response.completed and returned whole by encodeResponsesJSON.
func (e *responsesStreamEncoder) responseObject() map[string]any {
	model := e.model
	if model == "" {
		model = "unknown"
	}
	// inputTok is the total prompt including cached tokens (KindUsage
	// contract; Anthropic parsers fold cache_read/cache_creation in at the
	// boundary), so cached_tokens is always a subset of input_tokens.
	// input_tokens_details is emitted unconditionally: OpenAI always sends
	// it and SDK models with a non-optional field fail to decode without it.
	usage := map[string]any{
		"input_tokens":         e.inputTok,
		"output_tokens":        e.outputTok,
		"total_tokens":         e.inputTok + e.outputTok,
		"input_tokens_details": map[string]any{"cached_tokens": e.cacheRead},
	}
	output := e.doneItems
	if output == nil {
		output = []any{}
	}
	resp := map[string]any{
		"id":         e.responseID(),
		"object":     "response",
		"created_at": e.created,
		"model":      model,
		"status":     e.status,
		"output":     output,
		"usage":      usage,
	}
	if e.status == "incomplete" {
		resp["incomplete_details"] = map[string]any{"reason": "max_output_tokens"}
	}
	return resp
}

func (e *responsesStreamEncoder) Close() error {
	if !e.wrote {
		if err := e.ensureCreated(); err != nil {
			return err
		}
		return e.WriteEvent(messages.StreamEvent{Kind: messages.KindMessageStop})
	}
	return nil
}

// encodeResponsesSSE batch path for tests / non-pipe callers.
func encodeResponsesSSE(events []messages.StreamEvent, model string) ([]byte, error) {
	var buf strings.Builder
	enc := newResponsesStreamEncoder(&stringWriter{&buf}, model)
	for _, ev := range events {
		if err := enc.WriteEvent(ev); err != nil {
			return nil, err
		}
	}
	if err := enc.Close(); err != nil {
		return nil, err
	}
	if buf.Len() == 0 {
		return nil, fmt.Errorf("wire-translate: no responses stream events")
	}
	return []byte(buf.String()), nil
}

// encodeResponsesJSON builds the non-streaming Responses body from events.
// The stream encoder runs against io.Discard so item assembly (text,
// reasoning, function_call) shares one state machine.
func encodeResponsesJSON(events []messages.StreamEvent, model string) ([]byte, error) {
	enc := newResponsesStreamEncoder(io.Discard, model)
	for _, ev := range events {
		if err := enc.WriteEvent(ev); err != nil {
			return nil, err
		}
	}
	if err := enc.closeItem(); err != nil {
		return nil, err
	}
	return json.Marshal(enc.responseObject())
}
