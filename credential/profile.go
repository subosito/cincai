package credential

import (
	"fmt"
	"strings"

	oauthmod "github.com/subosito/cincai/internal/oauth"

	"github.com/subosito/cincai/credential/oauth/vendor"
	"github.com/subosito/cincai/gateway"
)

// VendorProfiles lists OAuth profile names accepted by Login.
func VendorProfiles() []string { return oauthmod.VendorProfiles() }

// OAuthProfile resolves login/refresh for vendor OAuth profiles.
// Kind is inferred from the CLI (login vs import), not host yaml — secrets live in broker.db.
// Profile names must match providers.yaml credential_profile and vendor modules (oauth/register).
func OAuthProfile(_ *gateway.ConfigFile, profile string) (gateway.Profile, error) {
	profile = strings.TrimSpace(profile)
	if profile == "" {
		return gateway.Profile{}, fmt.Errorf("profile is required")
	}
	if _, ok := vendor.ForProvider(profile); ok {
		return gateway.Profile{Kind: "oauth"}, nil
	}
	for _, id := range oauthmod.VendorProfiles() {
		if id == profile {
			return gateway.Profile{Kind: "oauth"}, nil
		}
	}
	return gateway.Profile{}, fmt.Errorf("profile %q: use cincai credential login for vendor oauth (%s); api keys use credential import",
		profile, strings.Join(oauthmod.VendorProfiles(), ", "))
}