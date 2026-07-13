package wire_test

import (
	"bytes"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/subosito/cincai/adaptersdk"
	"github.com/subosito/cincai/passthrough"
	"github.com/subosito/cincai/catalog"
	"github.com/subosito/cincai/credential/seal"
	"github.com/subosito/cincai/credential/store"
	"github.com/subosito/cincai/ingress/keyring"
	"github.com/subosito/cincai/upstream"
	"github.com/subosito/cincai/wire"
)

func TestImageGenerationsForward(t *testing.T) {
	var gotPath, gotAuth string
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotAuth = r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"data":[{"b64_json":"aGk="}]}`)
	}))
	defer up.Close()

	engine := mediaTestEngine(t, up.URL, `
providers:
  img:
    credential_profile: img
    surfaces:
      image_gen:
        protocol: openai-images
        base_url: `+up.URL+`
models:
  gpt-image-test:
    modalities:
      image:
        wire: openai-images-generations
        providers:
          - provider_ref: img
            model: gpt-image-2
`)
	ts := httptest.NewServer(engine.Handler())
	defer ts.Close()

	body := `{"model":"gpt-image-test","prompt":"a cat","n":1}`
	req, _ := http.NewRequest(http.MethodPost, ts.URL+"/v1/images/generations", strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+mediaTestSecret(t, engine))
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("status=%d %s", resp.StatusCode, b)
	}
	if gotPath != "/v1/images/generations" {
		t.Fatalf("path=%q", gotPath)
	}
	if gotAuth != "Bearer sk-img-upstream" {
		t.Fatalf("auth=%q", gotAuth)
	}
}

func TestImageEditsMultipartForward(t *testing.T) {
	var gotPath string
	var gotCT string
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotCT = r.Header.Get("Content-Type")
		w.WriteHeader(http.StatusOK)
	}))
	defer up.Close()

	engine := mediaTestEngine(t, up.URL, `
providers:
  img:
    credential_profile: img
    surfaces:
      image_gen:
        protocol: openai-images
        base_url: `+up.URL+`
models:
  edit-model:
    modalities:
      image:
        wire: openai-images-generations
        providers:
          - provider_ref: img
            model: gpt-image-1
`)
	ts := httptest.NewServer(engine.Handler())
	defer ts.Close()

	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	_ = w.WriteField("model", "edit-model")
	_ = w.WriteField("prompt", "add hat")
	fw, _ := w.CreateFormFile("image", "photo.png")
	_, _ = io.WriteString(fw, "pngbytes")
	_ = w.Close()

	req, _ := http.NewRequest(http.MethodPost, ts.URL+"/v1/images/edits", &buf)
	req.Header.Set("Content-Type", w.FormDataContentType())
	req.Header.Set("Authorization", "Bearer "+mediaTestSecret(t, engine))
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("status=%d %s", resp.StatusCode, b)
	}
	if gotPath != "/v1/images/edits" {
		t.Fatalf("path=%q", gotPath)
	}
	if !strings.HasPrefix(gotCT, "multipart/") {
		t.Fatalf("content-type=%q", gotCT)
	}
}

func TestTranscriptionForward(t *testing.T) {
	var gotPath string
	var gotCT string
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotCT = r.Header.Get("Content-Type")
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"text":"hello du"}`)
	}))
	defer up.Close()

	engine := mediaTestEngine(t, up.URL, `
providers:
  stt:
    credential_profile: stt
    surfaces:
      voice:
        protocol: openai-transcriptions
        base_url: `+up.URL+`
models:
  whisper-test:
    modalities:
      voice:
        wire: openai-audio-transcriptions
        providers:
          - provider_ref: stt
            model: whisper-large-v3-turbo
`)
	ts := httptest.NewServer(engine.Handler())
	defer ts.Close()

	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	_ = w.WriteField("model", "whisper-test")
	fw, _ := w.CreateFormFile("file", "note.ogg")
	_, _ = io.WriteString(fw, "oggbytes")
	_ = w.Close()

	req, _ := http.NewRequest(http.MethodPost, ts.URL+"/v1/audio/transcriptions", &buf)
	req.Header.Set("Content-Type", w.FormDataContentType())
	req.Header.Set("Authorization", "Bearer "+mediaTestSecret(t, engine))
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("status=%d %s", resp.StatusCode, b)
	}
	if gotPath != "/v1/audio/transcriptions" {
		t.Fatalf("path=%q", gotPath)
	}
	if !strings.HasPrefix(gotCT, "multipart/") {
		t.Fatalf("content-type=%q", gotCT)
	}
}

