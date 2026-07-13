package wire_test

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
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

// TestStreamUsage_injectMeterStrip drives a streaming chat request that does NOT
// ask for usage, and asserts: the gateway injects include_usage upstream, the
// injected usage frame is stripped from the client stream, and usage still lands
// on the ingress record.
func TestStreamUsage_injectMeterStrip(t *testing.T) {
	var mu sync.Mutex
	var upstreamBody []byte
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		mu.Lock()
		upstreamBody = b
		mu.Unlock()
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = io.WriteString(w, "data: {\"choices\":[{\"delta\":{\"content\":\"hi\"}}]}\n\n")
		_, _ = io.WriteString(w, "data: {\"choices\":[],\"usage\":{\"prompt_tokens\":5,\"completion_tokens\":7}}\n\n")
		_, _ = io.WriteString(w, "data: [DONE]\n\n")
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
	_, _ = st.PutAPIKey(t.Context(), "mock", "sk-x")

	reg := adaptersdk.NewRegistry()
	_ = passthrough.New().Register(reg)
	ks := keyring.NewMemoryStore()
	secret, _, _ := ks.Create(t.Context(), "client", keyring.KindStatic, 0, nil)

	engine := &wire.Engine{
		Catalog: cat, Store: st, Adapters: reg,
		Auth:   &keyring.Authenticator{Store: ks},
		Client: upstream.NewClient(),
	}
	buf := captureObservability(t)
	ts := httptest.NewServer(engine.Handler())
	defer ts.Close()

	// Client streams but does NOT ask for usage.
	body := `{"model":"mock-chat","stream":true,"messages":[{"role":"user","content":"hi"}]}`
	req, _ := http.NewRequest(http.MethodPost, ts.URL+"/v1/chat/completions", strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+secret)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	clientBody, _ := io.ReadAll(resp.Body)
	resp.Body.Close()

	// 1. Injection reached the upstream.
	mu.Lock()
	sawInclude := bytes.Contains(upstreamBody, []byte(`"include_usage":true`))
	mu.Unlock()
	if !sawInclude {
		t.Fatalf("upstream request lacked injected include_usage: %s", upstreamBody)
	}
	// 2. Client got the content but NOT the injected usage frame.
	if !strings.Contains(string(clientBody), `"delta"`) {
		t.Fatalf("client missing content frame: %s", clientBody)
	}
	if strings.Contains(string(clientBody), `"usage"`) {
		t.Fatalf("client stream should not contain the injected usage frame: %s", clientBody)
	}
	// 3. Usage still landed on the ingress record.
	var found bool
	for _, line := range strings.Split(strings.TrimSpace(buf.String()), "\n") {
		if !strings.Contains(line, `"ingress"`) {
			continue
		}
		var entry struct {
			Record struct {
				Model string `json:"model"`
				Usage *struct {
					InputTokens  int `json:"input_tokens"`
					OutputTokens int `json:"output_tokens"`
				} `json:"usage"`
			} `json:"record"`
		}
		if err := json.Unmarshal([]byte(line), &entry); err != nil || entry.Record.Model != "mock-chat" {
			continue
		}
		found = true
		if entry.Record.Usage == nil || entry.Record.Usage.InputTokens != 5 || entry.Record.Usage.OutputTokens != 7 {
			t.Fatalf("ingress usage = %+v, want in=5 out=7", entry.Record.Usage)
		}
	}
	if !found {
		t.Fatalf("no ingress record for mock-chat in:\n%s", buf.String())
	}
}
