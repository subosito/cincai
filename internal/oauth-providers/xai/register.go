package xai

import (
	oauthpack "github.com/subosito/cincai/credential/oauth/pack"
)

func init() {
	mod := Module{}
	// "xai" is the vendor module id; "xai-oauth" is the broker/providers.yaml
	// credential_profile used in prod (ddd). Both must resolve so login,
	// manual refresh, and on-read auto-refresh can find the module.
	oauthpack.Register(oauthpack.Entry{
		Profiles: []string{"xai", "xai-oauth"},
		Login:    mod.Login,
		Refresh:  mod.Refresh,
		Callback: oauthpack.Callback{Host: redirectHost, Port: redirectPort, Path: redirectPath},
	})
}
