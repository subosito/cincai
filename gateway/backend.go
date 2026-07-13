package gateway

import (
	"fmt"
	"sync"

	"github.com/subosito/cincai/credential/store"
)

// CredentialBackendOpener wraps sqlite with an optional named credential backend.
type CredentialBackendOpener func(f *ConfigFile, sqlite *store.SQLite) (store.Store, error)

var (
	backendMu      sync.RWMutex
	backendOpeners = map[string]CredentialBackendOpener{}
)

// RegisterCredentialBackend registers a named credential backend opener.
// Linked operator modules call this at init time.
func RegisterCredentialBackend(name string, open CredentialBackendOpener) {
	backendMu.Lock()
	defer backendMu.Unlock()
	backendOpeners[name] = open
}

func openCredentialBackend(name string, f *ConfigFile, sqlite *store.SQLite) (store.Store, error) {
	backendMu.RLock()
	open := backendOpeners[name]
	backendMu.RUnlock()
	if open == nil {
		return nil, fmt.Errorf("credential backend %q not registered (link the backend plugin in your operator binary)", name)
	}
	return open(f, sqlite)
}