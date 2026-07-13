package oauth

import (
	"fmt"
	"os"
	"strings"

	oauthpack "github.com/subosito/cincai/credential/oauth/pack"
)

// CallbackSpec is the loopback listener used during browser OAuth login.
type CallbackSpec struct {
	Host string
	Port int
	Path string
}

// CallbackForProfile returns the fixed loopback callback a vendor declared in its
// registration, if any. Derived from the registered providers — no separate table.
func CallbackForProfile(profile string) (CallbackSpec, bool) {
	profile = strings.TrimSpace(profile)
	for _, e := range oauthpack.Entries() {
		if e.Callback.Port == 0 {
			continue
		}
		for _, p := range e.Profiles {
			if p == profile {
				return CallbackSpec{Host: e.Callback.Host, Port: e.Callback.Port, Path: e.Callback.Path}, true
			}
		}
	}
	return CallbackSpec{}, false
}

// LikelyRemoteShell reports SSH or similar remote sessions where loopback OAuth needs a tunnel.
func LikelyRemoteShell() bool {
	if strings.TrimSpace(os.Getenv("SSH_CONNECTION")) != "" {
		return true
	}
	if strings.TrimSpace(os.Getenv("SSH_CLIENT")) != "" {
		return true
	}
	if strings.TrimSpace(os.Getenv("SSH_TTY")) != "" {
		return true
	}
	return false
}

// RemoteLoginNotes returns UX hints for browser OAuth (port-forward or manual flow).
func RemoteLoginNotes(profile string) string {
	spec, ok := CallbackForProfile(profile)
	if !ok {
		return ""
	}
	host := spec.Host
	if host == "" {
		host = "127.0.0.1"
	}
	redirect := fmt.Sprintf("http://%s:%d%s", host, spec.Port, spec.Path)

	var b strings.Builder
	if LikelyRemoteShell() {
		fmt.Fprintf(&b, "Remote session detected — OAuth callback listens on %s.\n", redirect)
		fmt.Fprintf(&b, "From your laptop, forward the port before opening the auth URL:\n")
		fmt.Fprintf(&b, "  ssh -L %d:127.0.0.1:%d user@YOUR_HOST\n", spec.Port, spec.Port)
		fmt.Fprintf(&b, "Then open the login URL in your local browser (not on the server).\n")
	} else {
		fmt.Fprintf(&b, "OAuth callback: %s (loopback on the machine running cincai).\n", redirect)
		fmt.Fprintf(&b, "If cincai runs on a remote server, use SSH port-forward:\n")
		fmt.Fprintf(&b, "  ssh -L %d:127.0.0.1:%d user@YOUR_HOST\n", spec.Port, spec.Port)
	}
	fmt.Fprintf(&b, "Alternative without port-forward: cincai credential login %s --flow manual\n", profile)
	return b.String()
}