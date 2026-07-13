// Package register wires Cincai vendor OAuth modules at link time.
package register

import (
	oauthreg "github.com/subosito/cincai/internal/oauth/register"
)

// Register links vendor OAuth modules (each also registers its own inject presets in
// init). Safe to call more than once.
func Register() {
	oauthreg.Register()
}
