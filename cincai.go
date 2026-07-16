// Package cincai is the batteries-included library entry point. Run and EmbedRun
// wire the curated catalog loader, adapter pack, and OAuth providers into the
// gateway engine — everything a self-hoster gets from `cincai serve`. For lower-
// level control, use the gateway package directly.
//
// Product binaries (chacha) and hosts (dududu-router) depend on this module via
// go.mod — they do not fork the engine sources.
package cincai

import (
	"context"
	"net/http"
	"strings"

	"github.com/subosito/cincai/adaptersdk"
	corecatalog "github.com/subosito/cincai/catalog"
	"github.com/subosito/cincai/compose"
	"github.com/subosito/cincai/gateway"
	"github.com/subosito/cincai/pack"
	"github.com/subosito/cincai/register"

	catalog "github.com/subosito/cincai/internal/catalog"

	_ "github.com/subosito/cincai/link" // register the default adapter/OAuth/wire-translate pack
)

// Options boots a standalone Cincai gateway (owns OTel via Boot).
type Options struct {
	ConfigPath      string
	ServiceName     string
	MetricPrefix    string
	// WrapDataHandler optionally wraps the data-plane handler (e.g. host
	// attribution for usage). Does not affect routing.
	WrapDataHandler func(http.Handler) http.Handler
}

// EmbedOptions runs the gateway inside a host process (host owns OTel Boot).
type EmbedOptions struct {
	ConfigPath      string
	ServiceName     string
	MetricPrefix    string
	WrapDataHandler func(http.Handler) http.Handler
	// AuxServers lets the host run extra HTTP servers alongside the data plane, sharing
	// the credential store, keyring, and shutdown lifecycle (e.g. a management/admin API).
	AuxServers gateway.AuxServersFunc
}

// Adapters returns every adapter linked into this binary: the stock defaults
// plus whatever registered with pack (the link package's blank imports, or your
// own pack.RegisterAdapter calls). This is the set adapters.enable selects from.
func Adapters() []adaptersdk.Adapter {
	return append(compose.DefaultAdapters(), pack.Adapters()...)
}

// LoadCatalog loads providers.yaml with the same capabilities→surfaces
// normalization used by Run/EmbedRun. Product CLIs use this for catalog validate
// without importing internal/catalog.
func LoadCatalog(path string) (*corecatalog.Catalog, error) {
	return catalog.Load(path)
}

// Run serves until ctx is cancelled. For `cincai serve` / product binaries.
func Run(ctx context.Context, opts Options) error {
	register.Register()
	return gateway.Serve(ctx, buildServeOptions(opts.ConfigPath, opts.ServiceName, opts.MetricPrefix, opts.WrapDataHandler, nil))
}

// EmbedRun serves without calling OTel Boot. For host binaries that embed the gateway.
func EmbedRun(ctx context.Context, opts EmbedOptions) error {
	register.Register()
	return gateway.EmbedServe(ctx, buildServeOptions(opts.ConfigPath, opts.ServiceName, opts.MetricPrefix, opts.WrapDataHandler, opts.AuxServers))
}

func buildServeOptions(configPath, serviceName, metricPrefix string, wrap func(http.Handler) http.Handler, aux gateway.AuxServersFunc) gateway.ServeOptions {
	cfgPath := strings.TrimSpace(configPath)
	if cfgPath == "" {
		cfgPath = "config/cincai.yaml"
	}
	name := strings.TrimSpace(serviceName)
	if name == "" {
		name = "cincai"
	}
	return gateway.ServeOptions{
		ConfigPath:      cfgPath,
		ServiceName:     name,
		MetricPrefix:    metricPrefix,
		WrapDataHandler: wrap,
		AuxServers:      aux,
		CatalogLoad:     catalog.Load,
		Registry: func(cfg *gateway.ConfigFile) (*adaptersdk.Registry, error) {
			return compose.FromConfig(cfg.Adapters.Enable, Adapters())
		},
	}
}
