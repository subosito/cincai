package wiretranslate

import (
	"encoding/json"
	"strings"
)

// OpenAI Responses API shapes (ingress side of the r2o/r2a translators).

type responsesRequest struct {
	Model              string              `json:"model"`
	Input              json.RawMessage     `json:"input"`
	Instructions       string              `json:"instructions,omitempty"`
	Tools              []responsesTool     `json:"tools,omitempty"`
	Reasoning          *responsesReasoning `json:"reasoning,omitempty"`
	ReasoningEffort    string              `json:"reasoning_effort,omitempty"`
	Store              *bool               `json:"store,omitempty"`
	Stream             bool                `json:"stream,omitempty"`
	PreviousResponseID string              `json:"previous_response_id,omitempty"`
	MaxOutputTokens    int                 `json:"max_output_tokens,omitempty"`
}

type responsesReasoning struct {
	Effort string `json:"effort,omitempty"`
}

// responsesTool is a Responses function tool: name is top-level, not nested
// under "function" like chat-completions.
type responsesTool struct {
	Type        string         `json:"type"`
	Name        string         `json:"name"`
	Description string         `json:"description,omitempty"`
	Parameters  map[string]any `json:"parameters,omitempty"`
}

// responsesInputItem is one entry in the request input list: a role message
// (bare {role, content} or {type:"message", ...}), a function_call, a
// function_call_output, or a reasoning item.
type responsesInputItem struct {
	Type      string          `json:"type,omitempty"`
	Role      string          `json:"role,omitempty"`
	ID        string          `json:"id,omitempty"`
	CallID    string          `json:"call_id,omitempty"`
	Name      string          `json:"name,omitempty"`
	Arguments string          `json:"arguments,omitempty"`
	Output    json.RawMessage `json:"output,omitempty"`
	Content   json.RawMessage `json:"content,omitempty"`
}

// statefulContinuation reports whether the request relies on server-side turn
// state, which a stateless chat/anthropic upstream cannot honor. store
// defaults to true in the Responses API, so an omitted store with a
// previous_response_id (what the OpenAI SDK sends) is still stateful.
func (r *responsesRequest) statefulContinuation() bool {
	if strings.TrimSpace(r.PreviousResponseID) == "" {
		return false
	}
	return r.Store == nil || *r.Store // omitted store == true
}

// effort returns the client reasoning effort, preferring the top-level
// reasoning_effort the catalog effort expander writes over reasoning.effort.
func (r *responsesRequest) effort() string {
	if s := strings.TrimSpace(r.ReasoningEffort); s != "" {
		return s
	}
	if r.Reasoning != nil {
		return strings.TrimSpace(r.Reasoning.Effort)
	}
	return ""
}

// parseResponsesInput normalizes the input field (a plain string or an item
// list) into items.
func parseResponsesInput(raw json.RawMessage) ([]responsesInputItem, error) {
	if len(raw) == 0 {
		return nil, nil
	}
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		if strings.TrimSpace(s) == "" {
			return nil, nil
		}
		return []responsesInputItem{{Role: "user", Content: raw}}, nil
	}
	var items []responsesInputItem
	if err := json.Unmarshal(raw, &items); err != nil {
		return nil, err
	}
	return items, nil
}

// role returns the effective item role. Bare role messages carry no type;
// message items carry type "message".
func (it responsesInputItem) messageRole() string {
	typ := strings.TrimSpace(it.Type)
	if typ != "" && typ != "message" {
		return ""
	}
	return strings.TrimSpace(it.Role)
}

// contentText flattens item content (string or input_text/output_text parts)
// to plain text. Multimodal parts are out of scope for v1.
func (it responsesInputItem) contentText() string {
	return responsesContentText(it.Content)
}

// outputText flattens a function_call_output's output. The Responses API
// permits a plain string, an array of content parts, or an error-shaped
// object; a typed string field hard-400s the whole request on the array
// form current SDKs emit for non-text tool results.
func (it responsesInputItem) outputText() string {
	raw := it.Output
	if len(raw) == 0 {
		return ""
	}
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		return s
	}
	if raw[0] == '[' {
		return responsesContentText(raw)
	}
	// Object form (e.g. an error payload): keep the raw JSON so the tool
	// result round-trips instead of being dropped.
	return string(raw)
}

func responsesContentText(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		return s
	}
	var parts []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	}
	if err := json.Unmarshal(raw, &parts); err != nil {
		return ""
	}
	var texts []string
	for _, p := range parts {
		switch p.Type {
		case "input_text", "output_text", "text":
			if t := strings.TrimSpace(p.Text); t != "" {
				texts = append(texts, p.Text)
			}
		}
	}
	return strings.Join(texts, "\n")
}
