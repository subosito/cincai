// Package cincai is the batteries-included library entry point. Run and EmbedRun
// wire the curated catalog loader, adapter pack, and OAuth providers into the
// gateway engine — everything a self-hoster gets from `cincai serve`. For lower-
// level control, use the gateway package directly.
package cincai

import (
	"context"
	"net/http"
	"strings"

	"github.com/subosito/cincai/adaptersdk"
	"github.com/subosito/cincai/compose"
	"github.com/subosito/cincai/gateway"
	"github.com/subosito/cincai/pack"
	"github.com/subosito/cincai/register"

	catalog "github.com/subosito/cincai/internal/catalog"

	_ "github.com/subosito/cincai/link" // register the default adapter/OAuth/wire-translate pack
)

// Options boots a standalone Cincai gateway (owns OTel via Boot).
type Options struct {
	ConfigPath   string
	ServiceName  string
	MetricPrefix string
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

// Run serves until ctx is cancelled. For `cincai serve`.
func Run(ctx context.Context, opts Options) error {
	register.Register()
	return gateway.Serve(ctx, buildServeOptions(opts.ConfigPath, opts.ServiceName, opts.MetricPrefix, nil, nil))
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
			all := append(compose.DefaultAdapters(), pack.Adapters()...)
			return compose.FromConfig(cfg.Adapters.Enable, all)
		},
	}
}
