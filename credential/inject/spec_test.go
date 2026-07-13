package inject_test

import (
	"net/http"
	"testing"

	"github.com/subosito/cincai/credential/inject"
	"github.com/subosito/cincai/credential/store"
)

func TestApplySpecMultiHeaderOAuth(t *testing.T) {
	req, _ := http.NewRequest(http.MethodPost, "http://example.com", nil)
	spec := inject.Spec{
		"authorization": "Bearer ${access}",
		"x-account-id": "${accountId}",
	}
	mat := store.Material{
		Kind:        store.KindOAuth,
		AccessToken: "oat-token",
		Extras:      map[string]string{"account_id": "acct-99"},
	}
	if err := inject.ApplySpec(mat, req, spec); err != nil {
		t.Fatal(err)
	}
	if got := req.Header.Get("Authorization"); got != "Bearer oat-token" {
		t.Fatalf("authorization=%q", got)
	}
	if got := req.Header.Get("x-account-id"); got != "acct-99" {
		t.Fatalf("account=%q", got)
	}
}

func TestApplyRouteInjectMapWinsOverPreset(t *testing.T) {
	req, _ := http.NewRequest(http.MethodPost, "http://example.com", nil)
	req.Header.Set("Authorization", "Bearer client")
	route := inject.Route{
		Spec:   inject.Spec{"x-vendor-api-key": "${key}"},
		Preset: "bearer",
	}
	mat := store.Material{Kind: store.KindAPIKey, APIKey: "sk-test"}
	if err := inject.ApplyRoute(mat, req, route, inject.AdapterDefault{}); err != nil {
		t.Fatal(err)
	}
	if got := req.Header.Get("x-vendor-api-key"); got != "sk-test" {
		t.Fatalf("x-vendor-api-key=%q", got)
	}
	if req.Header.Get("Authorization") != "" {
		t.Fatal("authorization should be empty")
	}
}

func TestApplyRouteAdapterDefaultSpec(t *testing.T) {
	req, _ := http.NewRequest(http.MethodPost, "http://example.com", nil)
	mat := store.Material{Kind: store.KindAPIKey, APIKey: "k"}
	def := inject.AdapterDefault{Spec: inject.Spec{"x-vendor-api-key": "${key}"}}
	if err := inject.ApplyRoute(mat, req, inject.Route{}, def); err != nil {
		t.Fatal(err)
	}
	if got := req.Header.Get("x-vendor-api-key"); got != "k" {
		t.Fatalf("x-vendor-api-key=%q", got)
	}
}