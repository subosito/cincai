package keyring_test

import (
	"context"
	"database/sql"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/subosito/cincai/credential/seal"
	"github.com/subosito/cincai/credential/store"
	"github.com/subosito/cincai/ingress/keyring"
)

func TestSecretEmbedsKeyID(t *testing.T) {
	ks := keyring.NewMemoryStore()
	secret, id, err := ks.Create(context.Background(), "embed", keyring.KindStatic, 0, nil)
	if err != nil {
		t.Fatal(err)
	}
	wantPrefix := "sk-dg-" + strconv.FormatInt(id, 10) + "."
	if !strings.HasPrefix(secret, wantPrefix) {
		t.Fatalf("secret=%q want prefix %q", secret, wantPrefix)
	}
}

func TestVerifyRejectsNonCanonicalFormat(t *testing.T) {
	ks := keyring.NewMemoryStore()
	if _, err := ks.Verify(context.Background(), "dr_kX9mN2pQ7vR1wL4sH8jK3nM5tY6bC0dE"); err == nil {
		t.Fatal("expected non-canonical key prefix to be rejected")
	}
	if _, err := ks.Verify(context.Background(), "sk-not-a-gateway-key"); err == nil {
		t.Fatal("expected non-gateway key to be rejected")
	}
}

func TestMemoryStoreVerify(t *testing.T) {
	ks := keyring.NewMemoryStore()
	secret, id, err := ks.Create(context.Background(), "test", keyring.KindStatic, 0, nil)
	if err != nil {
		t.Fatal(err)
	}
	if id != 1 {
		t.Fatalf("id=%d", id)
	}
	p, err := ks.Verify(context.Background(), secret)
	if err != nil || p.ID != "test" {
		t.Fatalf("verify: %v %+v", err, p)
	}
	if _, err := ks.Verify(context.Background(), secret+"x"); err == nil {
		t.Fatal("expected invalid key rejection")
	}
}

func TestExpiredIssuedKey(t *testing.T) {
	ks := keyring.NewMemoryStore()
	secret, _, err := ks.Create(context.Background(), "ttl", keyring.KindIssued, time.Millisecond, nil)
	if err != nil {
		t.Fatal(err)
	}
	time.Sleep(5 * time.Millisecond)
	if _, err := ks.Verify(context.Background(), secret); err == nil {
		t.Fatal("expected expired rejection")
	}
}

// TestSQLStoreSchemaHasScopesColumn enforces gateway_keys scopes column.
func TestSQLStoreSchemaHasScopesColumn(t *testing.T) {
	key, _ := seal.ParseKey("HHHHHHHHHHHHHHHHHHHHHHHHHHHHHHHHHHHHHHHHHHH=")
	path := filepath.Join(t.TempDir(), "broker.db")
	st, err := store.OpenSQLite(path, key)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	db, err := store.BrokerDB(st)
	if err != nil {
		t.Fatal(err)
	}
	rows, err := db.Query(`PRAGMA table_info(gateway_keys)`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	for rows.Next() {
		var cid int
		var name, ctype string
		var notnull, pk int
		var dflt sql.NullString
		if err := rows.Scan(&cid, &name, &ctype, &notnull, &dflt, &pk); err != nil {
			t.Fatal(err)
		}
		if name == "scopes" {
			return
		}
	}
	t.Fatal("gateway_keys must have scopes column")
}

func TestSQLStorePerSecretSalt(t *testing.T) {
	key, _ := seal.ParseKey("HHHHHHHHHHHHHHHHHHHHHHHHHHHHHHHHHHHHHHHHHHH=")
	path := filepath.Join(t.TempDir(), "broker.db")
	st, err := store.OpenSQLite(path, key)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	db, err := store.BrokerDB(st)
	if err != nil {
		t.Fatal(err)
	}
	ks := keyring.NewSQLStore(db)
	s1, _, err := ks.Create(context.Background(), "a", keyring.KindStatic, 0, nil)
	if err != nil {
		t.Fatal(err)
	}
	s2, _, err := ks.Create(context.Background(), "b", keyring.KindStatic, 0, nil)
	if err != nil {
		t.Fatal(err)
	}
	var h1, h2 string
	if err := db.QueryRow(`SELECT hash FROM gateway_keys WHERE name='a'`).Scan(&h1); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow(`SELECT hash FROM gateway_keys WHERE name='b'`).Scan(&h2); err != nil {
		t.Fatal(err)
	}
	if h1 == h2 {
		t.Fatal("distinct keys must hash differently (each has a unique random tail)")
	}
	// Gateway keys store the fast sha256 scheme (argon2 per request would be a DoS vector).
	if !strings.HasPrefix(h1, "sha256$") {
		t.Fatalf("stored hash %q is not the fast sha256 scheme", h1)
	}
	if _, err := ks.Verify(context.Background(), s1); err != nil {
		t.Fatalf("verify s1: %v", err)
	}
	if _, err := ks.Verify(context.Background(), s2); err != nil {
		t.Fatalf("verify s2: %v", err)
	}
}

func TestRevokedKey(t *testing.T) {
	ks := keyring.NewMemoryStore()
	secret, id, err := ks.Create(context.Background(), "rev", keyring.KindStatic, 0, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := ks.Revoke(context.Background(), id); err != nil {
		t.Fatal(err)
	}
	if _, err := ks.Verify(context.Background(), secret); err == nil {
		t.Fatal("expected revoked rejection")
	}
}