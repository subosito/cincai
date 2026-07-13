package observability

import (
	"context"
	"log/slog"
	"time"
)

// Boot initializes OTLP export for a standalone cincai process (CLI / operator binary).
// Library embedders must not call Boot — use Hook after the host app observability Boot.
func Boot(serviceName string) {
	if _, err := Init(serviceName); err != nil {
		slog.Error("observability: init failed", "service", serviceName, "err", err)
	}
}

// ShutdownGraceful flushes standalone OTLP exporters. No-op when Hooked (embedder owns shutdown).
func ShutdownGraceful() {
	if Hooked() {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := Shutdown(ctx); err != nil {
		slog.Warn("observability: shutdown", "err", err)
	}
}