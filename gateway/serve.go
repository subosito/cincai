package gateway

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/subosito/cincai/adaptersdk"
	"github.com/subosito/cincai/catalog"
	icatalog "github.com/subosito/cincai/internal/catalog"
	"github.com/subosito/cincai/internal/config"
	"github.com/subosito/cincai/observability"
)

// ServeOptions boots a standalone gateway from cincai.yaml.
type ServeOptions struct {
	ConfigPath      string
	ServiceName     string
	MetricPrefix    string
	Registry        func(cfg *ConfigFile) (*adaptersdk.Registry, error)
	CatalogLoad     func(path string) (*catalog.Catalog, error)
	DataMount       DataMount
	WrapDataHandler func(http.Handler) http.Handler
	AuxServers      AuxServersFunc
}

// Serve loads config, opens stores, and runs until ctx is cancelled or error.
// Standalone binaries call Boot/ShutdownGraceful; library embedders use EmbedServe.
func Serve(ctx context.Context, opts ServeOptions) error {
	gw, err := openGateway(opts)
	if err != nil {
		return err
	}
	defer gw.cfg.Store.Close()

	name := serviceName(opts.ServiceName)
	observability.BootWithPrefix(name, strings.TrimSpace(opts.MetricPrefix))
	defer observability.ShutdownGraceful()
	return gw.ListenAndServe(ctx)
}

// EmbedServe runs the gateway when the host already owns OTel export.
// Call observability.Hook via this helper after host Boot — do not call Boot here.
func EmbedServe(ctx context.Context, opts ServeOptions) error {
	gw, err := openGateway(opts)
	if err != nil {
		return err
	}
	defer gw.cfg.Store.Close()

	observability.HookWithPrefix(serviceName(opts.ServiceName), strings.TrimSpace(opts.MetricPrefix))
	return gw.ListenAndServe(ctx)
}

func serviceName(name string) string {
	if strings.TrimSpace(name) == "" {
		return "cincai"
	}
	return strings.TrimSpace(name)
}

func openGateway(opts ServeOptions) (*Gateway, error) {
	if opts.Registry == nil {
		return nil, fmt.Errorf("registry builder required")
	}
	cfgPath := resolveConfigPath(opts.ConfigPath)
	cfgFile, err := config.Load(cfgPath)
	if err != nil {
		return nil, fmt.Errorf("config: %w", err)
	}
	if _, err := config.BrokerKey(cfgFile); err != nil {
		return nil, err
	}
	resolvePaths(cfgFile, cfgPath)

	loadCatalog := opts.CatalogLoad
	if loadCatalog == nil {
		loadCatalog = icatalog.Load
	}
	cat, err := loadCatalog(cfgFile.Serve.Catalog)
	if err != nil {
		return nil, fmt.Errorf("catalog: %w", err)
	}
	st, ks, err := OpenStore(cfgFile)
	if err != nil {
		return nil, fmt.Errorf("store: %w", err)
	}

	reg, err := opts.Registry(cfgFile)
	if err != nil {
		st.Close()
		return nil, fmt.Errorf("adapters: %w", err)
	}

	var aux []*http.Server
	if opts.AuxServers != nil {
		aux, err = opts.AuxServers(st, ks, cfgFile)
		if err != nil {
			st.Close()
			return nil, fmt.Errorf("aux servers: %w", err)
		}
	}

	return New(Config{
		ConfigFile:      cfgPath,
		Config:          cfgFile,
		Catalog:         cat,
		Store:           st,
		KeyStore:        ks,
		Adapters:        reg,
		DataListen:      cfgFile.Serve.DataListen,
		DataMount:       opts.DataMount,
		WrapDataHandler: opts.WrapDataHandler,
		AuxServers:      aux,
	})
}

func resolveConfigPath(path string) string {
	if strings.TrimSpace(path) != "" {
		return strings.TrimSpace(path)
	}
	if _, err := os.Stat("cincai.yaml"); err == nil {
		return "cincai.yaml"
	}
	return "cincai.yaml"
}

func resolvePaths(cfg *config.File, configPath string) {
	base := filepath.Dir(configPath)
	if !filepath.IsAbs(cfg.Serve.Catalog) {
		cfg.Serve.Catalog = filepath.Join(base, cfg.Serve.Catalog)
	}
	if !filepath.IsAbs(cfg.Credential.Broker) {
		cfg.Credential.Broker = filepath.Join(base, cfg.Credential.Broker)
	}
}
