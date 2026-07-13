package wire_test

import (
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/subosito/cincai/adaptersdk"
	"github.com/subosito/cincai/passthrough"
	"github.com/subosito/cincai/catalog"
	"github.com/subosito/cincai/internal/testfixture"
	"github.com/subosito/cincai/credential/seal"
	"github.com/subosito/cincai/credential/store"
	"github.com/subosito/cincai/ingress/keyring"
	"github.com/subosito/cincai/upstream"
	"github.com/subosito/cincai/wire"
)

func TestChatForwardInjectsUpstreamStripsClient(t *testing.T) {
	var gotAuth string
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		if r.Header.Get("x-api-key") != "" {
			t.Fatal("client x-api-key should be stripped")
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"model":"echo-model","choices":[{"message":{"content":"hi"}}]}`)
	}))
	defer up.Close()

	providers := `
providers:
  mock:
    credential_profile: mock
    surfaces:
      chat:
        protocol: openai-chat-completions
        base_url: ` + up.URL + `
models:
  mock-chat:
    modalities:
      chat:
        wire: openai-chat-completions
        providers:
          - provider_ref: mock
            surface: chat
            model: echo-model
`
	p := filepath.Join(t.TempDir(), "providers.yaml")
	if err := os.WriteFile(p, []byte(providers), 0o600); err != nil {
		t.Fatal(err)
	}
	cat, err := catalog.Load(p)
	if err != nil {
		t.Fatal(err)
	}
	key, _ := seal.ParseKey("FFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFF=")
	st := store.NewMemory(key)
	_, _ = st.PutAPIKey(t.Context(), "mock", "sk-upstream-secret")

	reg := adaptersdk.NewRegistry()
	_ = passthrough.New().Register(reg)

	ks := keyring.NewMemoryStore()
	secret, _, _ := ks.Create(t.Context(), "client", keyring.KindStatic, 0, nil)

	engine := &wire.Engine{
		Catalog: cat, Store: st, Adapters: reg,
		Auth: &keyring.Authenticator{Store: ks},
		Client: upstream.NewClient(),
	}
	ts := httptest.NewServer(engine.Handler())
	defer ts.Close()

	body := `{"model":"mock-chat","messages":[{"role":"user","content":"hi"}]}`
	req, _ := http.NewRequest(http.MethodPost, ts.URL+"/v1/chat/completions", strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+secret)
	req.Header.Set("x-api-key", "client-should-strip")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("status=%d %s", resp.StatusCode, b)
	}
	if gotAuth != "Bearer sk-upstream-secret" {
		t.Fatalf("upstream auth=%q", gotAuth)
	}
}

func TestResponsesForward(t *testing.T) {
	var gotPath, gotAuth string
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotAuth = r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"id":"resp_1","output":[{"type":"message","content":"pong"}]}`)
	}))
	defer up.Close()

	providers := `
providers:
  openai:
    credential_profile: openai
    surfaces:
      responses:
        protocol: openai-responses
        base_url: ` + up.URL + `
models:
  gpt-responses:
    modalities:
      chat:
        wire: openai-responses
        providers:
          - provider_ref: openai
            surface: responses
            model: gpt-5.5
`
	p := filepath.Join(t.TempDir(), "providers.yaml")
	if err := os.WriteFile(p, []byte(providers), 0o600); err != nil {
		t.Fatal(err)
	}
	cat, err := catalog.Load(p)
	if err != nil {
		t.Fatal(err)
	}
	key, _ := seal.ParseKey("FFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFF=")
	st := store.NewMemory(key)
	_, _ = st.PutAPIKey(t.Context(), "openai", "sk-upstream")

	reg := adaptersdk.NewRegistry()
	_ = passthrough.New().Register(reg)
	ks := keyring.NewMemoryStore()
	secret, _, _ := ks.Create(t.Context(), "client", keyring.KindStatic, 0, nil)

	engine := &wire.Engine{
		Catalog: cat, Store: st, Adapters: reg,
		Auth: &keyring.Authenticator{Store: ks},
		Client: upstream.NewClient(),
	}
	ts := httptest.NewServer(engine.Handler())
	defer ts.Close()

	body := `{"model":"gpt-responses","input":"hi"}`
	req, _ := http.NewRequest(http.MethodPost, ts.URL+"/v1/responses", strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+secret)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("status=%d %s", resp.StatusCode, b)
	}
	if gotPath != "/v1/responses" {
		t.Fatalf("path=%q", gotPath)
	}
	if gotAuth != "Bearer sk-upstream" {
		t.Fatalf("auth=%q", gotAuth)
	}
}

