package mistral

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
)

func ocrResponse(t *testing.T, status int, body string) *http.Response {
	t.Helper()
	return &http.Response{
		StatusCode: status,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(strings.NewReader(body)),
	}
}

// The synthesized completion is what the client actually receives, so it has to
// be a valid chat.completion: a unique OpenAI-shaped id (it used to ship the
// literal "mistral-ocr-fake") and a created stamp (it used to omit the field).
func TestTranslateOcrRespToChatSynthesizesValidCompletion(t *testing.T) {
	const ocrBody = `{"model":"mistral-ocr-latest","pages":[{"markdown":"# Title"}]}`
	resp := translateOcrRespToChat(ocrResponse(t, 200, ocrBody))

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status=%d", resp.StatusCode)
	}
	var got map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&got); err != nil {
		t.Fatal(err)
	}

	id, _ := got["id"].(string)
	if !strings.HasPrefix(id, "chatcmpl-") {
		t.Errorf("id=%q, want a chatcmpl- prefix", id)
	}
	if strings.Contains(id, "fake") {
		t.Errorf("id=%q leaks a placeholder to the client", id)
	}
	if created, ok := got["created"].(float64); !ok || created <= 0 {
		t.Errorf("created=%v, want a unix timestamp", got["created"])
	}
	if got["object"] != "chat.completion" {
		t.Errorf("object=%v", got["object"])
	}
	if got["model"] != "mistral-ocr-latest" {
		t.Errorf("model=%v", got["model"])
	}
}

func TestChatCompletionIDIsUnique(t *testing.T) {
	seen := make(map[string]bool, 100)
	for range 100 {
		id := chatCompletionID()
		if seen[id] {
			t.Fatalf("duplicate id %q", id)
		}
		seen[id] = true
	}
}

// Upstream errors must reach the client unchanged rather than becoming an
// empty 200 completion.
func TestTranslateOcrRespToChatRelaysUpstreamError(t *testing.T) {
	const errBody = `{"detail":"Unauthorized"}`
	resp := translateOcrRespToChat(ocrResponse(t, http.StatusUnauthorized, errBody))

	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("status=%d want 401", resp.StatusCode)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(body, []byte(errBody)) {
		t.Fatalf("body=%s want %s", body, errBody)
	}
}
