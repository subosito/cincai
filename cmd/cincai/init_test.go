package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestRunInitIdempotent(t *testing.T) {
	root := t.TempDir()
	exampleDir := filepath.Join(root, "config")
	if err := os.MkdirAll(exampleDir, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"cincai.yaml.example", "providers.yaml.example"} {
		if err := os.WriteFile(filepath.Join(exampleDir, name), []byte("# example\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	if err := runInit(root, false); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"cincai.yaml", "providers.yaml", "cincai.dev.env"} {
		if _, err := os.Stat(filepath.Join(exampleDir, name)); err != nil {
			t.Fatalf("missing %s: %v", name, err)
		}
	}

	before, err := os.ReadFile(filepath.Join(exampleDir, "cincai.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if err := runInit(root, false); err != nil {
		t.Fatal(err)
	}
	after, err := os.ReadFile(filepath.Join(exampleDir, "cincai.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if string(before) != string(after) {
		t.Fatal("init should not overwrite existing files without --force")
	}
}
