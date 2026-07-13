package generic

import (
	"testing"

	"github.com/subosito/cincai/internal/config"
)

func TestRequireHTTPSEndpoints(t *testing.T) {
	cases := []struct {
		name    string
		o       config.OAuthProfile
		wantErr bool
	}{
		{"all https", config.OAuthProfile{AuthorizeURL: "https://a/x", TokenURL: "https://a/t"}, false},
		{"remote http token_url rejected", config.OAuthProfile{AuthorizeURL: "https://a/x", TokenURL: "http://evil.example/t"}, true},
		{"remote http authorize_url rejected", config.OAuthProfile{AuthorizeURL: "http://evil.example/x", TokenURL: "https://a/t"}, true},
		{"loopback http allowed", config.OAuthProfile{AuthorizeURL: "http://127.0.0.1:8080/x", TokenURL: "http://localhost:8080/t"}, false},
		{"ipv6 loopback http allowed", config.OAuthProfile{AuthorizeURL: "http://[::1]:8080/x", TokenURL: "https://a/t"}, false},
		{"device http remote rejected", config.OAuthProfile{AuthorizeURL: "https://a/x", TokenURL: "https://a/t", DeviceTokenURL: "http://evil.example/d"}, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := requireHTTPSEndpoints("p", tc.o)
			if (err != nil) != tc.wantErr {
				t.Fatalf("requireHTTPSEndpoints err=%v, wantErr=%v", err, tc.wantErr)
			}
		})
	}
}
