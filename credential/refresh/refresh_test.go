package refresh

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/subosito/cincai/credential/store"
)

// fakeStore is an in-memory credential store. Only Get and UpdateOAuth carry
// behavior; the rest satisfy the store.Store interface as no-ops.
type fakeStore struct {
	mu  sync.Mutex
	mat map[string]store.Material
}

func newFakeStore() *fakeStore { return &fakeStore{mat: map[string]store.Material{}} }

func (f *fakeStore) Get(_ context.Context, profile string) (store.Material, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	m, ok := f.mat[profile]
	if !ok {
		return store.Material{}, fmt.Errorf("not found: %s", profile)
	}
	return m, nil
}

func (f *fakeStore) UpdateOAuth(_ context.Context, profile string, m store.Material) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.mat[profile] = m
	return nil
}

func (f *fakeStore) PutAPIKey(context.Context, string, string) (int64, error)        { return 0, nil }
func (f *fakeStore) PutOAuth(context.Context, string, store.Material) (int64, error) { return 0, nil }
func (f *fakeStore) ListSummaries(context.Context) ([]store.CredentialSummary, error) {
	return nil, nil
}
func (f *fakeStore) GetSummary(context.Context, int64) (store.CredentialSummary, error) {
	return store.CredentialSummary{}, nil
}
func (f *fakeStore) Disable(context.Context, int64, string) error { return nil }
func (f *fakeStore) Enable(context.Context, int64) error          { return nil }
func (f *fakeStore) SnapshotMeta(context.Context) (store.SnapshotMeta, error) {
	return store.SnapshotMeta{}, nil
}
func (f *fakeStore) BumpGeneration(context.Context) error { return nil }
func (f *fakeStore) Close() error                         { return nil }

var epoch = time.Unix(1_700_000_000, 0)

// wrap builds a Store with a fixed clock and injected refresh behavior, treating
// every profile as refreshable (isolates the decorator from the vendor registry).
func wrap(inner store.Store, refresh refreshFunc) *Store {
	s := New(inner)
	s.now = func() time.Time { return epoch }
	s.refresh = refresh
	s.refreshable = func(string) bool { return true }
	return s
}

func oauth(access, refreshTok string, exp time.Time) store.Material {
	return store.Material{
		Profile:      "xai",
		Kind:         store.KindOAuth,
		AccessToken:  access,
		RefreshToken: refreshTok,
		ExpiresAt:    exp,
	}
}

func TestGet_freshTokenNotRefreshed(t *testing.T) {
	inner := newFakeStore()
	inner.mat["xai"] = oauth("old", "r", epoch.Add(time.Hour))
	var calls int32
	s := wrap(inner, func(context.Context, string, store.Material) (store.Material, error) {
		atomic.AddInt32(&calls, 1)
		return store.Material{}, nil
	})

	got, err := s.Get(context.Background(), "xai")
	if err != nil {
		t.Fatal(err)
	}
	if got.AccessToken != "old" {
		t.Fatalf("access = %q, want old (unchanged)", got.AccessToken)
	}
	if n := atomic.LoadInt32(&calls); n != 0 {
		t.Fatalf("refresh calls = %d, want 0", n)
	}
}

func TestGet_nearExpiryRefreshesAndPersists(t *testing.T) {
	inner := newFakeStore()
	inner.mat["xai"] = oauth("old", "r", epoch.Add(10*time.Second)) // inside 60s skew
	s := wrap(inner, func(_ context.Context, p string, _ store.Material) (store.Material, error) {
		return oauth("new", "r2", epoch.Add(time.Hour)), nil
	})

	got, err := s.Get(context.Background(), "xai")
	if err != nil {
		t.Fatal(err)
	}
	if got.AccessToken != "new" {
		t.Fatalf("access = %q, want new", got.AccessToken)
	}
	stored, _ := inner.Get(context.Background(), "xai")
	if stored.AccessToken != "new" || stored.RefreshToken != "r2" {
		t.Fatalf("stored = %q/%q, want new/r2 (rotation persisted)", stored.AccessToken, stored.RefreshToken)
	}
}

func TestGet_concurrentRefreshesOnce(t *testing.T) {
	inner := newFakeStore()
	inner.mat["xai"] = oauth("old", "r", epoch.Add(10*time.Second))
	var calls int32
	s := wrap(inner, func(_ context.Context, p string, _ store.Material) (store.Material, error) {
		atomic.AddInt32(&calls, 1)
		time.Sleep(20 * time.Millisecond) // hold the lock so callers pile up on it
		return oauth("new", "r", epoch.Add(time.Hour)), nil
	})

	const n = 8
	var wg sync.WaitGroup
	got := make([]string, n)
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			m, _ := s.Get(context.Background(), "xai")
			got[i] = m.AccessToken
		}(i)
	}
	wg.Wait()

	if c := atomic.LoadInt32(&calls); c != 1 {
		t.Fatalf("refresh calls = %d, want 1 (single-flight per profile)", c)
	}
	for i, a := range got {
		if a != "new" {
			t.Fatalf("goroutine %d saw access = %q, want new", i, a)
		}
	}
}

