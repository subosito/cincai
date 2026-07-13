package store

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/subosito/cincai/credential/seal"
)

type row struct {
	id            int64
	profile       string
	kind          Kind
	identity      string
	disabledCause string
	createdAt     int64
	updatedAt     int64
	dataEnc       []byte
}

// Memory is an in-memory encrypted credential store (no disk; process-local).
type Memory struct {
	key    seal.Key
	mu     sync.Mutex
	rows   map[int64]*row
	byProf map[string]int64
	nextID int64
	gen    int64
	genAt  int64
}

func NewMemory(key seal.Key) *Memory {
	now := time.Now().UnixMilli()
	return &Memory{
		key:    key,
		rows:   make(map[int64]*row),
		byProf: make(map[string]int64),
		gen:    1,
		genAt:  now,
	}
}

func (m *Memory) Close() error { return nil }

func (m *Memory) BumpGeneration(ctx context.Context) error {
	_ = ctx
	m.mu.Lock()
	defer m.mu.Unlock()
	m.gen++
	m.genAt = time.Now().UnixMilli()
	return nil
}

func (m *Memory) SnapshotMeta(ctx context.Context) (SnapshotMeta, error) {
	_ = ctx
	m.mu.Lock()
	defer m.mu.Unlock()
	return SnapshotMeta{
		Generation:  m.gen,
		GeneratedAt: m.genAt,
		ServerNowMs: time.Now().UnixMilli(),
	}, nil
}

func (m *Memory) decryptRow(r *row) (Material, error) {
	raw, err := m.key.Decrypt(r.dataEnc)
	if err != nil {
		return Material{}, err
	}
	return MaterialFromDecrypted(r.profile, r.kind, raw)
}

func (m *Memory) Get(ctx context.Context, profile string) (Material, error) {
	_ = ctx
	m.mu.Lock()
	defer m.mu.Unlock()
	id, ok := m.byProf[profile]
	if !ok {
		return Material{}, fmt.Errorf("credential profile %q not found", profile)
	}
	r := m.rows[id]
	if r.disabledCause != "" {
		return Material{}, fmt.Errorf("credential %d disabled: %s", id, r.disabledCause)
	}
	return m.decryptRow(r)
}

func (m *Memory) PutAPIKey(ctx context.Context, profile, key string) (int64, error) {
	_ = ctx
	enc, err := EncryptPayload(m.key, apiKeyPayload(key))
	if err != nil {
		return 0, err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	now := time.Now().UnixMilli()
	id := m.nextID + 1
	m.nextID = id
	m.rows[id] = &row{
		id: id, profile: profile, kind: KindAPIKey,
		createdAt: now, updatedAt: now, dataEnc: enc,
	}
	m.byProf[profile] = id
	m.gen++
	m.genAt = now
	return id, nil
}

func (m *Memory) PutOAuth(ctx context.Context, profile string, mat Material) (int64, error) {
	_ = ctx
	enc, err := EncryptPayload(m.key, oauthMaterialPayload(mat))
	if err != nil {
		return 0, err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	now := time.Now().UnixMilli()
	id := m.nextID + 1
	m.nextID = id
	identity := mat.Email
	m.rows[id] = &row{
		id: id, profile: profile, kind: KindOAuth, identity: identity,
		createdAt: now, updatedAt: now, dataEnc: enc,
	}
	m.byProf[profile] = id
	m.gen++
	m.genAt = now
	return id, nil
}

func (m *Memory) summary(r *row) CredentialSummary {
	st := "active"
	var cause *string
	if r.disabledCause != "" {
		st = "disabled"
		c := r.disabledCause
		cause = &c
	}
	var idk *string
	if r.identity != "" {
		i := r.identity
		idk = &i
	}
	return CredentialSummary{
		ID: r.id, Profile: r.profile, Kind: r.kind, Status: st,
		IdentityKey: idk, DisabledCause: cause,
		CreatedAt: r.createdAt, UpdatedAt: r.updatedAt,
	}
}

func (m *Memory) ListSummaries(ctx context.Context) ([]CredentialSummary, error) {
	_ = ctx
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]CredentialSummary, 0, len(m.rows))
	for _, r := range m.rows {
		out = append(out, m.summary(r))
	}
	return out, nil
}

func (m *Memory) GetSummary(ctx context.Context, id int64) (CredentialSummary, error) {
	_ = ctx
	m.mu.Lock()
	defer m.mu.Unlock()
	r, ok := m.rows[id]
	if !ok {
		return CredentialSummary{}, fmt.Errorf("credential %d not found", id)
	}
	return m.summary(r), nil
}

func (m *Memory) UpdateOAuth(ctx context.Context, profile string, mat Material) error {
	_ = ctx
	m.mu.Lock()
	defer m.mu.Unlock()
	id, ok := m.byProf[profile]
	if !ok {
		return fmt.Errorf("oauth credential profile %q not found", profile)
	}
	r := m.rows[id]
	if r.kind != KindOAuth {
		return fmt.Errorf("credential profile %q is not oauth", profile)
	}
	enc, err := EncryptPayload(m.key, oauthMaterialPayload(mat))
	if err != nil {
		return err
	}
	now := time.Now().UnixMilli()
	r.dataEnc = enc
	r.updatedAt = now
	if mat.Email != "" {
		r.identity = mat.Email
	}
	r.disabledCause = ""
	m.gen++
	m.genAt = now
	return nil
}

func (m *Memory) Disable(ctx context.Context, id int64, cause string) error {
	_ = ctx
	m.mu.Lock()
	defer m.mu.Unlock()
	r, ok := m.rows[id]
	if !ok {
		return fmt.Errorf("credential %d not found", id)
	}
	r.disabledCause = cause
	r.updatedAt = time.Now().UnixMilli()
	m.gen++
	m.genAt = r.updatedAt
	return nil
}

func (m *Memory) Enable(ctx context.Context, id int64) error {
	_ = ctx
	m.mu.Lock()
	defer m.mu.Unlock()
	r, ok := m.rows[id]
	if !ok {
		return fmt.Errorf("credential %d not found", id)
	}
	r.disabledCause = ""
	r.updatedAt = time.Now().UnixMilli()
	m.gen++
	m.genAt = r.updatedAt
	return nil
}