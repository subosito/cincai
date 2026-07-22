# Catalog inject — turning stored credentials into upstream headers

A stored credential is just a secret; each provider expects it in a different
place — `Authorization: Bearer`, `x-api-key`, `xi-api-key`, sometimes several
headers from one OAuth payload. The split of responsibilities: the **secret**
lives in the encrypted broker under the provider's `credential_profile`, and
the **header shape** is declared per provider in `config/providers.yaml`. This
doc covers the shaping side.

## Resolution order

For each provider, the first of these that applies wins:

```text
inject: (map)           → one or more headers, templated from the credential
inject_preset           → bearer or x-api-key
adapter default (code)  → vendor adapters ship one (e.g. ElevenLabs → xi-api-key)
bearer                  → the fallback when nothing else is declared
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