package xai

import (
	"github.com/subosito/cincai/adaptersdk"
)

// ImageAdapter registers the xAI image translator (OpenAI ingress → xAI /v1/images/*).
type ImageAdapter struct{}

func NewImage() *ImageAdapter { return &ImageAdapter{} }

func (a *ImageAdapter) Name() string { return "xai" }

func (a *ImageAdapter) Register(reg *adaptersdk.Registry) error {
	adaptersdk.RegisterImageAdapter(reg, a.Name(), &ImageHandler{})
	return nil
}