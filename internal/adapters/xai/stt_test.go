package xai

import (
	"bytes"
	"context"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/subosito/cincai/adaptersdk/handler"
	"github.com/subosito/cincai/catalog"
	"github.com/subosito/cincai/credential/store"
)

func TestTranscriptionForward_mapsToXAIStt(t *testing.T) {
	var gotPath, gotCT, gotAuth string
	var gotBody []byte
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotCT = r.Header.Get("Content-Type")
		gotAuth = r.Header.Get("Authorization")
		gotBody, _ = io.ReadAll(r.Body)
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"text":"hello id","language":"id","duration":1.2}`)
	}))
	defer up.Close()

	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	_ = mw.WriteField("model", "grok-stt")
	_ = mw.WriteField("language", "id")
	fw, _ := mw.CreateFormFile("file", "note.opus")
	_, _ = io.WriteString(fw, "opus-bytes")
	_ = mw.Close()

	h := &TranscriptionHandler{}
	resp, err := h.Forward(context.Background(), http.DefaultClient, handler.Target{
		Target: catalog.Target{
			BaseURL:       up.URL + "/v1",
			UpstreamModel: "grok-stt",
		},
		Material: store.Material{Kind: store.KindAPIKey, APIKey: "sk-test"},
	}, &buf, http.Header{"Content-Type": []string{mw.FormDataContentType()}})
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("status=%d %s", resp.StatusCode, b)
	}
	if gotPath != "/v1/stt" {
		t.Fatalf("path=%q want /v1/stt", gotPath)
	}
	if !strings.HasPrefix(gotCT, "multipart/") {
		t.Fatalf("content-type=%q", gotCT)
	}
	if gotAuth != "Bearer sk-test" {
		t.Fatalf("auth=%q", gotAuth)
	}
	// language + format before file; file present
	body := string(gotBody)
	if !strings.Contains(body, `name="language"`) || !strings.Contains(body, "id") {
		t.Fatalf("missing language: %s", body[:min(200, len(body))])
	}
	if !strings.Contains(body, `name="format"`) {
		t.Fatalf("missing format: %s", body[:min(200, len(body))])
	}
	if !strings.Contains(body, `name="file"`) || !strings.Contains(body, "opus-bytes") {
		t.Fatalf("missing file: %s", body[:min(200, len(body))])
	}
	out, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(out), `"text":"hello id"`) {
		t.Fatalf("body=%s", out)
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
