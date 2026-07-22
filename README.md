# Cincai

**Model-oriented AI gateway — pick a model, not a provider.**

Point your SDK's `base_url` at Cincai and ask for a model by name. Cincai works out
which of *your* configured providers can serve it — API-key providers and
subscription logins alike (Grok OAuth ships in the box) — and handles the rest:
load-balancing, failover, per-provider model-name mapping, and OpenAI↔Anthropic
wire translation. One endpoint and one model id, for chat, image, video, and speech.

```python
client = OpenAI(base_url="http://127.0.0.1:9420/v1", api_key="sk-dg-...")
client.chat.completions.create(model="example-model", ...)
```

The client names a model. Which provider serves it, under which upstream name, with
which credential — that's catalog policy in `providers.yaml`, not client code.

## Why

Every provider has its own endpoint, its own auth, and its own spelling of the same
model (`example-model` vs `vendor/example-model` vs
`accounts/vendor/models/example-model`). Baking those differences into every client
means touching every client each time a provider changes, rate-limits, or goes down.
Cincai moves that knowledge into one place: a catalog that maps a canonical model id
to a **pool** of providers with a failover or round-robin strategy. Swap a provider,
add a fallback, or rotate a credential by editing config — your clients never notice.

## Quick start

You need Go 1.26.4+ (`devenv shell` provides it, but any recent system Go works).

```bash
just build          # → bin/cincai
cincai init         # scaffold config/ and dev secrets from the tracked *.example templates
set -a && source config/cincai.dev.env && set +a
```

That last line loads `CINCAI_BROKER_KEY`, the key that encrypts the credential
broker — every `credential`/`keys`/`serve` command needs it in the environment.

Store the upstream credentials your catalog references (see
`config/providers.yaml`), and mint a gateway key for your app:

```bash
cincai credential import deepseek-api --api-key "$DEEPSEEK_API_KEY"
cincai credential login xai-oauth     # subscription OAuth; on a remote box see docs/oauth.md
cincai keys create                    # prints the sk-dg-... key your client sends as Bearer
```

Then check the catalog and serve:

```bash
cincai catalog validate
cincai serve
```

Point your SDK at `http://127.0.0.1:9420/v1` with the `sk-dg-...` key. All commands
default to `--config config/cincai.yaml`; pass the flag to use another path.
`just verify-smoke` runs the offline routing smokes (chat, image, speech, video) —
no network or real credentials needed.

## Docs

| File | Contents |
|------|----------|
| [docs/architecture.md](docs/architecture.md) | **Architecture** — request pipeline, packages, extension points |
| [docs/cli.md](docs/cli.md) | **CLI reference** — `init`, `serve`, `catalog`, `credential`, `keys` |
| [docs/configuration.md](docs/configuration.md) | **Configuration** — `cincai.yaml`, env vars, adapters |
| [docs/credential.md](docs/credential.md) | **Credentials** — broker, profiles, API keys vs OAuth |
| [docs/routing.md](docs/routing.md) | **Model-oriented routing** — pools, failover, name mapping |
| [docs/media.md](docs/media.md) | **Media routing** — image, speech, video |
| [docs/catalog-capabilities-modalities.md](docs/catalog-capabilities-modalities.md) | `capabilities` vs `modalities` naming |
| [docs/catalog-inject.md](docs/catalog-inject.md) | `inject:` map + `inject_preset` |
| [docs/oauth.md](docs/oauth.md) | OAuth login — **SSH port-forward** on remote servers |
| [docs/observability.md](docs/observability.md) | Logs, OTLP metrics/traces, embedding |

## Use as a library / extend

Cincai is also a Go library. All integration points are public packages — you never
import anything under `internal/`:

- **Embed the gateway** in your own binary: `cincai.EmbedRun` (or the lower-level
  `gateway` package). See [docs/observability.md](docs/observability.md).
- **Write a vendor adapter**: implement `adaptersdk.Adapter`, register with
  `pack.RegisterAdapter`, compose with `compose.FromConfig`.
- **Add an OAuth provider**: register with `credential/oauth/pack`.
- **Custom credential backend**: `gateway.RegisterCredentialBackend`.

```go
import "github.com/subosito/cincai"

func main() {
    _ = cincai.Run(context.Background(), cincai.Options{ConfigPath: "config/cincai.yaml"})
}
```

## Project status

Early and moving. The routing paths are covered by unit tests and offline smokes
(`just verify-smoke`, `go test -race ./...` in CI), not yet by years of production
mileage. Secure defaults are in place — loopback listener, encrypted credential
broker, hashed gateway keys — but read [SECURITY.md](SECURITY.md) before exposing
it beyond localhost.

## Contributing

See [CONTRIBUTING.md](CONTRIBUTING.md). Security reports: [SECURITY.md](SECURITY.md).

## License

MIT — see [LICENSE](LICENSE).
