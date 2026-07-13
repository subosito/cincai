package generic_test

import (
	"context"
	"encoding/json"
	"net"
	"strings"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync"
	"testing"
	"time"

	"github.com/subosito/cincai/credential/oauth/generic"
	"github.com/subosito/cincai/credential/seal"
	"github.com/subosito/cincai/credential/store"
	"github.com/subosito/cincai/internal/config"
)

func TestDeviceFlow(t *testing.T) {
	srv := newOAuthServer(t)
	defer srv.Close()

	prof := config.Profile{
		Kind: "oauth",
		OAuth: config.OAuthProfile{
			AuthorizeURL:       srv.URL + "/oauth/authorize",
			TokenURL:           srv.URL + "/oauth/token",
			DeviceAuthorizeURL: srv.URL + "/oauth/device/code",
			ClientID:           "test-client",
		},
	}
	go func() {
		time.Sleep(200 * time.Millisecond)
		srv.approveDevice()
	}()

	mat, err := generic.Login(context.Background(), "test", prof, generic.FlowDevice, "127.0.0.1:1", generic.Controller{})
	if err != nil {
		t.Fatal(err)
	}
	if mat.AccessToken != "access-xyz" || mat.RefreshToken != "refresh-xyz" {
		t.Fatalf("material: %+v", mat)
	}
}

func TestBrowserPKCE(t *testing.T) {
	srv := newOAuthServer(t)
	defer srv.Close()
	prof := config.Profile{
		Kind: "oauth",
		OAuth: config.OAuthProfile{
			AuthorizeURL: srv.URL + "/oauth/authorize",
			TokenURL:     srv.URL + "/oauth/token",
			ClientID:     "test-client",
			PKCE:         true,
		},
	}
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	callbackAddr := ln.Addr().String()
	_ = ln.Close()

	authURLCh := make(chan string, 1)
	go func() {
		time.Sleep(150 * time.Millisecond)
		authURL := <-authURLCh
		srv.simulateBrowserCallback(authURL, callbackAddr)
	}()

	mat, err := generic.Login(context.Background(), "test", prof, generic.FlowBrowser, callbackAddr, generic.Controller{
		OnAuth: func(info generic.AuthInfo) { authURLCh <- info.URL },
	})
	if err != nil {
		t.Fatal(err)
	}
	if mat.AccessToken != "access-xyz" {
		t.Fatalf("material: %+v", mat)
	}
}

func TestAutoHeadlessUsesDevice(t *testing.T) {
	srv := newOAuthServer(t)
	defer srv.Close()
	prof := config.Profile{
		Kind: "oauth",
		OAuth: config.OAuthProfile{
			AuthorizeURL:       srv.URL + "/oauth/authorize",
			TokenURL:           srv.URL + "/oauth/token",
			DeviceAuthorizeURL: srv.URL + "/oauth/device/code",
			ClientID:           "test-client",
		},
	}
	t.Setenv("CINCAI_FORCE_DEVICE", "1")
	go func() {
		time.Sleep(200 * time.Millisecond)
		srv.approveDevice()
	}()
	done := make(chan error, 1)
	go func() {
		_, err := generic.Login(context.Background(), "test", prof, generic.FlowAuto, "127.0.0.1:1", generic.Controller{})
		done <- err
	}()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("auto flow hung")
	}
}

