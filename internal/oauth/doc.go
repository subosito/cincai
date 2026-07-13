// Package oauth hosts generic Cincai OAuth machinery (bridge, remote UX).
//
// Vendor login/refresh packs live under internal/oauth-providers/* and register via
// credential/oauth/pack + link (same pattern as adapters/pack/link).
// Wire them into cincai via internal/oauth/register (cincai serve / credential login).
package oauth

import oauthpack "github.com/subosito/cincai/credential/oauth/pack"

// VendorProfiles returns the profile names wired through registered vendor OAuth modules
// (credential login). Derived from the registrations — providers are the single source.
func VendorProfiles() []string {
	var out []string
	for _, e := range oauthpack.Entries() {
		out = append(out, e.Profiles...)
	}
	return out
}
