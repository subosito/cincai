// Package gemini is the shared Gemini generateContent wire used by the vertex
// chat adapter (and reusable by zenmux / a future Vertex AI provider).
//
// It maps OpenAI chat-completions (including tool history) to native
// functionCall/functionResponse contents — not a full GCP product client.
package gemini

// ContentPart is one Gemini content part.
type ContentPart struct {
	Text             string            `json:"text,omitempty"`
	ThoughtSignature string            `json:"thoughtSignature,omitempty"`
	FunctionCall     *FunctionCall     `json:"functionCall,omitempty"`
	FunctionResponse *FunctionResponse `json:"functionResponse,omitempty"`
	// InlineData is base64 media (image / audio / short video) from OpenAI data URLs.
	InlineData *InlineData `json:"inlineData,omitempty"`
	// FileData is a remote media URI (https / gs) when the OpenAI part uses a URL.
	FileData *FileData `json:"fileData,omitempty"`
}

// InlineData holds base64 media (optional).
type InlineData struct {
	MimeType string `json:"mimeType,omitempty"`
	Data     string `json:"data,omitempty"`
}

// FileData references remote media by URI (Gemini generateContent).
type FileData struct {
	MimeType string `json:"mimeType,omitempty"`
	FileURI  string `json:"fileUri,omitempty"`
}

// FunctionCall is a model tool invocation.
// ID is intentionally omitted: Vertex/zenmux reject unknown name "id" on
// functionCall / functionResponse; OpenAI tool_call ids stay in-process only
// (tool_call_id → name map in FromOpenAI).
type FunctionCall struct {
	Name string         `json:"name"`
	Args map[string]any `json:"args,omitempty"`
}

// FunctionResponse is a tool result from the client.
type FunctionResponse struct {
	Name     string `json:"name"`
	Response any    `json:"response"`
}

// Content is one turn in generateContent.contents.
type Content struct {
	Role  string        `json:"role,omitempty"`
	Parts []ContentPart `json:"parts"`
}

// FunctionDeclaration is a tool schema entry.
type FunctionDeclaration struct {
	Name        string         `json:"name"`
	Description string         `json:"description,omitempty"`
	Parameters  map[string]any `json:"parameters,omitempty"`
}

// ToolGroup wraps functionDeclarations (Gemini tools array element).
type ToolGroup struct {
	FunctionDeclarations []FunctionDeclaration `json:"functionDeclarations"`
}

// GenerationConfig is a subset of generationConfig.
type GenerationConfig struct {
	MaxOutputTokens int             `json:"maxOutputTokens,omitempty"`
	Temperature     *float64        `json:"temperature,omitempty"`
	TopP            *float64        `json:"topP,omitempty"`
	ThinkingConfig  *ThinkingConfig `json:"thinkingConfig,omitempty"`
}

// ThinkingConfig controls Gemini thinking-budget style models.
type ThinkingConfig struct {
	ThinkingBudget int `json:"thinkingBudget,omitempty"`
}

// GenerateRequest is the body for generateContent / streamGenerateContent.
type GenerateRequest struct {
	SystemInstruction *Content          `json:"systemInstruction,omitempty"`
	Contents          []Content         `json:"contents"`
	GenerationConfig  GenerationConfig  `json:"generationConfig,omitempty"`
	Tools             []ToolGroup       `json:"tools,omitempty"`
}

// UsageMetadata is prompt/candidate token counts from Gemini.
type UsageMetadata struct {
	PromptTokenCount        int `json:"promptTokenCount"`
	CandidatesTokenCount    int `json:"candidatesTokenCount"`
	TotalTokenCount         int `json:"totalTokenCount"`
	CachedContentTokenCount int `json:"cachedContentTokenCount,omitempty"`
}

// Candidate is one generateContent candidate.
type Candidate struct {
	Content      Content `json:"content"`
	FinishReason string  `json:"finishReason,omitempty"`
}

// GenerateResponse is a non-stream (or single-chunk) generateContent response.
type GenerateResponse struct {
	Candidates    []Candidate   `json:"candidates"`
	UsageMetadata UsageMetadata `json:"usageMetadata"`
	ModelVersion  string        `json:"modelVersion,omitempty"`
	ResponseID    string        `json:"responseId,omitempty"`
	Error         *struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
		Status  string `json:"status"`
	} `json:"error,omitempty"`
}

