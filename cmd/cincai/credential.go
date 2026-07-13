package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/subosito/cincai/credential/oauth/generic"

	cred "github.com/subosito/cincai/credential"
	"github.com/subosito/cincai/internal/oauth"
	"github.com/subosito/cincai/register"
)

func credentialCmd(args []string) int {
	if len(args) == 0 {
		printCredentialUsage()
		return 2
	}
	switch args[0] {
	case "login":
		return credentialLoginCmd(args[1:])
	case "list":
		return credentialListCmd(args[1:])
	case "refresh":
		return credentialRefreshCmd(args[1:])
	case "import":
		return credentialImportCmd(args[1:])
	case "enable":
		return credentialSetEnabledCmd("enable", args[1:])
	case "disable":
		return credentialSetEnabledCmd("disable", args[1:])
	case "help", "-h", "--help":
		printCredentialUsage()
		return 0
	default:
		fmt.Fprintf(os.Stderr, "cincai credential: unknown subcommand %q\n", args[0])
		printCredentialUsage()
		return 2
	}
}

func credentialLoginCmd(args []string) int {
	fs := newFlagSet("credential login")
	configPath := fs.String("config", "config/cincai.yaml", "path to cincai.yaml config file")
	flowFlag := fs.String("flow", "auto", "login flow: auto, browser, device, or manual (paste redirect URL; no port-forward)")
	callbackFlag := fs.String("callback-listen", "127.0.0.1:0", "loopback address for browser OAuth callback (see docs/oauth.md for remote SSH -L)")
	listProviders := fs.Bool("list", false, "list vendor OAuth profiles and exit")
	if wantsHelp(args) {
		printCommandHelp("cincai credential login — OAuth sign-in",
			"  cincai credential login PROFILE [flags]\n  cincai credential login --list", fs)
		return 0
	}
	rest := flagsFirstCredential(args)
	if err := parseFlags(fs, rest); err != nil {
		return 2
	}
	if *listProviders {
		for _, id := range oauth.VendorProfiles() {
			fmt.Println(id)
		}
		return 0
	}
	if fs.NArg() != 1 {
		fmt.Fprintln(os.Stderr, "cincai credential login: profile name required")
		fmt.Fprintln(os.Stderr, "vendor profiles:", strings.Join(oauth.VendorProfiles(), ", "))
		return 2
	}
	register.Register()
	id, err := cred.Login(context.Background(), cred.LoginOptions{
		ConfigPath:     *configPath,
		Profile:        fs.Arg(0),
		Flow:           generic.Flow(strings.ToLower(*flowFlag)),
		CallbackListen: *callbackFlag,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "cincai credential login: %v\n", err)
		return 1
	}
	fmt.Printf("logged in id=%d profile=%s flow=%s\n", id, fs.Arg(0), strings.ToLower(*flowFlag))
	return 0
}

func credentialListCmd(args []string) int {
	fs := newFlagSet("credential list")
	configPath := fs.String("config", "config/cincai.yaml", "path to cincai.yaml config file")
	all := fs.Bool("all", false, "include disabled credentials")
	if wantsHelp(args) {
		printCommandHelp("cincai credential list — list broker credentials",
			"  cincai credential list [flags]", fs)
		return 0
	}
	if err := parseFlags(fs, args); err != nil {
		return 2
	}
	list, err := cred.ListSummaries(context.Background(), *configPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "cincai credential list: %v\n", err)
		return 1
	}
	if !*all {
		active := list[:0]
		for _, cs := range list {
			if cs.Status != "disabled" {
				active = append(active, cs)
			}
		}
		list = active
	}
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	if err := enc.Encode(list); err != nil {
		fmt.Fprintf(os.Stderr, "cincai credential list: %v\n", err)
		return 1
	}
	return 0
}

func credentialRefreshCmd(args []string) int {
	fs := newFlagSet("credential refresh")
	configPath := fs.String("config", "config/cincai.yaml", "path to cincai.yaml config file")
	if wantsHelp(args) {
		printCommandHelp("cincai credential refresh — refresh OAuth tokens",
			"  cincai credential refresh PROFILE [flags]", fs)
		return 0
	}
	rest := flagsFirstCredential(args)
	if err := parseFlags(fs, rest); err != nil {
		return 2
	}
	if fs.NArg() != 1 {
		fmt.Fprintln(os.Stderr, "cincai credential refresh: profile name required")
		return 2
	}
	profile := fs.Arg(0)
	register.Register()
	vault, err := cred.LoadVault(*configPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "cincai credential refresh: %v\n", err)
		return 1
	}
	defer vault.Close()
	if err := cred.Refresh(context.Background(), vault, profile); err != nil {
		fmt.Fprintf(os.Stderr, "cincai credential refresh: %v\n", err)
		return 1
	}
	fmt.Printf("refreshed profile=%s\n", profile)
	return 0
}

