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

A model's modality is a **pool** plus a **strategy**. The pool is either
**providers** (leaf model) or **models** (composite — ordered public model ids).

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

## Model groups (`groups:` root)

A **group** is a named set of public model ids for **client discovery** (menus,
pickers, “reviewer models”). The gateway does **not** route to a group id.

```yaml
groups:
  reviewers:
    description: Code / PR review
    models:
      - glm-5.2
      - gpt-5.6-luna
      - qwen3.8-max

models:
  glm-5.2: { … }
  # …
```

| Rule | Detail |
|------|--------|
| **Not callable** | `POST … {"model":"reviewers"}` → **400** (`model_group`, pick a member) |
| **Members** | Each entry must exist under `models:` (leaf or composite) |
| **No id collision** | Group id must not equal a model id |
| **GET /v1/models** | Group appears with `"object":"model_group"` and `"models":[…]` |
| **Leaf back-ref** | Member models may list `"groups":["reviewers"]` |

```json
{
  "id": "reviewers",
  "object": "model_group",
  "owned_by": "cincai",
  "description": "Code / PR review",
  "models": ["glm-5.2", "gpt-5.6-luna", "qwen3.8-max"]
}
```

Clients that only want callables: filter `object === "model"`.  
Order in `groups.*.models` is UI order, not failover (use composite `modality.models` for that).

## Composite models (`models:` hops)

Some public ids are **composites**: they do not pin a provider; they name an
ordered list of **other catalog model ids**. Same root key (`models:`), same
ingress (`model: agent-cheap`), same `/v1/models` listing — no separate
`aliases` / `chains` config.

```yaml
models:
  gpt-5.6-luna:
    modalities:
      chat:
        wire: openai-chat-completions
        provider_ref: codex
        model: gpt-5.6-luna

  gemini-3.6-flash:
    modalities:
      chat:
        wire: openai-chat-completions
        provider_ref: antigravity
        model: gemini-3.6-flash-medium

  # Composite: try luna, then gemini (each hop uses that hop's own provider pool).
  agent-cheap:
    modalities:
      chat:
        wire: openai-chat-completions
        strategy: failover
        models:                         # ordered public model ids (not providers)
          - gpt-5.6-luna
          - gemini-3.6-flash
```

Rules:

| Rule | Detail |
|------|--------|
| **XOR** | A modality sets **`providers`** (or single `provider_ref`) **or** **`models`**, never both. |
| **Hops** | Each entry is an existing public model id that supports the same wire. |
| **Cycles** | `a → b → a` is rejected at resolve/validate. |
| **Strategy** | `failover` (default): try hop1's full provider pool, then hop2, …. `round_robin`: pick one hop per request. |
| **Retryable only** | Cross-hop failover uses the same statuses as provider failover (429/502/503/504/402, dial errors) — not 400s such as context length. |
| **Effort** | Applied **per hop** from that hop's catalog efforts (not invented on the composite id). |
| **GET /v1/models** | Composite ids appear as normal models with `"models": ["gpt-5.6-luna", "gemini-3.6-flash"]`. Leaf models omit the field. |

Observability: ingress **`model`** is the **hop that served** (real catalog id);
**`alias`** is the composite id from the request (e.g. `agent-cheap`). Leaf
requests omit `alias`. Response body `model` is whatever the successful upstream
returned.

**Quality note:** hopping luna → gemini is intentional degradation, not a silent
retry of the “same” model. Name composites for policy (`agent-cheap`), not as
fake duplicates of a single vendor id.

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

Declare supported efforts in **`models.meta.yaml`** (same place as context/pricing),
not in `providers.yaml` pools:

```yaml
# models.meta.yaml
models:
  gpt-5.5:
    efforts: [none, low, medium, high, xhigh]
    default_effort: medium
  gemini-3.5-flash:
    efforts: [low]          # only tiers that work on this host
    default_effort: low
  deepseek-v4-flash:
    efforts: [none, low, high, xhigh, max]
    default_effort: high
```

```yaml
# providers.yaml — pool only (SKU tier when needed)
models:
  gemini-3.5-flash:
    modalities:
      chat:
        wire: openai-chat-completions
        provider_ref: antigravity
        model: gemini-3.5-flash-low   # default-tier pool SKU
```

- Client sends lean id + body `reasoning_effort`, `effort`, or Responses
  `reasoning.effort`. **One control only** — no separate `enable_thinking` field
  from clients (gateway expands hybrid models).
- Empty client effort → `default_effort` when set.
- **SKU-tier hosts** (Gemini): rewrites a pool model’s `-{tier}` suffix when that
  suffix is in `efforts`.
- **Body ladders** (GPT, DeepSeek, …): lean ids unchanged; gateway injects
  `reasoning_effort` / `reasoning.effort` for the resolved value.
- **Hybrid thinking** (`efforts: [none, on]`, e.g. Agnes): `none` → thinking off,
  `on` → `enable_thinking` + `chat_template_kwargs.enable_thinking` (Anthropic
  wire also sets `thinking.budget_tokens` to a fixed 2048 — not catalog config).
- Upstream ids without a listed tier suffix (e.g. `vendor/foo`) are left alone.
- Omit `efforts` when the model has no controllable effort.

---

## Starter models (bundled pack)

`config/providers.yaml.example` ships one recent public SKU per bundled adapter.
Update these when vendors rename products — keep the *pattern*, not a frozen year.

| Public id | Provider | Wire |
|-----------|----------|------|
| `deepseek-v4-flash` | deepseek | chat (+ Anthropic translate) |
| `grok-imagine-image-quality` | xai | image gen |
| `grok-imagine-video` | xai | video gen |
| `eleven_v3` | elevenlabs | speech |
| `mistral-ocr-latest` | mistral | OCR via chat |
| `gemini-3.6-flash` (commented) | vertex | Gemini generateContent |

### Vertex / generateContent

Adapter name: `vertex`. OpenAI `/v1/chat/completions` is translated to Gemini
`generateContent` / `streamGenerateContent`. Set `base_url` to a Vertex project
root or any generateContent-compatible host; pool `model:` is usually
`google/<model-id>`.

Shared conversion lives in public package
[`adapters/gemini`](../adapters/gemini) (tools, multimodal, thinking budget).

## See also

- [media.md](media.md) — image, speech, video pools use the same routing model
- [catalog-capabilities-modalities.md](catalog-capabilities-modalities.md) — yaml key naming
- [configuration.md](configuration.md) — catalog file path in `cincai.yaml`
