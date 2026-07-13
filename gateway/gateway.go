package gateway

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"net"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/subosito/cincai/adaptersdk"
	"github.com/subosito/cincai/catalog"
	"github.com/subosito/cincai/credential/refresh"
	"github.com/subosito/cincai/credential/seal"
	"github.com/subosito/cincai/credential/store"
	"github.com/subosito/cincai/ingress/keyring"
	"github.com/subosito/cincai/internal/config"
	"github.com/subosito/cincai/upstream"
	"github.com/subosito/cincai/wire"
)

// DataMount registers extra data-plane routes before the wire engine catch-all.
type DataMount func(mux *http.ServeMux, engine *wire.Engine)

// AuxServersFunc builds extra HTTP servers to run alongside the data plane, sharing the
// gateway's credential store, keyring, and shutdown lifecycle. It is the seam a host uses
// to add planes cincai core does not ship (e.g. a management/admin API on a second port).
// Each returned server must have Addr set; the gateway owns its Listen/Serve/Shutdown.
type AuxServersFunc func(store.Store, keyring.KeyStore, *ConfigFile) ([]*http.Server, error)

// Config assembles a running data-plane gateway (shared credential store).
type Config struct {
	ConfigFile string
	Config     *ConfigFile
	Catalog    *catalog.Catalog
	Store      store.Store
	KeyStore   keyring.KeyStore
	Adapters   *adaptersdk.Registry
	DataListen      string
	DataMount       DataMount
	WrapDataHandler func(http.Handler) http.Handler
	AuxServers      []*http.Server
}

// Gateway holds HTTP servers and lifecycle state.
type Gateway struct {
	cfg        Config
	dataSrv    *http.Server
	dataLn     net.Listener
	lnMu       sync.RWMutex
	active     sync.WaitGroup
	shutdownMu sync.Mutex
	draining   bool
}

// DataAddr returns bound data listener address (empty until ListenAndServe).
func (g *Gateway) DataAddr() string {
	g.lnMu.RLock()
	defer g.lnMu.RUnlock()
	if g.dataLn == nil {
		return ""
	}
	return g.dataLn.Addr().String()
}

// New builds a gateway from config.
func New(cfg Config) (*Gateway, error) {
	if cfg.Catalog == nil {
		return nil, fmt.Errorf("catalog required")
	}
	if cfg.Store == nil {
		return nil, fmt.Errorf("store required")
	}
	if cfg.KeyStore == nil {
		return nil, fmt.Errorf("keystore required")
	}
	if cfg.Adapters == nil {
		return nil, fmt.Errorf("adapters registry required")
	}
	return &Gateway{cfg: cfg}, nil
}

// OpenStore opens the credential store and gateway keyring from config.
// Built-in backends: sqlite (default), memory.
func OpenStore(f *ConfigFile) (store.Store, keyring.KeyStore, error) {
	keyB64, err := config.BrokerKey(f)
	if err != nil {
		return nil, nil, err
	}
	key, err := seal.ParseKey(keyB64)
	if err != nil {
		return nil, nil, err
	}

	backend := strings.TrimSpace(f.Credential.Backend)
	if backend == "" || backend == "sqlite" {
		sqlite, err := store.OpenSQLite(f.Credential.Broker, key)
		if err != nil {
			return nil, nil, err
		}
		// Default broker refreshes near-expiry vendor OAuth tokens on read.
		return refresh.New(sqlite), keyring.NewSQLStore(sqlite.DB()), nil
	}
	if backend == "memory" {
		return refresh.New(store.NewMemory(key)), keyring.NewMemoryStore(), nil
	}

	sqlite, err := store.OpenSQLite(f.Credential.Broker, key)
	if err != nil {
		return nil, nil, err
	}
	st, err := openCredentialBackend(backend, f, sqlite)
	if err != nil {
		sqlite.Close()
		return nil, nil, err
	}
	return st, keyring.NewSQLStore(sqlite.DB()), nil
}

func (g *Gateway) track(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		g.shutdownMu.Lock()
		if g.draining {
			g.shutdownMu.Unlock()
			http.Error(w, "shutting down", http.StatusServiceUnavailable)
			return
		}
		g.active.Add(1)
		g.shutdownMu.Unlock()
		defer g.active.Done()
		next.ServeHTTP(w, r)
	})
}

