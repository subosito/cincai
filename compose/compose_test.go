package compose_test

import (
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