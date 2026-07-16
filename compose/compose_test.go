package compose_test

import (
	"strings"
	"testing"

	"github.com/subosito/cincai/compose"
)

func TestFromConfigPassthrough(t *testing.T) {
	reg, err := compose.FromConfig([]string{"passthrough"}, compose.DefaultAdapters())
	if err != nil {
		t.Fatal(err)
	}
	if len(reg.ChatHandlers) == 0 {
		t.Fatal("expected chat handlers")
	}
}

func TestFromConfigRejectsEmpty(t *testing.T) {
	_, err := compose.FromConfig([]string{"nonexistent-adapter"}, compose.DefaultAdapters())
	if err == nil {
		t.Fatal("expected error for unknown adapter")
	}
}

// A typo next to a valid adapter used to be dropped silently: the registry came
// back non-empty, startup succeeded, and only the routes needing it failed.
func TestFromConfigRejectsUnknownBesideValid(t *testing.T) {
	_, err := compose.FromConfig([]string{"passthrough", "typoed-adapter"}, compose.DefaultAdapters())
	if err == nil {
		t.Fatal("expected error for unknown adapter alongside a valid one")
	}
	if !strings.Contains(err.Error(), "typoed-adapter") {
		t.Fatalf("error should name the offender, got: %v", err)
	}
}

func TestFromConfigIgnoresBlankEntries(t *testing.T) {
	reg, err := compose.FromConfig([]string{"passthrough", "", "  "}, compose.DefaultAdapters())
	if err != nil {
		t.Fatal(err)
	}
	if len(reg.ChatHandlers) == 0 {
		t.Fatal("expected chat handlers")
	}
}
