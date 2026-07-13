package seal_test

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/subosito/cincai/credential/seal"
)

func TestEncryptRoundTrip(t *testing.T) {
	key, err := seal.ParseKey("AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA=")
	if err != nil {
		t.Fatal(err)
	}
	plain := []byte(`{"type":"api_key","key":"sk-secret-value"}`)
	enc, err := key.Encrypt(plain)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(enc), "sk-secret") {
		t.Fatal("ciphertext must not contain plaintext secret")
	}
	out, err := key.Decrypt(enc)
	if err != nil {
		t.Fatal(err)
	}
	if string(out) != string(plain) {
		t.Fatalf("got %q", out)
	}
}

func TestStolenDBNoPlaintext(t *testing.T) {
	key, _ := seal.ParseKey("BBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBB=")
	enc, _ := key.Encrypt([]byte(`{"access":"tok","refresh":"ref"}`))
	var m map[string]any
	_ = json.Unmarshal(enc, &m)
	raw, _ := json.Marshal(m)
	if strings.Contains(string(raw), "tok") || strings.Contains(string(raw), "ref") {
		t.Fatal("envelope json must not embed secrets")
	}
}