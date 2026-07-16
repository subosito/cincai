package catalog_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/subosito/cincai/catalog"
	icatalog "github.com/subosito/cincai/internal/catalog"
)

func TestValidateRoutesDevExample(t *testing.T) {
	path := filepath.Join("..", "config", "providers.yaml.example")
	if _, err := os.Stat(path); err != nil {
		t.Skip("providers.yaml.example missing")
	}
	cat, err := icatalog.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := cat.ValidateRoutes(); err != nil {
		t.Fatal(err)
	}
	if cat.ModelCount() == 0 {
		t.Fatal("expected models")
	}
}

func TestValidateRoutesBrokenPool(t *testing.T) {
	yaml := `
providers:
  acme:
    credential_profile: acme
    surfaces:
      chat:
        protocol: openai-chat-completions
        base_url: https://api.example.com/v1
models:
  broken:
    modalities:
      chat:
        wire: openai-chat-completions
        providers:
          - provider_ref: missing
            model: m
`
	tmp := filepath.Join(t.TempDir(), "providers.yaml")
	if err := os.WriteFile(tmp, []byte(yaml), 0o644); err != nil {
		t.Fatal(err)
	}
	cat, err := catalog.Load(tmp)
	if err != nil {
		t.Fatal(err)
	}
	if err := cat.ValidateRoutes(); err == nil {
		t.Fatal("expected routing error")
	}
}
