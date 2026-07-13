package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	icatalog "github.com/subosito/cincai/internal/catalog"
	"github.com/subosito/cincai/internal/config"
)

func catalogCmd(args []string) int {
	if len(args) == 0 {
		printCatalogUsage()
		return 2
	}
	switch args[0] {
	case "validate":
		return catalogValidateCmd(args[1:])
	case "help", "-h", "--help":
		printCatalogUsage()
		return 0
	default:
		fmt.Fprintf(os.Stderr, "cincai catalog: unknown subcommand %q\n", args[0])
		printCatalogUsage()
		return 2
	}
}

func printCatalogUsage() {
	fmt.Fprintf(os.Stderr, `cincai catalog — providers.yaml tools

Usage:
  cincai catalog validate [--config PATH] [--catalog PATH]

`)
}

func catalogValidateCmd(args []string) int {
	fs := newFlagSet("catalog validate")
	configPath := fs.String("config", "config/cincai.yaml", "path to cincai.yaml config file")
	catalogPath := fs.String("catalog", "", "path to providers.yaml (overrides serve.catalog from config)")
	if wantsHelp(args) {
		printCommandHelp("cincai catalog validate — check providers.yaml loads and routes resolve",
			"  cincai catalog validate [--config PATH] [--catalog PATH]", fs)
		return 0
	}
	if err := parseFlags(fs, args); err != nil {
		return 2
	}

	path, err := resolveCatalogPath(*configPath, *catalogPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "cincai catalog validate: %v\n", err)
		return 1
	}

	cat, err := icatalog.Load(path)
	if err != nil {
		fmt.Fprintf(os.Stderr, "cincai catalog validate: load %s: %v\n", path, err)
		return 1
	}
	if err := cat.ValidateRoutes(); err != nil {
		fmt.Fprintf(os.Stderr, "cincai catalog validate: %v\n", err)
		return 1
	}
	fmt.Fprintf(os.Stdout, "catalog ok: %s (%d models)\n", path, cat.ModelCount())
	return 0
}

func resolveCatalogPath(configPath, catalogOverride string) (string, error) {
	if strings.TrimSpace(catalogOverride) != "" {
		return strings.TrimSpace(catalogOverride), nil
	}
	cfgPath := strings.TrimSpace(configPath)
	if cfgPath == "" {
		cfgPath = "config/cincai.yaml"
	}
	cfg, err := config.Load(cfgPath)
	if err != nil {
		return "", fmt.Errorf("config: %w", err)
	}
	catalogPath := cfg.Serve.Catalog
	if !filepath.IsAbs(catalogPath) {
		catalogPath = filepath.Join(filepath.Dir(cfgPath), catalogPath)
	}
	return catalogPath, nil
}
