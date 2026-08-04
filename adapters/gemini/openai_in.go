package gemini

import (
	"encoding/json"
	"fmt"
	"strings"
)

// OpenAIRequest is the subset of chat.completions we translate.
type OpenAIRequest struct {
	Model               string          `json:"model"`
	Messages            []OpenAIMessage `json:"messages"`
	MaxTokens           int             `json:"max_tokens,omitempty"`
	MaxCompletionTokens int             `json:"max_completion_tokens,omitempty"`
	Temperature         *float64        `json:"temperature,omitempty"`
	TopP                *float64        `json:"top_p,omitempty"`
	// ThinkingBudget is a common extension for Gemini thinking models (tokens).
	// Also accepted via nested thinking / thinkingConfig in raw JSON if unmarshaled here.
	ThinkingBudget *int `json:"thinking_budget,omitempty"`
	Stream         bool `json:"stream,omitempty"`
	Tools          []OpenAITool `json:"tools,omitempty"`
}

// OpenAIMessage is one chat message (possibly with tool_calls / tool_call_id).
type OpenAIMessage struct {
	Role       string           `json:"role"`
	Content    any              `json:"content"`
	Name       string           `json:"name,omitempty"`
	ToolCalls  []OpenAIToolCall `json:"tool_calls,omitempty"`
	ToolCallID string           `json:"tool_call_id,omitempty"`
}

// OpenAIToolCall is an assistant tool invocation.
type OpenAIToolCall struct {
	ID               string `json:"id"`
	Type             string `json:"type"`
	ThoughtSignature string `json:"thought_signature,omitempty"` // Gemini 3 multi-hop
	Function         struct {
		Name      string `json:"name"`
		Arguments string `json:"arguments"`
	} `json:"function"`
}

// OpenAITool is a tools[] function definition.
type OpenAITool struct {
	Type     string `json:"type"`
	Function struct {
		Name        string         `json:"name"`
		Description string         `json:"description"`
		Parameters  map[string]any `json:"parameters"`
	} `json:"function"`
}

// FromOpenAI converts an OpenAI chat-completions request into generateContent.
// tool_call_id on role=tool is resolved to function names from earlier assistant tool_calls.
func FromOpenAI(req OpenAIRequest) (GenerateRequest, error) {
	idToName := map[string]string{}
	var (
		contents []Content
		system   []string
	)

	for _, m := range req.Messages {
		role := strings.TrimSpace(m.Role)
		switch role {
		case "system", "developer":
			if s := messageText(m.Content); s != "" {
				system = append(system, s)
			}
		case "user":
			parts, err := userParts(m)
			if err != nil {
				return GenerateRequest{}, err
			}
			if len(parts) > 0 {
				contents = append(contents, Content{Role: "user", Parts: parts})
			}
		case "assistant":
			for _, tc := range m.ToolCalls {
				id := strings.TrimSpace(tc.ID)
				name := strings.TrimSpace(tc.Function.Name)
				if id != "" && name != "" {
					idToName[id] = name
				}
			}
			parts, err := assistantParts(m)
			if err != nil {
				return GenerateRequest{}, err
			}
			if len(parts) > 0 {
				contents = append(contents, Content{Role: "model", Parts: parts})
			}
		case "tool":
			name := strings.TrimSpace(m.Name)
			if name == "" {
				if id := strings.TrimSpace(m.ToolCallID); id != "" {
					name = idToName[id]
				}
			}
			if name == "" {
				name = "tool"
			}
			contents = append(contents, Content{
				Role: "user",
				Parts: []ContentPart{{
					FunctionResponse: &FunctionResponse{
						Name: name,
						Response: map[string]any{
							"content": messageText(m.Content),
						},
					},
				}},
			})
		default:
			return GenerateRequest{}, fmt.Errorf("gemini: unsupported openai role %q", role)
		}
	}

	maxTok := req.MaxTokens
	if maxTok == 0 {
		maxTok = req.MaxCompletionTokens
	}
	out := GenerateRequest{
		Contents: contents,
		GenerationConfig: GenerationConfig{
			MaxOutputTokens: maxTok,
			Temperature:     req.Temperature,
			TopP:            req.TopP,
		},
	}
	if req.ThinkingBudget != nil && *req.ThinkingBudget > 0 {
		out.GenerationConfig.ThinkingConfig = &ThinkingConfig{ThinkingBudget: *req.ThinkingBudget}
	}
	if len(system) > 0 {
		out.SystemInstruction = &Content{
			Parts: []ContentPart{{Text: strings.Join(system, "\n\n")}},
		}
	}
	decls := toolDeclarations(req.Tools)
	wantSearch := hasNativeWebSearch(req.Tools)
	if len(decls) > 0 {
		out.Tools = append(out.Tools, ToolGroup{FunctionDeclarations: decls})
	}
	// Mixing googleSearch with functionDeclarations requires
	// toolConfig.includeServerSideToolInvocations. Gemini API accepts it;
	// Vertex rejects it as unknown; Cloud Code strips it then 400s. Prefer
	// client function tools when both are requested so agent loops work on
	// every host; search-only still gets googleSearch.
	if wantSearch && len(decls) == 0 {
		out.Tools = append(out.Tools, ToolGroup{GoogleSearch: &GoogleSearch{}})
	}
	if len(out.Contents) == 0 {
		return GenerateRequest{}, fmt.Errorf("gemini: no messages")
	}
	return out, nil
}

