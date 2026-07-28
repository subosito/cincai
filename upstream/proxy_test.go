package upstream_test

import (
	"net/http"
	"net/url"
	"testing"

	"github.com/subosito/cincai/upstream"
)

func TestClientFor_cachesAndProxyModes(t *testing.T) {
	t.Parallel()
	a := upstream.ClientFor("http://127.0.0.1:8080")
	b := upstream.ClientFor("http://127.0.0.1:8080")
	if a != b {
		t.Fatal("same proxy URL should return cached client")
	}
	direct := upstream.ClientFor("direct")
	env := upstream.ClientFor("")
	if direct == a || env == a {
		t.Fatal("different proxy keys must not share clients")
	}

	// Fixed proxy: Transport.Proxy(req) should return that URL.
	req, _ := http.NewRequest(http.MethodGet, "https://api.example.com/", nil)
	tr, ok := a.HTTP.Transport.(*http.Transport)
	if !ok || tr.Proxy == nil {
		t.Fatalf("transport=%T", a.HTTP.Transport)
	}
	u, err := tr.Proxy(req)
	if err != nil {
		t.Fatal(err)
	}
	if u == nil || u.Host != "127.0.0.1:8080" {
		t.Fatalf("proxy URL=%v", u)
	}

	// direct: no proxy func / nil
	trD, ok := direct.HTTP.Transport.(*http.Transport)
	if !ok {
		t.Fatal("direct transport")
	}
	if trD.Proxy != nil {
		if pu, _ := trD.Proxy(req); pu != nil {
			t.Fatalf("direct should not proxy, got %v", pu)
		}
	}

	// NewClient is ClientFor("")
	if upstream.NewClient() != env {
		t.Fatal("NewClient should equal ClientFor(\"\")")
	}
	_ = url.URL{}
}