func TestDeviceFlowRequiresDeviceAuthorizeURL(t *testing.T) {
	prof := config.Profile{
		Kind: "oauth",
		OAuth: config.OAuthProfile{
			AuthorizeURL: "https://example.com/oauth/authorize",
			TokenURL:     "https://example.com/oauth2/access_token",
			ClientID:     "test-client",
		},
	}
	_, err := generic.Login(context.Background(), "test", prof, generic.FlowDevice, "127.0.0.1:1", generic.Controller{})
	if err == nil {
		t.Fatal("expected error when device_authorize_url cannot be derived")
	}
	if !strings.Contains(err.Error(), "device_authorize_url required") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRefresh(t *testing.T) {
	srv := newOAuthServer(t)
	defer srv.Close()
	o := config.OAuthProfile{TokenURL: srv.URL + "/oauth/token", ClientID: "test-client"}
	mat, err := generic.Refresh(context.Background(), o, "refresh-xyz")
	if err != nil {
		t.Fatal(err)
	}
	if mat.AccessToken != "access-refreshed" {
		t.Fatalf("refresh: %+v", mat)
	}
}

func TestManualFlow(t *testing.T) {
	srv := newOAuthServer(t)
	defer srv.Close()
	prof := config.Profile{
		Kind: "oauth",
		OAuth: config.OAuthProfile{
			AuthorizeURL: srv.URL + "/oauth/authorize",
			TokenURL:     srv.URL + "/oauth/token",
			ClientID:     "test-client",
			PKCE:         true,
		},
	}
	mat, err := generic.Login(context.Background(), "test", prof, generic.FlowManual, "127.0.0.1:1", generic.Controller{
		OnManualInput: func(ctx context.Context) (string, error) {
			return "auth-code-1", nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if mat.AccessToken != "access-xyz" {
		t.Fatalf("manual flow material: %+v", mat)
	}
}

func TestRefreshProfileUpdatesStore(t *testing.T) {
	srv := newOAuthServer(t)
	defer srv.Close()
	key, _ := seal.ParseKey("IIIIIIIIIIIIIIIIIIIIIIIIIIIIIIIIIIIIIIIIIII=")
	st := store.NewMemory(key)
	_, err := st.PutOAuth(context.Background(), "test-oauth", store.Material{
		Kind: store.KindOAuth, Profile: "test-oauth",
		AccessToken: "access-stale", RefreshToken: "refresh-xyz",
	})
	if err != nil {
		t.Fatal(err)
	}
	o := config.OAuthProfile{TokenURL: srv.URL + "/oauth/token", ClientID: "test-client"}
	if err := generic.RefreshProfile(context.Background(), st, "test-oauth", o); err != nil {
		t.Fatal(err)
	}
	mat, err := st.Get(context.Background(), "test-oauth")
	if err != nil {
		t.Fatal(err)
	}
	if mat.AccessToken != "access-refreshed" {
		t.Fatalf("vault access token not updated: %+v", mat)
	}
	if mat.RefreshToken != "refresh-xyz" {
		t.Fatalf("refresh token lost: %+v", mat)
	}
}

type oauthSrv struct {
	*httptest.Server
	mu         sync.Mutex
	deviceCode string
	approved   bool
}

func newOAuthServer(t *testing.T) *oauthSrv {
	t.Helper()
	o := &oauthSrv{}
	mux := http.NewServeMux()
	mux.HandleFunc("/oauth/device/code", func(w http.ResponseWriter, r *http.Request) {
		o.mu.Lock()
		o.deviceCode = "device-code-123"
		o.mu.Unlock()
		_ = json.NewEncoder(w).Encode(map[string]any{
			"device_code": "device-code-123", "user_code": "ABCD-1234",
			"verification_uri": o.URL + "/verify", "expires_in": 300, "interval": 1,
		})
	})
	mux.HandleFunc("/oauth/authorize", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	mux.HandleFunc("/oauth/token", func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		switch r.Form.Get("grant_type") {
		case "urn:ietf:params:oauth:grant-type:device_code":
			o.mu.Lock()
			ok := o.approved
			o.mu.Unlock()
			if !ok {
				w.WriteHeader(http.StatusBadRequest)
				_, _ = w.Write([]byte(`{"error":"authorization_pending"}`))
				return
			}
		case "authorization_code":
			if r.Form.Get("code_verifier") == "" {
				http.Error(w, "missing verifier", http.StatusBadRequest)
				return
			}
		case "refresh_token":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"access_token": "access-refreshed", "refresh_token": "refresh-xyz", "expires_in": 3600,
			})
			return
		default:
			http.Error(w, "bad grant", http.StatusBadRequest)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"access_token": "access-xyz", "refresh_token": "refresh-xyz", "expires_in": 3600,
		})
	})
	o.Server = httptest.NewServer(mux)
	return o
}

func (o *oauthSrv) approveDevice() {
	o.mu.Lock()
	o.approved = true
	o.mu.Unlock()
}

func (o *oauthSrv) simulateBrowserCallback(authorizeURL, callbackAddr string) {
	u, _ := url.Parse(authorizeURL)
	state := u.Query().Get("state")
	redirect := "http://" + callbackAddr + "/oauth/callback?code=auth-code-1&state=" + url.QueryEscape(state)
	req, _ := http.NewRequest(http.MethodGet, redirect, nil)
	_, _ = http.DefaultClient.Do(req)
}