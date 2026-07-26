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

// DefaultBudgetWindow is used when a key has a max-token budget but no window set.
const DefaultBudgetWindow = 24 * time.Hour

// Principal is an authenticated gateway client.
type Principal struct {
	ID     string
	KeyID  int64
	Scopes []string
	// BudgetMaxTokens is the rolling-window token cap (input+output). 0 = unlimited.
	BudgetMaxTokens int64
	// BudgetWindow is the rolling window for the cap. Zero means DefaultBudgetWindow when MaxTokens > 0.
	BudgetWindow time.Duration
}

// HasBudget reports whether this principal has an active token budget.
func (p Principal) HasBudget() bool { return p.BudgetMaxTokens > 0 }

// BudgetWindowOrDefault returns the effective rolling window.
func (p Principal) BudgetWindowOrDefault() time.Duration {
	if p.BudgetWindow > 0 {
		return p.BudgetWindow
	}
	return DefaultBudgetWindow
}

// KeyStore persists gateway keys (hashed).
type KeyStore interface {
	Create(ctx context.Context, name, kind string, ttl time.Duration, scopes []string) (secret string, id int64, err error)
	Verify(ctx context.Context, secret string) (Principal, error)
	List(ctx context.Context) ([]KeyMeta, error)
	Revoke(ctx context.Context, id int64) error
	// SetScopes replaces scopes on an existing key without rotating the secret.
	// Empty scopes mean "allow all" (same as Create). Revoked keys are rejected.
	SetScopes(ctx context.Context, id int64, scopes []string) error
	// SetName renames an existing key without rotating the secret.
	// Name is the principal_id shown in usage logs. Revoked keys are rejected.
	SetName(ctx context.Context, id int64, name string) error
	// SetBudget sets a rolling token budget (maxTokens=0 clears the budget).
	// window=0 with maxTokens>0 means DefaultBudgetWindow.
	SetBudget(ctx context.Context, id int64, maxTokens int64, window time.Duration) error
	// BudgetUsed returns tokens recorded for key id within the rolling window.
	BudgetUsed(ctx context.Context, keyID int64, window time.Duration) (int64, error)
	// BudgetRecord appends a usage chip (tokens) for the rolling budget ledger.
	BudgetRecord(ctx context.Context, keyID int64, principalID string, tokens int64) error
}

// KeyMeta is gateway key metadata.
type KeyMeta struct {
	ID        int64    `json:"id"`
	Name      string   `json:"name"`
	Kind      string   `json:"kind"`
	ExpiresAt *int64   `json:"expiresAt,omitempty"`
	Revoked   bool     `json:"revoked"`
	Scopes    []string `json:"scopes,omitempty"`
	CreatedAt int64    `json:"createdAt"`
	// BudgetMaxTokens is 0 when unlimited / unset.
	BudgetMaxTokens int64 `json:"budgetMaxTokens,omitempty"`
	// BudgetWindowSec is the rolling window in seconds (0 → default 24h when budget set).
	BudgetWindowSec int64 `json:"budgetWindowSec,omitempty"`
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
	var maxTok, winSec sql.NullInt64
	err := s.db.QueryRowContext(ctx, `
		SELECT name, hash, expires_at, scopes, revoked, budget_max_tokens, budget_window_sec
		FROM gateway_keys WHERE id = ?`, id).
		Scan(&name, &sealed, &exp, &scopesRaw, &revoked, &maxTok, &winSec)
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
	p := Principal{ID: name, KeyID: id, Scopes: decodeScopes(scopesRaw)}
	if maxTok.Valid && maxTok.Int64 > 0 {
		p.BudgetMaxTokens = maxTok.Int64
		if winSec.Valid && winSec.Int64 > 0 {
			p.BudgetWindow = time.Duration(winSec.Int64) * time.Second
		}
	}
	return p, nil
}

