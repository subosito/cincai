package upstream

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/textproto"
	"strings"
	"time"

	"github.com/subosito/cincai/credential/inject"
	"github.com/subosito/cincai/credential/store"
	"github.com/subosito/cincai/observability"
)

// Hop-by-hop headers must not be relayed (RFC 7230).
var hopByHopHeaders = []string{
	"Connection",
	"Keep-Alive",
	"Proxy-Authenticate",
	"Proxy-Authorization",
	"Te",
	"Trailer",
	"Transfer-Encoding",
	"Upgrade",
}

func removeHopByHopHeaders(h http.Header) {
	if c := h.Get("Connection"); c != "" {
		for _, f := range strings.Split(c, ",") {
			if f = textproto.TrimString(f); f != "" {
				h.Del(f)
			}
		}
	}
	for _, k := range hopByHopHeaders {
		h.Del(k)
	}
}

// Client performs outbound relay.
type Client struct {
	HTTP *http.Client
}

func NewClient() *Client {
	tr := &http.Transport{
		Proxy:                 http.ProxyFromEnvironment,
		DialContext:           (&net.Dialer{Timeout: 30 * time.Second, KeepAlive: 30 * time.Second}).DialContext,
		TLSHandshakeTimeout:   10 * time.Second,
		ResponseHeaderTimeout: DefaultTimeout,
		ExpectContinueTimeout: 1 * time.Second,
	}
	return &Client{HTTP: &http.Client{Transport: tr, Timeout: 0, CheckRedirect: checkRedirect}}
}

// checkRedirect refuses redirects that leave the original origin. Go strips only
// Authorization/Cookie/WWW-Authenticate across hosts, so a credential injected as a custom
// header would otherwise follow a redirect to an attacker-controlled destination and leak.
// The comparison is on host:port (a different port is a different destination), so it fails
// safe; a same-origin redirect (/foo → /bar) is allowed.
func checkRedirect(req *http.Request, via []*http.Request) error {
	if len(via) == 0 {
		return nil
	}
	if len(via) >= 10 {
		return fmt.Errorf("upstream: stopped after 10 redirects")
	}
	if !strings.EqualFold(req.URL.Host, via[0].URL.Host) {
		return fmt.Errorf("upstream: refusing cross-origin redirect to %q", req.URL.Host)
	}
	return nil
}

// Relay forwards request body to upstream URL with credential inject.
func (c *Client) Relay(ctx context.Context, baseURL, path string, mat store.Material, preset string, body io.Reader, headers http.Header) (*http.Response, error) {
	url := JoinURL(baseURL, path)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, body)
	if err != nil {
		return nil, err
	}
	inject.CopyHeaders(req, headers)
	inject.Apply(mat, req, preset)
	return observability.HTTPDo(ctx, c.HTTP, req)
}

// upstreamStrippedResponseHeaders are removed from an upstream response before it reaches the
// client: an upstream must not set cookies on the gateway's own origin. Provider-specific
// identity headers, if sensitive, are stripped by that provider's adapter. Rate-limit and
// request-id headers pass through.
var upstreamStrippedResponseHeaders = []string{
	"Set-Cookie",
}

func writeUpstreamHeaders(w http.ResponseWriter, resp *http.Response) {
	h := resp.Header.Clone()
	removeHopByHopHeaders(h)
	for _, k := range upstreamStrippedResponseHeaders {
		h.Del(k)
	}
	for k, vals := range h {
		for _, v := range vals {
			w.Header().Add(k, v)
		}
	}
}

// CopyResponse streams upstream response to client, with flush for SSE.
func CopyResponse(ctx context.Context, w http.ResponseWriter, resp *http.Response) error {
	writeUpstreamHeaders(w, resp)
	w.WriteHeader(resp.StatusCode)
	if resp.Body == nil {
		return nil
	}
	defer resp.Body.Close()
	if strings.Contains(resp.Header.Get("Content-Type"), "text/event-stream") {
		return copySSE(ctx, w, resp.Body)
	}
	_, err := io.Copy(w, resp.Body)
	return err
}

