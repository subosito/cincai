package catalog_test

import (
	"os"
	"path/filepath"
	"testing"

	corecatalog "github.com/subosito/cincai/catalog"

	icatalog "github.com/subosito/cincai/internal/catalog"
)

func TestChatStarterCatalog(t *testing.T) {
	path := filepath.Join("..", "..", "config", "providers.yaml.example")
	if _, err := os.Stat(path); err != nil {
		t.Skip("providers.yaml.example missing")
	}
	cat, err := icatalog.Load(path)
	if err != nil {
		t.Fatal(err)
	}

	chat, err := cat.Resolve("deepseek-v4-flash", corecatalog.WireOpenAIChat)
	if err != nil {
		t.Fatalf("deepseek: %v", err)
	}
	if chat.Targets[0].Protocol != "openai-chat-completions" {
		t.Fatalf("deepseek protocol=%q", chat.Targets[0].Protocol)
	}

	// anthropic-messages ingress over an openai-chat-completions upstream must
	// pick up the injected wire-translate adapter.
	translated, err := cat.Resolve("deepseek-v4-flash", corecatalog.WireAnthropicMsg)
	if err != nil {
		t.Fatalf("deepseek anthropic_chat: %v", err)
	}
	if translated.Targets[0].Adapter != icatalog.AdapterWireTranslateA2O {
		t.Fatalf("deepseek anthropic_chat adapter=%q", translated.Targets[0].Adapter)
	}
	if translated.Targets[0].Protocol != "openai-chat-completions" {
		t.Fatalf("deepseek anthropic_chat protocol=%q", translated.Targets[0].Protocol)
	}
}
