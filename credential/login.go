package credential

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/subosito/cincai/credential/oauth/generic"
	"github.com/subosito/cincai/credential/oauth/vendor"

	oauthmod "github.com/subosito/cincai/internal/oauth"
)

// LoginOptions controls OAuth login against the encrypted dev/prod broker.
type LoginOptions struct {
	ConfigPath    string
	Profile       string
	Flow          generic.Flow
	CallbackListen string
	Stderr        io.Writer
	OnManualInput func(context.Context) (string, error)
}

// Login runs vendor or generic OAuth login and stores the grant in the encrypted broker.
func Login(ctx context.Context, opts LoginOptions) (int64, error) {
	if strings.TrimSpace(opts.Profile) == "" {
		return 0, fmt.Errorf("profile is required")
	}
	vault, err := LoadVault(opts.ConfigPath)
	if err != nil {
		return 0, err
	}
	defer vault.Close()

	prof, err := OAuthProfile(vault.Config, opts.Profile)
	if err != nil {
		return 0, err
	}
	flow := opts.Flow
	if flow == "" {
		flow = generic.FlowAuto
	}
	callback := strings.TrimSpace(opts.CallbackListen)
	if callback == "" {
		callback = "127.0.0.1:0"
	}
	errOut := opts.Stderr
	if errOut == nil {
		errOut = os.Stderr
	}
	ctrl := generic.Controller{
		OnAuth: func(info generic.AuthInfo) {
			fmt.Fprintln(errOut)
			if info.UserCode != "" {
				fmt.Fprintf(errOut, "User code: %s\n", info.UserCode)
			}
			if info.URL != "" {
				fmt.Fprintf(errOut, "Open: %s\n", info.URL)
			}
			if info.Instructions != "" {
				fmt.Fprintln(errOut, info.Instructions)
			}
		},
		OnProgress: func(msg string) { fmt.Fprintln(errOut, msg) },
	}
	if flow == generic.FlowManual {
		read := opts.OnManualInput
		if read == nil {
			read = readManualInput
		}
		ctrl.OnManualInput = read
	} else if notes := oauthmod.RemoteLoginNotes(opts.Profile); notes != "" {
		fmt.Fprint(errOut, notes)
	}
	mat, err := vendor.Login(ctx, opts.Profile, prof, flow, callback, ctrl)
	if err != nil {
		return 0, err
	}
	id, err := vault.Store.PutOAuth(ctx, opts.Profile, mat)
	if err != nil {
		return 0, fmt.Errorf("store: %w", err)
	}
	return id, nil
}

func readManualInput(ctx context.Context) (string, error) {
	_ = ctx
	fmt.Fprint(os.Stderr, "Paste redirect URL or authorization code: ")
	line, err := bufio.NewReader(os.Stdin).ReadString('\n')
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(line), nil
}