package keyring

import (
	"strings"
	"testing"
)

func TestSealGatewayKeyUsesFastScheme(t *testing.T) {
	sealed := sealGatewayKey("sk-dg-1.abc")
	if !strings.HasPrefix(sealed, sha256Scheme+"$") {
		t.Fatalf("gateway keys must use the fast %q scheme, got %q", sha256Scheme, sealed)
	}
}

func TestVerifyGatewayKey(t *testing.T) {
	const secret = "sk-dg-1.super-secret-tail"

	sealed := sealGatewayKey(secret)
	if !verifyGatewayKey(secret, sealed) {
		t.Fatal("correct secret failed to verify")
	}
	if verifyGatewayKey("sk-dg-1.wrong-tail", sealed) {
		t.Fatal("wrong tail verified")
	}

	// Only the sha256 scheme is accepted; any other stored form rejects cleanly.
	for _, bad := range []string{
		"sha256$not-hex",  // right scheme, malformed digest
		"other$deadbeef",  // unknown scheme
		"",                // empty
	} {
		if verifyGatewayKey(secret, bad) {
			t.Fatalf("stored value %q must not verify", bad)
		}
	}
}
