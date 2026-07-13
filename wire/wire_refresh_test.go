package wire_test

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/subosito/cincai/adaptersdk"
	"github.com/subosito/cincai/catalog"
	"github.com/subosito/cincai/credential/seal"
	"github.com/subosito/cincai/credential/store"
	"github.com/subosito/cincai/ingress/keyring"
	"github.com/subosito/cincai/passthrough"
	"github.com/subosito/cincai/upstream"
	"github.com/subosito/cincai/wire"
)

// refreshingStore serves an OAuth token and rotates it on ForceRefresh, standing
// in for credential/refresh.Store so the wire engine's reactive 401 retry can be
// exercised without the vendor registry.
type refreshingStore struct {
	store.Store
	mu     sync.Mutex
	access string
	forced int
}

func (s *refreshingStore) material(profile string) store.Material {
	return store.Material{
		Profile:      profile,
		Kind:         store.KindOAuth,
		AccessToken:  s.access,
		RefreshToken: "r",
		ExpiresAt:    time.Unix(4102444800, 0), // year 2100 — never near expiry
	}
}

func (s *refreshingStore) Get(_ context.Context, profile string) (store.Material, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.material(profile), nil
}

func (s *refreshingStore) ForceRefresh(_ context.Context, profile string, _ store.Material) (store.Material, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.forced++
	s.access = "fresh-token"
	return s.material(profile), nil
}

func TestReactiveRefreshOn401(t *testing.T) {
	var mu sync.Mutex
	var seen []string
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth := r.Header.Get("Authorization")
		mu.Lock()
		seen = append(seen, auth)
		mu.Unlock()
		if auth != "Bearer fresh-token" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"model":"echo","choices":[{"message":{"content":"ok"}}]}`)
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
	st := &refreshingStore{Store: store.NewMemory(key), access: "stale-token"}

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
		t.Fatalf("status=%d %s (want 200 after reactive refresh)", resp.StatusCode, b)
	}
	if st.forced != 1 {
		t.Fatalf("ForceRefresh calls = %d, want 1", st.forced)
	}
	mu.Lock()
	defer mu.Unlock()
	if len(seen) != 2 || seen[0] != "Bearer stale-token" || seen[1] != "Bearer fresh-token" {
		t.Fatalf("upstream auth sequence = %v, want [stale, fresh]", seen)
	}
}