func TestGet_refreshFailureReturnsStale(t *testing.T) {
	inner := newFakeStore()
	inner.mat["xai"] = oauth("old", "r", epoch.Add(10*time.Second))
	s := wrap(inner, func(context.Context, string, store.Material) (store.Material, error) {
		return store.Material{}, fmt.Errorf("boom")
	})

	got, err := s.Get(context.Background(), "xai")
	if err != nil {
		t.Fatalf("Get error = %v, want nil (fail-soft)", err)
	}
	if got.AccessToken != "old" {
		t.Fatalf("access = %q, want stale old on refresh failure", got.AccessToken)
	}
}

func TestGet_apiKeyNotRefreshed(t *testing.T) {
	inner := newFakeStore()
	inner.mat["deepseek-api"] = store.Material{Profile: "deepseek-api", Kind: store.KindAPIKey, APIKey: "sk-x"}
	var calls int32
	s := wrap(inner, func(context.Context, string, store.Material) (store.Material, error) {
		atomic.AddInt32(&calls, 1)
		return store.Material{}, nil
	})

	got, err := s.Get(context.Background(), "deepseek-api")
	if err != nil {
		t.Fatal(err)
	}
	if got.APIKey != "sk-x" {
		t.Fatalf("apikey = %q, want sk-x", got.APIKey)
	}
	if n := atomic.LoadInt32(&calls); n != 0 {
		t.Fatalf("refresh calls = %d, want 0 for api key", n)
	}
}

func TestForceRefresh_refreshesRegardlessOfExpiry(t *testing.T) {
	inner := newFakeStore()
	prev := oauth("old", "r", epoch.Add(time.Hour)) // NOT near expiry
	inner.mat["xai"] = prev
	s := wrap(inner, func(_ context.Context, p string, _ store.Material) (store.Material, error) {
		return oauth("new", "r2", epoch.Add(time.Hour)), nil
	})

	got, err := s.ForceRefresh(context.Background(), "xai", prev)
	if err != nil {
		t.Fatal(err)
	}
	if got.AccessToken != "new" {
		t.Fatalf("access = %q, want new (forced despite not near expiry)", got.AccessToken)
	}
	stored, _ := inner.Get(context.Background(), "xai")
	if stored.AccessToken != "new" {
		t.Fatalf("stored = %q, want new (persisted)", stored.AccessToken)
	}
}

func TestForceRefresh_skipsWhenConcurrentlyRotated(t *testing.T) {
	inner := newFakeStore()
	prev := oauth("stale-401", "r", epoch.Add(10*time.Second))
	// The store already holds a newer token than the one that got the 401.
	inner.mat["xai"] = oauth("rotated", "r2", epoch.Add(time.Hour))
	var calls int32
	s := wrap(inner, func(context.Context, string, store.Material) (store.Material, error) {
		atomic.AddInt32(&calls, 1)
		return store.Material{}, nil
	})

	got, err := s.ForceRefresh(context.Background(), "xai", prev)
	if err != nil {
		t.Fatal(err)
	}
	if got.AccessToken != "rotated" {
		t.Fatalf("access = %q, want rotated (reuse concurrent refresh)", got.AccessToken)
	}
	if n := atomic.LoadInt32(&calls); n != 0 {
		t.Fatalf("refresh calls = %d, want 0 (already rotated)", n)
	}
}

func TestForceRefresh_nonRefreshableReturnsPrev(t *testing.T) {
	inner := newFakeStore()
	prev := oauth("old", "r", epoch.Add(10*time.Second))
	inner.mat["xai"] = prev
	var calls int32
	s := wrap(inner, func(context.Context, string, store.Material) (store.Material, error) {
		atomic.AddInt32(&calls, 1)
		return store.Material{}, nil
	})
	s.refreshable = func(string) bool { return false }

	got, err := s.ForceRefresh(context.Background(), "xai", prev)
	if err != nil {
		t.Fatal(err)
	}
	if got.AccessToken != "old" {
		t.Fatalf("access = %q, want old (no refresher)", got.AccessToken)
	}
	if n := atomic.LoadInt32(&calls); n != 0 {
		t.Fatalf("refresh calls = %d, want 0", n)
	}
}

func TestForceRefresh_errorPropagates(t *testing.T) {
	inner := newFakeStore()
	prev := oauth("old", "r", epoch.Add(10*time.Second))
	inner.mat["xai"] = prev
	s := wrap(inner, func(context.Context, string, store.Material) (store.Material, error) {
		return store.Material{}, fmt.Errorf("boom")
	})

	if _, err := s.ForceRefresh(context.Background(), "xai", prev); err == nil {
		t.Fatal("ForceRefresh error = nil, want propagated error")
	}
}

func TestGet_nonVendorProfileNotRefreshed(t *testing.T) {
	inner := newFakeStore()
	inner.mat["xai"] = oauth("old", "r", epoch.Add(10*time.Second))
	var calls int32
	s := wrap(inner, func(context.Context, string, store.Material) (store.Material, error) {
		atomic.AddInt32(&calls, 1)
		return store.Material{}, nil
	})
	s.refreshable = func(string) bool { return false } // no vendor module registered

	got, err := s.Get(context.Background(), "xai")
	if err != nil {
		t.Fatal(err)
	}
	if got.AccessToken != "old" {
		t.Fatalf("access = %q, want old (no refresher)", got.AccessToken)
	}
	if n := atomic.LoadInt32(&calls); n != 0 {
		t.Fatalf("refresh calls = %d, want 0 when not refreshable", n)
	}
}