func TestFailoverRetriesOnUpstreamError(t *testing.T) {
	var hits []string
	primary := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits = append(hits, "primary")
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer primary.Close()
	backup := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits = append(hits, "backup")
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"model":"mb","choices":[{"message":{"content":"ok"}}]}`)
	}))
	defer backup.Close()

	providers := `
providers:
  primary:
    credential_profile: primary
    surfaces:
      chat:
        protocol: openai-chat-completions
        base_url: ` + primary.URL + `
  backup:
    credential_profile: backup
    surfaces:
      chat:
        protocol: openai-chat-completions
        base_url: ` + backup.URL + `
models:
  m:
    modalities:
      chat:
        wire: openai-chat-completions
        strategy: failover
        providers:
          - provider_ref: primary
            surface: chat
            model: mp
          - provider_ref: backup
            surface: chat
            model: mb
`
	p := filepath.Join(t.TempDir(), "providers.yaml")
	if err := os.WriteFile(p, []byte(providers), 0o600); err != nil {
		t.Fatal(err)
	}
	cat, err := catalog.Load(p)
	if err != nil {
		t.Fatal(err)
	}
	key, _ := seal.ParseKey("FFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFF=")
	st := store.NewMemory(key)
	_, _ = st.PutAPIKey(t.Context(), "primary", "sk-primary")
	_, _ = st.PutAPIKey(t.Context(), "backup", "sk-backup")

	reg := adaptersdk.NewRegistry()
	_ = passthrough.New().Register(reg)
	ks := keyring.NewMemoryStore()
	secret, _, _ := ks.Create(t.Context(), "client", keyring.KindStatic, 0, nil)

	engine := &wire.Engine{
		Catalog: cat, Store: st, Adapters: reg,
		Auth:   &keyring.Authenticator{Store: ks},
		Client: upstream.NewClient(),
	}
	ts := httptest.NewServer(engine.Handler())
	defer ts.Close()

	body := `{"model":"m","messages":[{"role":"user","content":"hi"}]}`
	req, _ := http.NewRequest(http.MethodPost, ts.URL+"/v1/chat/completions", strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+secret)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("status=%d %s", resp.StatusCode, b)
	}
	if len(hits) != 2 || hits[0] != "primary" || hits[1] != "backup" {
		t.Fatalf("hits=%v", hits)
	}
}

func TestModelsListRequiresAuth(t *testing.T) {
	engine := testModelsEngine(t)
	ts := httptest.NewServer(engine.Handler())
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/v1/models")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("status=%d", resp.StatusCode)
	}
}

func TestModelsListReturnsCatalog(t *testing.T) {
	engine, secret := testModelsEngineWithKey(t)
	ts := httptest.NewServer(engine.Handler())
	defer ts.Close()

	req, _ := http.NewRequest(http.MethodGet, ts.URL+"/v1/models", nil)
	req.Header.Set("Authorization", "Bearer "+secret)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("status=%d %s", resp.StatusCode, b)
	}
	body, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(body), "mock-chat") {
		t.Fatalf("body=%s", body)
	}
	if !strings.Contains(string(body), `"object":"list"`) {
		t.Fatalf("body=%s", body)
	}
}

func testModelsEngine(t *testing.T) *wire.Engine {
	t.Helper()
	engine, _ := testModelsEngineWithKey(t)
	return engine
}

func testModelsEngineWithKey(t *testing.T) (*wire.Engine, string) {
	t.Helper()
	p := testfixture.ProvidersYAML()
	cat, err := catalog.Load(p)
	if err != nil {
		t.Fatal(err)
	}
	key, _ := seal.ParseKey("FFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFF=")
	st := store.NewMemory(key)
	reg := adaptersdk.NewRegistry()
	_ = passthrough.New().Register(reg)
	ks := keyring.NewMemoryStore()
	secret, _, _ := ks.Create(t.Context(), "client", keyring.KindStatic, 0, nil)
	return &wire.Engine{
		Catalog: cat, Store: st, Adapters: reg,
		Auth:   &keyring.Authenticator{Store: ks},
		Client: upstream.NewClient(),
	}, secret
}