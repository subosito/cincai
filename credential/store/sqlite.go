package store

import (
	"context"
	"database/sql"
	"embed"
	"fmt"
	"os"
	"time"

	"github.com/subosito/cincai/credential/seal"
	_ "modernc.org/sqlite"
)

//go:embed schema.sql
var schemaFS embed.FS

// SQLite opens an encrypted credential vault.
type SQLite struct {
	db  *sql.DB
	key seal.Key
}

func OpenSQLite(path string, key seal.Key) (*SQLite, error) {
	if err := ensurePrivateFile(path); err != nil {
		return nil, err
	}
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	// WAL lets readers and a writer proceed concurrently, and busy_timeout makes a
	// contended write wait instead of failing with SQLITE_BUSY — needed when more than
	// one handle shares the broker (e.g. `cincai serve` alongside the CLI, or a host that
	// runs its own management plane on the same broker.db).
	if _, err := db.Exec(`PRAGMA journal_mode=WAL; PRAGMA busy_timeout=5000;`); err != nil {
		db.Close()
		return nil, err
	}
	s := &SQLite{db: db, key: key}
	if err := s.migrate(); err != nil {
		db.Close()
		return nil, err
	}
	return s, nil
}

// ensurePrivateFile creates the broker at 0600 before SQLite touches it, and tightens
// an existing file. Beyond the encrypted credential blobs the DB stores plaintext
// identity keys (PII) and gateway-key hashes, so a world-readable file leaks them.
func ensurePrivateFile(path string) error {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	// O_CREATE only applies the mode on creation; fix files created before this.
	return os.Chmod(path, 0o600)
}

func (s *SQLite) migrate() error {
	schema, err := schemaFS.ReadFile("schema.sql")
	if err != nil {
		return err
	}
	if _, err := s.db.Exec(string(schema)); err != nil {
		return err
	}
	var n int
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM snapshot_meta WHERE id = 1`).Scan(&n); err != nil {
		return err
	}
	if n == 0 {
		now := time.Now().UnixMilli()
		_, err = s.db.Exec(`INSERT INTO snapshot_meta (id, generation, generated_at) VALUES (1, 1, ?)`, now)
		if err != nil {
			return err
		}
	}
	return s.migrateV2()
}

func (s *SQLite) migrateV2() error {
	var ver int
	_ = s.db.QueryRow(`SELECT COALESCE(MAX(version), 0) FROM schema_migrations`).Scan(&ver)
	if ver >= 2 {
		return nil
	}
	_, _ = s.db.Exec(`ALTER TABLE gateway_keys ADD COLUMN scopes TEXT`)
	_, err := s.db.Exec(`INSERT INTO schema_migrations (version) VALUES (2)`)
	return err
}

func (s *SQLite) Close() error { return s.db.Close() }

// DB exposes the underlying sqlite handle (shared with keyring).
func (s *SQLite) DB() *sql.DB { return s.db }

func (s *SQLite) BumpGeneration(ctx context.Context) error {
	now := time.Now().UnixMilli()
	_, err := s.db.ExecContext(ctx, `UPDATE snapshot_meta SET generation = generation + 1, generated_at = ? WHERE id = 1`, now)
	return err
}

func (s *SQLite) SnapshotMeta(ctx context.Context) (SnapshotMeta, error) {
	var gen, at int64
	err := s.db.QueryRowContext(ctx, `SELECT generation, generated_at FROM snapshot_meta WHERE id = 1`).Scan(&gen, &at)
	if err != nil {
		return SnapshotMeta{}, err
	}
	return SnapshotMeta{Generation: gen, GeneratedAt: at, ServerNowMs: time.Now().UnixMilli()}, nil
}

func (s *SQLite) decryptData(dataEnc string) ([]byte, error) {
	return s.key.Decrypt([]byte(dataEnc))
}

func (s *SQLite) materialFromRow(profile, kind, dataEnc, identity string) (Material, error) {
	raw, err := s.decryptData(dataEnc)
	if err != nil {
		return Material{}, err
	}
	return MaterialFromDecrypted(profile, Kind(kind), raw)
}

func (s *SQLite) Get(ctx context.Context, profile string) (Material, error) {
	var id int64
	var kind, data, disabled sql.NullString
	err := s.db.QueryRowContext(ctx, `
		SELECT id, type, data, disabled_cause FROM credentials
		WHERE provider = ? ORDER BY id DESC LIMIT 1`, profile).Scan(&id, &kind, &data, &disabled)
	if err == sql.ErrNoRows {
		return Material{}, fmt.Errorf("credential profile %q not found", profile)
	}
	if err != nil {
		return Material{}, err
	}
	if disabled.Valid && disabled.String != "" {
		return Material{}, fmt.Errorf("credential %d disabled: %s", id, disabled.String)
	}
	return s.materialFromRow(profile, kind.String, data.String, "")
}

func (s *SQLite) PutAPIKey(ctx context.Context, profile, key string) (int64, error) {
	enc, err := EncryptPayload(s.key, apiKeyPayload(key))
	if err != nil {
		return 0, err
	}
	now := time.Now().UnixMilli()
	res, err := s.db.ExecContext(ctx, `
		INSERT INTO credentials (provider, type, data, created_at, updated_at)
		VALUES (?, 'api_key', ?, ?, ?)`, profile, string(enc), now, now)
	if err != nil {
		return 0, err
	}
	id, _ := res.LastInsertId()
	_ = s.BumpGeneration(ctx)
	return id, nil
}

func (s *SQLite) PutOAuth(ctx context.Context, profile string, mat Material) (int64, error) {
	enc, err := EncryptPayload(s.key, oauthMaterialPayload(mat))
	if err != nil {
		return 0, err
	}
	now := time.Now().UnixMilli()
	var identity any
	if mat.Email != "" {
		identity = mat.Email
	}
	res, err := s.db.ExecContext(ctx, `
		INSERT INTO credentials (provider, type, data, identity_key, created_at, updated_at)
		VALUES (?, 'oauth', ?, ?, ?, ?)`, profile, string(enc), identity, now, now)
	if err != nil {
		return 0, err
	}
	id, _ := res.LastInsertId()
	_ = s.BumpGeneration(ctx)
	return id, nil
}

func (s *SQLite) scanSummary(rows *sql.Rows) ([]CredentialSummary, error) {
	var out []CredentialSummary
	for rows.Next() {
		var cs CredentialSummary
		var kind string
		var identity, disabled sql.NullString
		var created, updated int64
		if err := rows.Scan(&cs.ID, &cs.Profile, &kind, &identity, &disabled, &created, &updated); err != nil {
			return nil, err
		}
		cs.Kind = Kind(kind)
		cs.Status = "active"
		if disabled.Valid && disabled.String != "" {
			cs.Status = "disabled"
			c := disabled.String
			cs.DisabledCause = &c
		}
		if identity.Valid && identity.String != "" {
			i := identity.String
			cs.IdentityKey = &i
		}
		cs.CreatedAt = created
		cs.UpdatedAt = updated
		out = append(out, cs)
	}
	return out, rows.Err()
}

func (s *SQLite) ListSummaries(ctx context.Context) ([]CredentialSummary, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, provider, type, identity_key, disabled_cause, created_at, updated_at
		FROM credentials ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return s.scanSummary(rows)
}

func (s *SQLite) GetSummary(ctx context.Context, id int64) (CredentialSummary, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, provider, type, identity_key, disabled_cause, created_at, updated_at
		FROM credentials WHERE id = ?`, id)
	if err != nil {
		return CredentialSummary{}, err
	}
	defer rows.Close()
	list, err := s.scanSummary(rows)
	if err != nil {
		return CredentialSummary{}, err
	}
	if len(list) == 0 {
		return CredentialSummary{}, fmt.Errorf("credential %d not found", id)
	}
	return list[0], nil
}

