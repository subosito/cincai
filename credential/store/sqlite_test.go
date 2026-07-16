package store_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/subosito/cincai/credential/seal"
	"github.com/subosito/cincai/credential/store"
)

func TestSQLiteBrokerFileIsPrivate(t *testing.T) {
	key, _ := seal.ParseKey("CCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCC=")
	dir := t.TempDir()

	// New broker: created 0600, not the 0644 the sqlite driver would leave under umask 022.
	fresh := filepath.Join(dir, "fresh.db")
	st, err := store.OpenSQLite(fresh, key)
	if err != nil {
		t.Fatal(err)
	}
	st.Close()
	fi, err := os.Stat(fresh)
	if err != nil {
		t.Fatal(err)
	}
	if got := fi.Mode().Perm(); got != 0o600 {
		t.Fatalf("fresh broker mode=%04o want 0600", got)
	}

	// An existing world-readable broker file is tightened to 0600 on open.
	existing := filepath.Join(dir, "existing.db")
	if err := os.WriteFile(existing, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	st2, err := store.OpenSQLite(existing, key)
	if err != nil {
		t.Fatal(err)
	}
	st2.Close()
	fi2, err := os.Stat(existing)
	if err != nil {
		t.Fatal(err)
	}
	if got := fi2.Mode().Perm(); got != 0o600 {
		t.Fatalf("existing broker mode=%04o want 0600", got)
	}
}

func TestSQLiteEncryptAtRest(t *testing.T) {
	key, _ := seal.ParseKey("CCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCC=")
	path := filepath.Join(t.TempDir(), "broker.db")
	st, err := store.OpenSQLite(path, key)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	_, err = st.PutAPIKey(context.Background(), "mock", "sk-secret-key-value")
	if err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), "sk-secret") {
		t.Fatal("broker.db must not contain plaintext secret")
	}
	list, err := st.ListSummaries(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 1 || list[0].Profile != "mock" {
		t.Fatalf("summary: %+v", list)
	}
	mat, err := st.Get(context.Background(), "mock")
	if err != nil || mat.APIKey != "sk-secret-key-value" {
		t.Fatalf("get: %v %+v", err, mat)
	}
}

func TestSQLiteDisableEnable(t *testing.T) {
	key, _ := seal.ParseKey("CCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCC=")
	path := filepath.Join(t.TempDir(), "broker.db")
	st, err := store.OpenSQLite(path, key)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	ctx := context.Background()
	id, err := st.PutAPIKey(ctx, "mock", "sk-secret-key-value")
	if err != nil {
		t.Fatal(err)
	}

	if err := st.Disable(ctx, id, "manual"); err != nil {
		t.Fatal(err)
	}
	if _, err := st.Get(ctx, "mock"); err == nil || !strings.Contains(err.Error(), "disabled") {
		t.Fatalf("get after disable: want disabled error, got %v", err)
	}
	list, err := st.ListSummaries(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 1 || list[0].Status != "disabled" || list[0].DisabledCause == nil || *list[0].DisabledCause != "manual" {
		t.Fatalf("summary after disable: %+v", list)
	}

	if err := st.Enable(ctx, id); err != nil {
		t.Fatal(err)
	}
	mat, err := st.Get(ctx, "mock")
	if err != nil || mat.APIKey != "sk-secret-key-value" {
		t.Fatalf("get after enable: %v %+v", err, mat)
	}
	list, err = st.ListSummaries(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 1 || list[0].Status != "active" || list[0].DisabledCause != nil {
		t.Fatalf("summary after enable: %+v", list)
	}

	if err := st.Enable(ctx, 9999); err == nil {
		t.Fatal("enable unknown id: want error")
	}
}
