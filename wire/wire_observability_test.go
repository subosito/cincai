package wire_test

import (
	"bytes"
	"io"
	"log/slog"
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
	"github.com/subosito/cincai/observability"
	"github.com/subosito/cincai/passthrough"
	"github.com/subosito/cincai/upstream"
	"github.com/subosito/cincai/wire"
)

func testEngine(t *testing.T) (*wire.Engine, string) {
	t.Helper()
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, `{"choices":[]}`)
	}))
	t.Cleanup(up.Close)

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
            model: echo
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
	_, _ = st.PutAPIKey(t.Context(), "mock", "sk-up")
	reg := adaptersdk.NewRegistry()
	_ = passthrough.New().Register(reg)
	ks := keyring.NewMemoryStore()
	secret, _, _ := ks.Create(t.Context(), "client", keyring.KindStatic, 0, nil)
	return &wire.Engine{
		Catalog: cat, Store: st, Adapters: reg,
		Auth: &keyring.Authenticator{Store: ks}, Client: upstream.NewClient(),
	}, secret
}

func captureObservability(t *testing.T) *bytes.Buffer {
	t.Helper()
	var buf bytes.Buffer
	observability.SetTestLogger(slog.New(slog.NewJSONHandler(&buf, nil)))
	t.Cleanup(func() { observability.SetTestLogger(slog.Default()) })
	return &buf
}

func TestRootEmitsIngressLog(t *testing.T) {
	engine, _ := testEngine(t)
	buf := captureObservability(t)
	ts := httptest.NewServer(engine.Handler())
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("status=%d", resp.StatusCode)
	}
	out := buf.String()
	if !strings.Contains(out, `"wire":"root"`) && !strings.Contains(out, "root") {
		t.Fatalf("missing root ingress log: %s", out)
	}
}

func TestHealthzEmitsIngressLog(t *testing.T) {
	engine, _ := testEngine(t)
	buf := captureObservability(t)
	ts := httptest.NewServer(engine.Handler())
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/v1/healthz")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("status=%d", resp.StatusCode)
	}
	out := buf.String()
	if !strings.Contains(out, `"wire":"healthz"`) && !strings.Contains(out, "healthz") {
		t.Fatalf("missing healthz ingress log: %s", out)
	}
}

func TestUnauthorizedEmitsIngressLog(t *testing.T) {
	engine, _ := testEngine(t)
	buf := captureObservability(t)
	ts := httptest.NewServer(engine.Handler())
	defer ts.Close()

	req, _ := http.NewRequest(http.MethodPost, ts.URL+"/v1/chat/completions", strings.NewReader(`{"model":"mock-chat"}`))
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != 401 {
		t.Fatalf("status=%d", resp.StatusCode)
	}
	out := buf.String()
	if !strings.Contains(out, `"status":401`) && !strings.Contains(out, "status") {
		t.Fatalf("missing 401 ingress log: %s", out)
	}
}

func TestNotFoundEmitsIngressLog(t *testing.T) {
	engine, _ := testEngine(t)
	buf := captureObservability(t)
	ts := httptest.NewServer(engine.Handler())
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/v1/unknown-route")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != 404 {
		t.Fatalf("status=%d", resp.StatusCode)
	}
	out := buf.String()
	if !strings.Contains(out, "/v1/unknown-route") && !strings.Contains(out, `"status":404`) {
		t.Fatalf("missing 404 ingress log: %s", out)
	}
}

func TestBadModelEmitsIngressLog(t *testing.T) {
	engine, secret := testEngine(t)
	buf := captureObservability(t)
	ts := httptest.NewServer(engine.Handler())
	defer ts.Close()

	req, _ := http.NewRequest(http.MethodPost, ts.URL+"/v1/chat/completions", strings.NewReader(`{"model":"unknown-model"}`))
	req.Header.Set("Authorization", "Bearer "+secret)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != 400 {
		t.Fatalf("status=%d", resp.StatusCode)
	}
	out := buf.String()
	if !strings.Contains(out, `"status":400`) && !strings.Contains(out, "status") {
		t.Fatalf("missing 400 ingress log: %s", out)
	}
}
