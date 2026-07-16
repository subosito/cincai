package main

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

func TestResolveCatalogPathFromConfig(t *testing.T) {
	dir := t.TempDir()
	cfg := filepath.Join(dir, "cincai.yaml")
	catalog := filepath.Join(dir, "providers.yaml")
	if err := os.WriteFile(cfg, []byte("serve:\n  catalog: providers.yaml\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(catalog, []byte("providers: {}\nmodels: {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	got, err := resolveCatalogPath(cfg, "")
	if err != nil {
		t.Fatal(err)
	}
	if got != catalog {
		t.Fatalf("got %q want %q", got, catalog)
	}
}

func TestCatalogValidateCmdOK(t *testing.T) {
	dir := t.TempDir()
	catalog := filepath.Join(dir, "providers.yaml")
	example := filepath.Join("..", "..", "config", "providers.yaml.example")
	raw, err := os.ReadFile(example)
	if err != nil {
		t.Skip("providers.yaml.example missing")
	}
	if err := os.WriteFile(catalog, raw, 0o644); err != nil {
		t.Fatal(err)
	}
	if code := catalogValidateCmd([]string{"--catalog", catalog}); code != 0 {
		t.Fatalf("validate exit=%d want 0", code)
	}
}

// Routes resolving is not enough: a catalog may name an adapter that no enabled
// adapter registers, which used to validate clean and then fail per-request.
func TestCatalogValidateCmdHandlerRegistration(t *testing.T) {
	const providers = `providers:
  p:
    credential_profile: p-api
    surfaces:
      chat:
        protocol: openai-chat-completions
        base_url: https://example.test
models:
  m:
    modalities:
      chat:
        wire: openai-chat-completions
        providers:
          - provider_ref: p
            adapter: %s
`
	tests := []struct {
		name    string
		adapter string
		enable  string
		want    int
	}{
		{"registered adapter passes", "wire-translate-a2o", "passthrough\n    - wire-translate", 0},
		{"unregistered adapter fails", "no-such-adapter", "passthrough\n    - wire-translate", 1},
		{"linked but not enabled fails", "mistral", "passthrough", 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			cfg := filepath.Join(dir, "cincai.yaml")
			cat := filepath.Join(dir, "providers.yaml")
			cfgBody := "serve:\n  catalog: providers.yaml\nadapters:\n  enable:\n    - " + tt.enable + "\n"
			if err := os.WriteFile(cfg, []byte(cfgBody), 0o644); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(cat, []byte(fmt.Sprintf(providers, tt.adapter)), 0o644); err != nil {
				t.Fatal(err)
			}
			if code := catalogValidateCmd([]string{"--config", cfg}); code != tt.want {
				t.Fatalf("validate exit=%d want %d", code, tt.want)
			}
		})
	}
}
