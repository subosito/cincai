# AGENTS.md — working in the cincai repo

Cincai is a model-oriented AI gateway: a client asks for a *model* by name at
one OpenAI-compatible endpoint, and cincai routes to whichever configured provider can
serve it — API keys or subscription OAuth — with load-balancing, failover, and
OpenAI↔Anthropic wire translation. Chat, image, video, and speech.

## Build, test, run

Requires **Go 1.26.4+**, pinned by the `go` directive in `go.mod`.
`devenv shell` provides it from nixpkgs and sets `GOTOOLCHAIN=local`, so the
toolchain is whatever `devenv.lock` pins — no downloads. If the locked nixpkgs
ever drops below the `go.mod` floor, builds fail outright
(`go.mod requires go >= …; GOTOOLCHAIN=local`); fix with `devenv update nixpkgs`.
Outside devenv, use a 1.26.4+ Go, or prefix commands with `GOTOOLCHAIN=go1.26.4`.

```bash
just verify         # go vet + go test ./...  — run before committing
just build          # -> bin/cincai
just verify-smoke   # offline routing smokes (chat/image/speech/video)
go test -race ./... # what CI runs
```

There is no separate lint step; `just verify` (vet + tests) is the gate. Format new
code with `gofmt`.

Optional multi-module workspace: create a local `go.work` (gitignored) with
`go work init .` — do not commit it.

## Request flow (data plane)

A `/v1/*` request travels: **ingress auth** (`ingress/keyring` verifies the `sk-dg-`
gateway key) → **scope check** (`keyring.Authorize`) → **catalog resolve**
(`catalog` turns the model name + wire into a target pool; optional **composite**
`modality.models` hops, then **effort** rewrite per hop) → **wire engine** (`wire`)
dispatches to an **adapter** (`adaptersdk` / `passthrough` / wire-translate) →
**upstream relay** (`upstream`) with the provider credential injected
(`credential/inject`), using the provider's optional **proxy**. Study
`wire/wire.go` first — it's the spine.

## Catalog conventions (operators + examples)

Tracked templates are **generic** (no host-specific proxy ports, no product-only
SKUs). Prefer recent, widely available model ids that match the bundled adapters:

| Pattern | Example public id | Notes |
|---------|-------------------|--------|
| Chat passthrough | `deepseek-v4-flash` | OpenAI chat + optional Anthropic translate |
| Gemini generateContent | `gemini-3.6-flash` | adapter `vertex` + `adapters/gemini` convert |
| Image gen | `grok-imagine-image-quality` | xAI image adapter |
| Video gen | `grok-imagine-video` | OpenAI videos passthrough |
| Speech | `eleven_v3` | ElevenLabs TTS |
| OCR | `mistral-ocr-latest` | Mistral OCR on chat wire |

**Provider `proxy` (optional):** fixed HTTP(S) proxy for one provider, or `direct`
to ignore process `HTTP(S)_PROXY`. Document with neutral examples
(`http://127.0.0.1:8080`), not a deployment's private egress host.

**Model meta (`serve.model_meta` → `models.meta.yaml`, optional):** context window,
pricing, and **`efforts` / `default_effort`** live outside `providers.yaml`.
Incomplete is fine — only list models with credible numbers. Facets inherit base
meta. `GET /v1/models` exposes `context_window`, `max_output_tokens`, `pricing`,
`efforts`, and `default_effort` when present.

**Efforts:** clients send body `reasoning_effort` / `effort` / Responses
`reasoning.effort`. Catalog lists only **supported** values so clients do not
hardcode tiers. SKU-tier hosts (Gemini `…-low`) rewrite the pool model suffix;
body-only hosts (GPT, DeepSeek, lean ids like `qwen3.8-max`) keep the upstream id
and rely on the body — never invent `publicID-effort` when public equals upstream
(including facets `base:image`).

**Composite models:** modality sets **`models: [id, …]`** (xor `providers`).
Resolve flattens hops under `strategy: failover` (retryable statuses only).
`GET /v1/models` exposes the hop list as JSON `"models"`. Ingress: **`model`** =
hop that served, **`alias`** = composite request id. Public surface: `catalog`
(`Modality.Models`, `RoutePlan.Models`, `ModelListItem.Models`) and
`observability` (`RequestLog.Alias`, `UsageEvent.Alias`) — not under `internal/`.

**Model groups:** root **`groups:`** — named member lists for UI discovery
(`object: "model_group"` on `GET /v1/models`). Not callable. Public:
`catalog.ModelGroup`, `ObjectModelGroup`, `IsModelGroup`. Leaves may advertise
`groups: ["reviewers"]`.

See `config/providers.yaml.example` and [docs/routing.md](docs/routing.md).

## Layout and the public/internal split

- **Public packages** (top-level) are the integration surface: `adaptersdk` (write an
  adapter), `adapters/gemini` (OpenAI ↔ generateContent convert helpers), `gateway`
  (embed the server), `catalog`, `compose`/`pack`/`link`/`register` (assemble adapters +
  OAuth into a binary), `credential/...`, `ingress/...`, `upstream`, `observability`,
  `wire`. The **vertex** adapter (GCP Vertex / generateContent base URLs) is registered
  via `link` (`internal/adapters/vertex`).
- **`internal/`** is implementation detail and not a compatibility surface. If something
  an integrator genuinely needs is trapped in `internal/`, that's a bug to fix by adding
  a public re-export (see `gateway/config_export.go` for the pattern) — don't tell people
  to import `internal/`.
- `cmd/cincai/` is the CLI; `cincai.go` is the batteries-included library entry
  (`Run`, `EmbedRun`).
- Tests are colocated (`*_test.go`); internal-only tests use `package <pkg>` in a
  `*_internal_test.go` file.

## Conventions

- **Conventional Commits** (`feat(scope): …`, `fix: …`, `docs: …`, `chore: …`).
- Match the surrounding style in any file you touch; keep diffs scoped, no drive-by
  refactors.
- Add a test for non-trivial logic; follow the nearest table-driven example rather than
  inventing a harness.
- Open-source docs and `*.example` configs stay **vendor-neutral** (example-model,
  api.example.com). Host-specific catalogs live in product repos, not here.

## Security invariants (don't regress these)

- **Loopback by default:** `data_listen` defaults to `127.0.0.1`. Binding wider is an
  explicit operator choice and logs a cleartext warning; expose only behind TLS.
- **Secrets live in the encrypted broker** (`credential/seal`, AES-256-GCM); the broker
  file is `0600`. Never write credentials to yaml or logs. No secrets in logs/metrics.
- **Gateway keys** are high-entropy random tokens verified with SHA-256 (fast — argon2
  per request would be a DoS vector).
- Client credentials are stripped before forwarding upstream, and upstream
  `Set-Cookie`/identity headers are stripped before returning to the client.

## Gotchas

- `config/cincai.yaml`, `config/providers.yaml`, `config/cincai.dev.env`, and `data/`
  are gitignored — only the `*.example` templates are tracked. Never commit real config
  or a broker.
- `go.work` / `go.work.sum` are gitignored when present; create locally if needed.
- The smoke scripts (`scripts/smoke-*.sh`) stage their config and broker in a `mktemp`
  directory and remove it on exit, so they never touch `config/*.yaml` or a real broker.
  Keep it that way: pass `--config "$CONFIG"`, never a path under `config/`.
