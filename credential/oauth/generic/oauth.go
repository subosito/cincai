package generic

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/subosito/cincai/credential/store"
	"github.com/subosito/cincai/internal/config"
	"golang.org/x/term"
)

const oauthHTTPTimeout = 60 * time.Second

var oauthHTTP = &http.Client{Timeout: oauthHTTPTimeout}

// Flow is OAuth login flow.
type Flow string

const (
	FlowAuto    Flow = "auto"
	FlowBrowser Flow = "browser"
	FlowDevice  Flow = "device"
	FlowManual  Flow = "manual"
)

// Controller drives interactive login UX.
type Controller struct {
	OnAuth        func(AuthInfo)
	OnProgress    func(string)
	OnManualInput func(context.Context) (string, error)
}

// AuthInfo is shown to the operator during login.
type AuthInfo struct {
	URL          string
	UserCode     string
	Instructions string
}

// Login performs OAuth2 login and returns material for store.
func Login(ctx context.Context, profile string, prof config.Profile, flow Flow, callbackAddr string, ctrl Controller) (store.Material, error) {
	flow, err := resolveFlow(flow)
	if err != nil {
		return store.Material{}, err
	}
	o := prof.OAuth
	if o.AuthorizeURL == "" || o.TokenURL == "" {
		return store.Material{}, fmt.Errorf("profile %q: oauth authorize_url and token_url required", profile)
	}
	if err := requireHTTPSEndpoints(profile, o); err != nil {
		return store.Material{}, err
	}
	switch flow {
	case FlowDevice:
		return loginDevice(ctx, profile, o, ctrl)
	case FlowManual:
		return loginManual(ctx, profile, o, ctrl)
	default:
		return loginBrowser(ctx, profile, o, callbackAddr, ctrl)
	}
}

// requireHTTPSEndpoints rejects any configured OAuth endpoint that would send
// authorization codes or refresh tokens in cleartext over a network. These URLs come
// from operator profile config. Plain http is allowed only for loopback hosts (local
// dev/test), which never leave the machine; every real remote host must use https.
func requireHTTPSEndpoints(profile string, o config.OAuthProfile) error {
	for _, e := range []struct{ name, raw string }{
		{"authorize_url", o.AuthorizeURL},
		{"token_url", o.TokenURL},
		{"device_authorize_url", o.DeviceAuthorizeURL},
		{"device_token_url", o.DeviceTokenURL},
	} {
		raw := strings.TrimSpace(e.raw)
		if raw == "" {
			continue
		}
		u, err := url.Parse(raw)
		if err != nil {
			return fmt.Errorf("profile %q: %s is not a valid URL: %w", profile, e.name, err)
		}
		if strings.EqualFold(u.Scheme, "https") {
			continue
		}
		if strings.EqualFold(u.Scheme, "http") && isLoopbackHost(u.Hostname()) {
			continue
		}
		return fmt.Errorf("profile %q: %s must use https (http allowed only for loopback), got %q", profile, e.name, raw)
	}
	return nil
}

func isLoopbackHost(host string) bool {
	if strings.EqualFold(host, "localhost") {
		return true
	}
	if ip := net.ParseIP(host); ip != nil {
		return ip.IsLoopback()
	}
	return false
}

func resolveFlow(flow Flow) (Flow, error) {
	switch flow {
	case FlowAuto:
		if isInteractive() {
			return FlowBrowser, nil
		}
		return FlowDevice, nil
	case FlowBrowser, FlowDevice, FlowManual:
		return flow, nil
	default:
		return "", fmt.Errorf("unknown flow %q", flow)
	}
}

func isInteractive() bool {
	if os.Getenv("CINCAI_FORCE_DEVICE") == "1" {
		return false
	}
	if term.IsTerminal(int(os.Stdin.Fd())) {
		if os.Getenv("DISPLAY") != "" || os.Getenv("WAYLAND_DISPLAY") != "" {
			return true
		}
		// macOS / local terminal without explicit display
		if os.Getenv("SSH_CONNECTION") == "" && os.Getenv("SSH_TTY") == "" {
			return true
		}
	}
	return false
}

