# Model-oriented routing — pick a model, not a provider

Cincai's core idea: you point one `base_url` at Cincai and ask for a **model id**.
Cincai knows which of *your* providers can serve it — under whatever name each
one uses — and applies your **policy**. You never wire up providers per request.

```
client:   POST /v1/chat/completions   {"model": "example-model", ...}
             │
cincai:   model example-model  →  pool [vendor, openrouter, fireworks]  →  strategy
             │                                                     │
     vendor: "example-model"   openrouter: "vendor/example-model"   fireworks: "accounts/…/example-model"
```

## The three pieces

A model's modality is a **pool** of providers plus a **strategy**:

```yaml
models:
  example-model:
    modalities:
      chat:
        wire: openai-chat-completions
        strategy: failover          # or round_robin
        providers:
          - provider_ref: vendor      # the model's own API
            model: example-model
          - provider_ref: openrouter  # same model, different upstream name
            model: vendor/example-model
          - provider_ref: fireworks
            model: accounts/vendor/models/example-model
```

1. **Canonical id** (`example-model`) — what the client asks for. Stable across providers.
2. **Per-provider name mapping** (`model:`) — each upstream calls it something
   different; Cincai substitutes the right name before forwarding.
3. **Strategy** —
   - `failover`: try providers in order; on error/retryable status, fall to the next.
   - `round_robin`: load-balance across the pool.
   - omit the pool entirely (single `provider_ref:`) to pin one provider.

Wire translation still applies underneath: hit `/v1/chat/completions` or
`/v1/messages`, and Cincai translates when a provider's protocol differs.

## Why the catalog matters

Pools, failover, and round-robin are table stakes. What makes `base_url + model`
*just work* is the **catalog** — knowing that one canonical id maps to these providers
under these upstream names with sane defaults.
Cincai ships a starter catalog (`config/providers.yaml.example`); full pool examples are in this doc. You maintain `config/providers.yaml` for your deployment.

## One caveat: same id ≠ identical model

The "same" model from two providers can differ — quant, context window, tool-call
fidelity, rate limits. Failing over from a full-precision provider to a smaller
quant silently degrades a request. So the catalog should carry per-provider
quality/param metadata and failover should respect it — don't drop onto a
materially worse variant without meaning to. Good catalogs encode that metadata
so failover can respect it.

## Provider proxy (optional)

Some upstreams need a fixed egress path (corporate proxy, geo workarounds). Set
`proxy` on the **provider**, not the model:

```yaml
providers:
  vendor:
    credential_profile: vendor-api
    proxy: http://127.0.0.1:8080   # or "direct" to ignore HTTP(S)_PROXY
    capabilities:
      chat:
        protocol: openai-chat-completions
        base_url: https://api.example.com
```

| Value | Effect |
|-------|--------|
| *(omit / empty)* | Process default (`HTTP_PROXY` / `HTTPS_PROXY` / `ProxyFromEnvironment`) |
| `http://host:port` (or `https://…`) | Fixed proxy for every upstream call on this provider |
| `direct` | No proxy for this provider, even if env is set |

## Reasoning effort (optional)

When an upstream encodes intensity in the **model name** (e.g. `example-model-medium`),
keep the public id lean and declare which tiers actually work:

```yaml
models:
  example-model:
    efforts: [low, medium, high]   # only tiers your upstream accepts
    default_effort: medium
    modalities:
      chat:
        wire: openai-chat-completions
        provider_ref: vendor
        model: example-model-medium   # default-tier pool SKU
```

- Client sends lean id + optional body field `reasoning_effort` (or `effort`).
- Empty client effort → `default_effort` when set.
- Cincai rewrites the pool model’s `-{tier}` suffix to the requested effort
  (`example-model-medium` + `high` → `example-model-high`).
- Upstream ids **without** a listed tier suffix are left alone (e.g. a failover
  partner that uses a different naming scheme).
- `GET /v1/models` exposes `efforts` and `default_effort` so clients can avoid a
  static none\|low\|medium\|high list.

Omit `efforts` when the model has no tiered SKUs — body fields then pass through
unchanged (e.g. OpenAI `reasoning_effort` on a lean model id).

---

## See also

- [media.md](media.md) — image, speech, video pools use the same routing model
- [catalog-capabilities-modalities.md](catalog-capabilities-modalities.md) — yaml key naming
- [configuration.md](configuration.md) — catalog file path in `cincai.yaml`
