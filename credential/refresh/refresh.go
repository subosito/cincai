// Package refresh decorates a credential store, renewing near-expiry vendor
// OAuth access tokens on read. It is the default wrapper for the sqlite broker
// so a running gateway never forwards an expired access token while a valid
// refresh token exists — no background loop, no manual `credential refresh`.
package refresh

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/subosito/cincai/credential/oauth/vendor"
	"github.com/subosito/cincai/credential/store"
)

// DefaultSkew renews a token this long before it actually expires, absorbing
// clock skew and request latency.
const DefaultSkew = 60 * time.Second

// refreshFunc renews OAuth material for a profile. Defaults to the vendor registry.
type refreshFunc func(ctx context.Context, profile string, cur store.Material) (store.Material, error)

// Store wraps an inner credential store with on-demand OAuth refresh. Every
// method except Get passes through to the inner store unchanged.
type Store struct {
	store.Store

	skew        time.Duration
	now         func() time.Time
	refresh     refreshFunc
	refreshable func(profile string) bool

	mu    sync.Mutex
	locks map[string]*sync.Mutex
}

// New wraps inner, refreshing vendor OAuth tokens via the registered vendor modules.
func New(inner store.Store) *Store {
	return &Store{
		Store: inner,
		skew:  DefaultSkew,
		now:   time.Now,
		refresh: func(ctx context.Context, profile string, cur store.Material) (store.Material, error) {
			m, ok := vendor.ForProvider(profile)
			if !ok {
				return store.Material{}, fmt.Errorf("refresh: no vendor module for profile %q", profile)
			}
			return m.Refresh(ctx, profile, cur)
		},
		refreshable: func(profile string) bool {
			_, ok := vendor.ForProvider(profile)
			return ok
		},
		locks: make(map[string]*sync.Mutex),
	}
}

// Get returns credential material, first renewing it when it is a vendor OAuth
// token within the skew window of expiry. Refresh failures are soft: the stale
// token is returned so the upstream 401 surfaces the real cause rather than the
// gateway converting a transient refresh error into a hard failure.
func (s *Store) Get(ctx context.Context, profile string) (store.Material, error) {
	mat, err := s.Store.Get(ctx, profile)
	if err != nil || !s.needsRefresh(profile, mat) {
		return mat, err
	}
	return s.renew(ctx, profile, mat), nil
}

// BrokerDB exposes the inner sqlite handle so admin/keyring routes keep working
// through the wrapper (store.BrokerDB checks the BrokerBacked interface).
func (s *Store) BrokerDB() *sql.DB {
	db, _ := store.BrokerDB(s.Store)
	return db
}

func (s *Store) needsRefresh(profile string, m store.Material) bool {
	if m.Kind != store.KindOAuth || m.RefreshToken == "" || m.ExpiresAt.IsZero() {
		return false
	}
	if !s.refreshable(profile) {
		return false
	}
	return s.now().Add(s.skew).After(m.ExpiresAt)
}

func (s *Store) renew(ctx context.Context, profile string, cur store.Material) store.Material {
	mu := s.lockFor(profile)
	mu.Lock()
	defer mu.Unlock()

	// A concurrent request may have refreshed while we waited on the lock;
	// re-read and bail out if the token is already fresh.
	if latest, err := s.Store.Get(ctx, profile); err == nil {
		if !s.needsRefresh(profile, latest) {
			return latest
		}
		cur = latest
	}
	mat, err := s.refreshAndStore(ctx, profile, "on_demand", cur)
	if err != nil {
		slog.WarnContext(ctx, "oauth_refresh_failed", "profile", profile, "error", err)
		return cur // fail-soft: upstream 401 surfaces the real cause
	}
	return mat
}

// ForceRefresh renews profile's OAuth credential regardless of expiry, for the
// reactive path when an upstream returns 401 (an access token that lapsed
// between the proactive check and the call, or an early revocation). If a
// concurrent request already rotated the token — its access token differs from
// prev — that fresh token is returned without a second refresh. Profiles with
// no vendor refresher return prev unchanged.
func (s *Store) ForceRefresh(ctx context.Context, profile string, prev store.Material) (store.Material, error) {
	if !s.refreshable(profile) {
		return prev, nil
	}
	mu := s.lockFor(profile)
	mu.Lock()
	defer mu.Unlock()

	cur := prev
	if latest, err := s.Store.Get(ctx, profile); err == nil {
		if latest.AccessToken != "" && latest.AccessToken != prev.AccessToken {
			return latest, nil
		}
		cur = latest
	}
	return s.refreshAndStore(ctx, profile, "reactive_401", cur)
}

// refreshAndStore refreshes cur, persists the result, and returns it. The caller
// must hold lockFor(profile). A persist failure is non-fatal (the fresh token is
// still returned); a refresh failure is returned to the caller.
func (s *Store) refreshAndStore(ctx context.Context, profile, trigger string, cur store.Material) (store.Material, error) {
	mat, err := s.refresh(ctx, profile, cur)
	if err != nil {
		return store.Material{}, err
	}
	if err := s.Store.UpdateOAuth(ctx, profile, mat); err != nil {
		slog.WarnContext(ctx, "oauth_refresh_persist_failed", "profile", profile, "error", err)
		return mat, nil
	}
	slog.InfoContext(ctx, "oauth_refresh", "profile", profile, "trigger", trigger,
		"expires_in_ms", time.Until(mat.ExpiresAt).Milliseconds())
	return mat, nil
}

func (s *Store) lockFor(profile string) *sync.Mutex {
	s.mu.Lock()
	defer s.mu.Unlock()
	mu, ok := s.locks[profile]
	if !ok {
		mu = &sync.Mutex{}
		s.locks[profile] = mu
	}
	return mu
}