// UsageObserver receives the response bytes as they stream to the client.
// It must not block or mutate them; it sees exactly what the client sees.
type UsageObserver interface {
	Observe(p []byte)
}

// CopyResponseWithUsage streams the upstream response while feeding its bytes to
// obs (usage metering). drop, when non-nil, is called per SSE line: a line it
// returns true for is metered but NOT forwarded to the client — used to hide an
// injected stream_options.include_usage frame the client never asked for, so the
// client stream stays byte-identical. obs and drop may be nil.
func CopyResponseWithUsage(ctx context.Context, w http.ResponseWriter, resp *http.Response, obs UsageObserver, drop func(line []byte) bool) error {
	writeUpstreamHeaders(w, resp)
	w.WriteHeader(resp.StatusCode)
	if resp.Body == nil {
		return nil
	}
	defer resp.Body.Close()
	if strings.Contains(resp.Header.Get("Content-Type"), "text/event-stream") {
		return copySSEMetered(ctx, w, resp.Body, obs, drop)
	}
	src := io.Reader(resp.Body)
	if obs != nil {
		src = io.TeeReader(resp.Body, observerWriter{obs})
	}
	_, err := io.Copy(w, src)
	return err
}

type observerWriter struct{ obs UsageObserver }

func (o observerWriter) Write(p []byte) (int, error) {
	o.obs.Observe(p)
	return len(p), nil
}

// maxSSELineBytes caps a single SSE line/event. A misbehaving or hostile upstream that
// streams bytes without ever sending '\n' would otherwise grow an unbounded in-memory
// buffer per request; readSSELine bounds it instead.
const maxSSELineBytes = 8 << 20 // 8 MiB

// readSSELine reads one '\n'-terminated line, bounding memory to ~maxSSELineBytes. Unlike
// bufio.Reader.ReadString it accumulates in fixed chunks (ReadSlice) and fails once a
// single line exceeds the cap, rather than buffering the whole line first.
func readSSELine(br *bufio.Reader) (string, error) {
	var buf []byte
	for {
		slice, err := br.ReadSlice('\n')
		if len(buf)+len(slice) > maxSSELineBytes {
			return "", fmt.Errorf("upstream: SSE line exceeds %d bytes", maxSSELineBytes)
		}
		buf = append(buf, slice...)
		if err == bufio.ErrBufferFull {
			continue
		}
		return string(buf), err
	}
}

// copySSEMetered is copySSE plus per-line metering and an optional drop filter.
func copySSEMetered(ctx context.Context, w http.ResponseWriter, body io.Reader, obs UsageObserver, drop func([]byte) bool) error {
	flusher, ok := w.(http.Flusher)
	br := bufio.NewReader(body)
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		line, err := readSSELine(br)
		if len(line) > 0 {
			b := []byte(line)
			if obs != nil {
				obs.Observe(b)
			}
			if drop == nil || !drop(b) {
				if _, werr := io.WriteString(w, line); werr != nil {
					return werr
				}
				if ok {
					flusher.Flush()
				}
			}
		}
		if err != nil {
			if err == io.EOF {
				return nil
			}
			return err
		}
	}
}

func copySSE(ctx context.Context, w http.ResponseWriter, body io.Reader) error {
	flusher, ok := w.(http.Flusher)
	br := bufio.NewReader(body)
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		line, err := readSSELine(br)
		if len(line) > 0 {
			if _, werr := io.WriteString(w, line); werr != nil {
				return werr
			}
			if ok {
				flusher.Flush()
			}
		}
		if err != nil {
			if err == io.EOF {
				return nil
			}
			return err
		}
	}
}

// DrainReader reads body until EOF (for shutdown tests).
func DrainReader(ctx context.Context, r io.Reader) error {
	done := make(chan error, 1)
	go func() {
		_, err := io.Copy(io.Discard, r)
		done <- err
	}()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case err := <-done:
		return err
	}
}

// DefaultTimeout for non-streaming posts.
const DefaultTimeout = 120 * time.Second