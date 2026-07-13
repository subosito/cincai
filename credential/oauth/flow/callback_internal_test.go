package flow

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

// A callback request with the wrong (or absent) state must be answered but must not
// resolve the pending login — otherwise any local process or drive-by page could
// abort the operator's flow by hitting the loopback callback.
func TestCallbackIgnoresWrongState(t *testing.T) {
	const want = "the-real-state"

	resultCh := make(chan callbackWaitResult, 1)

	// Wrong state, even with a valid-looking code: must not push a result.
	req := httptest.NewRequest(http.MethodGet, "/callback?code=abc&state=forged", nil)
	rec := httptest.NewRecorder()
	handleCallbackRequest(rec, req, want, resultCh)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("wrong-state response=%d, want 400", rec.Code)
	}
	select {
	case got := <-resultCh:
		t.Fatalf("wrong-state request resolved the flow: %+v", got)
	default:
		// good: flow still pending
	}

	// Missing state entirely: also ignored.
	req = httptest.NewRequest(http.MethodGet, "/callback?code=abc", nil)
	handleCallbackRequest(httptest.NewRecorder(), req, want, resultCh)
	select {
	case got := <-resultCh:
		t.Fatalf("missing-state request resolved the flow: %+v", got)
	default:
	}

	// Correct state + code: completes the flow.
	req = httptest.NewRequest(http.MethodGet, "/callback?code=abc&state="+want, nil)
	rec = httptest.NewRecorder()
	handleCallbackRequest(rec, req, want, resultCh)
	if rec.Code != http.StatusOK {
		t.Fatalf("valid callback response=%d, want 200", rec.Code)
	}
	select {
	case got := <-resultCh:
		if got.err != nil {
			t.Fatalf("valid callback errored: %v", got.err)
		}
		if got.result.Code != "abc" {
			t.Fatalf("code=%q, want abc", got.result.Code)
		}
	default:
		t.Fatal("valid callback did not resolve the flow")
	}
}

// A provider error redirect carries the matching state and remains terminal.
func TestCallbackProviderErrorIsTerminal(t *testing.T) {
	const want = "s"
	resultCh := make(chan callbackWaitResult, 1)
	req := httptest.NewRequest(http.MethodGet, "/callback?error=access_denied&error_description=denied&state="+want, nil)
	handleCallbackRequest(httptest.NewRecorder(), req, want, resultCh)
	select {
	case got := <-resultCh:
		if got.err == nil {
			t.Fatal("provider error should be terminal (non-nil err)")
		}
	default:
		t.Fatal("provider error with correct state should resolve the flow")
	}
}
