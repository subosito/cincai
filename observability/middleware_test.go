package observability_test

import (
	"bytes"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/subosito/cincai/observability"
)

func TestIngressLogEmitsOncePerRequest(t *testing.T) {
	var buf bytes.Buffer
	observability.SetTestLogger(slog.New(slog.NewJSONHandler(&buf, nil)))
	t.Cleanup(func() { observability.SetTestLogger(slog.Default()) })

	h := observability.IngressLog("test-wire", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if rec := observability.RecorderFrom(r.Context()); rec != nil {
			rec.Model = "m1"
			rec.PrincipalID = "p1"
		}
		w.WriteHeader(http.StatusTeapot)
	}))

	req := httptest.NewRequest(http.MethodGet, "/v1/foo", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusTeapot {
		t.Fatalf("status=%d", rec.Code)
	}
	out := buf.String()
	if strings.Count(out, `"msg":"ingress"`) != 1 {
		t.Fatalf("expected one ingress line, got: %s", out)
	}
	if !strings.Contains(out, "test-wire") && !strings.Contains(out, `"wire":"test-wire"`) {
		t.Fatalf("missing wire: %s", out)
	}
}
