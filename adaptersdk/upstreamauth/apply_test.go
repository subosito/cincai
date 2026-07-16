package upstreamauth_test

import (
	"bytes"
	"net/http"
	"testing"

	"github.com/subosito/cincai/adaptersdk/handler"
	"github.com/subosito/cincai/adaptersdk/upstreamauth"
	"github.com/subosito/cincai/catalog"
	"github.com/subosito/cincai/credential/inject"
	"github.com/subosito/cincai/credential/store"
)

func TestBearerDefaultPreset(t *testing.T) {
	d := upstreamauth.BearerDefault()
	if d.Preset != upstreamauth.PresetBearer {
		t.Fatalf("preset=%q", d.Preset)
	}
	if len(d.Spec) != 0 {
		t.Fatalf("spec=%v", d.Spec)
	}
}

func TestApplyTranslatedStripsIngressBodyHeaders(t *testing.T) {
	body := []byte(`{"model":"grok-imagine-image-quality","prompt":"dot"}`)
	req, err := http.NewRequest(http.MethodPost, "https://api.x.ai/v1/images/generations", bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	ingress := http.Header{}
	ingress.Set("Content-Type", "application/json")
	ingress.Set("Content-Length", "9999")
	ingress.Set("X-Request-Id", "trace-1")

	mat := store.Material{Kind: store.KindOAuth, AccessToken: "oat"}
	target := handler.Target{
		Target:   catalog.Target{},
		Material: mat,
	}
	if err := upstreamauth.ApplyTranslated(target, req, ingress, upstreamauth.BearerDefault()); err != nil {
		t.Fatal(err)
	}
	if got := req.Header.Get("Content-Type"); got != "application/json" {
		t.Fatalf("content-type=%q", got)
	}
	if vals := req.Header.Values("Content-Length"); len(vals) != 0 {
		t.Fatalf("content-length should be unset for transport, got %v", vals)
	}
	if req.ContentLength != int64(len(body)) {
		t.Fatalf("content-length field=%d want %d", req.ContentLength, len(body))
	}
	if req.Header.Get("X-Request-Id") != "trace-1" {
		t.Fatalf("trace header dropped")
	}
	if req.Header.Get("Authorization") != "Bearer oat" {
		t.Fatalf("auth=%q", req.Header.Get("Authorization"))
	}
}

func TestCatalogInjectPresetRejectsXiAPIKey(t *testing.T) {
	req, _ := http.NewRequest(http.MethodPost, "http://example.com", nil)
	mat := store.Material{Kind: store.KindAPIKey, APIKey: "k"}
	err := inject.ApplyRoute(mat, req, inject.Route{Preset: "xi-api-key"}, inject.AdapterDefault{})
	if err == nil {
		t.Fatal("expected error for xi-api-key inject_preset")
	}
}
