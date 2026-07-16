package observability_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/subosito/cincai/observability"
)

func TestStashHostAttribution(t *testing.T) {
	var got observability.HostAttribution
	var leaked string
	h := observability.StashHostAttribution(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = observability.HostAttributionFrom(r.Context())
		leaked = r.Header.Get(observability.HeaderActor)
		w.WriteHeader(http.StatusNoContent)
	}))
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	req.Header.Set(observability.HeaderActor, "svc-main")
	req.Header.Set(observability.HeaderSession, "c1")
	req.Header.Set(observability.HeaderComponent, "turn.chat")
	req.Header.Set("X-Correlation-Id", "corr-1")
	h.ServeHTTP(httptest.NewRecorder(), req)

	if got.Actor != "svc-main" || got.Component != "turn.chat" || got.Session != "c1" {
		t.Fatalf("attribution=%+v", got)
	}
	if got.CorrelationID != "corr-1" {
		t.Fatalf("correlation=%q", got.CorrelationID)
	}
	if leaked != "" {
		t.Fatal("actor must be stripped before upstream")
	}
}

func TestStashHostAttribution_optional(t *testing.T) {
	var got observability.HostAttribution
	h := observability.StashHostAttribution(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = observability.HostAttributionFrom(r.Context())
		w.WriteHeader(http.StatusNoContent)
	}))
	req := httptest.NewRequest(http.MethodGet, "/v1/models", nil)
	h.ServeHTTP(httptest.NewRecorder(), req)
	if got.Actor != "" || got.Component != "" {
		t.Fatalf("expected empty attribution, got %+v", got)
	}
}

func TestStashHostAttribution_ignoresUnknown(t *testing.T) {
	var got observability.HostAttribution
	h := observability.StashHostAttribution(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = observability.HostAttributionFrom(r.Context())
		w.WriteHeader(http.StatusNoContent)
	}))
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	req.Header.Set("X-Chacha-Actor", "legacy-must-ignore")
	req.Header.Set("X-Bot-Slug", "should-ignore")
	h.ServeHTTP(httptest.NewRecorder(), req)
	if got.Actor != "" || got.Session != "" || got.Component != "" {
		t.Fatalf("unknown headers must not be read: %+v", got)
	}
}

func TestStashHostAttributionWith_aliasesFirstWins(t *testing.T) {
	const (
		hostActor     = "X-Host-Actor"
		hostSession   = "X-Host-Session"
		hostComponent = "X-Host-Component"
	)
	cfg := observability.AttributionConfig{
		Actor:     []string{hostActor, observability.HeaderActor},
		Session:   []string{hostSession, observability.HeaderSession},
		Component: []string{hostComponent, observability.HeaderComponent},
	}
	var got observability.HostAttribution
	var leakedHost, leakedDefault string
	h := observability.StashHostAttributionWith(cfg)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = observability.HostAttributionFrom(r.Context())
		leakedHost = r.Header.Get(hostActor)
		leakedDefault = r.Header.Get(observability.HeaderActor)
		w.WriteHeader(http.StatusNoContent)
	}))

	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	req.Header.Set(hostActor, "host-a")
	req.Header.Set(observability.HeaderActor, "fallback") // later alias — ignored when host present
	req.Header.Set(hostSession, "s1")
	req.Header.Set(hostComponent, "tool.generate_image")
	h.ServeHTTP(httptest.NewRecorder(), req)

	if got.Actor != "host-a" || got.Session != "s1" || got.Component != "tool.generate_image" {
		t.Fatalf("attribution=%+v", got)
	}
	if leakedHost != "" || leakedDefault != "" {
		t.Fatalf("all aliases must be stripped: host=%q default=%q", leakedHost, leakedDefault)
	}
}

func TestStashHostAttributionWith_fallbackToDefaultAlias(t *testing.T) {
	cfg := observability.AttributionConfig{
		Actor:     []string{"X-Host-Actor", observability.HeaderActor},
		Session:   []string{"X-Host-Session", observability.HeaderSession},
		Component: []string{"X-Host-Component", observability.HeaderComponent},
	}
	var got observability.HostAttribution
	h := observability.StashHostAttributionWith(cfg)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = observability.HostAttributionFrom(r.Context())
		w.WriteHeader(http.StatusNoContent)
	}))
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	req.Header.Set(observability.HeaderActor, "from-cincai-header")
	req.Header.Set(observability.HeaderSession, "sess")
	req.Header.Set(observability.HeaderComponent, "turn.chat")
	h.ServeHTTP(httptest.NewRecorder(), req)
	if got.Actor != "from-cincai-header" || got.Session != "sess" || got.Component != "turn.chat" {
		t.Fatalf("attribution=%+v", got)
	}
}

func TestStashHostAttributionWith_emptyConfigUsesDefaults(t *testing.T) {
	var got observability.HostAttribution
	h := observability.StashHostAttributionWith(observability.AttributionConfig{})(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = observability.HostAttributionFrom(r.Context())
		w.WriteHeader(http.StatusNoContent)
	}))
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	req.Header.Set(observability.HeaderActor, "default-path")
	h.ServeHTTP(httptest.NewRecorder(), req)
	if got.Actor != "default-path" {
		t.Fatalf("actor=%q", got.Actor)
	}
}

func TestDefaultAttributionConfig(t *testing.T) {
	d := observability.DefaultAttributionConfig()
	if len(d.Actor) != 1 || d.Actor[0] != observability.HeaderActor {
		t.Fatalf("actor=%v", d.Actor)
	}
	if len(d.Session) != 1 || d.Session[0] != observability.HeaderSession {
		t.Fatalf("session=%v", d.Session)
	}
	if len(d.Component) != 1 || d.Component[0] != observability.HeaderComponent {
		t.Fatalf("component=%v", d.Component)
	}
}