func pkcePair() (verifier, challenge string, err error) {
	b := make([]byte, 32)
	if _, err = rand.Read(b); err != nil {
		return "", "", err
	}
	verifier = base64.RawURLEncoding.EncodeToString(b)
	sum := sha256.Sum256([]byte(verifier))
	challenge = base64.RawURLEncoding.EncodeToString(sum[:])
	return verifier, challenge, nil
}

func loginBrowser(ctx context.Context, profile string, o config.OAuthProfile, callbackAddr string, ctrl Controller) (store.Material, error) {
	verifier, challenge, err := pkcePair()
	if err != nil {
		return store.Material{}, err
	}
	state, err := randomState()
	if err != nil {
		return store.Material{}, err
	}
	listenAddr := callbackAddr
	if listenAddr == "" {
		listenAddr = "127.0.0.1:0"
	}
	codeCh := make(chan string, 1)
	errCh := make(chan error, 1)

	mux := http.NewServeMux()
	mux.HandleFunc("/oauth/callback", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("state") != state {
			errCh <- fmt.Errorf("oauth state mismatch")
			http.Error(w, "state mismatch", http.StatusBadRequest)
			return
		}
		if e := r.URL.Query().Get("error"); e != "" {
			errCh <- fmt.Errorf("oauth error: %s", e)
			http.Error(w, e, http.StatusBadRequest)
			return
		}
		code := r.URL.Query().Get("code")
		if code == "" {
			errCh <- fmt.Errorf("missing code")
			http.Error(w, "missing code", http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "text/html")
		_, _ = io.WriteString(w, "<html><body><p>Login complete. You may close this window.</p></body></html>")
		codeCh <- code
	})

	ln, err := net.Listen("tcp", listenAddr)
	if err != nil {
		return store.Material{}, err
	}
	redirectURI := "http://" + ln.Addr().String() + "/oauth/callback"
	srv := &http.Server{Handler: mux}
	go func() { _ = srv.Serve(ln) }()
	defer func() {
		shutCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = srv.Shutdown(shutCtx)
	}()

	authURL, err := buildAuthorizeURL(o, redirectURI, state, challenge)
	if err != nil {
		return store.Material{}, err
	}
	if ctrl.OnAuth != nil {
		ctrl.OnAuth(AuthInfo{URL: authURL, Instructions: "Complete login in your browser."})
	}

	var code string
	select {
	case <-ctx.Done():
		return store.Material{}, ctx.Err()
	case err := <-errCh:
		return store.Material{}, err
	case code = <-codeCh:
	}

	tok, err := exchangeCode(ctx, o, code, redirectURI, verifier)
	if err != nil {
		return store.Material{}, err
	}
	return tokenToMaterial(profile, tok), nil
}

func loginManual(ctx context.Context, profile string, o config.OAuthProfile, ctrl Controller) (store.Material, error) {
	verifier, challenge, err := pkcePair()
	if err != nil {
		return store.Material{}, err
	}
	state, err := randomState()
	if err != nil {
		return store.Material{}, err
	}
	redirectURI := "http://127.0.0.1/oauth/callback"
	authURL, err := buildAuthorizeURL(o, redirectURI, state, challenge)
	if err != nil {
		return store.Material{}, err
	}
	if ctrl.OnAuth != nil {
		ctrl.OnAuth(AuthInfo{URL: authURL, Instructions: "Open URL and paste redirect URL or code."})
	}
	var input string
	if ctrl.OnManualInput != nil {
		input, err = ctrl.OnManualInput(ctx)
	} else {
		return store.Material{}, fmt.Errorf("manual flow requires OnManualInput")
	}
	if err != nil {
		return store.Material{}, err
	}
	code := extractCode(input)
	if code == "" {
		return store.Material{}, fmt.Errorf("could not parse authorization code")
	}
	tok, err := exchangeCode(ctx, o, code, redirectURI, verifier)
	if err != nil {
		return store.Material{}, err
	}
	return tokenToMaterial(profile, tok), nil
}

func deviceAuthorizeURL(o config.OAuthProfile) (string, error) {
	if u := strings.TrimSpace(o.DeviceAuthorizeURL); u != "" {
		return u, nil
	}
	base := strings.TrimSuffix(o.TokenURL, "/token")
	if base != o.TokenURL {
		return base + "/device/code", nil
	}
	return "", fmt.Errorf("device_authorize_url required for device flow (token_url %q does not end with /token)", o.TokenURL)
}

