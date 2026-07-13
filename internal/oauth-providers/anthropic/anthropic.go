package anthropic

import (
	"context"
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

// Module implements anthropic OAuth login and refresh.
type Module struct{}

const (
	// clientID is Anthropic's public OAuth client id for CLI logins; it is not a secret.
	clientID      = "9d1c250a-e61b-44d9-88ed-5944d1962f5e"
	authorizeURL  = "https://claude.ai/oauth/authorize"
	tokenURL      = "https://api.anthropic.com/v1/oauth/token"
	callbackPort  = 54545
	callbackPath  = "/callback"
	scopes        = "org:create_api_key user:profile user:inference"
	expiresSkewMs = 5 * 60 * 1000
)

func (Module) Login(ctx context.Context, ctrl flow.Controller) (wire.OAuthPayload, error) {
	cb := flow.CallbackFlow{
		PreferredPort: callbackPort,
		CallbackPath:  callbackPath,
		Controller:    ctrl,
	}

	var verifier, challenge string
	var cred wire.OAuthPayload

	err := cb.Run(ctx,
		func(state, redirectURI string) (flow.AuthInfo, error) {
			var err error
			verifier, challenge, err = flow.GeneratePKCE()
			if err != nil {
				return flow.AuthInfo{}, err
			}
			authURL, err := buildAuthorizeURL(state, redirectURI, challenge)
			if err != nil {
				return flow.AuthInfo{}, err
			}
			return flow.AuthInfo{
				URL: authURL,
				Instructions: "Complete login in your browser. If the browser cannot reach this machine, " +
					"paste the final redirect URL or authorization code when prompted.",
			}, nil
		},
		func(code, state, redirectURI string) error {
			exchangeCode, exchangeState := splitCodeState(code, state)
			data, err := exchangeAuthorizationCode(ctx, exchangeCode, exchangeState, redirectURI, verifier)
			if err != nil {
				return err
			}
			cred = tokenToPayload(data)
			return nil
		},
	)
	if err != nil {
		return wire.OAuthPayload{}, err
	}
	return cred, nil
}

func (Module) Refresh(ctx context.Context, cred wire.OAuthPayload) (wire.OAuthPayload, error) {
	data, err := refreshTokenRequest(ctx, cred.Refresh)
	if err != nil {
		return wire.OAuthPayload{}, err
	}
	return tokenToPayload(data), nil
}

func buildAuthorizeURL(state, redirectURI, challenge string) (string, error) {
	params := url.Values{
		"code":                  {"true"},
		"client_id":             {clientID},
		"response_type":         {"code"},
		"redirect_uri":          {redirectURI},
		"scope":                 {scopes},
		"code_challenge":        {challenge},
		"code_challenge_method": {"S256"},
		"state":                 {state},
	}
	return authorizeURL + "?" + params.Encode(), nil
}

func splitCodeState(code, state string) (string, string) {
	if i := strings.IndexByte(code, '#'); i >= 0 {
		fragState := code[i+1:]
		code = code[:i]
		if fragState != "" {
			state = fragState
		}
	}
	return code, state
}

type tokenResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	ExpiresIn    int64  `json:"expires_in"`
	Account      *struct {
		UUID         string `json:"uuid"`
		EmailAddress string `json:"email_address"`
	} `json:"account"`
}

func exchangeAuthorizationCode(ctx context.Context, code, state, redirectURI, verifier string) (tokenResponse, error) {
	return postToken(ctx, map[string]string{
		"grant_type":    "authorization_code",
		"client_id":     clientID,
		"code":          code,
		"state":         state,
		"redirect_uri":  redirectURI,
		"code_verifier": verifier,
	})
}

func refreshTokenRequest(ctx context.Context, refreshToken string) (tokenResponse, error) {
	return postToken(ctx, map[string]string{
		"grant_type":    "refresh_token",
		"client_id":     clientID,
		"refresh_token": refreshToken,
	})
}

func postToken(ctx context.Context, body map[string]string) (tokenResponse, error) {
	payload, err := json.Marshal(body)
	if err != nil {
		return tokenResponse{}, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, tokenURL, strings.NewReader(string(payload)))
	if err != nil {
		return tokenResponse{}, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	client := &http.Client{Timeout: 30 * time.Second}
	res, err := client.Do(req)
	if err != nil {
		return tokenResponse{}, fmt.Errorf("anthropic token request: %w", err)
	}
	defer res.Body.Close()

	respBody, err := io.ReadAll(io.LimitReader(res.Body, 1<<20))
	if err != nil {
		return tokenResponse{}, err
	}
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return tokenResponse{}, fmt.Errorf("anthropic token HTTP %d: %s", res.StatusCode, strings.TrimSpace(string(respBody)))
	}

	var data tokenResponse
	if err := json.Unmarshal(respBody, &data); err != nil {
		return tokenResponse{}, fmt.Errorf("anthropic token json: %w", err)
	}
	if data.AccessToken == "" {
		return tokenResponse{}, fmt.Errorf("anthropic token: missing access_token")
	}
	return data, nil
}

func tokenToPayload(data tokenResponse) wire.OAuthPayload {
	refresh := data.RefreshToken
	expires := time.Now().UnixMilli() + data.ExpiresIn*1000 - expiresSkewMs
	out := wire.OAuthPayload{
		Type:    "oauth",
		Refresh: refresh,
		Access:  data.AccessToken,
		Expires: expires,
	}
	if data.Account != nil {
		if data.Account.UUID != "" {
			out.AccountID = data.Account.UUID
		}
		if data.Account.EmailAddress != "" {
			out.Email = data.Account.EmailAddress
		}
	}
	return out
}