package handler

import (
	"context"
	"io"
	"net/http"
)

// Document handles document processing like OCR.
type Document interface {
	Protocol() string
	Forward(ctx context.Context, client *http.Client, t Target, body io.Reader, hdr http.Header) (*http.Response, error)
}
