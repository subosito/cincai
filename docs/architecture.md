# Architecture

Cincai is a **model-oriented AI gateway**: a client asks for a *model* at one
OpenAI-compatible endpoint, and cincai routes the request to whichever configured
provider can serve it — with load-balancing, failover, and OpenAI↔Anthropic wire
translation.

Cincai has a single responsibility: **run the data plane**. Credentials and gateway
keys are managed out-of-band with the `cincai` CLI (acting on the local broker) — there
is no built-in remote admin API. Operators who need remote/self-service management
compose it in their own binary from the public packages.

## Entry points

| Entry | Package | Use |
|-------|---------|-----|
| `cincai serve` | `cmd/cincai` | Run the gateway as a standalone binary |
| `cincai.Run` | root (`cincai.go`) | Library entry; owns OTel boot |
| `cincai.EmbedRun` | root | Library entry when the host already owns OTel |
| `gateway` package | `gateway` | Lower-level composition (bring your own store/registry) |

## Request pipeline

A `/v1/*` request flows through these stages:

```
client ── /v1/chat/completions ─▶ ingress auth ─▶ scope check ─▶ catalog resolve
   (sk-dg-… gateway key)            (keyring)      (keyring)        (model → target pool)
                                                                          │
   provider response ◀── upstream relay ◀── credential inject ◀── wire engine / adapter
        (headers filtered)         (outbound)      (broker)         (protocol translate)
```

1. **Ingress auth** (`ingress/keyring`) — verifies the `sk-dg-<id>.<tail>` gateway key.
   Keys are 192-bit random tokens; only a SHA-256 hash is stored.
2. **Scope check** (`ingress/keyring` scopes) — the key's scopes must allow this
   model + wire.
3. **Catalog resolve** (`catalog`) — the model name + ingress wire resolves to a
   **pool of targets** (provider + upstream model name), with a `failover` or
   `round_robin` strategy.
4. **Wire engine** (`wire`) — the dispatcher. Picks a target, invokes its adapter,
   handles failover to the next pool member.
5. **Adapter** (`adaptersdk`, `passthrough`, wire-translate) — translates the ingress
   protocol shape to the provider's, or passes through when they already match.
6. **Credential inject** (`credential/inject`, `adaptersdk/upstreamauth`) — attaches the
   provider credential from the broker; strips client auth / cookies / billing headers.
7. **Upstream relay** (`upstream`) — outbound HTTP to the provider. Refuses cross-origin
   redirects (so injected credentials can't leak), strips `Set-Cookie` / provider
   identity headers from the response, bounds SSE lines.

`wire/wire.go` is the spine — start there when reading the code.

## Capabilities

One OpenAI-compatible surface for chat, image, video, speech, plus transcription and
embeddings. Ingress paths and their wire ids:

| Path | Wire | Modality |
|------|------|----------|
| `POST /v1/chat/completions` | `openai-chat-completions` | chat |
| `POST /v1/messages` | `anthropic-messages` | chat (Anthropic shape) |
| `POST /v1/responses` | `openai-responses` | chat |
| `POST /v1/images/generations`, `/v1/images/edits` | `openai-images-generations` | image |
| `POST /v1/videos/generations`, `GET /v1/videos/{id}` | `openai-videos` | video |
| `POST /v1/audio/speech` | `openai-audio-speech` | speech (TTS) |
| `POST /v1/audio/transcriptions` | `openai-audio-transcriptions` | voice |
| `POST /v1/embeddings` | `openai-embeddings` | embeddings |
| `GET /v1/models`, `GET /v1/healthz` | — | discovery/health |

**Wire translation** (`wire` + the wire-translate adapter) lets an OpenAI-shaped client
reach an Anthropic-shaped provider and vice-versa, so `/v1/chat/completions` and
`/v1/messages` both work regardless of the upstream's native protocol.

## Catalog and routing

`providers.yaml` defines two things:

- **providers** — an upstream API + a `credential_profile` (the broker key to use).
- **models** — what a client asks for. Each model modality is a **pool** of provider
  targets plus a **strategy** (`failover` | `round_robin`), with per-target upstream
  model-name mapping.

The catalog resolver (`catalog` + normalization in `internal/catalog`) turns
`(model, wire, modality)` into an ordered list of targets. Failover walks the pool; a
retryable status (429/502/503/504/402) or a *connection-setup* error advances to the
next target — post-send errors do not, so a non-idempotent generation isn't billed twice.

## Credentials

- **Broker** (`credential/store`) — the credential vault. Backends: `sqlite`
  (AES-256-GCM at rest, file `0600`) or `memory`.
- **Kinds** — `api_key` and `oauth` (subscription logins).
- **OAuth** (`credential/oauth/*`) — interactive login with PKCE + loopback callback, a
  config-driven generic provider, and token refresh (proactive near expiry + reactive on
  upstream 401).
- **Sealing** (`credential/seal`) — the AES key handling. The broker key comes from
  `CINCAI_BROKER_KEY` or a key file; it is never written by the gateway.

Management is CLI-only: `cincai credential import|login|list|enable|disable` and
`cincai keys create|revoke` operate directly on the broker.

## Assembly and extension points

Adapters and OAuth providers are registered at init time and composed into the binary:

```
adapter init() ─▶ pack.RegisterAdapter ─┐
                                         ├─▶ compose.FromConfig(enable, all) ─▶ Registry
default pack (link) ────────────────────┘
```

All extension points are **public** packages — integrators never import `internal/`:

| To… | Use |
|-----|-----|
| Write a vendor adapter | `adaptersdk.Adapter` + `pack.RegisterAdapter` + `compose` |
| Add an OAuth provider | `credential/oauth/pack` |
| Add a credential backend | `gateway.RegisterCredentialBackend` |
| Embed the gateway | `cincai.EmbedRun` / `gateway` + `DataMount` |
| Bundle the defaults | blank-import `link`; call `register.Register()` |

## Observability & lifecycle

- **`observability`** — structured logs (slog), OTLP metrics + traces (OpenTelemetry),
  HTTP instrumentation, and usage metering (tokens + media units).
- **Lifecycle** — the server sets `ReadHeaderTimeout` / `IdleTimeout` (Slowloris
  hardening), tracks in-flight requests, and drains gracefully on SIGINT/SIGTERM.

## Package map

**Public (integration surface):** `adaptersdk` (+ `handler`, `messages`, `upstreamauth`),
`catalog`, `compose`, `pack`, `link`, `register`, `gateway`, `wire`, `upstream`,
`passthrough`, `observability`, `credential` (+ `store`, `seal`, `inject`, `refresh`,
`oauth/*`), `ingress/keyring`.

**Internal (`internal/`, not a compatibility surface):** config parsing, catalog
normalization (flatten/inject/wire-translate), the bundled adapters and OAuth providers,
message shapes, and limits.

## Configuration

Two files, resolved relative to the config directory:

- **`cincai.yaml`** — runtime: `serve` (listen/catalog), `credential` (broker), `adapters`
  (enabled drivers), `ingress` (client auth).
- **`providers.yaml`** — the catalog: providers, models, routing pools.

The data plane defaults to `127.0.0.1:9420` (loopback). Binding wider is an explicit
choice and logs a cleartext warning — front it with TLS.
