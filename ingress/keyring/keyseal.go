package keyring

import (
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"strings"
)

const sha256Scheme = "sha256"

// sealGatewayKey hashes a gateway-key secret for storage. Gateway keys carry 192 bits of
// crypto/rand entropy (see randomTail), so a single SHA-256 is cryptographically sufficient:
// recovering the token from the stored hash is a 2^192 search and forging one needs the full
// random tail — both infeasible. A memory-hard hash (argon2) only helps *guessable* secrets,
// which a random token is not; using one per request would instead be a DoS vector, since key
// ids are sequential and embedded in the token, so a fast hash is both correct and cheap. This
// matches how high-entropy API tokens are stored elsewhere (GitHub, Stripe).
func sealGatewayKey(secret string) string {
	sum := sha256.Sum256([]byte(secret))
	return sha256Scheme + "$" + hex.EncodeToString(sum[:])
}

// verifyGatewayKey checks a secret against a stored hash. Only the SHA-256 scheme is accepted.
func verifyGatewayKey(secret, sealed string) bool {
	rest, ok := strings.CutPrefix(sealed, sha256Scheme+"$")
	if !ok {
		return false
	}
	want, err := hex.DecodeString(rest)
	if err != nil {
		return false
	}
	got := sha256.Sum256([]byte(secret))
	return subtle.ConstantTimeCompare(got[:], want) == 1
}
