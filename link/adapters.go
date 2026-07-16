// Package link registers Cincai's full default pack — every bundled adapter,
// OAuth provider, and the wire-translate bridge — via blank import (each pack's
// init() calls the public registration APIs). cincai.Run pulls this in
// automatically. A custom gateway that composes its own registry (using the
// public gateway/compose/pack packages) instead of cincai.Run can:
//
//	import _ "github.com/subosito/cincai/link"
//
// to get the whole default pack in one line, without importing the individual
// (internal) adapter packages. Register only your own adapters via
// pack.RegisterAdapter if you don't want the defaults.
package link

import (
	_ "github.com/subosito/cincai/internal/adapters/elevenlabs"
	_ "github.com/subosito/cincai/internal/adapters/mistral"
	_ "github.com/subosito/cincai/internal/adapters/xai"
)
