// Package pack collects OAuth vendor module registrations from oauth-providers/* init().
//
// Cincai's own providers live under internal/oauth-providers/*. Downstream binaries
// register extra providers the same way: call Register from an init(), blank-import
// the package, and cincai/register.Register drains the entries into
// credential/oauth/vendor.
package pack

import (
	"context"

	"github.com/subosito/cincai/credential/oauth/flow"
	"github.com/subosito/cincai/credential/oauth/wire"
)

type LoginFn func(context.Context, flow.Controller) (wire.OAuthPayload, error)
type RefreshFn func(context.Context, wire.OAuthPayload) (wire.OAuthPayload, error)

// Callback is a vendor's fixed loopback OAuth callback. It is optional and used only for
// the remote-login (SSH port-forward) hints; the login flow itself owns the live listener.
type Callback struct {
	Host string
	Port int
	Path string
}

// Entry wires one vendor module to one or more broker profile names (providers.yaml credential_profile).
type Entry struct {
	Profiles []string
	Login    LoginFn
	Refresh  RefreshFn
	Device   LoginFn  // optional
	Callback Callback // optional: loopback callback for remote-login hints
}

var entries []Entry

func Register(e Entry) {
	if e.Login == nil || e.Refresh == nil || len(e.Profiles) == 0 {
		return
	}
	entries = append(entries, e)
}

func Entries() []Entry {
	out := make([]Entry, len(entries))
	copy(out, entries)
	return out
}