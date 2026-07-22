package xai

import (
	oauthpack "github.com/subosito/cincai/credential/oauth/pack"
)

func init() {
	mod := Module{}
	// OAuth vault / providers.yaml credential_profile always uses the -oauth
	// suffix. Catalog provider id stays "xai" (no suffix).
	oauthpack.Register(oauthpack.Entry{
		Profiles: []string{"xai-oauth"},
		Login:    mod.Login,
		Refresh:  mod.Refresh,
		Callback: oauthpack.Callback{Host: redirectHost, Port: redirectPort, Path: redirectPath},
	})
}