func TestSpeechForward(t *testing.T) {
	var gotPath string
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		w.Header().Set("Content-Type", "audio/mpeg")
		_, _ = io.WriteString(w, "audio")
	}))
	defer up.Close()

	engine := mediaTestEngine(t, up.URL, `
providers:
  tts:
    credential_profile: tts
    surfaces:
      speech_gen:
        protocol: openai-tts
        base_url: `+up.URL+`
models:
  tts-1:
    modalities:
      speech:
        wire: openai-audio-speech
        providers:
          - provider_ref: tts
            model: tts-1-hd
`)
	ts := httptest.NewServer(engine.Handler())
	defer ts.Close()

	body := `{"model":"tts-1","input":"hello","voice":"alloy"}`
	req, _ := http.NewRequest(http.MethodPost, ts.URL+"/v1/audio/speech", strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+mediaTestSecret(t, engine))
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("status=%d %s", resp.StatusCode, b)
	}
	if gotPath != "/v1/audio/speech" {
		t.Fatalf("path=%q", gotPath)
	}
}

func TestVideoSubmitAndPoll(t *testing.T) {
	var paths []string
	var methods []string
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		paths = append(paths, r.URL.Path)
		methods = append(methods, r.Method)
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"request_id":"vid_1","status":"pending"}`)
	}))
	defer up.Close()

	engine := mediaTestEngine(t, up.URL, `
providers:
  vid:
    credential_profile: vid
    surfaces:
      video_gen:
        protocol: openai-videos
        base_url: `+up.URL+`
models:
  demo-video:
    modalities:
      video:
        wire: openai-videos
        providers:
          - provider_ref: vid
            model: demo-video-1
`)
	ts := httptest.NewServer(engine.Handler())
	defer ts.Close()
	secret := mediaTestSecret(t, engine)

	submitBody := `{"model":"demo-video","prompt":"waves"}`
	req, _ := http.NewRequest(http.MethodPost, ts.URL+"/v1/videos/generations", strings.NewReader(submitBody))
	req.Header.Set("Authorization", "Bearer "+secret)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("submit status=%d", resp.StatusCode)
	}

	pollReq, _ := http.NewRequest(http.MethodGet, ts.URL+"/v1/videos/vid_1?model=demo-video", nil)
	pollReq.Header.Set("Authorization", "Bearer "+secret)
	pollResp, err := http.DefaultClient.Do(pollReq)
	if err != nil {
		t.Fatal(err)
	}
	pollResp.Body.Close()
	if pollResp.StatusCode != 200 {
		t.Fatalf("poll status=%d", pollResp.StatusCode)
	}
	if len(paths) != 2 {
		t.Fatalf("paths=%v", paths)
	}
	if paths[0] != "/v1/videos/generations" || paths[1] != "/v1/videos/vid_1" {
		t.Fatalf("paths=%v", paths)
	}
	if methods[0] != http.MethodPost || methods[1] != http.MethodGet {
		t.Fatalf("methods=%v", methods)
	}
}

func mediaTestEngine(t *testing.T, _ string, providersYAML string) *wire.Engine {
	t.Helper()
	p := filepath.Join(t.TempDir(), "providers.yaml")
	if err := os.WriteFile(p, []byte(providersYAML), 0o600); err != nil {
		t.Fatal(err)
	}
	cat, err := catalog.Load(p)
	if err != nil {
		t.Fatal(err)
	}
	key, _ := seal.ParseKey("FFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFF=")
	st := store.NewMemory(key)
	for _, prof := range []struct{ name, secret string }{
		{"img", "sk-img-upstream"},
		{"stt", "sk-stt-upstream"},
		{"tts", "sk-tts-upstream"},
		{"vid", "sk-vid-upstream"},
	} {
		_, _ = st.PutAPIKey(t.Context(), prof.name, prof.secret)
	}
	reg := adaptersdk.NewRegistry()
	_ = passthrough.New().Register(reg)
	ks := keyring.NewMemoryStore()
	return &wire.Engine{
		Catalog: cat, Store: st, Adapters: reg,
		Auth:   &keyring.Authenticator{Store: ks},
		Client: upstream.NewClient(),
	}
}

func mediaTestSecret(t *testing.T, engine *wire.Engine) string {
	t.Helper()
	ks := engine.Auth.Store.(*keyring.MemoryStore)
	secret, _, _ := ks.Create(t.Context(), "client", keyring.KindStatic, 0, nil)
	return secret
}