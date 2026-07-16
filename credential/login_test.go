package credential_test

import (
	"testing"

	credpkg "github.com/subosito/cincai/credential"
	oauthmod "github.com/subosito/cincai/internal/oauth"
	oauthreg "github.com/subosito/cincai/internal/oauth/register"

	"github.com/subosito/cincai/gateway"
)

func TestOAuthProfileVendorWithoutYAML(t *testing.T) {
	oauthreg.Register()
	prof, err := credpkg.OAuthProfile(&gateway.ConfigFile{}, "xai")
	if err != nil {
		t.Fatal(err)
	}
	if prof.Kind != "oauth" {
		t.Fatalf("kind=%q", prof.Kind)
	}
}

func TestOAuthProfileUnknown(t *testing.T) {
	_, err := credpkg.OAuthProfile(&gateway.ConfigFile{}, "not-a-profile")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestVendorProfilesDocumented(t *testing.T) {
	got := make(map[string]bool, len(oauthmod.VendorProfiles()))
	for _, p := range oauthmod.VendorProfiles() {
		got[p] = true
	}
	for _, want := range []string{"xai"} {
		if !got[want] {
			t.Fatalf("missing vendor profile %q; got %v", want, oauthmod.VendorProfiles())
		}
	}
}
