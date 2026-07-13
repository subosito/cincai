# Model-oriented routing — pick a model, not a provider

Cincai's core idea: you point one `base_url` at Cincai and ask for a **model id**.
Cincai knows which of *your* providers can serve it — under whatever name each
one uses — and applies your **policy**. You never wire up providers per request.

```
client:   POST /v1/chat/completions   {"model": "glm-5.2", ...}
             │
cincai:   model glm-5.2  →  pool [zhipu, openrouter, fireworks]  →  strategy
             │                                                     │
           zhipu: "glm-5.2"   openrouter: "z-ai/glm-5.2"   fireworks: "accounts/…/glm-5.2"
```

## The three pieces

A model's modality is a **pool** of providers plus a **strategy**:

```yaml
models:
  glm-5.2:
    modalities:
      chat:
        wire: openai-chat-completions
        strategy: failover          # or round_robin
        providers:
          - provider_ref: zhipu       # native name
            model: glm-5.2
          - provider_ref: openrouter  # same model, different upstream name
            model: z-ai/glm-5.2
          - provider_ref: fireworks
            model: zhipu/glm-5.2
```

1. **Canonical id** (`glm-5.2`) — what the client asks for. Stable across providers.
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
*just work* is the **catalog** — knowing that `glm-5.2` maps to these providers
under these upstream names with sane defaults.
Cincai ships a starter catalog (`config/providers.yaml.example`); full pool examples are in this doc. You maintain `config/providers.yaml` for your deployment.

## One caveat: same id ≠ identical model

The "same" model from two providers can differ — quant, context window, tool-call
fidelity, rate limits. Failing over from a full-precision provider to a smaller
quant silently degrades a request. So the catalog should carry per-provider
quality/param metadata and failover should respect it — don't drop onto a
materially worse variant without meaning to. Good catalogs encode that metadata
so failover can respect it.

---

## See also

- [media.md](media.md) — image, speech, video pools use the same routing model
- [catalog-capabilities-modalities.md](catalog-capabilities-modalities.md) — yaml key naming
- [configuration.md](configuration.md) — catalog file path in `cincai.yaml`
