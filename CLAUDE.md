# CLAUDE.md — working in the cincai repo

Cincai is a model-oriented AI gateway: a client asks for a *model* (e.g. `glm-5.2`) at
one OpenAI-compatible endpoint, and cincai routes to whichever configured provider can
serve it — API keys or subscription OAuth — with load-balancing, failover, and
OpenAI↔Anthropic wire translation. Chat, image, video, and speech.

## Build, test, run

Requires **Go 1.26.4+** (pinned in `go.mod` and `go.work`; `devenv shell` provides it).
If your shell Go is older than the pin, prefix commands with `GOTOOLCHAIN=go1.26.4`.

```bash
just verify         # go vet + go test ./...  — run before committing
just build          # -> bin/cincai
just verify-smoke   # offline routing smokes (chat/image/speech/video)
go test -race ./... # what CI runs
```

There is no separate lint step; `just verify` (vet + tests) is the gate. Format new
code with `gofmt`.

## Request flow (data plane)

A `/v1/*` request travels: **ingress auth** (`ingress/keyring` verifies the `sk-dg-`
gateway key) → **scope check** (`keyring.Authorize`) → **catalog resolve**
(`catalog` turns the model name + wire into a target pool) → **wire engine**
(`wire`) dispatches to an **adapter** (`adaptersdk` / `passthrough` / wire-translate)
→ **upstream relay** (`upstream`) with the provider credential injected
(`credential/inject`). Study `wire/wire.go` first — it's the spine.

## Layout and the public/internal split

- **Public packages** (top-level) are the integration surface: `adaptersdk` (write an
  adapter), `gateway` (embed the server), `catalog`, `compose`/`pack`/`link`/`register`
  (assemble adapters + OAuth into a binary), `credential/...`, `ingress/...`,
  `upstream`, `observability`, `wire`.
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
- `go.work` is gitignored; copy `go.work.example` for a local multi-module workspace.
- The smoke scripts (`scripts/smoke-*.sh`) write to a `broker.db` and copy example
  configs over `config/*.yaml`; run them against a scratch directory, not a real
  deployment's config/broker.
