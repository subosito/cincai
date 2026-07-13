package upstream

import (
	"errors"
	"net"
	"net/http"
)

// Retryable reports whether another pool member may be tried.
//
// For a transport error, only a connection-setup ("dial") failure is retryable: it proves
// the request never reached the upstream, so retrying on another pool member cannot cause a
// duplicate. Once bytes may have hit the wire (response-header timeout, mid-stream reset),
// retrying a non-idempotent call (chat/image/video generation) risks double execution and
// double billing, so it is not retried.
func Retryable(status int, err error) bool {
	if err != nil {
		return isDialError(err)
	}
	switch status {
	case http.StatusPaymentRequired, // upstream account/plan exhausted — try next pool member
		http.StatusTooManyRequests,
		http.StatusBadGateway,
		http.StatusServiceUnavailable,
		http.StatusGatewayTimeout:
		return true
	default:
		return false
	}
}

// isDialError reports whether err is a connection-establishment failure (the request was
// never delivered): dial failures, DNS resolution failures, connection refused, etc. all
// surface as a *net.OpError with Op == "dial".
func isDialError(err error) bool {
	var opErr *net.OpError
	if errors.As(err, &opErr) {
		return opErr.Op == "dial"
	}
	return false
}
