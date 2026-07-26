package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/subosito/cincai/gateway"
	"github.com/subosito/cincai/ingress/keyring"
)

func keysCmd(args []string) int {
	if len(args) == 0 {
		printKeysUsage()
		return 2
	}
	switch args[0] {
	case "create":
		return keysCreateCmd(args[1:])
	case "list":
		return keysListCmd(args[1:])
	case "set-scopes":
		return keysSetScopesCmd(args[1:])
	case "set-name":
		return keysSetNameCmd(args[1:])
	case "revoke":
		return keysRevokeCmd(args[1:])
	case "help", "-h", "--help":
		printKeysUsage()
		return 0
	default:
		fmt.Fprintf(os.Stderr, "cincai keys: unknown subcommand %q\n", args[0])
		printKeysUsage()
		return 2
	}
}

func keysCreateCmd(args []string) int {
	fs := newFlagSet("keys create")
	configPath := fs.String("config", "config/cincai.yaml", "path to cincai.yaml config file")
	name := fs.String("name", "default", "gateway key display name")
	static := fs.Bool("static", true, "create a static (non-expiring) key")
	ttlStr := fs.String("ttl", "720h", "TTL for issued keys when --static=false")
	scopesStr := fs.String("scopes", "*", "comma-separated scopes (model:ID, wire:ID, or *)")
	if wantsHelp(args) {
		printCommandHelp("cincai keys create — mint a gateway client key",
			"  cincai keys create [flags]", fs)
		return 0
	}
	if err := parseFlags(fs, args); err != nil {
		return 2
	}

	cfgFile, err := gateway.LoadConfig(*configPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "cincai keys create: %v\n", err)
		return 1
	}
	resolveBrokerPath(cfgFile, *configPath)

	st, ks, err := gateway.OpenStore(cfgFile)
	if err != nil {
		fmt.Fprintf(os.Stderr, "cincai keys create: %v\n", err)
		return 1
	}
	defer st.Close()

	kind := keyring.KindIssued
	var ttl time.Duration
	if *static {
		kind = keyring.KindStatic
	} else {
		ttl, err = time.ParseDuration(*ttlStr)
		if err != nil {
			fmt.Fprintf(os.Stderr, "cincai keys create: ttl: %v\n", err)
			return 1
		}
	}

	scopes := parseScopesCSV(*scopesStr)
	secret, id, err := ks.Create(context.Background(), *name, kind, ttl, scopes)
	if err != nil {
		fmt.Fprintf(os.Stderr, "cincai keys create: %v\n", err)
		return 1
	}
	fmt.Printf("id=%d name=%s kind=%s key=%s\n", id, *name, kind, secret)
	return 0
}

func keysListCmd(args []string) int {
	fs := newFlagSet("keys list")
	configPath := fs.String("config", "config/cincai.yaml", "path to cincai.yaml config file")
	if wantsHelp(args) {
		printCommandHelp("cincai keys list — list gateway client keys",
			"  cincai keys list [flags]", fs)
		return 0
	}
	if err := parseFlags(fs, args); err != nil {
		return 2
	}

	cfgFile, err := gateway.LoadConfig(*configPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "cincai keys list: %v\n", err)
		return 1
	}
	resolveBrokerPath(cfgFile, *configPath)

	st, ks, err := gateway.OpenStore(cfgFile)
	if err != nil {
		fmt.Fprintf(os.Stderr, "cincai keys list: %v\n", err)
		return 1
	}
	defer st.Close()

	keys, err := ks.List(context.Background())
	if err != nil {
		fmt.Fprintf(os.Stderr, "cincai keys list: %v\n", err)
		return 1
	}
	for _, k := range keys {
		fmt.Printf("id=%d name=%s kind=%s scopes=%v expires=%s revoked=%v\n",
			k.ID, k.Name, k.Kind, k.Scopes, formatKeyExpiry(k.ExpiresAt), k.Revoked)
	}
	return 0
}

func keysSetScopesCmd(args []string) int {
	fs := newFlagSet("keys set-scopes")
	configPath := fs.String("config", "config/cincai.yaml", "path to cincai.yaml config file")
	scopesStr := fs.String("scopes", "", "comma-separated scopes (model:ID, wire:ID, or *); required")
	if wantsHelp(args) {
		printCommandHelp("cincai keys set-scopes — replace scopes on an existing key (no secret rotation)",
			"  cincai keys set-scopes ID --scopes model:demo,wire:openai-chat-completions [flags]", fs)
		return 0
	}
	rest := flagsFirstCredential(args)
	if err := parseFlags(fs, rest); err != nil {
		return 2
	}
	if fs.NArg() != 1 {
		fmt.Fprintln(os.Stderr, "cincai keys set-scopes: key ID required (see: cincai keys list)")
		return 2
	}
	if strings.TrimSpace(*scopesStr) == "" {
		fmt.Fprintln(os.Stderr, "cincai keys set-scopes: --scopes is required")
		return 2
	}
	id, err := strconv.ParseInt(fs.Arg(0), 10, 64)
	if err != nil {
		fmt.Fprintf(os.Stderr, "cincai keys set-scopes: invalid key ID %q\n", fs.Arg(0))
		return 2
	}
	scopes := parseScopesCSV(*scopesStr)

	cfgFile, err := gateway.LoadConfig(*configPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "cincai keys set-scopes: %v\n", err)
		return 1
	}
	resolveBrokerPath(cfgFile, *configPath)

	st, ks, err := gateway.OpenStore(cfgFile)
	if err != nil {
		fmt.Fprintf(os.Stderr, "cincai keys set-scopes: %v\n", err)
		return 1
	}
	defer st.Close()

	if err := ks.SetScopes(context.Background(), id, scopes); err != nil {
		fmt.Fprintf(os.Stderr, "cincai keys set-scopes: %v\n", err)
		return 1
	}
	fmt.Printf("id=%d scopes=%v\n", id, scopes)
	return 0
}

