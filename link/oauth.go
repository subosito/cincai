// OAuth provider packs — blank-import so init() registers with internal/oauth/pack.
package link

import (
	_ "github.com/subosito/cincai/internal/oauth-providers/anthropic"
	_ "github.com/subosito/cincai/internal/oauth-providers/xai"
)