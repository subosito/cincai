package mistral

import (
	"github.com/subosito/cincai/adaptersdk"
	"github.com/subosito/cincai/credential/inject"
)

// DefaultInject for Mistral (uses Bearer by default).
var DefaultInject = inject.Spec{"authorization": "Bearer ${key}"}

// Adapter registers Mistral for chat (passthrough) and OCR (via chat translation).
type Adapter struct{}

func New() *Adapter { return &Adapter{} }

func (a *Adapter) Name() string { return "mistral" }

func (a *Adapter) Register(reg *adaptersdk.Registry) error {
	adaptersdk.RegisterChatAdapter(reg, a.Name(), &ChatHandler{})
	return nil
}
