package anthropic

import (
	"net/http"

	"github.com/subosito/cincai/credential/inject"
	oauthpack "github.com/subosito/cincai/credential/oauth/pack"
)

func init() {
	mod := Module{}
	oauthpack.Register(oauthpack.Entry{
		Profiles: []string{"anthropic"},
		Login:    mod.Login,
		Refresh:  mod.Refresh,
		Callback: oauthpack.Callback{Host: "localhost", Port: callbackPort, Path: callbackPath},
	})
	// Anthropic OAuth access tokens require the oauth beta header on upstream requests;
	// catalogs select it with inject_preset: anthropic_oauth.
	inject.RegisterOAuthPreset("anthropic_oauth", func(r *http.Request) {
		r.Header.Set("anthropic-beta", "oauth-2025-04-20")
	})
}
