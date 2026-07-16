package upstream_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/subosito/cincai/upstream"
)

func TestJoinURL(t *testing.T) {
	tests := []struct {
		base, path, want string
	}{
		{
			base: "https://api.example.com",
			path: "/v1/chat/completions",
			want: "https://api.example.com/v1/chat/completions",
		},
		{
			base: "https://host.example/v1",
			path: "/v1/chat/completions",
			want: "https://host.example/v1/chat/completions",
		},
		{
			base: "https://compat.example/v1",
			path: "/v1/chat/completions",
			want: "https://compat.example/v1/chat/completions",
		},
		{
			base: "https://api.anthropic.com",
			path: "/v1/messages",
			want: "https://api.anthropic.com/v1/messages",
		},
		{
			base: "https://api.example/v1/",
			path: "/v1/responses",
			want: "https://api.example/v1/responses",
		},
		{
			base: "https://embed.example/compatible-mode/v1",
			path: "/v1/embeddings",
			want: "https://embed.example/compatible-mode/v1/embeddings",
		},
	}
	for _, tc := range tests {
		if got := upstream.JoinURL(tc.base, tc.path); got != tc.want {
			t.Fatalf("JoinURL(%q, %q) = %q, want %q", tc.base, tc.path, got, tc.want)
		}
	}
}

func TestImageUpstreamPath(t *testing.T) {
	tests := []struct {
		base, ingress, want string
	}{
		{
			base:    "https://api.x.ai/v1/images",
			ingress: "/v1/images/generations",
			want:    "https://api.x.ai/v1/images/generations",
		},
		{
			base:    "https://api.openai.com/v1/images",
			ingress: "/v1/images/edits",
			want:    "https://api.openai.com/v1/images/edits",
		},
		{
			base:    "https://api.tokenrouter.com",
			ingress: "/v1/images/generations",
			want:    "https://api.tokenrouter.com/v1/images/generations",
		},
	}
	for _, tc := range tests {
		if got := upstream.ImageUpstreamPath(tc.base, tc.ingress); got != tc.want {
			t.Fatalf("ImageUpstreamPath(%q, %q) = %q, want %q", tc.base, tc.ingress, got, tc.want)
		}
	}
}

func TestVideoUpstreamPath(t *testing.T) {
	tests := []struct {
		base, ingress, want string
	}{
		{
			base:    "https://api.x.ai/v1/videos",
			ingress: "/v1/videos/generations",
			want:    "https://api.x.ai/v1/videos/generations",
		},
		{
			base:    "https://api.x.ai/v1/videos",
			ingress: "/v1/videos/vid_abc123",
			want:    "https://api.x.ai/v1/videos/vid_abc123",
		},
		{
			base:    "https://api.example.com",
			ingress: "/v1/videos/generations",
			want:    "https://api.example.com/v1/videos/generations",
		},
		// A client-controlled id arrives already percent-decoded; a raw '?' must be
		// re-escaped so it cannot open a query string on the upstream request.
		{
			base:    "https://api.x.ai/v1/videos",
			ingress: "/v1/videos/vid?admin=1",
			want:    "https://api.x.ai/v1/videos/vid%3Fadmin=1",
		},
		// A raw '/' must not introduce extra upstream path segments.
		{
			base:    "https://api.x.ai/v1/videos",
			ingress: "/v1/videos/a/b",
			want:    "https://api.x.ai/v1/videos/a%2Fb",
		},
		// Escaping also applies on the JoinURL fallback (base without /videos).
		{
			base:    "https://api.example.com",
			ingress: "/v1/videos/vid?x=1",
			want:    "https://api.example.com/v1/videos/vid%3Fx=1",
		},
	}
	for _, tc := range tests {
		if got := upstream.VideoUpstreamPath(tc.base, tc.ingress); got != tc.want {
			t.Fatalf("VideoUpstreamPath(%q, %q) = %q, want %q", tc.base, tc.ingress, got, tc.want)
		}
	}
}

// A redirect crossing to a different host must be refused so injected credential
// headers cannot leak; a same-host redirect is still followed.
func TestClientRefusesCrossHostRedirect(t *testing.T) {
	client := upstream.NewClient()

	t.Run("cross-host refused", func(t *testing.T) {
		evil := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			t.Errorf("cross-host redirect target must not be reached: %s", r.URL)
			w.WriteHeader(http.StatusOK)
		}))
		defer evil.Close()
		origin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			http.Redirect(w, r, evil.URL+"/steal", http.StatusFound)
		}))
		defer origin.Close()

		req, _ := http.NewRequest(http.MethodGet, origin.URL, nil)
		resp, err := client.HTTP.Do(req)
		if err == nil {
			resp.Body.Close()
			t.Fatalf("expected cross-host redirect to be refused, got status %d", resp.StatusCode)
		}
		if !strings.Contains(err.Error(), "cross-origin redirect") {
			t.Fatalf("want cross-host redirect error, got %v", err)
		}
	})

	t.Run("same-host followed", func(t *testing.T) {
		var mux http.ServeMux
		mux.HandleFunc("/start", func(w http.ResponseWriter, r *http.Request) {
			http.Redirect(w, r, "/final", http.StatusFound)
		})
		mux.HandleFunc("/final", func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusNoContent)
		})
		srv := httptest.NewServer(&mux)
		defer srv.Close()

		req, _ := http.NewRequest(http.MethodGet, srv.URL+"/start", nil)
		resp, err := client.HTTP.Do(req)
		if err != nil {
			t.Fatalf("same-host redirect should succeed: %v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusNoContent {
			t.Fatalf("status=%d want 204 (redirect followed to /final)", resp.StatusCode)
		}
	})
}
