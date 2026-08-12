package xai_test

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

	"github.com/subosito/cincai/internal/adapters/xai"
)

func TestImageHandlerEditMultiImages(t *testing.T) {
	t.Parallel()
	var gotBody map[string]any
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(raw, &gotBody)
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"data":[{"b64_json":"aGk="}]}`)
	}))
	defer up.Close()

	h := &xai.ImageHandler{}
	// Dududu multi-edit sends OpenAI-compat "images": [b64, b64]
	body := `{"model":"grok-imagine-image-quality","prompt":"compose flyer","images":["aaa","bbb"]}`
	resp, err := h.Forward(context.Background(), http.DefaultClient, handler.Target{
		Target: catalog.Target{
			BaseURL:       up.URL + "/v1/images",
			UpstreamModel: "grok-imagine-image-quality",
		},
		Material: store.Material{Kind: store.KindAPIKey, APIKey: "xai-secret"},
	}, "/v1/images/edits", strings.NewReader(body), http.Header{"Content-Type": []string{"application/json"}})
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("status=%d", resp.StatusCode)
	}
	imgs, ok := gotBody["images"].([]any)
	if !ok || len(imgs) != 2 {
		t.Fatalf("want images[2], got %#v", gotBody)
	}
	first, _ := imgs[0].(map[string]any)
	if first["type"] != "image_url" {
		t.Fatalf("item0=%#v", first)
	}
	url, _ := first["url"].(string)
	if !strings.HasPrefix(url, "data:image/png;base64,") {
		t.Fatalf("url=%q", url)
	}
	if _, has := gotBody["image"]; has {
		t.Fatalf("single image field should be omitted for multi: %#v", gotBody)
	}
}

func TestImageHandlerEditSingleImageStillWorks(t *testing.T) {
	t.Parallel()
	var gotBody map[string]any
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(raw, &gotBody)
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"data":[{"b64_json":"aGk="}]}`)
	}))
	defer up.Close()

	h := &xai.ImageHandler{}
	body := `{"model":"grok-imagine-image-quality","prompt":"sketch","image":"abc123"}`
	resp, err := h.Forward(context.Background(), http.DefaultClient, handler.Target{
		Target: catalog.Target{
			BaseURL:       up.URL + "/v1/images",
			UpstreamModel: "grok-imagine-image-quality",
		},
		Material: store.Material{Kind: store.KindAPIKey, APIKey: "xai-secret"},
	}, "/v1/images/edits", strings.NewReader(body), http.Header{"Content-Type": []string{"application/json"}})
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	img, ok := gotBody["image"].(map[string]any)
	if !ok || img["type"] != "image_url" {
		t.Fatalf("image=%#v", gotBody["image"])
	}
}

func TestImageHandlerForwardsXAIPathAndBody(t *testing.T) {
	t.Parallel()
	var gotPath, gotAuth string
	var gotBody map[string]any
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotAuth = r.Header.Get("Authorization")
		raw, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(raw, &gotBody)
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"data":[{"b64_json":"aGk="}]}`)
	}))
	defer up.Close()

	h := &xai.ImageHandler{}
	body := `{"model":"grok-imagine-image-quality","prompt":"red circle","n":1}`
	ingress := http.Header{}
	ingress.Set("Content-Type", "application/json")
	ingress.Set("Content-Length", "9999")

	resp, err := h.Forward(context.Background(), http.DefaultClient, handler.Target{
		Target: catalog.Target{
			BaseURL:       up.URL + "/v1/images",
			UpstreamModel: "grok-imagine-image-quality",
		},
		Material: store.Material{Kind: store.KindAPIKey, APIKey: "xai-secret"},
	}, "/v1/images/generations", strings.NewReader(body), ingress)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("status=%d", resp.StatusCode)
	}
	if gotPath != "/v1/images/generations" {
		t.Fatalf("path=%q", gotPath)
	}
	if gotAuth != "Bearer xai-secret" {
		t.Fatalf("auth=%q", gotAuth)
	}
	if gotBody["response_format"] != "b64_json" {
		t.Fatalf("body=%v", gotBody)
	}
}
