# Cincai

**Model-oriented AI gateway — pick a model, not a provider.** Point one `base_url` at Cincai, ask for a model (e.g. `glm-5.2`), and it routes to whichever of *your* providers can serve it — API keys or subscription logins (Grok, Claude via OAuth) — with load-balancing, failover, and OpenAI↔Anthropic wire translation. One endpoint, one model name; chat, image, video, speech.

**Status:** offline smokes (`just verify-smoke`).

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

## Quick start

```bash
just build
cincai init
set -a && source config/cincai.dev.env && set +a
cincai catalog validate --config config/cincai.yaml
cincai serve --help          # flags: docs/cli.md
cincai keys create --config config/cincai.yaml
cincai credential import deepseek-api --api-key "$DEEPSEEK_API_KEY"
# cincai credential login xai
# Remote server? See docs/oauth.md (ssh -L 56121:127.0.0.1:56121 …)
cincai serve --config config/cincai.yaml
just verify-smoke       # offline routing smokes
```

Point your SDK at `http://127.0.0.1:9420`.

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

## Contributing

See [CONTRIBUTING.md](CONTRIBUTING.md). Security reports: [SECURITY.md](SECURITY.md).

## License

**cincai** is licensed under the **MIT License** — see [LICENSE](LICENSE).