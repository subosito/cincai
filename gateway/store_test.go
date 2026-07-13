package gateway_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/subosito/cincai/gateway"
	"github.com/subosito/cincai/internal/config"
)

const testBrokerKey = "CCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCC="

func TestOpenStoreMemory(t *testing.T) {
	t.Setenv(config.BrokerKeyEnv, testBrokerKey)

	dir := t.TempDir()
	f := &config.File{}
	f.Credential.Backend = "memory"
	// If memory accidentally opens sqlite, it would write here.
	f.Credential.Broker = filepath.Join(dir, "must-not-exist.db")
	f.Credential.Encryption.KeyEnv = config.BrokerKeyEnv

	st, ks, err := gateway.OpenStore(f)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	if _, err := os.Stat(f.Credential.Broker); !os.IsNotExist(err) {
		t.Fatalf("memory backend must not create broker file: err=%v", err)
	}

	id, err := st.PutAPIKey(context.Background(), "dev-api", "sk-test")
	if err != nil {
		t.Fatal(err)
	}
	mat, err := st.Get(context.Background(), "dev-api")
	if err != nil {
		t.Fatal(err)
	}
	if mat.APIKey != "sk-test" {
		t.Fatalf("api key = %q", mat.APIKey)
	}
	if id == 0 {
		t.Fatal("expected non-zero credential id")
	}

	secret, keyID, err := ks.Create(context.Background(), "test", "static", 0, nil)
	if err != nil {
		t.Fatal(err)
	}
	if secret == "" || keyID == 0 {
		t.Fatalf("key create: secret empty or id=%d", keyID)
	}
	if _, err := ks.Verify(context.Background(), secret); err != nil {
		t.Fatalf("verify: %v", err)
	}
}

func TestOpenStoreSQLite(t *testing.T) {
	t.Setenv(config.BrokerKeyEnv, testBrokerKey)

	path := filepath.Join(t.TempDir(), "broker.db")
	f := &config.File{}
	f.Credential.Backend = "sqlite"
	f.Credential.Broker = path
	f.Credential.Encryption.KeyEnv = config.BrokerKeyEnv

	st, ks, err := gateway.OpenStore(f)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	if _, err := os.Stat(path); err != nil {
		t.Fatalf("broker file: %v", err)
	}
	if _, err := st.PutAPIKey(context.Background(), "dev-api", "sk-test"); err != nil {
		t.Fatal(err)
	}
	if _, _, err := ks.Create(context.Background(), "test", "static", 0, nil); err != nil {
		t.Fatal(err)
	}
}
