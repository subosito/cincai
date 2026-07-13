package elevenlabs

import (
	"github.com/subosito/cincai/adaptersdk"
	"github.com/subosito/cincai/credential/inject"
)

// DefaultInject applies when providers.yaml omits inject (override with inject: map if needed).
var DefaultInject = inject.Spec{"xi-api-key": "${key}"}

// Adapter registers ElevenLabs translate speech.
//
// Operator wiring (providers.yaml):
//
//	providers:
//	  elevenlabs:
//	    credential_profile: elevenlabs-api
//	    capabilities:
//	      speech_gen:
//	        adapter: elevenlabs
//	        base_url: https://api.elevenlabs.io
//
//	models:
//	  eleven-multilingual-v2:
//	    modalities:
//	      speech_gen:
//	        wire: openai-audio-speech
//	        providers:
//	          - provider_ref: elevenlabs
//	            surface: speech_gen
//	            model: eleven_multilingual_v2
type Adapter struct{}

func New() *Adapter { return &Adapter{} }

func (a *Adapter) Name() string { return "elevenlabs" }

func (a *Adapter) Register(reg *adaptersdk.Registry) error {
	adaptersdk.RegisterSpeechAdapter(reg, a.Name(), &SpeechHandler{})
	return nil
}