package upstream_test

import (
	"errors"
	"net"
	"net/http"
	"testing"

	"github.com/subosito/cincai/upstream"
)

func TestRetryable(t *testing.T) {
	dialErr := &net.OpError{Op: "dial", Err: errors.New("connection refused")}
	sentErr := &net.OpError{Op: "read", Err: errors.New("i/o timeout")}
	cases := []struct {
		status int
		err    error
		want   bool
	}{
		{0, dialErr, true},                 // never connected → safe to fail over
		{0, sentErr, false},                // request may have been sent → do not retry
		{0, errors.New("generic"), false},  // unknown error → assume it may have executed
		{http.StatusOK, nil, false},
		{http.StatusBadRequest, nil, false},
		{http.StatusPaymentRequired, nil, true},
		{http.StatusTooManyRequests, nil, true},
		{http.StatusBadGateway, nil, true},
		{http.StatusServiceUnavailable, nil, true},
		{http.StatusGatewayTimeout, nil, true},
	}
	for _, tc := range cases {
		if got := upstream.Retryable(tc.status, tc.err); got != tc.want {
			t.Fatalf("status=%d err=%v got=%v want=%v", tc.status, tc.err, got, tc.want)
		}
	}
}