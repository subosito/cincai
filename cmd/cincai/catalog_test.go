package main

import (
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