package credential

import (
	"context"

	"github.com/subosito/cincai/credential/store"
)

// ListSummaries returns metadata-only credential rows from the encrypted broker.
func ListSummaries(ctx context.Context, configPath string) ([]store.CredentialSummary, error) {
	vault, err := LoadVault(configPath)
	if err != nil {
		return nil, err
	}
	defer vault.Close()
	return vault.Store.ListSummaries(ctx)
}
