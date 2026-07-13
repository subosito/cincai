# Catalog inject (Cincai)

Secrets live in `broker.db` (`credential_profile`). Header shaping is in `config/providers.yaml`.

## Resolution order

```text
inject: (map)           → one or more headers
inject_preset           → bearer or x-api-key
adapter default (code)  → vendor adapters (e.g. ElevenLabs)
bearer
```

## `inject:` — header map

```yaml
providers:
  elevenlabs:
    credential_profile: elevenlabs-api
    capabilities:
      speech_gen:
        adapter: elevenlabs
        base_url: https://api.elevenlabs.io

  vendor-oauth:
    credential_profile: some-oauth
    inject:
      authorization: "Bearer ${access}"
      x-account-id: "${accountId}"
    capabilities:
      chat:
        protocol: openai-chat-completions
        base_url: https://api.example.com
```

`${access}` / `${accountId}` are filled from the stored OAuth payload at request time.

ElevenLabs omits `inject` — Cincai adapter `internal/adapters/elevenlabs` supplies `xi-api-key` via `DefaultInject` (override with `inject:` map if needed).

## `inject_preset` — bearer and x-api-key

```yaml
providers:
  deepseek:
    credential_profile: deepseek-api
    inject_preset: bearer
    capabilities:
      chat:
        protocol: openai-chat-completions
        base_url: https://api.deepseek.com
```

Most providers omit `inject_preset` (defaults to bearer).

## See also

- [catalog-capabilities-modalities.md](catalog-capabilities-modalities.md)
- [credential.md](credential.md) — secrets in `broker.db`