package xai

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/subosito/cincai/credential/oauth/flow"
	"github.com/subosito/cincai/credential/oauth/wire"
)

// Module implements xai login and refresh.
type Module struct{}

const (
	clientID         = "b1a00492-073a-47ea-816f-4c329264a828"
	discoveryURL     = "https://auth.x.ai/.well-known/openid-configuration"
	redirectHost     = "127.0.0.1"
	redirectPort     = 56121
	redirectPath     = "/callback"
	scope            = "openid profile email offline_access grok-cli:access api:access"
	expiresSkewMs    = 5 * 60 * 1000
	requestTimeout   = 20 * time.Second
	discoveryTimeout = 15 * time.Second
)

var fixedRedirectURI = fmt.Sprintf("http://%s:%d%s", redirectHost, redirectPort, redirectPath)

func (Module) Login(ctx context.Context, ctrl flow.Controller) (wire.OAuthPayload, error) {
	var verifier string
	var out wire.OAuthPayload
	cb := flow.CallbackFlow{
		PreferredPort:    redirectPort,
		CallbackPath:     redirectPath,
		Hostname:         redirectHost,
		FixedRedirectURI: fixedRedirectURI,
		Controller:       ctrl,
	}
	err := cb.Run(ctx,
		func(state, redirectURI string) (flow.AuthInfo, error) {
			var challenge string
			var err error
			verifier, challenge, err = flow.GeneratePKCE()
			if err != nil {
				return flow.AuthInfo{}, err
			}
			discovery, err := fetchDiscovery(ctx)
			if err != nil {
				return flow.AuthInfo{}, err
			}
			nonce, err := randomNonce()
			if err != nil {
				return flow.AuthInfo{}, err
			}
			params := url.Values{
				"response_type":         {"code"},
				"client_id":             {clientID},
				"redirect_uri":          {redirectURI},
				"scope":                 {scope},
				"code_challenge":        {challenge},
				"code_challenge_method": {"S256"},
				"state":                 {state},
				"nonce":                 {nonce},
				"plan":                  {"generic"},
				"referrer":              {"cincai"},
			}
			return flow.AuthInfo{
				URL:          discovery.AuthorizationEndpoint + "?" + params.Encode(),
				Instructions: "Complete login in your browser for xAI Grok (SuperGrok).",
			}, nil
		},
		func(code, _, redirectURI string) error {
			discovery, err := fetchDiscovery(ctx)
			if err != nil {
				return err
			}
			token, err := exchangeCode(ctx, discovery.TokenEndpoint, code, redirectURI, verifier)
			if err != nil {
				return err
			}
			out = token
			return nil
		},
	)
	if err != nil {
		return wire.OAuthPayload{}, err
	}
	return out, nil
}

func (Module) Refresh(ctx context.Context, cred wire.OAuthPayload) (wire.OAuthPayload, error) {
	discovery, err := fetchDiscovery(ctx)
	if err != nil {
		return wire.OAuthPayload{}, err
	}
	body := url.Values{
		"grant_type":    {"refresh_token"},
		"client_id":     {clientID},
		"refresh_token": {cred.Refresh},
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, discovery.TokenEndpoint, strings.NewReader(body.Encode()))
	if err != nil {
		return wire.OAuthPayload{}, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")
	client := &http.Client{Timeout: requestTimeout}
	res, err := client.Do(req)
	if err != nil {
		return wire.OAuthPayload{}, err
	}
	defer res.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(res.Body, 1<<20))
	if err != nil {
		return wire.OAuthPayload{}, err
	}
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return wire.OAuthPayload{}, fmt.Errorf("xai token refresh HTTP %d: %s", res.StatusCode, strings.TrimSpace(string(raw)))
	}
	return parseTokenResponse(raw, cred.Refresh, cred.Email, cred.AccountID)
}

type discoveryDoc struct {
	AuthorizationEndpoint string
	TokenEndpoint         string
}

