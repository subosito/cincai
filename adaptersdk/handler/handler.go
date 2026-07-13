package handler

import (
	"context"
	"io"
	"net/http"

	"github.com/subosito/cincai/catalog"
	"github.com/subosito/cincai/credential/store"
)

// Target is resolved route + material.
type Target struct {
	catalog.Target
	Material store.Material
}

// Chat handles chat protocols.
type Chat interface {
	Protocol() string
	Forward(ctx context.Context, client *http.Client, t Target, body io.Reader, hdr http.Header) (*http.Response, error)
}

// Embed handles embedding protocols.
type Embed interface {
	Protocol() string
	Forward(ctx context.Context, client *http.Client, t Target, body io.Reader, hdr http.Header) (*http.Response, error)
}