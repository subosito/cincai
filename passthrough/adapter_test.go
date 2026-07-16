package passthrough_test

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/subosito/cincai/adaptersdk"
	"github.com/subosito/cincai/adaptersdk/handler"
	"github.com/subosito/cincai/catalog"
	"github.com/subosito/cincai/credential/store"
	"github.com/subosito/cincai/passthrough"
)

func TestRegisterProtocols(t *testing.T) {
	reg := adaptersdk.NewRegistry()
	if err := passthrough.New().Register(reg); err != nil {
		t.Fatal(err)
	}
	for _, p := range []string{"openai-chat-completions", "anthropic-messages", "openai-responses"} {
		if _, ok := reg.ChatHandlers[p]; !ok {
			t.Fatalf("missing chat handler for %s", p)
		}
	}
	if _, ok := reg.EmbedHandlers["openai-embeddings"]; !ok {
		t.Fatal("missing embed handler")
	}
	if _, ok := reg.ImageHandlers["openai-images"]; !ok {
		t.Fatal("missing openai-images handler")
	}
	if _, ok := reg.SpeechHandlers["openai-tts"]; !ok {
		t.Fatal("missing speech handler")
	}
	if _, ok := reg.VideoHandlers["openai-videos"]; !ok {
		t.Fatal("missing openai-videos handler")
	}
}

func TestChatForwardDedupesV1InBaseURL(t *testing.T) {
	var gotPath string
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		_, _ = io.WriteString(w, `{"ok":true}`)
	}))
	defer up.Close()

	reg := adaptersdk.NewRegistry()
	if err := passthrough.New().Register(reg); err != nil {
		t.Fatal(err)
	}
	h := reg.ChatHandlers["openai-chat-completions"]
	tgt := handler.Target{
		Target: catalog.Target{
			BaseURL: up.URL + "/v1",
		},
		Material: store.Material{Kind: store.KindAPIKey, APIKey: "sk-upstream"},
	}
	resp, err := h.Forward(context.Background(), http.DefaultClient, tgt, strings.NewReader(`{"model":"m"}`), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if gotPath != "/v1/chat/completions" {
		t.Fatalf("path=%s want /v1/chat/completions", gotPath)
	}
}

func TestChatForwardInjectsUpstreamStripsClient(t *testing.T) {
	var gotAuth string
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		if r.Header.Get("x-api-key") != "" {
			t.Error("client x-api-key must not reach upstream")
		}
		if r.URL.Path != "/v1/chat/completions" {
			t.Errorf("path=%s", r.URL.Path)
		}
		_, _ = io.WriteString(w, `{"ok":true}`)
	}))
	defer up.Close()

	reg := adaptersdk.NewRegistry()
	if err := passthrough.New().Register(reg); err != nil {
		t.Fatal(err)
	}
	h := reg.ChatHandlers["openai-chat-completions"]
	tgt := handler.Target{
		Target: catalog.Target{
			BaseURL: up.URL, InjectPreset: "",
		},
		Material: store.Material{Kind: store.KindAPIKey, APIKey: "sk-upstream"},
	}
	hdr := make(http.Header)
	hdr.Set("Authorization", "Bearer client-token")
	hdr.Set("x-api-key", "client-key-should-strip")
	body := strings.NewReader(`{"model":"m","messages":[]}`)

	resp, err := h.Forward(context.Background(), http.DefaultClient, tgt, body, hdr)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if gotAuth != "Bearer sk-upstream" {
		t.Fatalf("upstream auth=%q", gotAuth)
	}
}