func (s *SQLite) UpdateOAuth(ctx context.Context, profile string, mat Material) error {
	var id int64
	err := s.db.QueryRowContext(ctx, `
		SELECT id FROM credentials WHERE provider = ? AND type = 'oauth'
		ORDER BY id DESC LIMIT 1`, profile).Scan(&id)
	if err == sql.ErrNoRows {
		return fmt.Errorf("oauth credential profile %q not found", profile)
	}
	if err != nil {
		return err
	}
	enc, err := EncryptPayload(s.key, oauthMaterialPayload(mat))
	if err != nil {
		return err
	}
	now := time.Now().UnixMilli()
	var identity any
	if mat.Email != "" {
		identity = mat.Email
	}
	res, err := s.db.ExecContext(ctx, `
		UPDATE credentials SET data = ?, identity_key = ?, updated_at = ?, disabled_cause = NULL
		WHERE id = ?`, string(enc), identity, now, id)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("credential %d not found", id)
	}
	return s.BumpGeneration(ctx)
}

func (s *SQLite) Disable(ctx context.Context, id int64, cause string) error {
	now := time.Now().UnixMilli()
	res, err := s.db.ExecContext(ctx, `
		UPDATE credentials SET disabled_cause = ?, updated_at = ? WHERE id = ?`, cause, now, id)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("credential %d not found", id)
	}
	return s.BumpGeneration(ctx)
}

func (s *SQLite) Enable(ctx context.Context, id int64) error {
	now := time.Now().UnixMilli()
	res, err := s.db.ExecContext(ctx, `
		UPDATE credentials SET disabled_cause = NULL, updated_at = ? WHERE id = ?`, now, id)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("credential %d not found", id)
	}
	return s.BumpGeneration(ctx)
}
