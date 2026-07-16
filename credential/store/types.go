package store

import (
	"context"
	"strings"
	"time"
)

// Kind is upstream credential type.
type Kind string

const (
	KindAPIKey Kind = "api_key"
	KindOAuth  Kind = "oauth"
)

// Material is decrypted upstream credential for inject.
type Material struct {
	Profile      string
	Kind         Kind
	APIKey       string
	AccessToken  string
	RefreshToken string
	ExpiresAt    time.Time
	Email        string
	// Extras is opaque key-value metadata from the credential store.
	// Adapters and inject extensions read keys they own; core never interprets vendor keys.
	Extras map[string]string
}

// Extra returns a trimmed metadata value by key.
func (m Material) Extra(key string) string {
	if m.Extras == nil {
		return ""
	}
	return strings.TrimSpace(m.Extras[key])
}

// CredentialSummary is operator-visible metadata (no secrets).
type CredentialSummary struct {
	ID            int64    `json:"id"`
	Profile       string   `json:"profile"`
	Kind          Kind     `json:"kind"`
	Status        string   `json:"status"`
	IdentityKey   *string  `json:"identityKey,omitempty"`
	Hosts         []string `json:"hosts,omitempty"`       // reserved — not populated yet
	RotatesInMs   *int64   `json:"rotatesInMs,omitempty"` // reserved — not populated yet
	DisabledCause *string  `json:"disabledCause,omitempty"`
	CreatedAt     int64    `json:"createdAt"`
	UpdatedAt     int64    `json:"updatedAt"`
	Source        string   `json:"source,omitempty"`
}

// SnapshotMeta is generation heartbeat only.
type SnapshotMeta struct {
	Generation  int64 `json:"generation"`
	GeneratedAt int64 `json:"generatedAt"`
	ServerNowMs int64 `json:"serverNowMs"`
}

// Store persists upstream credentials.
type Store interface {
	Get(ctx context.Context, profile string) (Material, error)
	PutAPIKey(ctx context.Context, profile, key string) (int64, error)
	PutOAuth(ctx context.Context, profile string, m Material) (int64, error)
	UpdateOAuth(ctx context.Context, profile string, m Material) error
	ListSummaries(ctx context.Context) ([]CredentialSummary, error)
	GetSummary(ctx context.Context, id int64) (CredentialSummary, error)
	Disable(ctx context.Context, id int64, cause string) error
	Enable(ctx context.Context, id int64) error
	SnapshotMeta(ctx context.Context) (SnapshotMeta, error)
	BumpGeneration(ctx context.Context) error
	Close() error
}