func loginDevice(ctx context.Context, profile string, o config.OAuthProfile, ctrl Controller) (store.Material, error) {
	deviceAuth, err := deviceAuthorizeURL(o)
	if err != nil {
		return store.Material{}, err
	}
	deviceToken := o.DeviceTokenURL
	if deviceToken == "" {
		deviceToken = o.TokenURL
	}
	form := url.Values{}
	if o.ClientID != "" {
		form.Set("client_id", o.ClientID)
	}
	if len(o.Scopes) > 0 {
		form.Set("scope", strings.Join(o.Scopes, " "))
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, deviceAuth, strings.NewReader(form.Encode()))
	if err != nil {
		return store.Material{}, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := oauthHTTP.Do(req)
	if err != nil {
		return store.Material{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(resp.Body)
		return store.Material{}, fmt.Errorf("device authorize %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	var dev deviceResponse
	if err := json.NewDecoder(resp.Body).Decode(&dev); err != nil {
		return store.Material{}, err
	}
	verifyURL := dev.VerificationURI
	if dev.VerificationURIComplete != "" {
		verifyURL = dev.VerificationURIComplete
	}
	if ctrl.OnAuth != nil {
		ctrl.OnAuth(AuthInfo{
			URL: verifyURL, UserCode: dev.UserCode,
			Instructions: "Enter the code on the verification page.",
		})
	}
	interval := dev.Interval
	if interval <= 0 {
		interval = 5
	}
	deadline := time.Now().Add(time.Duration(dev.ExpiresIn) * time.Second)
	for time.Now().Before(deadline) {
		if ctrl.OnProgress != nil {
			ctrl.OnProgress("polling device token…")
		}
		select {
		case <-ctx.Done():
			return store.Material{}, ctx.Err()
		case <-time.After(time.Duration(interval) * time.Second):
		}
		tok, pending, err := pollDevice(ctx, deviceToken, o.ClientID, dev.DeviceCode)
		if pending {
			continue
		}
		if err != nil {
			return store.Material{}, err
		}
		return tokenToMaterial(profile, tok), nil
	}
	return store.Material{}, fmt.Errorf("device flow timed out")
}

type deviceResponse struct {
	DeviceCode              string `json:"device_code"`
	UserCode                string `json:"user_code"`
	VerificationURI         string `json:"verification_uri"`
	VerificationURIComplete string `json:"verification_uri_complete"`
	ExpiresIn               int    `json:"expires_in"`
	Interval                int    `json:"interval"`
}

type tokenResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	ExpiresIn    int    `json:"expires_in"`
	TokenType    string `json:"token_type"`
	Scope        string `json:"scope"`
}

func pollDevice(ctx context.Context, tokenURL, clientID, deviceCode string) (tokenResponse, bool, error) {
	form := url.Values{
		"grant_type":  {"urn:ietf:params:oauth:grant-type:device_code"},
		"device_code": {deviceCode},
	}
	if clientID != "" {
		form.Set("client_id", clientID)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, tokenURL, strings.NewReader(form.Encode()))
	if err != nil {
		return tokenResponse{}, false, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := oauthHTTP.Do(req)
	if err != nil {
		return tokenResponse{}, false, err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode == 400 && strings.Contains(string(body), "authorization_pending") {
		return tokenResponse{}, true, nil
	}
	if resp.StatusCode >= 400 {
		return tokenResponse{}, false, fmt.Errorf("device token %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	var tok tokenResponse
	if err := json.Unmarshal(body, &tok); err != nil {
		return tokenResponse{}, false, err
	}
	return tok, false, nil
}

func buildAuthorizeURL(o config.OAuthProfile, redirectURI, state, challenge string) (string, error) {
	u, err := url.Parse(o.AuthorizeURL)
	if err != nil {
		return "", err
	}
	q := u.Query()
	q.Set("response_type", "code")
	if o.ClientID != "" {
		q.Set("client_id", o.ClientID)
	}
	q.Set("redirect_uri", redirectURI)
	q.Set("state", state)
	if len(o.Scopes) > 0 {
		q.Set("scope", strings.Join(o.Scopes, " "))
	}
	if o.PKCE || challenge != "" {
		q.Set("code_challenge", challenge)
		q.Set("code_challenge_method", "S256")
	}
	u.RawQuery = q.Encode()
	return u.String(), nil
}

func exchangeCode(ctx context.Context, o config.OAuthProfile, code, redirectURI, verifier string) (tokenResponse, error) {
	form := url.Values{
		"grant_type":   {"authorization_code"},
		"code":         {code},
		"redirect_uri": {redirectURI},
	}
	if o.ClientID != "" {
		form.Set("client_id", o.ClientID)
	}
	if verifier != "" {
		form.Set("code_verifier", verifier)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, o.TokenURL, strings.NewReader(form.Encode()))
	if err != nil {
		return tokenResponse{}, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := oauthHTTP.Do(req)
	if err != nil {
		return tokenResponse{}, err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 400 {
		return tokenResponse{}, fmt.Errorf("token exchange %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	var tok tokenResponse
	if err := json.Unmarshal(body, &tok); err != nil {
		return tokenResponse{}, err
	}
	return tok, nil
}

// RefreshProfile loads oauth material, refreshes tokens, and updates the vault row.
func RefreshProfile(ctx context.Context, st store.Store, profile string, o config.OAuthProfile) error {
	cur, err := st.Get(ctx, profile)
	if err != nil {
		return err
	}
	if cur.Kind != store.KindOAuth {
		return fmt.Errorf("profile %q is not oauth", profile)
	}
	if strings.TrimSpace(cur.RefreshToken) == "" {
		return fmt.Errorf("profile %q has no refresh token", profile)
	}
	refreshed, err := Refresh(ctx, o, cur.RefreshToken)
	if err != nil {
		return err
	}
	refreshed.Profile = profile
	refreshed.Kind = store.KindOAuth
	if refreshed.Email == "" {
		refreshed.Email = cur.Email
	}
	refreshed.Extras = store.MergeExtras(cur.Extras, refreshed.Extras)
	var lastErr error
	for attempt := 0; attempt < 3; attempt++ {
		if err := st.UpdateOAuth(ctx, profile, refreshed); err == nil {
			return nil
		} else {
			lastErr = err
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(time.Duration(attempt+1) * 100 * time.Millisecond):
			}
		}
	}
	return fmt.Errorf("oauth refresh: upstream ok, store write failed after retries: %w", lastErr)
}

// Refresh exchanges a refresh token for new access token.
func Refresh(ctx context.Context, o config.OAuthProfile, refreshToken string) (store.Material, error) {
	if err := requireHTTPSEndpoints("", o); err != nil {
		return store.Material{}, err
	}
	form := url.Values{
		"grant_type":    {"refresh_token"},
		"refresh_token": {refreshToken},
	}
	if o.ClientID != "" {
		form.Set("client_id", o.ClientID)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, o.TokenURL, strings.NewReader(form.Encode()))
	if err != nil {
		return store.Material{}, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := oauthHTTP.Do(req)
	if err != nil {
		return store.Material{}, err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 400 {
		return store.Material{}, fmt.Errorf("refresh %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	var tok tokenResponse
	if err := json.Unmarshal(body, &tok); err != nil {
		return store.Material{}, err
	}
	mat := tokenToMaterial("", tok)
	if refreshToken != "" && tok.RefreshToken == "" {
		mat.RefreshToken = refreshToken
	}
	return mat, nil
}

func tokenToMaterial(profile string, tok tokenResponse) store.Material {
	mat := store.Material{
		Profile:      profile,
		Kind:         store.KindOAuth,
		AccessToken:  tok.AccessToken,
		RefreshToken: tok.RefreshToken,
	}
	if tok.ExpiresIn > 0 {
		mat.ExpiresAt = time.Now().Add(time.Duration(tok.ExpiresIn) * time.Second)
	}
	return mat
}

func randomState() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

func extractCode(input string) string {
	input = strings.TrimSpace(input)
	if input == "" {
		return ""
	}
	if !strings.Contains(input, "://") && !strings.Contains(input, "?") {
		return input
	}
	u, err := url.Parse(input)
	if err != nil {
		return input
	}
	if c := u.Query().Get("code"); c != "" {
		return c
	}
	return input
}