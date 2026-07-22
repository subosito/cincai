# Credentials

Cincai separates two kinds of secrets:

| Kind | Stored in | How to add | Used for |
|------|-----------|------------|----------|
| **Upstream credentials** | broker (`sqlite` or `memory`) | `cincai credential` | API keys and OAuth tokens sent to providers |
| **Gateway client keys** | gateway keyring (same backend) | `cincai keys` | `Authorization: Bearer` on ingress requests |

This doc covers **upstream credentials**. Client keys are in [cli.md](cli.md#cincai-keys).

---

## Broker

The broker holds upstream secrets. Select it with `credential.backend`:

| `backend` | Persistence |
|-----------|-------------|
| `sqlite` (default) | Encrypted SQLite file (`credential.broker`, e.g. `broker.db`) |
| `memory` | Process memory only — gone on restart |

It holds:

- API keys (`cincai credential import`)
- OAuth access/refresh tokens (`cincai credential login`)

Encryption key: `CINCAI_BROKER_KEY` (or `credential.encryption.key_file`). **Keep this key stable** for `sqlite` — rotating it without re-importing credentials makes the broker unreadable.

```bash
cincai init
set -a && source config/cincai.dev.env && set +a
```

---

## Profile names

Every provider in `providers.yaml` declares `credential_profile`:

```yaml
providers:
  deepseek:
    credential_profile: deepseek-api
    capabilities:
      chat:
        protocol: openai-chat-completions
        base_url: https://api.deepseek.com
```

The profile string is the **only** link between catalog and broker:

- `cincai credential import deepseek-api --api-key "$DEEPSEEK_API_KEY"`
- `cincai credential login xai-oauth` → profile `xai-oauth`

There is no `credential_profiles` section in `cincai.yaml`. OAuth endpoints and client IDs live in code (vendor modules), not operator yaml.

List wired OAuth vendors:

```bash
cincai credential login --list
```

---

## API keys

For providers that use a static API key:

```bash
cincai credential import PROFILE --api-key KEY [--config config/cincai.yaml]
```

Example:

```bash
cincai credential import deepseek-api --api-key "$DEEPSEEK_API_KEY"
cincai credential import elevenlabs-api --api-key "$ELEVENLABS_API_KEY"
```

Upstream header shape (`Authorization: Bearer`, `xi-api-key`, custom `inject:` map) is configured in `providers.yaml` — see [catalog-inject.md](catalog-inject.md).

---

## OAuth (subscription logins)

Stock Cincai includes **xAI (Grok)** subscription OAuth. Other providers typically use an **API key** (`cincai credential import`). Extra OAuth providers can be linked via `credential/oauth/pack` (see [architecture.md](architecture.md)).

```bash
cincai credential login PROFILE [--flow auto|browser|device|manual] [--config config/cincai.yaml]
```

| Profile | Provider example |
|---------|------------------|
| `xai-oauth` | Grok subscription |

Use `https` for authorize/token endpoints — a plain-`http` `token_url` would send codes and refresh tokens in cleartext.

Full remote-server guide: [oauth.md](oauth.md).

While `cincai serve` runs, OAuth access tokens refresh automatically (proactive near expiry + reactive on 401). Run **one** refresher per `broker.db` — do not run two processes that refresh the same grants concurrently.

Manual refresh:

```bash
cincai credential refresh xai-oauth --config config/cincai.yaml
```

---

## List, enable, disable

```bash
cincai credential list [--all] [--config config/cincai.yaml]
```

JSON output includes `id`, `profile`, `status`, `kind` (`api_key` or `oauth`).

Temporarily stop using a credential without deleting it:

```bash
cincai credential disable ID [--cause REASON]
cincai credential enable ID
```

`ID` is the numeric id from `list`. Disabled credentials are skipped during routing.

---

## Typical workflow

```bash
# 1. Bootstrap secrets
cincai init
set -a && source config/cincai.dev.env && set +a

# 2. Upstream credentials (pick what your catalog uses)
cincai credential import deepseek-api --api-key "$DEEPSEEK_API_KEY"
cincai credential login xai-oauth --config config/cincai.yaml

# 3. Gateway client key for your app
cincai keys create --config config/cincai.yaml

# 4. Serve
cincai serve --config config/cincai.yaml
```

---

## See also

- [oauth.md](oauth.md) — SSH port-forward, manual flow, device flow
- [catalog-inject.md](catalog-inject.md) — how secrets become upstream headers
- [configuration.md](configuration.md) — `credential:` block in `cincai.yaml`
- [cli.md](cli.md) — full flag reference