func keysSetNameCmd(args []string) int {
	fs := newFlagSet("keys set-name")
	configPath := fs.String("config", "config/cincai.yaml", "path to cincai.yaml config file")
	name := fs.String("name", "", "new display name (principal_id in logs); required")
	if wantsHelp(args) {
		printCommandHelp("cincai keys set-name — rename an existing key (no secret rotation)",
			"  cincai keys set-name ID --name friend-alice [flags]", fs)
		return 0
	}
	rest := flagsFirstCredential(args)
	if err := parseFlags(fs, rest); err != nil {
		return 2
	}
	if fs.NArg() != 1 {
		fmt.Fprintln(os.Stderr, "cincai keys set-name: key ID required (see: cincai keys list)")
		return 2
	}
	if strings.TrimSpace(*name) == "" {
		fmt.Fprintln(os.Stderr, "cincai keys set-name: --name is required")
		return 2
	}
	id, err := strconv.ParseInt(fs.Arg(0), 10, 64)
	if err != nil {
		fmt.Fprintf(os.Stderr, "cincai keys set-name: invalid key ID %q\n", fs.Arg(0))
		return 2
	}

	cfgFile, err := gateway.LoadConfig(*configPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "cincai keys set-name: %v\n", err)
		return 1
	}
	resolveBrokerPath(cfgFile, *configPath)

	st, ks, err := gateway.OpenStore(cfgFile)
	if err != nil {
		fmt.Fprintf(os.Stderr, "cincai keys set-name: %v\n", err)
		return 1
	}
	defer st.Close()

	if err := ks.SetName(context.Background(), id, *name); err != nil {
		fmt.Fprintf(os.Stderr, "cincai keys set-name: %v\n", err)
		return 1
	}
	fmt.Printf("id=%d name=%s\n", id, strings.TrimSpace(*name))
	return 0
}

func keysRevokeCmd(args []string) int {
	fs := newFlagSet("keys revoke")
	configPath := fs.String("config", "config/cincai.yaml", "path to cincai.yaml config file")
	if wantsHelp(args) {
		printCommandHelp("cincai keys revoke — revoke a gateway client key",
			"  cincai keys revoke ID [flags]", fs)
		return 0
	}
	rest := flagsFirstCredential(args)
	if err := parseFlags(fs, rest); err != nil {
		return 2
	}
	if fs.NArg() != 1 {
		fmt.Fprintln(os.Stderr, "cincai keys revoke: key ID required (see: cincai keys list)")
		return 2
	}
	id, err := strconv.ParseInt(fs.Arg(0), 10, 64)
	if err != nil {
		fmt.Fprintf(os.Stderr, "cincai keys revoke: invalid key ID %q\n", fs.Arg(0))
		return 2
	}

	cfgFile, err := gateway.LoadConfig(*configPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "cincai keys revoke: %v\n", err)
		return 1
	}
	resolveBrokerPath(cfgFile, *configPath)

	st, ks, err := gateway.OpenStore(cfgFile)
	if err != nil {
		fmt.Fprintf(os.Stderr, "cincai keys revoke: %v\n", err)
		return 1
	}
	defer st.Close()

	if err := ks.Revoke(context.Background(), id); err != nil {
		fmt.Fprintf(os.Stderr, "cincai keys revoke: %v\n", err)
		return 1
	}
	fmt.Printf("revoked id=%d\n", id)
	return 0
}

func parseScopesCSV(s string) []string {
	var scopes []string
	for _, part := range strings.Split(s, ",") {
		if t := strings.TrimSpace(part); t != "" {
			scopes = append(scopes, t)
		}
	}
	return scopes
}

func resolveBrokerPath(cfgFile *gateway.ConfigFile, configPath string) {
	base := filepath.Dir(configPath)
	brokerPath := cfgFile.Credential.Broker
	if !filepath.IsAbs(brokerPath) {
		brokerPath = filepath.Join(base, brokerPath)
	}
	cfgFile.Credential.Broker = brokerPath
}

func formatKeyExpiry(exp *int64) string {
	if exp == nil || *exp == 0 {
		return "never"
	}
	return time.UnixMilli(*exp).UTC().Format(time.RFC3339)
}

func printKeysUsage() {
	fmt.Fprintf(os.Stderr, `cincai keys — gateway client keys

Usage:
  cincai keys create [flags]
  cincai keys list [flags]
  cincai keys set-scopes ID --scopes model:ID,wire:ID [flags]
  cincai keys set-name ID --name NAME [flags]
  cincai keys revoke ID [flags]

Run "cincai keys <subcommand> --help" for flags.

`)
}
