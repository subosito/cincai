# Contributing to Cincai

Thanks for your interest in improving Cincai. This guide covers how to build, test,
and submit changes.

## Development setup

Cincai targets **Go 1.26.4+**, pinned by the `go` directive in `go.mod`. The repository
ships a [devenv](https://devenv.sh) environment that provides the Go toolchain and dev
tools (`just`, `openssl`):

```bash
devenv shell     # or: direnv allow, if you use direnv
```

You can also work with a system Go 1.26.4+ toolchain directly — no devenv required.

## Build and test

The [`justfile`](justfile) has the common tasks:

```bash
just build          # build the CLI to bin/cincai
just verify         # go vet + go test ./...   (run this before every PR)
just test           # go test ./...
just vet            # go vet ./...
just verify-smoke   # offline routing smoke tests (chat, image, speech, video)
```

`just verify` must pass before a change is submitted. Tests run with the race
detector in CI (`go test -race ./...`), so prefer running that locally for anything
touching concurrency:

```bash
go test -race ./...
```

## Coding conventions

- **Format with `gofmt`** (`go fmt ./...`) — the Go ecosystem standard.
- **Match the surrounding style** in a file you touch; avoid unrelated reformatting or
  refactoring in the same change.
- **Keep the diff scoped** to the change you're making.
- **Add a test** for non-trivial logic — the repo has table-driven tests throughout;
  follow the nearest example rather than introducing a new harness.
- Public API lives in top-level packages (`adaptersdk`, `gateway`, `catalog`,
  `compose`, `pack`, `credential/...`, etc.). Implementation details live under
  `internal/` and are not part of the compatibility surface — see the
  [project layout](#project-layout).

## Commit messages

Cincai uses [Conventional Commits](https://www.conventionalcommits.org/):

```
<type>(<optional scope>): <summary>
```

Common types: `feat`, `fix`, `docs`, `refactor`, `test`, `chore`, `ci`. The scope, when
present, names the package or area touched — e.g. `feat(catalog): …`,
`fix(keyring): …`. Keep the summary in the imperative mood.

## Pull requests

1. Branch from `main`.
2. Make your change with tests; run `just verify` (and `go test -race ./...`).
3. Open a PR describing the change and the motivation. Link any related issue.
4. Keep PRs focused — smaller, single-purpose PRs are easier to review and land.

## Project layout

| Path | What it is |
|------|------------|
| `cmd/cincai/` | CLI entry point (`init`, `serve`, `catalog`, `credential`, `keys`) |
| `cincai.go` | Batteries-included library entry (`Run`, `EmbedRun`) |
| `gateway/` | HTTP data plane, lifecycle, credential store wiring |
| `catalog/` | Model catalog: providers, models, routing pools |
| `wire/` | The data-plane engine: auth, scope checks, dispatch to adapters |
| `adaptersdk/` | SDK for writing vendor adapters |
| `compose/`, `pack/`, `link/`, `register/` | Assembling adapters/OAuth into a binary |
| `credential/` | Credential broker (encrypted store), OAuth, injection |
| `ingress/` | Inbound auth: gateway keyring + scopes |
| `upstream/` | Outbound relay to providers |
| `observability/` | Logs, OTLP metrics/traces |
| `internal/` | Implementation details — **not** a public/compat surface |
| `docs/` | Configuration, routing, credential, CLI, and integration docs |

## Security

Please do not file public issues for security-sensitive reports — see
[SECURITY.md](SECURITY.md) for how to report a vulnerability privately.

## License

By contributing, you agree that your contributions are licensed under the project's
[MIT License](LICENSE).
