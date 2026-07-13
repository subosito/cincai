// Package vendor is the internal registry that binds vendor OAuth modules to the
// credential store. It is wired automatically by register.Register().
//
// To add your own OAuth provider, register it with credential/oauth/pack — do not
// depend on this package directly.
package vendor

import (
	"context"
	"fmt"
	"sync"

	"github.com/subosito/cincai/credential/oauth/generic"
	"github.com/subosito/cincai/credential/store"
	"github.com/subosito/cincai/internal/config"
)

// Module performs vendor-specific OAuth login and refresh.
type Module interface {
	Login(ctx context.Context, profile string, flow generic.Flow, callbackAddr string, ctrl generic.Controller) (store.Material, error)
	Refresh(ctx context.Context, profile string, cur store.Material) (store.Material, error)
}

var (
	mu      sync.RWMutex
	modules = map[string]Module{}
)

// Register binds a vendor module to one or more credential profile ids.
func Register(providerID string, m Module) {
	mu.Lock()
	defer mu.Unlock()
	modules[providerID] = m
}

// ForProvider returns a registered vendor module.
func ForProvider(providerID string) (Module, bool) {
	mu.RLock()
	defer mu.RUnlock()
	m, ok := modules[providerID]
	return m, ok
}

// Login uses a vendor module when registered, otherwise generic OAuth2.
func Login(ctx context.Context, profile string, prof config.Profile, flow generic.Flow, callbackAddr string, ctrl generic.Controller) (store.Material, error) {
	if m, ok := ForProvider(profile); ok {
		return m.Login(ctx, profile, flow, callbackAddr, ctrl)
	}
	return generic.Login(ctx, profile, prof, flow, callbackAddr, ctrl)
}

// Refresh uses a vendor module when registered, otherwise generic refresh exchange only.
func Refresh(ctx context.Context, profile string, prof config.Profile, cur store.Material) (store.Material, error) {
	if m, ok := ForProvider(profile); ok {
		return m.Refresh(ctx, profile, cur)
	}
	refreshed, err := generic.Refresh(ctx, prof.OAuth, cur.RefreshToken)
	if err != nil {
		return store.Material{}, err
	}
	refreshed.Profile = profile
	if refreshed.Email == "" {
		refreshed.Email = cur.Email
	}
	refreshed.Extras = store.MergeExtras(cur.Extras, refreshed.Extras)
	return refreshed, nil
}

// OAuthProviders returns sorted registered profile ids.
func OAuthProviders() []string {
	mu.RLock()
	defer mu.RUnlock()
	out := make([]string, 0, len(modules))
	for id := range modules {
		out = append(out, id)
	}
	sortStrings(out)
	return out
}

func sortStrings(s []string) {
	for i := 1; i < len(s); i++ {
		for j := i; j > 0 && s[j-1] > s[j]; j-- {
			s[j-1], s[j] = s[j], s[j-1]
		}
	}
}

// MustRegister panics when provider id is empty.
func MustRegister(providerID string, m Module) {
	if providerID == "" || m == nil {
		panic("vendor: invalid Register")
	}
	Register(providerID, m)
}

// LoginOrErr wraps missing vendor as error for callers that require registration.
func LoginOrErr(ctx context.Context, profile string, prof config.Profile, flow generic.Flow, callbackAddr string, ctrl generic.Controller) (store.Material, error) {
	mat, err := Login(ctx, profile, prof, flow, callbackAddr, ctrl)
	if err != nil {
		return store.Material{}, err
	}
	if mat.AccessToken == "" {
		return store.Material{}, fmt.Errorf("oauth login %q: empty access token", profile)
	}
	return mat, nil
}