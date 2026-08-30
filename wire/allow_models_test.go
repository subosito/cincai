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
	"github.com/subosito/cincai/catalog"
	"github.com/subosito/cincai/credential/seal"
	"github.com/subosito/cincai/credential/store"
	"github.com/subosito/cincai/ingress/keyring"
	"github.com/subosito/cincai/passthrough"
	"github.com/subosito/cincai/upstream"
	"github.com/subosito/cincai/wire"
)

func TestAllowModelsForward(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name       string
		allow      []string
		upstream   string
		wantStatus int
		wantHit    bool
		wantBody   string
	}{
		{
			name:       "empty allowlist forwards",
			allow:      nil,
			upstream:   "echo-model",
			wantStatus: http.StatusOK,
			wantHit:    true,
		},
		{
			name:       "listed model forwards",
			allow:      []string{"echo-model"},
			upstream:   "echo-model",
			wantStatus: http.StatusOK,
			wantHit:    true,
		},
		{
			name:       "unlisted model rejected",
			allow:      []string{"allowed-only"},
			upstream:   "blocked-model",
			wantStatus: http.StatusForbidden,
			wantHit:    false,
			wantBody:   `provider "mock": upstream model "blocked-model" not in allow_models`,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			var upstreamHit bool
			up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				upstreamHit = true
				w.Header().Set("Content-Type", "application/json")
				_, _ = io.WriteString(w, `{"model":"`+tc.upstream+`","choices":[{"message":{"content":"hi"}}]}`)
			}))
			defer up.Close()

			allowYAML := ""
			if len(tc.allow) > 0 {
				allowYAML = "    allow_models:\n"
				for _, m := range tc.allow {
					allowYAML += "      - " + m + "\n"
				}
			}
			providers := `
providers:
  mock:
    credential_profile: mock
` + allowYAML + `    surfaces:
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
            model: ` + tc.upstream + `
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
				Auth:   &keyring.Authenticator{Store: ks},
				Client: upstream.NewClient(),
			}
			ts := httptest.NewServer(engine.Handler())
			defer ts.Close()

			body := `{"model":"mock-chat","messages":[{"role":"user","content":"hi"}]}`
			req, _ := http.NewRequest(http.MethodPost, ts.URL+"/v1/chat/completions", strings.NewReader(body))
			req.Header.Set("Authorization", "Bearer "+secret)
			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				t.Fatal(err)
			}
			defer resp.Body.Close()
			respBody, _ := io.ReadAll(resp.Body)
			if resp.StatusCode != tc.wantStatus {
				t.Fatalf("status=%d body=%s", resp.StatusCode, respBody)
			}
			if upstreamHit != tc.wantHit {
				t.Fatalf("upstreamHit=%v want %v", upstreamHit, tc.wantHit)
			}
			if tc.wantBody != "" && strings.TrimSpace(string(respBody)) != tc.wantBody {
				t.Fatalf("body=%q want %q", respBody, tc.wantBody)
			}
		})
	}
}

func TestAllowModelsEffortRewrite(t *testing.T) {
	t.Parallel()
	var gotModel string
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		_ = r.Body.Close()
		gotModel = string(b)
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"choices":[{"message":{"content":"ok"}}]}`)
	}))
	defer up.Close()

	providers := `
providers:
  mock:
    credential_profile: mock
    allow_models:
      - sku-high
    surfaces:
      chat:
        protocol: openai-chat-completions
        base_url: ` + up.URL + `
models:
  effort-model:
    efforts: [low, high]
    default_effort: low
    modalities:
      chat:
        wire: openai-chat-completions
        providers:
          - provider_ref: mock
            surface: chat
            models:
              low: sku-low
              high: sku-high
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
		Auth:   &keyring.Authenticator{Store: ks},
		Client: upstream.NewClient(),
	}
	ts := httptest.NewServer(engine.Handler())
	defer ts.Close()

	body := `{"model":"effort-model","reasoning_effort":"high","messages":[{"role":"user","content":"hi"}]}`
	req, _ := http.NewRequest(http.MethodPost, ts.URL+"/v1/chat/completions", strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+secret)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("status=%d body=%s", resp.StatusCode, b)
	}
	if !strings.Contains(gotModel, `"sku-high"`) {
		t.Fatalf("upstream body=%s want sku-high", gotModel)
	}

	gotModel = ""
	bodyLow := `{"model":"effort-model","reasoning_effort":"low","messages":[{"role":"user","content":"hi"}]}`
	reqLow, _ := http.NewRequest(http.MethodPost, ts.URL+"/v1/chat/completions", strings.NewReader(bodyLow))
	reqLow.Header.Set("Authorization", "Bearer "+secret)
	respLow, err := http.DefaultClient.Do(reqLow)
	if err != nil {
		t.Fatal(err)
	}
	defer respLow.Body.Close()
	if respLow.StatusCode != http.StatusForbidden {
		b, _ := io.ReadAll(respLow.Body)
		t.Fatalf("low status=%d body=%s", respLow.StatusCode, b)
	}
	if gotModel != "" {
		t.Fatalf("low effort should not reach upstream, got body=%s", gotModel)
	}
}

func TestAllowModelsFailover(t *testing.T) {
	t.Parallel()
	var hitPrimary, hitBackup bool
	primary := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hitPrimary = true
		w.WriteHeader(http.StatusOK)
		_, _ = io.WriteString(w, `{"choices":[{"message":{"content":"primary"}}]}`)
	}))
	defer primary.Close()
	backup := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hitBackup = true
		w.WriteHeader(http.StatusOK)
		_, _ = io.WriteString(w, `{"choices":[{"message":{"content":"backup"}}]}`)
	}))
	defer backup.Close()

	providers := `
providers:
  blocked:
    credential_profile: mock
    allow_models:
      - allowed-only
    surfaces:
      chat:
        protocol: openai-chat-completions
        base_url: ` + primary.URL + `
  open:
    credential_profile: mock
    surfaces:
      chat:
        protocol: openai-chat-completions
        base_url: ` + backup.URL + `
models:
  mock-chat:
    modalities:
      chat:
        wire: openai-chat-completions
        strategy: failover
        providers:
          - provider_ref: blocked
            surface: chat
            model: rejected-model
          - provider_ref: open
            surface: chat
            model: backup-model
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
		Auth:   &keyring.Authenticator{Store: ks},
		Client: upstream.NewClient(),
	}
	ts := httptest.NewServer(engine.Handler())
	defer ts.Close()

	body := `{"model":"mock-chat","messages":[{"role":"user","content":"hi"}]}`
	req, _ := http.NewRequest(http.MethodPost, ts.URL+"/v1/chat/completions", strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+secret)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("status=%d body=%s", resp.StatusCode, b)
	}
	if hitPrimary {
		t.Fatal("blocked provider should not receive request")
	}
	if !hitBackup {
		t.Fatal("backup provider should receive request after allowlist skip")
	}
}
