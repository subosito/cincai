package config_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/subosito/cincai/internal/config"
)

func TestBrokerKeyRequired(t *testing.T) {
	f := &config.File{}
	f.Credential.Encryption.KeyEnv = "CINCAI_BROKER_KEY_TEST_MISSING"
	t.Setenv("CINCAI_BROKER_KEY_TEST_MISSING", "")
	if _, err := config.BrokerKey(f); err == nil {
		t.Fatal("expected broker key error")
	}
}

func TestBrokerKeyFromEnv(t *testing.T) {
	f := &config.File{}
	f.Credential.Encryption.KeyEnv = "CINCAI_BROKER_KEY_TEST"
	t.Setenv("CINCAI_BROKER_KEY_TEST", "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA=")
	key, err := config.BrokerKey(f)
	if err != nil {
		t.Fatalf("broker key: %v", err)
	}
	if key == "" {
		t.Fatal("empty key")
	}
}

func TestLoadRejectsCredentialProfiles(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "cincai.yaml")
	if err := os.WriteFile(path, []byte("credential_profiles:\n  foo:\n    kind: oauth\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := config.Load(path); err == nil {
		t.Fatal("expected credential_profiles error")
	}
}

