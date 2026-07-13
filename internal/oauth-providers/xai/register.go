package xai

import (
	oauthpack "github.com/subosito/cincai/credential/oauth/pack"
)

func init() {
	mod := Module{}
	oauthpack.Register(oauthpack.Entry{
		Profiles: []string{"xai"},
		Login:    mod.Login,
		Refresh:  mod.Refresh,
		Callback: oauthpack.Callback{Host: redirectHost, Port: redirectPort, Path: redirectPath},
	})
}