func (s *SQLStore) List(ctx context.Context) ([]KeyMeta, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, name, kind, expires_at, scopes, revoked, created_at,
		       budget_max_tokens, budget_window_sec
		FROM gateway_keys ORDER BY id`)
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
		var maxTok, winSec sql.NullInt64
		if err := rows.Scan(&m.ID, &m.Name, &m.Kind, &exp, &scopesRaw, &rev, &m.CreatedAt, &maxTok, &winSec); err != nil {
			return nil, err
		}
		m.Scopes = decodeScopes(scopesRaw)
		if exp.Valid {
			v := exp.Int64
			m.ExpiresAt = &v
		}
		m.Revoked = rev != 0
		if maxTok.Valid && maxTok.Int64 > 0 {
			m.BudgetMaxTokens = maxTok.Int64
		}
		if winSec.Valid && winSec.Int64 > 0 {
			m.BudgetWindowSec = winSec.Int64
		}
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

func (s *SQLStore) SetScopes(ctx context.Context, id int64, scopes []string) error {
	scopesEnc, err := encodeScopes(scopes)
	if err != nil {
		return err
	}
	if err := s.requireActiveKey(ctx, id); err != nil {
		return err
	}
	_, err = s.db.ExecContext(ctx, `UPDATE gateway_keys SET scopes = ? WHERE id = ?`, scopesEnc, id)
	return err
}

func (s *SQLStore) SetName(ctx context.Context, id int64, name string) error {
	name = strings.TrimSpace(name)
	if name == "" {
		return fmt.Errorf("name is required")
	}
	if err := s.requireActiveKey(ctx, id); err != nil {
		return err
	}
	_, err := s.db.ExecContext(ctx, `UPDATE gateway_keys SET name = ? WHERE id = ?`, name, id)
	return err
}

func (s *SQLStore) SetBudget(ctx context.Context, id int64, maxTokens int64, window time.Duration) error {
	if err := s.requireActiveKey(ctx, id); err != nil {
		return err
	}
	if maxTokens < 0 {
		return fmt.Errorf("max tokens must be >= 0")
	}
	if maxTokens == 0 {
		_, err := s.db.ExecContext(ctx, `
			UPDATE gateway_keys SET budget_max_tokens = NULL, budget_window_sec = NULL WHERE id = ?`, id)
		return err
	}
	winSec := int64(0)
	if window > 0 {
		winSec = int64(window / time.Second)
		if winSec < 1 {
			winSec = 1
		}
	}
	_, err := s.db.ExecContext(ctx, `
		UPDATE gateway_keys SET budget_max_tokens = ?, budget_window_sec = ? WHERE id = ?`,
		maxTokens, nullIfZero(winSec), id)
	return err
}

func nullIfZero(v int64) any {
	if v == 0 {
		return nil
	}
	return v
}

func (s *SQLStore) BudgetUsed(ctx context.Context, keyID int64, window time.Duration) (int64, error) {
	if window <= 0 {
		window = DefaultBudgetWindow
	}
	since := time.Now().Add(-window).UnixMilli()
	var sum sql.NullInt64
	err := s.db.QueryRowContext(ctx, `
		SELECT COALESCE(SUM(tokens), 0) FROM key_budget_usage
		WHERE key_id = ? AND created_at >= ?`, keyID, since).Scan(&sum)
	if err != nil {
		return 0, err
	}
	if !sum.Valid {
		return 0, nil
	}
	return sum.Int64, nil
}

func (s *SQLStore) BudgetRecord(ctx context.Context, keyID int64, principalID string, tokens int64) error {
	if tokens <= 0 || keyID <= 0 {
		return nil
	}
	now := time.Now().UnixMilli()
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO key_budget_usage (key_id, principal_id, tokens, created_at)
		VALUES (?, ?, ?, ?)`, keyID, principalID, tokens, now)
	if err != nil {
		return err
	}
	// Opportunistic prune: drop chips older than 48h so the table stays small.
	// Rolling check only needs the active window (≤24h default).
	cutoff := time.Now().Add(-48 * time.Hour).UnixMilli()
	_, _ = s.db.ExecContext(ctx, `DELETE FROM key_budget_usage WHERE created_at < ?`, cutoff)
	return nil
}

