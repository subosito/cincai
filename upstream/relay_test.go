package upstream_test

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/subosito/cincai/credential/store"
	"github.com/subosito/cincai/upstream"
)

func TestRelayStripsClientInjectsUpstream(t *testing.T) {
	var gotAuth, gotClientKey string
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		gotClientKey = r.Header.Get("x-api-key")
		_, _ = io.WriteString(w, `{"ok":true}`)
	}))
	defer up.Close()

	client := upstream.NewClient()
	mat := store.Material{Kind: store.KindAPIKey, APIKey: "sk-upstream-only"}
	hdr := make(http.Header)
	hdr.Set("Authorization", "Bearer client-token")
	hdr.Set("x-api-key", "client-key")
	hdr.Set("Content-Type", "application/json")

	resp, err := client.Relay(t.Context(), up.URL, "/v1/chat/completions", mat, "", strings.NewReader(`{"model":"m"}`), hdr)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if gotAuth != "Bearer sk-upstream-only" {
		t.Fatalf("upstream auth=%q", gotAuth)
	}
	if gotClientKey != "" {
		t.Fatalf("client x-api-key leaked: %q", gotClientKey)
	}
}

func TestCopyResponseStripsHopByHopHeaders(t *testing.T) {
	rec := httptest.NewRecorder()
	err := upstream.CopyResponse(context.Background(), rec, &http.Response{
		StatusCode: 200,
		Header: http.Header{
			"Content-Type":      []string{"application/json"},
			"Transfer-Encoding": []string{"chunked"},
			"Connection":        []string{"keep-alive, X-Foo"},
			"X-Foo":             []string{"bar"},
			"X-Custom":          []string{"keep"},
		},
		Body: io.NopCloser(strings.NewReader(`{"ok":true}`)),
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, h := range []string{"Transfer-Encoding", "Connection", "X-Foo"} {
		if rec.Header().Get(h) != "" {
			t.Fatalf("hop-by-hop header %q should be stripped, got %q", h, rec.Header().Get(h))
		}
	}
	if rec.Header().Get("X-Custom") != "keep" {
		t.Fatalf("end-to-end header missing: %v", rec.Header())
	}
}

func TestCopySSELongLine(t *testing.T) {
	long := strings.Repeat("x", 2<<20) // 2 MiB — exceeds old Scanner cap
	body := "data: " + long + "\n\n"
	rec := httptest.NewRecorder()
	err := upstream.CopyResponse(context.Background(), rec, &http.Response{
		StatusCode: 200,
		Header:     http.Header{"Content-Type": []string{"text/event-stream"}},
		Body:       io.NopCloser(strings.NewReader(body)),
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(rec.Body.String(), long) {
		t.Fatal("long SSE line truncated")
	}
}

func TestCopySSEHonorsContextCancel(t *testing.T) {
	pr, pw := io.Pipe()
	go func() {
		for i := 0; i < 100; i++ {
			_, _ = io.WriteString(pw, "data: line\n\n")
			time.Sleep(20 * time.Millisecond)
		}
		_ = pw.Close()
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	rec := httptest.NewRecorder()
	err := upstream.CopyResponse(ctx, rec, &http.Response{
		StatusCode: 200,
		Header:     http.Header{"Content-Type": []string{"text/event-stream"}},
		Body:       pr,
	})
	if err == nil {
		t.Fatal("expected context cancellation during SSE copy")
	}
}
