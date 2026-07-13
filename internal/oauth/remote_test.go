package oauth_test

import (
	"strings"
	"testing"

	"github.com/subosito/cincai/internal/oauth"

	// Load vendor providers so their registered callbacks are available, as the CLI does.
	_ "github.com/subosito/cincai/internal/oauth-providers/anthropic"
	_ "github.com/subosito/cincai/internal/oauth-providers/xai"
)

func TestRemoteLoginNotesXAI(t *testing.T) {
	notes := oauth.RemoteLoginNotes("xai")
	if !strings.Contains(notes, "56121") {
		t.Fatalf("missing port: %q", notes)
	}
	if !strings.Contains(notes, "--flow manual") {
		t.Fatalf("missing manual hint: %q", notes)
	}
}

func TestRemoteLoginNotesUnknownProfile(t *testing.T) {
	if notes := oauth.RemoteLoginNotes("not-a-vendor-profile"); notes != "" {
		t.Fatalf("want empty notes, got %q", notes)
	}
}

func TestCallbackForProfile(t *testing.T) {
	spec, ok := oauth.CallbackForProfile("xai")
	if !ok || spec.Port != 56121 {
		t.Fatalf("spec=%+v ok=%v", spec, ok)
	}
}
