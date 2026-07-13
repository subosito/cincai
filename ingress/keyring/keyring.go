package keyring

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"
)

const (
	KindStatic = "static"
	KindIssued = "issued"
	prefix     = "sk-dg-"
)

// Principal is an authenticated gateway client.
type Principal struct {
	ID     string
	KeyID  int64
	Scopes []string
}

// KeyStore persists gateway keys (hashed).
type KeyStore interface {
	Create(ctx context.Context, name, kind string, ttl time.Duration, scopes []string) (secret string, id int64, err error)
	Verify(ctx context.Context, secret string) (Principal, error)
	List(ctx context.Context) ([]KeyMeta, error)
	Revoke(ctx context.Context, id int64) error
}

// KeyMeta is gateway key metadata.
type KeyMeta struct {
	ID        int64  `json:"id"`
	Name      string `json:"name"`
	Kind      string `json:"kind"`
	ExpiresAt *int64   `json:"expiresAt,omitempty"`
	Revoked   bool     `json:"revoked"`
	Scopes    []string `json:"scopes,omitempty"`
	CreatedAt int64    `json:"createdAt"`
}

// Authenticator verifies ingress Bearer tokens.
type Authenticator struct {
	Store KeyStore
}

func (a *Authenticator) Authenticate(ctx context.Context, r *http.Request) (Principal, error) {
	tok, err := ingressToken(r)
	if err != nil {
		return Principal{}, err
	}
	return a.Store.Verify(ctx, tok)
}

// ingressToken accepts Authorization: Bearer (OpenAI-style) or x-api-key
// (Anthropic-style clients send the gateway key there on /v1/messages).
func ingressToken(r *http.Request) (string, error) {
	h := r.Header.Get("Authorization")
	if strings.HasPrefix(h, "Bearer ") {
		t := strings.TrimSpace(strings.TrimPrefix(h, "Bearer "))
		if t == "" {
			return "", errors.New("empty bearer token")
		}
		return t, nil
	}
	if t := strings.TrimSpace(r.Header.Get("x-api-key")); t != "" {
		return t, nil
	}
	return "", errors.New("missing bearer token or x-api-key")
}

// SQLStore implements KeyStore on sqlite.
type SQLStore struct {
	db *sql.DB
}

func NewSQLStore(db *sql.DB) *SQLStore { return &SQLStore{db: db} }

func randomTail() (string, error) {
	b := make([]byte, 24)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

func formatSecret(id int64) (string, error) {
	tail, err := randomTail()
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%s%d.%s", prefix, id, tail), nil
}

func parseKeyID(secret string) (int64, bool) {
	if !strings.HasPrefix(secret, prefix) {
		return 0, false
	}
	rest := strings.TrimPrefix(secret, prefix)
	dot := strings.Index(rest, ".")
	if dot <= 0 {
		return 0, false
	}
	id, err := strconv.ParseInt(rest[:dot], 10, 64)
	if err != nil || id <= 0 || dot+1 >= len(rest) {
		return 0, false
	}
	return id, true
}

func encodeScopes(scopes []string) (sql.NullString, error) {
	if len(scopes) == 0 {
		return sql.NullString{}, nil
	}
	raw, err := json.Marshal(scopes)
	if err != nil {
		return sql.NullString{}, err
	}
	return sql.NullString{String: string(raw), Valid: true}, nil
}

func decodeScopes(raw sql.NullString) []string {
	if !raw.Valid || raw.String == "" {
		return nil
	}
	var scopes []string
	_ = json.Unmarshal([]byte(raw.String), &scopes)
	return scopes
}

func (s *SQLStore) Create(ctx context.Context, name, kind string, ttl time.Duration, scopes []string) (string, int64, error) {
	now := time.Now().UnixMilli()
	var exp *int64
	if ttl > 0 {
		v := time.Now().Add(ttl).UnixMilli()
		exp = &v
	} else if kind != KindStatic {
		return "", 0, fmt.Errorf("issued keys require ttl")
	}
	scopesEnc, err := encodeScopes(scopes)
	if err != nil {
		return "", 0, err
	}
	res, err := s.db.ExecContext(ctx, `
		INSERT INTO gateway_keys (name, kind, hash, expires_at, scopes, created_at)
		VALUES (?, ?, '', ?, ?, ?)`, name, kind, exp, scopesEnc, now)
	if err != nil {
		return "", 0, err
	}
	id, _ := res.LastInsertId()
	secret, err := formatSecret(id)
	if err != nil {
		return "", 0, err
	}
	sealed := sealGatewayKey(secret)
	if _, err := s.db.ExecContext(ctx, `UPDATE gateway_keys SET hash = ? WHERE id = ?`, sealed, id); err != nil {
		return "", 0, err
	}
	return secret, id, nil
}

func (s *SQLStore) Verify(ctx context.Context, secret string) (Principal, error) {
	id, ok := parseKeyID(secret)
	if !ok {
		return Principal{}, errInvalidGatewayKey
	}
	return s.verifyByID(ctx, id, secret)
}

var errInvalidGatewayKey = errors.New("invalid gateway key")

func (s *SQLStore) verifyByID(ctx context.Context, id int64, secret string) (Principal, error) {
	var name, sealed string
	var exp sql.NullInt64
	var scopesRaw sql.NullString
	var revoked int
	err := s.db.QueryRowContext(ctx, `
		SELECT name, hash, expires_at, scopes, revoked FROM gateway_keys WHERE id = ?`, id).
		Scan(&name, &sealed, &exp, &scopesRaw, &revoked)
	if err == sql.ErrNoRows {
		return Principal{}, errInvalidGatewayKey
	}
	if err != nil {
		return Principal{}, err
	}
	if revoked != 0 {
		return Principal{}, errInvalidGatewayKey
	}
	if exp.Valid && time.Now().UnixMilli() > exp.Int64 {
		return Principal{}, errInvalidGatewayKey
	}
	if !verifyGatewayKey(secret, sealed) {
		return Principal{}, errInvalidGatewayKey
	}
	return Principal{ID: name, KeyID: id, Scopes: decodeScopes(scopesRaw)}, nil
}

func (s *SQLStore) List(ctx context.Context) ([]KeyMeta, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, name, kind, expires_at, scopes, revoked, created_at FROM gateway_keys ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []KeyMeta
	for rows.Next() {
		var m KeyMeta
		var exp sql.NullInt64
		var scopesRaw sql.NullString
		var rev int
		if err := rows.Scan(&m.ID, &m.Name, &m.Kind, &exp, &scopesRaw, &rev, &m.CreatedAt); err != nil {
			return nil, err
		}
		m.Scopes = decodeScopes(scopesRaw)
		if exp.Valid {
			v := exp.Int64
			m.ExpiresAt = &v
		}
		m.Revoked = rev != 0
		out = append(out, m)
	}
	return out, rows.Err()
}

func (s *SQLStore) Revoke(ctx context.Context, id int64) error {
	res, err := s.db.ExecContext(ctx, `UPDATE gateway_keys SET revoked = 1 WHERE id = ?`, id)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("gateway key %d not found", id)
	}
	return nil
}

