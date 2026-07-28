package vertex

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/subosito/cincai/adaptersdk/handler"
	"github.com/subosito/cincai/adaptersdk/messages"
	"github.com/subosito/cincai/adaptersdk/upstreamauth"
	"github.com/subosito/cincai/observability"

	"github.com/subosito/cincai/adapters/gemini"
)

type upstreamError struct {
	status int
	header http.Header
	body   []byte
}

func (e *upstreamError) Error() string {
	return fmt.Sprintf("vertex upstream status %d", e.status)
}

func callGenerate(ctx context.Context, client *http.Client, t handler.Target, model string, gen gemini.GenerateRequest, stream bool) ([]messages.StreamEvent, error) {
	body, err := json.Marshal(gen)
	if err != nil {
		return nil, err
	}
	targetURL, err := generateContentURL(t.BaseURL, model, stream)
	if err != nil {
		return nil, err
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, targetURL, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	if stream {
		httpReq.Header.Set("Accept", "text/event-stream")
	} else {
		httpReq.Header.Set("Accept", "application/json")
	}
	if err := upstreamauth.ApplyTranslated(t, httpReq, nil, upstreamauth.BearerDefault()); err != nil {
		return nil, err
	}
	resp, err := observability.HTTPDo(ctx, client, httpReq)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	payload, err := io.ReadAll(io.LimitReader(resp.Body, gemini.MaxUpstreamBodyBytes))
	if err != nil {
		return nil, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, &upstreamError{status: resp.StatusCode, header: resp.Header.Clone(), body: payload}
	}
	if stream {
		var events []messages.StreamEvent
		err := gemini.ParseGenerateStream(bytes.NewReader(payload), model, func(ev messages.StreamEvent) error {
			events = append(events, ev)
			return nil
		})
		if err != nil {
			return nil, err
		}
		return events, nil
	}
	return gemini.ParseGenerateResponse(payload, model)
}

func upstreamHTTPResponse(err error) (*http.Response, bool) {
	ue, ok := err.(*upstreamError)
	if !ok {
		return nil, false
	}
	h := ue.header
	if h == nil {
		h = http.Header{}
	}
	if h.Get("Content-Type") == "" {
		h.Set("Content-Type", "application/json")
	}
	return &http.Response{
		StatusCode: ue.status,
		Header:     h,
		Body:       io.NopCloser(bytes.NewReader(ue.body)),
	}, true
}
