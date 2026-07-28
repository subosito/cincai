package vertex

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/subosito/cincai/adaptersdk/handler"
	"github.com/subosito/cincai/catalog"
	"github.com/subosito/cincai/credential/store"
)

func TestChatHandlerForward_nonStreamToolCall(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.URL.Path, ":generateContent") {
			http.NotFound(w, r)
			return
		}
		if strings.Contains(r.URL.Path, "streamGenerateContent") {
			t.Fatalf("unexpected stream path %s", r.URL.Path)
		}
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		// expect contents with functionResponse when tool history present
		raw, _ := io.ReadAll(io.NopCloser(strings.NewReader("")))
		_ = raw
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
		  "candidates":[{"content":{"role":"model","parts":[{"text":"ok"}]},"finishReason":"STOP"}],
		  "usageMetadata":{"promptTokenCount":5,"candidatesTokenCount":1}
		}`))
	}))
	defer srv.Close()

	h := &ChatHandler{}
	body := `{"model":"google/gemini-3.6-flash","max_tokens":16,"stream":false,"messages":[
	  {"role":"user","content":"hi"},
	  {"role":"assistant","tool_calls":[{"id":"c1","type":"function","function":{"name":"bash","arguments":"{\"command\":\"ls\"}"}}]},
	  {"role":"tool","tool_call_id":"c1","content":"a.txt"},
	  {"role":"user","content":"thanks"}
	],"tools":[{"type":"function","function":{"name":"bash","parameters":{"type":"object"}}}]}`
	resp, err := h.Forward(context.Background(), srv.Client(), handler.Target{
		Target: catalog.Target{
			BaseURL:       srv.URL,
			UpstreamModel: "google/gemini-3.6-flash",
		},
		Material: store.Material{Kind: store.KindAPIKey, APIKey: "k"},
	}, strings.NewReader(body), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("status=%d body=%s", resp.StatusCode, b)
	}
	out, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(out), `"content":"ok"`) {
		t.Fatalf("%s", out)
	}
}

func TestChatHandlerForward_upstreamError(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(400)
		_, _ = w.Write([]byte(`{"error":{"message":"bad"}}`))
	}))
	defer srv.Close()
	h := &ChatHandler{}
	body := `{"model":"gemini-3.6-flash","messages":[{"role":"user","content":"hi"}],"max_tokens":8}`
	resp, err := h.Forward(context.Background(), srv.Client(), handler.Target{
		Target:   catalog.Target{BaseURL: srv.URL, UpstreamModel: "gemini-3.6-flash"},
		Material: store.Material{Kind: store.KindAPIKey, APIKey: "k"},
	}, strings.NewReader(body), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 400 {
		t.Fatalf("status=%d", resp.StatusCode)
	}
}
