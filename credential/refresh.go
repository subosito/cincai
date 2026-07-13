package credential

import (
	"context"

	"github.com/subosito/cincai/credential/oauth/vendor"
)

// Refresh re-mints profile's OAuth access token from its refresh token and
// persists it to the vault. It is the manual counterpart to the gateway's
// automatic on-read refresh, used by `credential refresh` CLIs.
func Refresh(ctx context.Context, vault *Vault, profile string) error {
	prof, err := OAuthProfile(vault.Config, profile)
	if err != nil {
		return err
	}
	cur, err := vault.Store.Get(ctx, profile)
	if err != nil {
		return err
	}
	mat, err := vendor.Refresh(ctx, profile, prof, cur)
	if err != nil {
		return err
	}
	return vault.Store.UpdateOAuth(ctx, profile, mat)
}
