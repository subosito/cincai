package gemini

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestFromOpenAI_toolHistoryResolvesNameByCallID(t *testing.T) {
	t.Parallel()
	req := OpenAIRequest{
		Model: "gemini-3.6-flash",
		Messages: []OpenAIMessage{
			{Role: "system", Content: "You are helpful."},
			{Role: "user", Content: "do the thing"},
			{
				Role: "assistant",
				ToolCalls: []OpenAIToolCall{{
					ID:   "call_abc",
					Type: "function",
					Function: struct {
						Name      string `json:"name"`
						Arguments string `json:"arguments"`
					}{Name: "bash", Arguments: `{"command":"ls"}`},
				}},
			},
			// dududu-style: tool role with tool_call_id, no name
			{Role: "tool", ToolCallID: "call_abc", Content: "file.txt\n"},
			{Role: "user", Content: "thanks"},
		},
		Tools: []OpenAITool{{
			Type: "function",
			Function: struct {
				Name        string         `json:"name"`
				Description string         `json:"description"`
				Parameters  map[string]any `json:"parameters"`
			}{
				Name:        "bash",
				Description: "run shell",
				Parameters:  map[string]any{"type": "object"},
			},
		}},
		MaxTokens: 256,
	}
	gen, err := FromOpenAI(req)
	if err != nil {
		t.Fatal(err)
	}
	if gen.SystemInstruction == nil || len(gen.SystemInstruction.Parts) != 1 {
		t.Fatalf("system: %+v", gen.SystemInstruction)
	}
	// user, model(tool_call), user(functionResponse), user
	if len(gen.Contents) != 4 {
		t.Fatalf("contents=%d %+v", len(gen.Contents), gen.Contents)
	}
	modelTurn := gen.Contents[1]
	if modelTurn.Role != "model" || len(modelTurn.Parts) != 1 || modelTurn.Parts[0].FunctionCall == nil {
		t.Fatalf("model turn: %+v", modelTurn)
	}
	fc := modelTurn.Parts[0].FunctionCall
	if fc.Name != "bash" {
		t.Fatalf("functionCall: %+v", fc)
	}
	if fc.Args["command"] != "ls" {
		t.Fatalf("args: %+v", fc.Args)
	}
	toolTurn := gen.Contents[2]
	if toolTurn.Role != "user" || toolTurn.Parts[0].FunctionResponse == nil {
		t.Fatalf("tool turn: %+v", toolTurn)
	}
	fr := toolTurn.Parts[0].FunctionResponse
	if fr.Name != "bash" {
		t.Fatalf("functionResponse name=%q want bash (resolved via tool_call_id)", fr.Name)
	}
	// Vertex rejects id on functionCall/functionResponse.
	wire, err := json.Marshal(gen)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(wire), `"id"`) {
		t.Fatalf("wire must omit functionCall/functionResponse id: %s", wire)
	}
	if len(gen.Tools) != 1 || len(gen.Tools[0].FunctionDeclarations) != 1 {
		t.Fatalf("tools: %+v", gen.Tools)
	}
	if gen.GenerationConfig.MaxOutputTokens != 256 {
		t.Fatalf("maxOutputTokens=%d", gen.GenerationConfig.MaxOutputTokens)
	}
}

func TestFromOpenAI_imageDataURL(t *testing.T) {
	t.Parallel()
	// Minimal 1x1 red PNG base64 (valid enough for wire shape).
	b64 := "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mP8z8BQDwAEhQGAhKmMIQAAAABJRU5ErkJggg=="
	req := OpenAIRequest{
		Model: "google/gemini-3.6-flash",
		Messages: []OpenAIMessage{{
			Role: "user",
			Content: []any{
				map[string]any{"type": "text", "text": "What color?"},
				map[string]any{
					"type": "image_url",
					"image_url": map[string]any{
						"url": "data:image/png;base64," + b64,
					},
				},
			},
		}},
		MaxTokens: 32,
	}
	gen, err := FromOpenAI(req)
	if err != nil {
		t.Fatal(err)
	}
	if len(gen.Contents) != 1 || len(gen.Contents[0].Parts) != 2 {
		t.Fatalf("contents=%+v", gen.Contents)
	}
	if gen.Contents[0].Parts[0].Text != "What color?" {
		t.Fatalf("text part: %+v", gen.Contents[0].Parts[0])
	}
	id := gen.Contents[0].Parts[1].InlineData
	if id == nil || id.MimeType != "image/png" || id.Data != b64 {
		t.Fatalf("inlineData: %+v", gen.Contents[0].Parts[1])
	}
}

