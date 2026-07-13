package flow

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const (
	defaultCallbackTimeout = 5 * time.Minute
	defaultCallbackHost    = "localhost"
	defaultCallbackPath    = "/callback"
)

// CallbackResult holds the authorization code from the OAuth redirect.
type CallbackResult struct {
	Code  string
	State string
}

// CallbackFlow runs a local HTTP callback server for OAuth login.
type CallbackFlow struct {
	PreferredPort int
	CallbackPath  string
	Hostname      string
	// FixedRedirectURI, when set, is advertised to the provider and must match the
	// listener address. Port fallback is disabled (xAI, OpenAI Codex).
	FixedRedirectURI string
	Controller       Controller
}

// Run executes the browser callback leg of an OAuth login.
func (f *CallbackFlow) Run(
	ctx context.Context,
	generateAuthURL func(state, redirectURI string) (AuthInfo, error),
	exchangeToken func(code, state, redirectURI string) error,
) error {
	if f.CallbackPath == "" {
		f.CallbackPath = defaultCallbackPath
	}
	if f.Hostname == "" {
		f.Hostname = defaultCallbackHost
	}

	state, err := randomState()
	if err != nil {
		return err
	}

	resultCh := make(chan callbackWaitResult, 1)
	_, listener, redirectURI, err := f.bindCallback()
	if err != nil {
		return err
	}

	mux := http.NewServeMux()
	mux.HandleFunc(f.CallbackPath, func(w http.ResponseWriter, r *http.Request) {
		handleCallbackRequest(w, r, state, resultCh)
	})
	srv := &http.Server{
		Handler:           mux,
		ReadHeaderTimeout: 10 * time.Second,
	}
	go func() { _ = srv.Serve(listener) }()
	defer func() {
		_ = srv.Close()
		_ = listener.Close()
	}()

	info, err := generateAuthURL(state, redirectURI)
	if err != nil {
		return err
	}
	if f.Controller.OnAuth != nil {
		f.Controller.OnAuth(info)
	}
	if f.Controller.OnProgress != nil {
		f.Controller.OnProgress("Waiting for browser authentication...")
	}

	result, err := f.waitForCallback(ctx, state, resultCh)
	if err != nil {
		return err
	}
	if f.Controller.OnProgress != nil {
		f.Controller.OnProgress("Exchanging authorization code for tokens...")
	}
	return exchangeToken(result.Code, result.State, redirectURI)
}

type callbackWaitResult struct {
	result CallbackResult
	err    error
}

func handleCallbackRequest(w http.ResponseWriter, r *http.Request, expectedState string, resultCh chan<- callbackWaitResult) {
	code := r.URL.Query().Get("code")
	state := r.URL.Query().Get("state")
	oauthErr := r.URL.Query().Get("error")
	desc := r.URL.Query().Get("error_description")
	if desc == "" {
		desc = oauthErr
	}

	// Only a request carrying the expected state may complete the flow. The loopback
	// callback is reachable by any local process or a drive-by web page, so a request
	// with a wrong or absent state is stray/forged: answer it but do NOT resolve the
	// pending login, otherwise anyone could abort the operator's in-progress flow.
	// The OAuth error redirect and the success redirect both carry state, so genuine
	// outcomes still match.
	if expectedState != "" && state != expectedState {
		writeCallbackPage(w, http.StatusBadRequest,
			map[string]any{"ok": false, "error": "State mismatch — ignored (this request did not originate from this login)"})
		return
	}

	var payload map[string]any
	var out callbackWaitResult
	switch {
	case oauthErr != "":
		payload = map[string]any{"ok": false, "error": "Authorization failed: " + desc}
		out.err = fmt.Errorf("authorization failed: %s", desc)
	case code == "":
		payload = map[string]any{"ok": false, "error": "Missing authorization code"}
		out.err = fmt.Errorf("missing authorization code")
	default:
		payload = map[string]any{"ok": true}
		out.result = CallbackResult{Code: code, State: state}
	}

	status := http.StatusOK
	if out.err != nil {
		status = http.StatusInternalServerError
	}
	writeCallbackPage(w, status, payload)

	select {
	case resultCh <- out:
	default:
	}
}

