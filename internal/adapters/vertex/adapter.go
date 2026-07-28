package vertex

import (
	"github.com/subosito/cincai/adaptersdk"
)

// Adapter registers the Vertex/Gemini generateContent chat wire
// (OpenAI chat ingress → generateContent upstream).
type Adapter struct{}

func New() *Adapter { return &Adapter{} }

func (a *Adapter) Name() string { return "vertex" }

func (a *Adapter) Register(reg *adaptersdk.Registry) error {
	adaptersdk.RegisterChatAdapter(reg, a.Name(), &ChatHandler{})
	return nil
}
