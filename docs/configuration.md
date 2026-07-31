# Configuration

Cincai reads two yaml files:

| File | Purpose |
|------|---------|
| `config/cincai.yaml` | Gateway runtime — listeners, broker, adapters, ingress |
| `config/providers.yaml` | Model catalog — providers, models, routing pools |

Scaffold local config and dev secrets:

```bash
just build
cincai init
set -a && source config/cincai.dev.env && set +a
```

Paths in `cincai.yaml` are resolved relative to the config file directory unless absolute.

---

## `cincai.yaml`

### `serve`

| Key | Default | Description |
|-----|---------|-------------|
| `data_listen` | `127.0.0.1:9420` | Data-plane HTTP listen address (loopback by default) |
| `catalog` | `providers.yaml` | Catalog file path (relative to config dir) |
| `model_meta` | — | Optional `models.meta.yaml` (context, pricing, efforts for `GET /v1/models`) |

### `credential`

| Key | Default | Description |
|-----|---------|-------------|
| `backend` | `sqlite` | Broker storage: `sqlite` or `memory` |
| `broker` | `broker.db` | Path to encrypted credential database (`sqlite` only) |
| `encryption.key_env` | `CINCAI_BROKER_KEY` | Env var for broker AES key |
| `encryption.key_file` | — | Alternative: read key from file |

The broker holds upstream API keys and OAuth tokens. **Do not** put secrets in yaml.

`backend: memory` keeps credentials and gateway keys in process memory only (lost on restart). Still uses `CINCAI_BROKER_KEY` to encrypt blobs in RAM.

### `adapters.enable`

List of adapter drivers linked into the binary. The starter config enables the bundled pack:

```yaml
adapters:
  enable:
    - passthrough
    - wire-translate
    - xai
    - elevenlabs
    - mistral
    - vertex
```

The bundled pack is these drivers; `providers.yaml.example` shows one provider
per adapter pattern. Add providers/models and pools as needed.

- **passthrough** — relay when ingress wire matches upstream protocol
- **wire-translate** — OpenAI ↔ Anthropic protocol conversion (including
  multi-system-message → separate Anthropic system text blocks on the
  OpenAI→Anthropic path; see [architecture.md](architecture.md))
- **xai** — image generation
- **elevenlabs** — speech generation
- **mistral** — chat, and OCR translated onto the chat wire
- **vertex** — OpenAI chat → Gemini `generateContent` (GCP Vertex or any
  compatible base_url; convert helpers in package `adapters/gemini`)
Naming a driver here that no linked adapter provides is an error at startup, so a
typo fails fast rather than at route time. Enabling a driver you have no catalog
entry for is harmless. `cincai catalog validate` checks both directions: it fails
if `providers.yaml` names a protocol or adapter that no enabled driver registers.

Adapters beyond the bundled pack are registered with `pack.RegisterAdapter` and composed into your own operator binary; nothing else in `cincai.yaml` changes.

---

## Environment variables

Set in `config/cincai.dev.env` (gitignored) or your process manager.

| Variable | Required | Used by |
|----------|----------|---------|
| `CINCAI_BROKER_KEY` | Yes (serve, credential CLI) | Encrypt/decrypt `broker.db` |

Generate a broker key:

```bash
openssl rand -base64 32
```

### OAuth testing

| Variable | Effect |
|----------|--------|
| `CINCAI_FORCE_DEVICE` | Force device-code OAuth flow (non-interactive) |

---

## `providers.yaml`

The catalog defines **providers** (upstream APIs + credentials) and **models** (what clients request).

- Provider generate surfaces: `capabilities.image_gen`, `video_gen`, `speech_gen`, `chat`, …
- Optional per provider: `proxy` (fixed HTTP(S) proxy or `direct`) — see [routing.md](routing.md#provider-proxy-optional)
- Model routes: `modalities.chat`, `image_gen`, `voice`, …
- Effort lists live in `models.meta.yaml` (`efforts`, `default_effort`) — see [routing.md](routing.md#reasoning-effort-optional)

See:

- [catalog-capabilities-modalities.md](catalog-capabilities-modalities.md) — naming rules
- [catalog-inject.md](catalog-inject.md) — upstream header injection
- [routing.md](routing.md) — model pools, failover, proxy, efforts
- [media.md](media.md) — image, speech, video wires

`credential_profile` on each provider must match a profile stored in `broker.db` via `cincai credential import` or `cincai credential login`. There is no `credential_profiles` block in `cincai.yaml`.

---

## `models.meta.yaml` (optional)

Standalone product metadata for catalog models — **not** routing. Point at it with `serve.model_meta`. Incomplete is fine: only list models with credible numbers.

```yaml
billing:
  currency: USD
  version: "2026-07-31"   # bump when rates / efforts change

models:
  deepseek-v4-flash:
    context_window: 1000000
    efforts: [none, low, high, xhigh, max]
    default_effort: high
    pricing:
      input_per_mtok: 0.14
      output_per_mtok: 0.28
      cache_read_per_mtok: 0.0028

  grok-imagine-video:
    pricing:
      per_unit: 0.05
      unit: second
```

| Field | Meaning |
|-------|---------|
| `context_window` | Tokens clients may assume for this public id |
| `max_output_tokens` | Optional generation cap hint |
| `efforts` / `default_effort` | Supported reasoning efforts for clients (`GET /v1/models`) |
| `pricing.input_per_mtok` / `output_per_mtok` | USD per 1M tokens (operator estimate) |
| `pricing.cache_read_per_mtok` / `cache_write_per_mtok` | Optional cache rates |
| `pricing.per_unit` + `unit` | Non-token media (`image`, `second`, `character`, `hour`, …) |

Rules:

- Keys must exist in `providers.yaml` (or be a base id of expanded `base:facet` clones). Unknown keys fail `catalog validate` / serve.
- Expanded facets inherit base meta unless listed explicitly.
- Missing models simply omit the fields on `GET /v1/models` — no requirement that every model has meta.
- One price per catalog id (no subscription vs PAYG split).

---

## Minimal production checklist

1. `cincai init` (or copy and edit `cincai.yaml` + `providers.yaml`)
2. Set `CINCAI_BROKER_KEY` (stable — changing it invalidates the broker)
3. Import API keys / OAuth login for each `credential_profile` you use
4. `cincai keys create` — mint a gateway client key for your app
5. `cincai serve --config /path/to/cincai.yaml`
6. Optional: OTLP — see [observability.md](observability.md)

Verify offline:

```bash
just verify-chat   # chat routing smoke
just verify-media  # image + speech + video smokes
just verify-smoke  # all offline smokes
```

---

## See also

- [cli.md](cli.md) — command reference
- [credential.md](credential.md) — broker and profiles
- [observability.md](observability.md) — logs and metrics