// Package observability provides ingress structured logs, optional OpenTelemetry
// export, and optional host attribution (semantic slots; default wire X-Cincai-*).
//
// Every data-plane request emits one JSON line on stderr (no secrets). Spans and
// metrics are noop until the process initializes OTel.
//
// Standalone binaries (cincai serve, operator examples) own OTLP export:
//
//	observability.Boot("cincai")
//	defer observability.ShutdownGraceful()
//
// Library embedders must not call Boot when the host already
// exports OTel. Call Hook after the host observability Boot:
//
//	observability.Hook("cincai")
//	gw.ListenAndServe(ctx) // does not Boot or Shutdown OTel
//
// Embedders that want custom metric names use HookWithPrefix:
//
//	observability.HookWithPrefix("my-app-router", "my-app.gateway")
//
// Hook is idempotent. ShutdownGraceful is a no-op when Hooked().
// See docs/observability.md.
package observability
