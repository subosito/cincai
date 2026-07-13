// Package seal provides AES-256-GCM encryption at rest for credential blobs.
package seal

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
)

const envelopeVersion = 1

type envelope struct {
	V    int    `json:"v"`
	Alg  string `json:"alg"`
	Nonce string `json:"nonce"`
	CT   string `json:"ct"`
}

// Key holds a 32-byte AES-256 key.
type Key []byte

// ParseKey decodes a base64-encoded 32-byte key.
func ParseKey(b64 string) (Key, error) {
	raw, err := base64.StdEncoding.DecodeString(b64)
	if err != nil {
		return nil, fmt.Errorf("broker key: invalid base64: %w", err)
	}
	if len(raw) != 32 {
		return nil, fmt.Errorf("broker key: want 32 bytes, got %d", len(raw))
	}
	return Key(raw), nil
}

// Encrypt seals plaintext with AES-256-GCM.
func (k Key) Encrypt(plaintext []byte) ([]byte, error) {
	if len(k) != 32 {
		return nil, errors.New("seal: key must be 32 bytes")
	}
	block, err := aes.NewCipher(k)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, err
	}
	ct := gcm.Seal(nil, nonce, plaintext, nil)
	env := envelope{
		V:     envelopeVersion,
		Alg:   "aes-256-gcm",
		Nonce: base64.StdEncoding.EncodeToString(nonce),
		CT:    base64.StdEncoding.EncodeToString(ct),
	}
	return json.Marshal(env)
}

// Decrypt opens a sealed blob.
func (k Key) Decrypt(data []byte) ([]byte, error) {
	if len(k) != 32 {
		return nil, errors.New("seal: key must be 32 bytes")
	}
	var env envelope
	if err := json.Unmarshal(data, &env); err != nil {
		return nil, fmt.Errorf("seal: envelope: %w", err)
	}
	if env.V != envelopeVersion || env.Alg != "aes-256-gcm" {
		return nil, errors.New("seal: unsupported envelope")
	}
	nonce, err := base64.StdEncoding.DecodeString(env.Nonce)
	if err != nil {
		return nil, err
	}
	ct, err := base64.StdEncoding.DecodeString(env.CT)
	if err != nil {
		return nil, err
	}
	block, err := aes.NewCipher(k)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	plain, err := gcm.Open(nil, nonce, ct, nil)
	if err != nil {
		return nil, fmt.Errorf("seal: decrypt: %w", err)
	}
	return plain, nil
}