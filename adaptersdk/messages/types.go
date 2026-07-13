// Package messages holds Anthropic Messages API shapes used by translate adapters.
package messages

import (
	"encoding/json"
	"fmt"
)

// Kind classifies normalized stream events.
type Kind int

const (
	KindTextDelta Kind = iota + 1
	KindThinkingDelta
	KindSignatureDelta
	KindToolUseStart
	KindToolInputDelta
	KindToolUseStop
	KindCompactBoundary
	KindAPIRetry
	KindAPIError
	KindUsage
	KindTelemetry
	KindMessageStart
	KindMessageStop
	KindPing
)

// StreamEvent is a provider-neutral streaming unit.
type StreamEvent struct {
	Kind Kind

	Text string

	Thinking  string
	Signature string

	ToolIndex        int
	ToolID           string
	ToolName         string
	ThoughtSignature string

	PartialJSON string

	Message string
	Code    string

	InputTokens  int
	OutputTokens int

	MessageID string
	Model     string
}

// ContentBlock is one Anthropic message content element.
type ContentBlock struct {
	Type             string          `json:"type"`
	Text             string          `json:"text,omitempty"`
	ID               string          `json:"id,omitempty"`
	Name             string          `json:"name,omitempty"`
	Input            json.RawMessage `json:"input,omitempty"`
	ToolUseID        string          `json:"tool_use_id,omitempty"`
	ThoughtSignature string          `json:"thought_signature,omitempty"`
	Content          string          `json:"content,omitempty"`
	IsError          bool            `json:"is_error,omitempty"`
}

// APIMessage is one Messages API message.
type APIMessage struct {
	Role             string `json:"role"`
	Content          any    `json:"content"`
	ReasoningContent string `json:"reasoning_content,omitempty"`
}

// ParseMessages decodes a messages JSON array.
func ParseMessages(raw json.RawMessage) ([]APIMessage, error) {
	var msgs []APIMessage
	if err := json.Unmarshal(raw, &msgs); err != nil {
		return nil, err
	}
	return msgs, nil
}

// MarshalMessages JSON-encodes the message list.
func MarshalMessages(msgs []APIMessage) (json.RawMessage, error) {
	if len(msgs) == 0 {
		return nil, fmt.Errorf("messages: empty")
	}
	return json.Marshal(msgs)
}