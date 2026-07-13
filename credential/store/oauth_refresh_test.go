package store_test

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/subosito/cincai/credential/seal"
	"github.com/subosito/cincai/credential/store"
)

func TestSQLiteUpdateOAuth(t *testing.T) {
	key, _ := seal.ParseKey("JJJJJJJJJJJJJJJJJJJJJJJJJJJJJJJJJJJJJJJJJJJ=")
	path := filepath.Join(t.TempDir(), "broker.db")
	st, err := store.OpenSQLite(path, key)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	_, err = st.PutOAuth(context.Background(), "oauth-prof", store.Material{
		Kind: store.KindOAuth, AccessToken: "old-access", RefreshToken: "old-refresh",
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := st.UpdateOAuth(context.Background(), "oauth-prof", store.Material{
		Kind: store.KindOAuth, AccessToken: "new-access", RefreshToken: "new-refresh",
	}); err != nil {
		t.Fatal(err)
	}
	mat, err := st.Get(context.Background(), "oauth-prof")
	if err != nil {
		t.Fatal(err)
	}
	if mat.AccessToken != "new-access" {
		t.Fatalf("got %q", mat.AccessToken)
	}
}