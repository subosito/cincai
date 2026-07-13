package gateway

import (
	"net/http"
	"testing"
	"time"
)

// Both listeners must bound header reads and idle connections, or a client can hold
// a connection open indefinitely by dribbling headers (Slowloris). WriteTimeout is
// deliberately zero: responses stream.
func TestNewServerHardening(t *testing.T) {
	srv := newServer("127.0.0.1:0", http.NewServeMux())

	if srv.ReadHeaderTimeout <= 0 {
		t.Errorf("ReadHeaderTimeout=%v, want > 0", srv.ReadHeaderTimeout)
	}
	if srv.IdleTimeout <= 0 {
		t.Errorf("IdleTimeout=%v, want > 0", srv.IdleTimeout)
	}
	if srv.MaxHeaderBytes <= 0 {
		t.Errorf("MaxHeaderBytes=%d, want > 0", srv.MaxHeaderBytes)
	}
	if srv.WriteTimeout != 0 {
		t.Errorf("WriteTimeout=%v, want 0 (streaming responses)", srv.WriteTimeout)
	}
	if srv.ReadHeaderTimeout > 30*time.Second {
		t.Errorf("ReadHeaderTimeout=%v is too permissive", srv.ReadHeaderTimeout)
	}
}

func TestIsLoopbackListen(t *testing.T) {
	for _, tc := range []struct {
		addr string
		want bool
	}{
		{"127.0.0.1:9420", true},
		{"127.0.0.1:0", true},
		{"[::1]:9420", true},
		{"localhost:9420", true},
		{":9420", false},   // all interfaces
		{"0.0.0.0:9420", false},
		{"192.168.1.5:9420", false},
		{"[::]:9420", false}, // unspecified v6
	} {
		if got := isLoopbackListen(tc.addr); got != tc.want {
			t.Errorf("isLoopbackListen(%q) = %v, want %v", tc.addr, got, tc.want)
		}
	}
}
