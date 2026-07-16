package elevenlabs_test

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
	"github.com/subosito/cincai/internal/adapters/elevenlabs"
)

func TestSpeechTranslateOpenAIToElevenLabs(t *testing.T) {
	var gotPath, gotKey, gotBody, gotAccept string
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotKey = r.Header.Get("xi-api-key")
		gotAccept = r.Header.Get("Accept")
		b, _ := io.ReadAll(r.Body)
		gotBody = string(b)
		w.Header().Set("Content-Type", "audio/ogg")
		_, _ = io.WriteString(w, "oggbytes")
	}))
	defer up.Close()

	reg := adaptersdk.NewRegistry()
	if err := elevenlabs.New().Register(reg); err != nil {
		t.Fatal(err)
	}
	h := reg.SpeechAdapters["elevenlabs"]
	tgt := handler.Target{
		Target: catalog.Target{
			BaseURL:       up.URL,
			UpstreamModel: "eleven_multilingual_v2",
		},
		Material: store.Material{Kind: store.KindAPIKey, APIKey: "el-key"},
	}
	body := strings.NewReader(`{"model":"eleven-multilingual-v2","input":"hello","voice":"voice123","response_format":"mp3"}`)
	resp, err := h.Forward(context.Background(), http.DefaultClient, tgt, body, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("status=%d %s", resp.StatusCode, b)
	}
	if gotPath != "/v1/text-to-speech/voice123" {
		t.Fatalf("path=%q", gotPath)
	}
	if gotKey != "el-key" {
		t.Fatalf("xi-api-key=%q", gotKey)
	}
	if !strings.Contains(gotBody, `"text":"hello"`) || !strings.Contains(gotBody, `"model_id":"eleven_multilingual_v2"`) {
		t.Fatalf("body=%s", gotBody)
	}
	if gotAccept != "audio/mpeg" {
		t.Fatalf("accept=%q", gotAccept)
	}
	if !strings.Contains(resp.Header.Get("Content-Type"), "audio/ogg") {
		t.Fatalf("content-type=%q", resp.Header.Get("Content-Type"))
	}
}
