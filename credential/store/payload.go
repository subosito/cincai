package store

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/subosito/cincai/credential/seal"
)

type apiPayload struct {
	Type string `json:"type"`
	Key  string `json:"key"`
}

type oauthPayload struct {
	Type    string            `json:"type"`
	Access  string            `json:"access"`
	Refresh string            `json:"refresh"`
	Expires int64             `json:"expires"`
	Email   string            `json:"email,omitempty"`
	Extras  map[string]string `json:"extras,omitempty"`
}

// EncryptPayload JSON-marshals v and encrypts with key.
func EncryptPayload(key seal.Key, v any) ([]byte, error) {
	raw, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}
	return key.Encrypt(raw)
}

// MaterialFromDecrypted builds Material from decrypted credential JSON.
func MaterialFromDecrypted(profile string, kind Kind, raw []byte) (Material, error) {
	mat := Material{Profile: profile, Kind: kind}
	switch kind {
	case KindAPIKey:
		var p apiPayload
		if err := json.Unmarshal(raw, &p); err != nil {
			return Material{}, err
		}
		mat.APIKey = p.Key
	case KindOAuth:
		var p oauthPayload
		if err := json.Unmarshal(raw, &p); err != nil {
			return Material{}, err
		}
		mat.AccessToken = p.Access
		mat.RefreshToken = p.Refresh
		if p.Expires > 0 {
			mat.ExpiresAt = time.UnixMilli(p.Expires)
		}
		mat.Email = p.Email
		mat.Extras = cloneExtras(p.Extras)
		if len(mat.Extras) == 0 {
			mat.Extras = nil
		}
	default:
		return Material{}, fmt.Errorf("unknown kind %q", kind)
	}
	return mat, nil
}

func apiKeyPayload(key string) apiPayload {
	return apiPayload{Type: "api_key", Key: key}
}

func oauthMaterialPayload(mat Material) oauthPayload {
	var exp int64
	if !mat.ExpiresAt.IsZero() {
		exp = mat.ExpiresAt.UnixMilli()
	}
	extras := mat.Extras
	if extras == nil {
		extras = map[string]string{}
	}
	return oauthPayload{
		Type: "oauth", Access: mat.AccessToken, Refresh: mat.RefreshToken,
		Expires: exp, Email: mat.Email, Extras: extras,
	}
}

func cloneExtras(in map[string]string) map[string]string {
	if len(in) == 0 {
		return map[string]string{}
	}
	out := make(map[string]string, len(in))
	for k, v := range in {
		if s := strings.TrimSpace(v); s != "" {
			out[k] = s
		}
	}
	return out
}

// MergeExtras copies keys from cur into refreshed when refreshed omits them.
func MergeExtras(cur, refreshed map[string]string) map[string]string {
	if len(cur) == 0 {
		return refreshed
	}
	out := cloneExtras(refreshed)
	for k, v := range cur {
		if _, ok := out[k]; !ok {
			out[k] = v
		}
	}
	return out
}