func (g *Gateway) dataHandler() http.Handler {
	engine := &wire.Engine{
		Catalog: g.cfg.Catalog,
		Store:   g.cfg.Store,
		Adapters: g.cfg.Adapters,
		Auth:    &keyring.Authenticator{Store: g.cfg.KeyStore},
		Client:  upstream.NewClient(),
	}
	mux := http.NewServeMux()
	if g.cfg.DataMount != nil {
		g.cfg.DataMount(mux, engine)
	}
	mux.Handle("/", g.track(engine.Handler()))
	h := http.Handler(mux)
	if g.cfg.WrapDataHandler != nil {
		h = g.cfg.WrapDataHandler(h)
	}
	return h
}

// Server hardening. WriteTimeout stays unset on purpose: chat and media responses
// stream (SSE) and have no bounded write deadline.
const (
	readHeaderTimeout = 10 * time.Second
	idleTimeout       = 120 * time.Second
	maxHeaderBytes    = 1 << 20 // 1 MiB, Go's default made explicit
)

func newServer(addr string, h http.Handler) *http.Server {
	return &http.Server{
		Addr:              addr,
		Handler:           h,
		ReadHeaderTimeout: readHeaderTimeout,
		IdleTimeout:       idleTimeout,
		MaxHeaderBytes:    maxHeaderBytes,
	}
}

// isLoopbackListen reports whether a listen address binds loopback only. An empty
// host (":9420") or 0.0.0.0 binds every interface and is not loopback.
func isLoopbackListen(addr string) bool {
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		host = addr
	}
	if host == "" {
		return false
	}
	if strings.EqualFold(host, "localhost") {
		return true
	}
	if ip := net.ParseIP(host); ip != nil {
		return ip.IsLoopback()
	}
	return false
}

// ListenAndServe starts the data-plane listener until signal or error.
// Observability: standalone binaries call observability.Boot before this; library embedders call observability.Hook.
func (g *Gateway) ListenAndServe(ctx context.Context) error {
	g.dataSrv = newServer(g.cfg.DataListen, g.dataHandler())

	errCh := make(chan error, 1+len(g.cfg.AuxServers))
	go func() {
		ln, err := net.Listen("tcp", g.cfg.DataListen)
		if err != nil {
			errCh <- fmt.Errorf("data listen: %w", err)
			return
		}
		g.lnMu.Lock()
		g.dataLn = ln
		g.lnMu.Unlock()
		slog.Info("gateway listening", "plane", "data", "addr", ln.Addr().String())
		if !isLoopbackListen(g.cfg.DataListen) {
			slog.Warn("data plane bound beyond loopback; gateway keys travel in cleartext — front it with TLS",
				"addr", g.cfg.DataListen)
		}
		errCh <- g.dataSrv.Serve(ln)
	}()
	for _, srv := range g.cfg.AuxServers {
		srv := srv
		go func() {
			ln, err := net.Listen("tcp", srv.Addr)
			if err != nil {
				errCh <- fmt.Errorf("aux listen %s: %w", srv.Addr, err)
				return
			}
			slog.Info("gateway listening", "plane", "aux", "addr", ln.Addr().String())
			errCh <- srv.Serve(ln)
		}()
	}

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	select {
	case <-ctx.Done():
		return g.Shutdown(context.Background())
	case sig := <-sigCh:
		slog.Info("gateway drain", "signal", sig.String())
		return g.Shutdown(context.Background())
	case err := <-errCh:
		if err == http.ErrServerClosed {
			return nil
		}
		_ = g.Shutdown(context.Background())
		return err
	}
}

// Shutdown stops accepts and drains in-flight requests.
func (g *Gateway) Shutdown(ctx context.Context) error {
	g.shutdownMu.Lock()
	g.draining = true
	g.shutdownMu.Unlock()

	slog.Info("gateway drain", "phase", "stop_accept")
	shutCtx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()

	done := make(chan struct{})
	go func() {
		g.active.Wait()
		close(done)
	}()

	if g.dataSrv != nil {
		_ = g.dataSrv.Shutdown(shutCtx)
	}
	for _, srv := range g.cfg.AuxServers {
		_ = srv.Shutdown(shutCtx)
	}

	select {
	case <-done:
		slog.Info("gateway drain", "phase", "complete")
	case <-shutCtx.Done():
		slog.Warn("gateway drain", "phase", "timeout")
	}
	return nil
}

// ActiveWaitGroup exposes in-flight tracking for tests.
func (g *Gateway) ActiveWaitGroup() *sync.WaitGroup { return &g.active }