func fetchDiscovery(ctx context.Context) (discoveryDoc, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, discoveryURL, nil)
	if err != nil {
		return discoveryDoc{}, err
	}
	req.Header.Set("Accept", "application/json")
	client := &http.Client{Timeout: discoveryTimeout}
	res, err := client.Do(req)
	if err != nil {
		return discoveryDoc{}, err
	}
	defer res.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(res.Body, 1<<20))
	if err != nil {
		return discoveryDoc{}, err
	}
	if res.StatusCode != http.StatusOK {
		return discoveryDoc{}, fmt.Errorf("xai discovery HTTP %d", res.StatusCode)
	}
	var doc struct {
		AuthorizationEndpoint string `json:"authorization_endpoint"`
		TokenEndpoint         string `json:"token_endpoint"`
	}
	if err := json.Unmarshal(raw, &doc); err != nil {
		return discoveryDoc{}, err
	}
	if err := validateXAIEndpoint(doc.AuthorizationEndpoint, "authorization_endpoint"); err != nil {
		return discoveryDoc{}, err
	}
	if err := validateXAIEndpoint(doc.TokenEndpoint, "token_endpoint"); err != nil {
		return discoveryDoc{}, err
	}
	return discoveryDoc{
		AuthorizationEndpoint: doc.AuthorizationEndpoint,
		TokenEndpoint:         doc.TokenEndpoint,
	}, nil
}

func exchangeCode(ctx context.Context, tokenEndpoint, code, redirectURI, verifier string) (wire.OAuthPayload, error) {
	body := url.Values{
		"grant_type":    {"authorization_code"},
		"client_id":     {clientID},
		"code":          {code},
		"redirect_uri":  {redirectURI},
		"code_verifier": {verifier},
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, tokenEndpoint, strings.NewReader(body.Encode()))
	if err != nil {
		return wire.OAuthPayload{}, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")
	client := &http.Client{Timeout: requestTimeout}
	res, err := client.Do(req)
	if err != nil {
		return wire.OAuthPayload{}, err
	}
	defer res.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(res.Body, 1<<20))
	if err != nil {
		return wire.OAuthPayload{}, err
	}
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return wire.OAuthPayload{}, fmt.Errorf("xai token exchange HTTP %d: %s", res.StatusCode, strings.TrimSpace(string(raw)))
	}
	return parseTokenResponse(raw, "", "", "")
}

func parseTokenResponse(raw []byte, fallbackRefresh, email, accountID string) (wire.OAuthPayload, error) {
	var data struct {
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
		ExpiresIn    int64  `json:"expires_in"`
	}
	if err := json.Unmarshal(raw, &data); err != nil {
		return wire.OAuthPayload{}, err
	}
	if data.AccessToken == "" || data.ExpiresIn == 0 {
		return wire.OAuthPayload{}, fmt.Errorf("xai token response missing fields")
	}
	refresh := data.RefreshToken
	if refresh == "" {
		refresh = fallbackRefresh
	}
	if refresh == "" {
		return wire.OAuthPayload{}, fmt.Errorf("xai token response missing refresh_token")
	}
	return wire.OAuthPayload{
		Type:      "oauth",
		Refresh:   refresh,
		Access:    data.AccessToken,
		Expires:   time.Now().UnixMilli() + data.ExpiresIn*1000 - expiresSkewMs,
		Email:     email,
		AccountID: accountID,
	}, nil
}

func validateXAIEndpoint(rawURL, field string) error {
	u, err := url.Parse(rawURL)
	if err != nil || u.Scheme != "https" {
		return fmt.Errorf("invalid xAI %s: %s", field, rawURL)
	}
	host := strings.ToLower(u.Hostname())
	if host != "x.ai" && !strings.HasSuffix(host, ".x.ai") {
		return fmt.Errorf("invalid xAI %s: %s", field, rawURL)
	}
	return nil
}

func randomNonce() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}