func TestFromOpenAI_videoDataURL(t *testing.T) {
	t.Parallel()
	b64 := "AAAA" // shape only
	req := OpenAIRequest{
		Model: "google/gemini-3.6-flash",
		Messages: []OpenAIMessage{{
			Role: "user",
			Content: []any{
				map[string]any{
					"type": "video_url",
					"video_url": map[string]any{"url": "data:video/mp4;base64," + b64},
				},
				map[string]any{"type": "text", "text": "describe"},
			},
		}},
	}
	gen, err := FromOpenAI(req)
	if err != nil {
		t.Fatal(err)
	}
	if len(gen.Contents[0].Parts) != 2 {
		t.Fatalf("parts=%+v", gen.Contents[0].Parts)
	}
	if gen.Contents[0].Parts[0].InlineData == nil || gen.Contents[0].Parts[0].InlineData.MimeType != "video/mp4" {
		t.Fatalf("video part: %+v", gen.Contents[0].Parts[0])
	}
	if gen.Contents[0].Parts[1].Text != "describe" {
		t.Fatalf("text: %+v", gen.Contents[0].Parts[1])
	}
}

func TestFromOpenAI_imageHTTPSFileData(t *testing.T) {
	t.Parallel()
	req := OpenAIRequest{
		Model: "google/gemini-3.1-pro",
		Messages: []OpenAIMessage{{
			Role: "user",
			Content: []any{
				map[string]any{
					"type":      "image_url",
					"image_url": map[string]any{"url": "https://example.com/x.jpg"},
				},
			},
		}},
	}
	gen, err := FromOpenAI(req)
	if err != nil {
		t.Fatal(err)
	}
	fd := gen.Contents[0].Parts[0].FileData
	if fd == nil || fd.FileURI != "https://example.com/x.jpg" || fd.MimeType != "image/jpeg" {
		t.Fatalf("fileData: %+v", gen.Contents[0].Parts[0])
	}
}

func TestFromOpenAI_roundTripJSON(t *testing.T) {
	t.Parallel()
	raw := []byte(`{
	  "model":"gemini-3.6-flash",
	  "messages":[
	    {"role":"user","content":"hi"},
	    {"role":"assistant","content":null,"tool_calls":[{"id":"t1","type":"function","function":{"name":"recall","arguments":"{\"query\":\"x\"}"}}]},
	    {"role":"tool","tool_call_id":"t1","content":"result"}
	  ],
	  "tools":[{"type":"function","function":{"name":"recall","parameters":{"type":"object"}}}],
	  "max_tokens":64
	}`)
	var req OpenAIRequest
	if err := json.Unmarshal(raw, &req); err != nil {
		t.Fatal(err)
	}
	gen, err := FromOpenAI(req)
	if err != nil {
		t.Fatal(err)
	}
	if len(gen.Contents) != 3 {
		t.Fatalf("contents=%d", len(gen.Contents))
	}
	// null assistant content + tool_calls still produces model functionCall only
	if gen.Contents[1].Parts[0].FunctionCall == nil || gen.Contents[1].Parts[0].FunctionCall.Name != "recall" {
		t.Fatalf("expected recall functionCall: %+v", gen.Contents[1])
	}
	if gen.Contents[2].Parts[0].FunctionResponse == nil || gen.Contents[2].Parts[0].FunctionResponse.Name != "recall" {
		t.Fatalf("expected recall functionResponse: %+v", gen.Contents[2])
	}
}