// MemoryStore is an in-memory KeyStore for tests.
type MemoryStore struct {
	records []memoryRecord
	next    int64
}

type memoryRecord struct {
	meta   KeyMeta
	sealed string
	scopes []string
}

func NewMemoryStore() *MemoryStore { return &MemoryStore{} }

func (m *MemoryStore) Create(ctx context.Context, name, kind string, ttl time.Duration, scopes []string) (string, int64, error) {
	_ = ctx
	m.next++
	id := m.next
	secret, err := formatSecret(id)
	if err != nil {
		return "", 0, err
	}
	sealed := sealGatewayKey(secret)
	now := time.Now().UnixMilli()
	meta := KeyMeta{ID: id, Name: name, Kind: kind, CreatedAt: now, Scopes: append([]string(nil), scopes...)}
	if ttl > 0 {
		v := time.Now().Add(ttl).UnixMilli()
		meta.ExpiresAt = &v
	} else if kind != KindStatic {
		return "", 0, fmt.Errorf("issued keys require ttl")
	}
	m.records = append(m.records, memoryRecord{meta: meta, sealed: sealed, scopes: append([]string(nil), scopes...)})
	return secret, id, nil
}

func (m *MemoryStore) Verify(ctx context.Context, secret string) (Principal, error) {
	_ = ctx
	id, ok := parseKeyID(secret)
	if !ok {
		return Principal{}, errInvalidGatewayKey
	}
	for _, rec := range m.records {
		if rec.meta.ID != id || rec.meta.Revoked {
			continue
		}
		if rec.meta.ExpiresAt != nil && time.Now().UnixMilli() > *rec.meta.ExpiresAt {
			return Principal{}, errInvalidGatewayKey
		}
		if verifyGatewayKey(secret, rec.sealed) {
			return Principal{ID: rec.meta.Name, KeyID: rec.meta.ID, Scopes: append([]string(nil), rec.scopes...)}, nil
		}
		return Principal{}, errInvalidGatewayKey
	}
	return Principal{}, errInvalidGatewayKey
}

func (m *MemoryStore) List(ctx context.Context) ([]KeyMeta, error) {
	_ = ctx
	out := make([]KeyMeta, 0, len(m.records))
	for _, rec := range m.records {
		out = append(out, rec.meta)
	}
	return out, nil
}

func (m *MemoryStore) Revoke(ctx context.Context, id int64) error {
	_ = ctx
	for i := range m.records {
		if m.records[i].meta.ID == id {
			m.records[i].meta.Revoked = true
			return nil
		}
	}
	return fmt.Errorf("gateway key %d not found", id)
}