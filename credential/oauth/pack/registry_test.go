package pack_test

import (
	"testing"

	"github.com/subosito/cincai/credential/oauth/pack"
	_ "github.com/subosito/cincai/link"
)

func TestRegisteredOAuthProviders(t *testing.T) {
	t.Parallel()
	profiles := make(map[string]bool)
	for _, e := range pack.Entries() {
		for _, p := range e.Profiles {
			profiles[p] = true
		}
	}
	for _, want := range []string{"xai-oauth"} {
		if !profiles[want] {
			t.Fatalf("missing profile %q; got %v", want, profiles)
		}
	}
}
