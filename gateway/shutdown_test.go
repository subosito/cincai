package gateway_test

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/subosito/cincai/adaptersdk"
	"github.com/subosito/cincai/catalog"
	"github.com/subosito/cincai/credential/seal"
	"github.com/subosito/cincai/credential/store"
	"github.com/subosito/cincai/gateway"
	"github.com/subosito/cincai/ingress/keyring"
	"github.com/subosito/cincai/passthrough"
)

func TestShutdownDrainsInFlightSSE(t *testing.T) {
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		fl, _ := w.(http.Flusher)
		for i := 0; i < 5; i++ {
			_, _ = io.WriteString(w, "data: chunk\n\n")
			fl.Flush()
			time.Sleep(100 * time.Millisecond)
		}
	}))
	defer up.Close()

	cat := mustCatalog(t, up.URL)
	key, _ := seal.ParseKey("AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA=")
	st := store.NewMemory(key)
	_, _ = st.PutAPIKey(t.Context(), "mock", "sk-up")
	reg := adaptersdk.NewRegistry()
	_ = passthrough.New().Register(reg)
	ks := keyring.NewMemoryStore()
	secret, _, _ := ks.Create(t.Context(), "gw", keyring.KindStatic, 0, nil)

	gw, err := gateway.New(gateway.Config{
		Catalog: cat, Store: st, KeyStore: ks, Adapters: reg,
		DataListen: "127.0.0.1:0",
	})
	if err != nil {
		t.Fatal(err)
	}

	serveDone := make(chan error, 1)
	go func() { serveDone <- gw.ListenAndServe(context.Background()) }()

	var dataAddr string
	for i := 0; i < 100; i++ {
		dataAddr = gw.DataAddr()
		if dataAddr != "" {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if dataAddr == "" {
		t.Fatal("gateway data listener not ready")
	}

	body := `{"model":"mock-chat","messages":[{"role":"user","content":"hi"}],"stream":true}`
	req, _ := http.NewRequest(http.MethodPost, "http://"+dataAddr+"/v1/chat/completions", strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+secret)

	readDone := make(chan int, 1)
	go func() {
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			readDone <- -1
			return
		}
		defer resp.Body.Close()
		buf := make([]byte, 64)
		n, _ := io.ReadAtLeast(resp.Body, buf, 1)
		readDone <- n
	}()

	time.Sleep(150 * time.Millisecond)
	shutDone := make(chan error, 1)
	go func() { shutDone <- gw.Shutdown(context.Background()) }()

	select {
	case n := <-readDone:
		if n <= 0 {
			t.Fatal("expected SSE bytes before shutdown completed")
		}
	case <-time.After(10 * time.Second):
		t.Fatal("read timed out")
	}

	select {
	case err := <-shutDone:
		if err != nil {
			t.Fatalf("shutdown: %v", err)
		}
	case <-time.After(30 * time.Second):
		t.Fatal("shutdown timed out")
	}
}

func mustCatalog(t *testing.T, base string) *catalog.Catalog {
	t.Helper()
	raw := `
providers:
  mock:
    credential_profile: mock
    surfaces:
      chat:
        protocol: openai-chat-completions
        base_url: ` + base + `
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
	p := filepath.Join(t.TempDir(), "p.yaml")
	if err := os.WriteFile(p, []byte(raw), 0o600); err != nil {
		t.Fatal(err)
	}
	cat, err := catalog.Load(p)
	if err != nil {
		t.Fatal(err)
	}
	return cat
}
