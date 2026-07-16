# Observability

Cincai emits structured ingress logs on every data-plane request. OpenTelemetry traces and metrics are **optional** — enabled when OTLP env vars are set.

No secrets (API keys, bearer tokens) appear in logs or metric labels.

---

## Ingress logs

Each request writes one JSON line to stderr:

```json
{
  "wire": "openai-chat-completions",
  "model": "example-model",
  "provider_ref": "vendor",
  "protocol": "openai-chat-completions",
  "status": 200,
  "latency_ms": 842,
  "principal_id": "key:1",
  "usage": {
    "input_tokens": 120,
    "output_tokens": 45
  }
}
```

Media wires may include `usage.units` and `usage.unit` (`image`, `second`, `character`) instead of tokens.

Fields:

| Field | Description |
|-------|-------------|
| `wire` | Ingress wire id (e.g. `openai-images-generations`) |
| `model` | Catalog model id from the request |
| `provider_ref` | Selected provider from the catalog |
| `protocol` | Upstream protocol / adapter surface |
| `status` | HTTP status returned to the client |
| `latency_ms` | End-to-end ingress latency |
| `principal_id` | Authenticated gateway key (when keyring auth) |
| `usage` | Token or media units parsed from upstream (omitted when zero) |

When OTLP is enabled, logs include `trace_id` and `span_id` for correlation.

---

## Optional host attribution

Labels only — **not** used for routing (path + body `model` only). Semantic
slots are product-agnostic; wire header **names** are configurable aliases.

| Slot | Meaning | Default header |
|------|---------|----------------|
| Actor | Who is calling (service, bot, app) | `X-Cincai-Actor` |
| Session | Conversation / session id | `X-Cincai-Session` |
| Component | Feature bucket (`turn.chat`, `tool.generate_image`, …) | `X-Cincai-Component` |
| (always) | Shared correlation | `X-Correlation-Id` |

**Default:** `StashHostAttribution` accepts the `X-Cincai-*` names above.

**Aliases:** `StashHostAttributionWith(AttributionConfig{…})` lists alternate
headers per slot. First non-empty alias wins; **every** configured alias is
stripped before upstream. Empty slots fall back to the defaults.

```go
cfg := observability.AttributionConfig{
    Actor:     []string{"X-Host-Actor", observability.HeaderActor},
    Session:   []string{"X-Host-Session", observability.HeaderSession},
    Component: []string{"X-Host-Component", observability.HeaderComponent},
}
opts.WrapDataHandler = observability.StashHostAttributionWith(cfg)
```

Clients send their own header names; the gateway operator lists the aliases to
accept. Headers not in the config are ignored.

Package: `observability` (`HostAttribution`, `HostAttributionFrom`, constants
`HeaderActor` / `HeaderSession` / `HeaderComponent`).

---

## OpenTelemetry (standalone)

`cincai serve` calls `observability.Boot("cincai")` on startup and shuts down exporters on exit.

Export is **off** unless you set an OTLP endpoint. Standard env vars:

| Variable | Description |
|----------|-------------|
| `OTEL_EXPORTER_OTLP_ENDPOINT` | OTLP HTTP endpoint (required to enable export) |
| `OTEL_SERVICE_NAME` | Service name (default: `cincai`) |
| `OTEL_EXPORTER_OTLP_HEADERS` | Comma-separated `key=value` headers |
| `OTEL_EXPORTER_AUTH_TOKEN` | Shorthand: sets `Authorization: Bearer …` |
| `OTEL_SDK_DISABLED` | `true` / `1` — force noop exporters |

Example (local collector):

```bash
export OTEL_EXPORTER_OTLP_ENDPOINT=http://127.0.0.1:4318
export OTEL_SERVICE_NAME=cincai
cincai serve --config config/cincai.yaml
```

Traces, metrics, and logs export over OTLP HTTP when enabled.

---

## Metrics

Default metric prefix: `cincai`.

| Metric | Description |
|--------|-------------|
| `cincai.ingress.requests` | Ingress request count (labels: `wire`, `model`, `provider_ref`, `protocol`, `status`, `status_class`) |
| `cincai.ingress.duration_ms` | Ingress latency histogram |
| `cincai.upstream.requests` | Upstream HTTP count (label: `host`) |
| `cincai.upstream.duration_ms` | Upstream latency histogram |

When OTLP is disabled, instruments are noop — zero overhead beyond the stderr log line.

---

## Embedding Cincai in another binary

If the host process already exports OpenTelemetry, **do not** call `Boot`. Hook Cincai after the host initializes OTel:

```go
observability.Hook("cincai")
// then gateway.EmbedRun / ListenAndServe
```

Custom metric prefix (e.g. host-branded dashboards):

```go
observability.HookWithPrefix("my-host-router", "myhost.gateway")
```

`Hook` is idempotent. `ShutdownGraceful` is a no-op when only `Hook` was used.

---

## Usage sink (embedders)

To persist per-request usage outside stderr logs, register a `UsageSink`:

```go
observability.SetUsageSink(mySink)
```

The sink receives `UsageEvent` with wire, model, provider, latency, and measured usage. Implementations must not block the request path. Cincai defaults to no sink — the stderr log line is the only output.

---

## See also

- [configuration.md](configuration.md) — listeners and process setup
- [media.md](media.md) — media unit fields in `usage`