// Package register wires Cincai OAuth vendor modules into cincai.
package register

import (
	"sync"

	"github.com/subosito/cincai/internal/oauth/bridge"
	oauthpack "github.com/subosito/cincai/credential/oauth/pack"

	_ "github.com/subosito/cincai/link"

	"github.com/subosito/cincai/credential/oauth/vendor"
)

var once sync.Once

// Register links vendor OAuth modules. Safe to call multiple times.
func Register() {
	once.Do(func() {
		for _, e := range oauthpack.Entries() {
			mod := bridge.FromEntry(e)
			for _, profile := range e.Profiles {
				vendor.MustRegister(profile, mod)
			}
		}
	})
}