func (s *SQLStore) requireActiveKey(ctx context.Context, id int64) error {
	var revoked int
	err := s.db.QueryRowContext(ctx, `SELECT revoked FROM gateway_keys WHERE id = ?`, id).Scan(&revoked)
	if err == sql.ErrNoRows {
		return fmt.Errorf("gateway key %d not found", id)
	}
	if err != nil {
		return err
	}
	if revoked != 0 {
		return fmt.Errorf("gateway key %d is revoked", id)
	}
	return nil
}

// MemoryStore is an in-memory KeyStore for tests.
type MemoryStore struct {
	records []memoryRecord
	chips   []budgetChip
	next    int64
}

type memoryRecord struct {
	meta   KeyMeta
	sealed string
	scopes []string
}

type budgetChip struct {
	keyID        int64
	principalID  string
	tokens       int64
	createdAtMs  int64
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
			p := Principal{ID: rec.meta.Name, KeyID: rec.meta.ID, Scopes: append([]string(nil), rec.scopes...)}
			if rec.meta.BudgetMaxTokens > 0 {
				p.BudgetMaxTokens = rec.meta.BudgetMaxTokens
				if rec.meta.BudgetWindowSec > 0 {
					p.BudgetWindow = time.Duration(rec.meta.BudgetWindowSec) * time.Second
				}
			}
			return p, nil
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

func (m *MemoryStore) SetScopes(ctx context.Context, id int64, scopes []string) error {
	_ = ctx
	for i := range m.records {
		if m.records[i].meta.ID != id {
			continue
		}
		if m.records[i].meta.Revoked {
			return fmt.Errorf("gateway key %d is revoked", id)
		}
		cp := append([]string(nil), scopes...)
		m.records[i].scopes = cp
		m.records[i].meta.Scopes = append([]string(nil), cp...)
		return nil
	}
	return fmt.Errorf("gateway key %d not found", id)
}

func (m *MemoryStore) SetName(ctx context.Context, id int64, name string) error {
	_ = ctx
	name = strings.TrimSpace(name)
	if name == "" {
		return fmt.Errorf("name is required")
	}
	for i := range m.records {
		if m.records[i].meta.ID != id {
			continue
		}
		if m.records[i].meta.Revoked {
			return fmt.Errorf("gateway key %d is revoked", id)
		}
		m.records[i].meta.Name = name
		return nil
	}
	return fmt.Errorf("gateway key %d not found", id)
}

func (m *MemoryStore) SetBudget(ctx context.Context, id int64, maxTokens int64, window time.Duration) error {
	_ = ctx
	if maxTokens < 0 {
		return fmt.Errorf("max tokens must be >= 0")
	}
	for i := range m.records {
		if m.records[i].meta.ID != id {
			continue
		}
		if m.records[i].meta.Revoked {
			return fmt.Errorf("gateway key %d is revoked", id)
		}
		if maxTokens == 0 {
			m.records[i].meta.BudgetMaxTokens = 0
			m.records[i].meta.BudgetWindowSec = 0
			return nil
		}
		m.records[i].meta.BudgetMaxTokens = maxTokens
		if window > 0 {
			sec := int64(window / time.Second)
			if sec < 1 {
				sec = 1
			}
			m.records[i].meta.BudgetWindowSec = sec
		} else {
			m.records[i].meta.BudgetWindowSec = 0
		}
		return nil
	}
	return fmt.Errorf("gateway key %d not found", id)
}

func (m *MemoryStore) BudgetUsed(ctx context.Context, keyID int64, window time.Duration) (int64, error) {
	_ = ctx
	if window <= 0 {
		window = DefaultBudgetWindow
	}
	since := time.Now().Add(-window).UnixMilli()
	var sum int64
	for _, c := range m.chips {
		if c.keyID == keyID && c.createdAtMs >= since {
			sum += c.tokens
		}
	}
	return sum, nil
}

func (m *MemoryStore) BudgetRecord(ctx context.Context, keyID int64, principalID string, tokens int64) error {
	_ = ctx
	if tokens <= 0 || keyID <= 0 {
		return nil
	}
	m.chips = append(m.chips, budgetChip{
		keyID: keyID, principalID: principalID, tokens: tokens,
		createdAtMs: time.Now().UnixMilli(),
	})
	return nil
}
