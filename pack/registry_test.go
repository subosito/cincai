package pack_test

import (
	"testing"

	_ "github.com/subosito/cincai/link"
	"github.com/subosito/cincai/pack"
)

func TestRegisteredAdapters(t *testing.T) {
	t.Parallel()
	names := make(map[string]bool, len(pack.Adapters()))
	for _, a := range pack.Adapters() {
		names[a.Name()] = true
	}
	for _, want := range []string{"wire-translate", "xai", "mistral", "elevenlabs"} {
		if !names[want] {
			t.Fatalf("missing adapter %q; got %v", want, names)
		}
	}
}
