package xai

import (
	"github.com/subosito/cincai/adaptersdk"
)

// ImageAdapter registers xAI translate handlers (images + STT).
// Name stays "xai" for catalog adapter: xai on image_gen and voice surfaces.
type ImageAdapter struct{}

func NewImage() *ImageAdapter { return &ImageAdapter{} }

func (a *ImageAdapter) Name() string { return "xai" }

func (a *ImageAdapter) Register(reg *adaptersdk.Registry) error {
	adaptersdk.RegisterImageAdapter(reg, a.Name(), &ImageHandler{})
	adaptersdk.RegisterTranscriptionAdapter(reg, a.Name(), &TranscriptionHandler{})
	return nil
}