func credentialImportCmd(args []string) int {
	fs := newFlagSet("credential import")
	configPath := fs.String("config", "config/cincai.yaml", "path to cincai.yaml config file")
	apiKey := fs.String("api-key", "", "API key value (required)")
	if wantsHelp(args) {
		printCommandHelp("cincai credential import — store an API key",
			"  cincai credential import PROFILE --api-key KEY [flags]", fs)
		return 0
	}
	rest := flagsFirstCredential(args)
	if err := parseFlags(fs, rest); err != nil {
		return 2
	}
	if fs.NArg() != 1 {
		fmt.Fprintln(os.Stderr, "cincai credential import: profile name required")
		return 2
	}
	if strings.TrimSpace(*apiKey) == "" {
		fmt.Fprintln(os.Stderr, "cincai credential import: --api-key required")
		return 2
	}
	vault, err := cred.LoadVault(*configPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "cincai credential import: %v\n", err)
		return 1
	}
	defer vault.Close()
	profile := fs.Arg(0)
	id, err := vault.Store.PutAPIKey(context.Background(), profile, strings.TrimSpace(*apiKey))
	if err != nil {
		fmt.Fprintf(os.Stderr, "cincai credential import: %v\n", err)
		return 1
	}
	fmt.Printf("imported id=%d profile=%s\n", id, profile)
	return 0
}

func credentialSetEnabledCmd(action string, args []string) int {
	fs := newFlagSet("credential " + action)
	configPath := fs.String("config", "config/cincai.yaml", "path to cincai.yaml config file")
	cause := fs.String("cause", "manual", "reason recorded when disabling (disable only)")
	if wantsHelp(args) {
		printCommandHelp("cincai credential "+action+" — "+action+" a credential",
			fmt.Sprintf("  cincai credential %s ID [flags]", action), fs)
		return 0
	}
	rest := flagsFirstCredential(args)
	if err := parseFlags(fs, rest); err != nil {
		return 2
	}
	if fs.NArg() != 1 {
		fmt.Fprintf(os.Stderr, "cincai credential %s: credential ID required (see: cincai credential list)\n", action)
		return 2
	}
	id, err := strconv.ParseInt(fs.Arg(0), 10, 64)
	if err != nil {
		fmt.Fprintf(os.Stderr, "cincai credential %s: invalid credential ID %q\n", action, fs.Arg(0))
		return 2
	}
	register.Register()
	vault, err := cred.LoadVault(*configPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "cincai credential %s: %v\n", action, err)
		return 1
	}
	defer vault.Close()
	ctx := context.Background()
	if action == "enable" {
		err = vault.Store.Enable(ctx, id)
	} else {
		err = vault.Store.Disable(ctx, id, *cause)
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "cincai credential %s: %v\n", action, err)
		return 1
	}
	fmt.Printf("%sd id=%d\n", action, id)
	return 0
}

func printCredentialUsage() {
	fmt.Fprintf(os.Stderr, `cincai credential — encrypted broker admin

Usage:
  cincai credential login PROFILE [flags]
  cincai credential login --list
  cincai credential import PROFILE --api-key KEY [flags]
  cincai credential list [flags]
  cincai credential refresh PROFILE [flags]
  cincai credential disable ID [flags]
  cincai credential enable ID [flags]

Run "cincai credential <subcommand> --help" for flags.
Requires CINCAI_BROKER_KEY (see config/cincai.dev.env).

Vendor OAuth: %s
`, strings.Join(oauth.VendorProfiles(), ", "))
}

func flagsFirstCredential(args []string) []string {
	var flags, pos []string
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if strings.HasPrefix(arg, "-") {
			flags = append(flags, arg)
			if credentialFlagTakesValue(arg) && i+1 < len(args) && !strings.HasPrefix(args[i+1], "-") {
				flags = append(flags, args[i+1])
				i++
			}
		} else {
			pos = append(pos, arg)
		}
	}
	return append(flags, pos...)
}

func credentialFlagTakesValue(arg string) bool {
	if arg == "-" || arg == "--" {
		return false
	}
	if strings.Contains(arg, "=") {
		return false
	}
	name := strings.TrimLeft(arg, "-")
	if len(name) > 1 {
		return true
	}
	switch name {
	case "h", "v":
		return false
	default:
		return true
	}
}