func writeCallbackPage(w http.ResponseWriter, status int, payload map[string]any) {
	w.Header().Set("Content-Type", "text/html")
	w.WriteHeader(status)
	body, _ := json.Marshal(payload)
	_, _ = io.WriteString(w, callbackHTML(string(body)))
}

func (f *CallbackFlow) waitForCallback(ctx context.Context, expectedState string, resultCh <-chan callbackWaitResult) (CallbackResult, error) {
	timeoutCtx, cancel := context.WithTimeout(ctx, defaultCallbackTimeout)
	defer cancel()

	if f.Controller.OnManualInput != nil {
		for {
			select {
			case <-timeoutCtx.Done():
				return CallbackResult{}, timeoutCtx.Err()
			case res := <-resultCh:
				if res.err != nil {
					return CallbackResult{}, res.err
				}
				return res.result, nil
			default:
				input, err := f.Controller.OnManualInput(timeoutCtx)
				if err != nil {
					return CallbackResult{}, err
				}
				parsed := ParseCallbackInput(input)
				if parsed.Code == "" {
					continue
				}
				if expectedState != "" && parsed.State != expectedState {
					continue
				}
				return parsed, nil
			}
		}
	}

	select {
	case <-timeoutCtx.Done():
		return CallbackResult{}, timeoutCtx.Err()
	case res := <-resultCh:
		if res.err != nil {
			return CallbackResult{}, res.err
		}
		return res.result, nil
	}
}

func (f *CallbackFlow) bindCallback() (int, net.Listener, string, error) {
	if f.FixedRedirectURI != "" {
		u, err := url.Parse(f.FixedRedirectURI)
		if err != nil {
			return 0, nil, "", fmt.Errorf("fixed redirect uri: %w", err)
		}
		host := u.Hostname()
		if host == "" {
			host = "127.0.0.1"
		}
		port := u.Port()
		if port == "" {
			return 0, nil, "", fmt.Errorf("fixed redirect uri missing port")
		}
		ln, err := net.Listen("tcp", net.JoinHostPort(host, port))
		if err != nil {
			return 0, nil, "", fmt.Errorf("oauth callback port %s unavailable: %w", u.Host, err)
		}
		p, _ := strconv.Atoi(port)
		return p, ln, f.FixedRedirectURI, nil
	}

	port, ln, err := listenCallback(f.PreferredPort)
	if err != nil {
		return 0, nil, "", err
	}
	redirectURI := fmt.Sprintf("http://%s:%d%s", f.Hostname, port, f.CallbackPath)
	return port, ln, redirectURI, nil
}

func listenCallback(preferredPort int) (int, net.Listener, error) {
	if preferredPort > 0 {
		ln, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", preferredPort))
		if err == nil {
			return preferredPort, ln, nil
		}
	}
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, nil, fmt.Errorf("callback listen: %w", err)
	}
	addr := ln.Addr().(*net.TCPAddr)
	return addr.Port, ln, nil
}

func randomState() (string, error) {
	buf := make([]byte, 16)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("oauth state: %w", err)
	}
	out := make([]byte, 32)
	for i, b := range buf {
		const hex = "0123456789abcdef"
		out[i*2] = hex[b>>4]
		out[i*2+1] = hex[b&0x0f]
	}
	return string(out), nil
}

// ParseCallbackInput extracts code/state from a redirect URL or raw code string.
func ParseCallbackInput(input string) CallbackResult {
	value := strings.TrimSpace(input)
	if value == "" {
		return CallbackResult{}
	}
	if u, err := url.Parse(value); err == nil && u.Scheme != "" {
		return CallbackResult{
			Code:  u.Query().Get("code"),
			State: u.Query().Get("state"),
		}
	}
	if strings.Contains(value, "code=") {
		q := strings.TrimPrefix(strings.TrimPrefix(value, "?"), "#")
		if vals, err := url.ParseQuery(q); err == nil {
			return CallbackResult{
				Code:  vals.Get("code"),
				State: vals.Get("state"),
			}
		}
	}
	code, state, _ := strings.Cut(value, "#")
	return CallbackResult{Code: code, State: state}
}

func callbackHTML(stateJSON string) string {
	return `<!DOCTYPE html><html><body><p>OAuth complete. You may close this window.</p><script>window.close()</script>` +
		`<pre>` + stateJSON + `</pre></body></html>`
}