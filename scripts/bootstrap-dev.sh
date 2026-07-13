#!/usr/bin/env bash
# Thin wrapper around cincai init (kept for scripts that still call this path).
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"

BIN="${CINCAI_BIN:-./bin/cincai}"
if [[ ! -x "$BIN" ]]; then
  go build -o bin/cincai ./cmd/cincai
fi

"$BIN" init "$@"

# shellcheck disable=SC1091
set -a && source config/cincai.dev.env && set +a

if [[ -f data/.auth/broker.db ]] && ! "$BIN" credential list --config config/cincai.yaml >/dev/null 2>&1; then
  echo "warning: broker.db exists but CINCAI_BROKER_KEY does not decrypt it" >&2
  echo "  rm data/.auth/broker.db   # then re-run cincai init" >&2
  exit 1
fi