package adaptersdk_test

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/subosito/cincai/adaptersdk"
	"github.com/subosito/cincai/adaptersdk/handler"
	"github.com/subosito/cincai/catalog"
	"github.com/subosito/cincai/credential/store"
	"github.com/subosito/cincai/passthrough"
)

func TestNewRegistryInitialized(t *testing.T) {
	reg := adaptersdk.NewRegistry()
	if reg.ChatHandlers == nil || reg.EmbedHandlers == nil {
		t.Fatal("registry maps not initialized")
	}
}

func TestRegistryLookupAndDispatch(t *testing.T) {
	reg := adaptersdk.NewRegistry()
	if err := passthrough.New().Register(reg); err != nil {
		t.Fatal(err)
	}
	chat, ok := reg.ChatHandlers["openai-chat-completions"]
	if !ok {
		t.Fatal("chat handler missing")
	}
	if chat.Protocol() != "openai-chat-completions" {
		t.Fatalf("protocol=%s", chat.Protocol())
	}

	var saw bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		saw = true
		_, _ = io.WriteString(w, `{}`)
	}))
	defer srv.Close()

	tgt := handler.Target{
		Target:   catalog.Target{BaseURL: srv.URL},
		Material: store.Material{Kind: store.KindAPIKey, APIKey: "k"},
	}
	resp, err := chat.Forward(context.Background(), http.DefaultClient, tgt, strings.NewReader(`{}`), nil)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if !saw {
		t.Fatal("handler did not reach upstream")
	}
}