package keyring_test

import (
	"testing"

	"github.com/subosito/cincai/ingress/keyring"
)

func TestAuthorizeEmptyAllowsAll(t *testing.T) {
	if err := keyring.Authorize(nil, "m", "openai-chat-completions"); err != nil {
		t.Fatal(err)
	}
}

func TestAuthorizeModelScope(t *testing.T) {
	if err := keyring.Authorize([]string{"model:demo"}, "demo", "openai-responses"); err != nil {
		t.Fatal(err)
	}
	if err := keyring.Authorize([]string{"model:demo"}, "other", "openai-responses"); err == nil {
		t.Fatal("expected deny")
	}
}

func TestFilterModelsWireScope(t *testing.T) {
	if !keyring.FilterModels([]string{"wire:openai-responses"}, "demo", []string{"openai-responses"}) {
		t.Fatal("expected wire scope match")
	}
	if keyring.FilterModels([]string{"wire:openai-embeddings"}, "demo", []string{"openai-responses"}) {
		t.Fatal("expected deny")
	}
}