// hasNativeWebSearch reports whether the client asked for provider-executed
// web search. Clients speak the OpenAI/xAI spelling ({"type":"web_search"});
// Gemini's equivalent is a google_search tool group, so the adapter translates
// rather than making every client learn a per-vendor shape.
func hasNativeWebSearch(tools []OpenAITool) bool {
	for _, tool := range tools {
		switch strings.ToLower(strings.TrimSpace(tool.Type)) {
		case "web_search", "web_search_preview", "google_search":
			return true
		}
	}
	return false
}

func toolDeclarations(tools []OpenAITool) []FunctionDeclaration {
	var decls []FunctionDeclaration
	for _, tool := range tools {
		if strings.TrimSpace(tool.Type) != "" && tool.Type != "function" {
			continue
		}
		name := strings.TrimSpace(tool.Function.Name)
		if name == "" {
			continue
		}
		decls = append(decls, FunctionDeclaration{
			Name:        name,
			Description: tool.Function.Description,
			Parameters:  tool.Function.Parameters,
		})
	}
	return decls
}

func messageText(content any) string {
	switch c := content.(type) {
	case string:
		return strings.TrimSpace(c)
	case nil:
		return ""
	default:
		raw, err := json.Marshal(c)
		if err != nil {
			return ""
		}
		var parts []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		}
		if err := json.Unmarshal(raw, &parts); err != nil {
			return strings.TrimSpace(string(raw))
		}
		var texts []string
		for _, p := range parts {
			if strings.TrimSpace(p.Type) == "text" && strings.TrimSpace(p.Text) != "" {
				texts = append(texts, p.Text)
			}
		}
		return strings.Join(texts, "\n")
	}
}

func userParts(m OpenAIMessage) ([]ContentPart, error) {
	// Plain string content stays text-only.
	if s, ok := m.Content.(string); ok {
		s = strings.TrimSpace(s)
		if s == "" {
			return nil, nil
		}
		return []ContentPart{{Text: s}}, nil
	}
	if m.Content == nil {
		return nil, nil
	}

	raw, err := json.Marshal(m.Content)
	if err != nil {
		return nil, fmt.Errorf("gemini: user content: %w", err)
	}
	// Multimodal array: text + image_url + video_url (OpenAI chat shape).
	var arr []map[string]any
	if err := json.Unmarshal(raw, &arr); err != nil {
		// Fallback: treat as opaque text (legacy).
		if s := messageText(m.Content); s != "" {
			return []ContentPart{{Text: s}}, nil
		}
		return nil, nil
	}
	if len(arr) == 0 {
		return nil, nil
	}

	var parts []ContentPart
	for i, p := range arr {
		typ, _ := p["type"].(string)
		typ = strings.ToLower(strings.TrimSpace(typ))
		switch typ {
		case "text", "input_text", "":
			if t, ok := p["text"].(string); ok && strings.TrimSpace(t) != "" {
				parts = append(parts, ContentPart{Text: t})
			}
		case "image_url", "input_image", "image":
			med, err := MediaFromOpenAIPart(p, "image_url", "image/png")
			if err != nil {
				return nil, fmt.Errorf("gemini: user content[%d] image: %w", i, err)
			}
			if med != nil {
				parts = append(parts, ContentPartFromMedia(med))
			}
		case "video_url", "input_video", "video":
			med, err := MediaFromOpenAIPart(p, "video_url", "video/mp4")
			if err != nil {
				return nil, fmt.Errorf("gemini: user content[%d] video: %w", i, err)
			}
			if med != nil {
				parts = append(parts, ContentPartFromMedia(med))
			}
		case "input_audio", "audio_url", "audio":
			med, err := MediaFromOpenAIPart(p, "audio_url", "audio/wav")
			if err != nil {
				return nil, fmt.Errorf("gemini: user content[%d] audio: %w", i, err)
			}
			if med != nil {
				parts = append(parts, ContentPartFromMedia(med))
			}
		default:
			// Ignore unknown part types (e.g. file placeholders) rather than fail the turn.
		}
	}
	return parts, nil
}

func assistantParts(m OpenAIMessage) ([]ContentPart, error) {
	var parts []ContentPart
	if s := messageText(m.Content); s != "" {
		parts = append(parts, ContentPart{Text: s})
	}
	for _, tc := range m.ToolCalls {
		name := strings.TrimSpace(tc.Function.Name)
		if name == "" {
			continue
		}
		parts = append(parts, ContentPart{
			FunctionCall: &FunctionCall{
				Name: name,
				Args: parseArgsObject(tc.Function.Arguments),
			},
			// Gemini 3: required when replaying prior functionCall parts.
			ThoughtSignature: strings.TrimSpace(tc.ThoughtSignature),
		})
	}
	return parts, nil
}

func parseArgsObject(raw string) map[string]any {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return map[string]any{}
	}
	var obj map[string]any
	if err := json.Unmarshal([]byte(raw), &obj); err != nil {
		return map[string]any{}
	}
	if obj == nil {
		return map[string]any{}
	}
	return obj
}
