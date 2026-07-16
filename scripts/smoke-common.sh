# Shared helpers for offline smoke scripts. Source only — do not execute.
set -euo pipefail

smoke_root() {
  ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
  cd "$ROOT"
  unset GOFLAGS
}

smoke_env() {
  export CINCAI_BROKER_KEY="${CINCAI_BROKER_KEY:-$(openssl rand -base64 32)}"
}

smoke_cleanup() {
  if [[ -n "${SERVE_PID:-}" ]]; then
    kill "$SERVE_PID" 2>/dev/null || true
  fi
  if [[ -n "${SMOKE_DIR:-}" && -d "${SMOKE_DIR:-}" ]]; then
    rm -rf "$SMOKE_DIR"
  fi
}

# Stage config + broker in a throwaway directory and export CONFIG for callers.
# The gateway resolves serve.catalog and credential.broker relative to the config
# file, so pointing --config here keeps the whole run inside SMOKE_DIR: a smoke
# must never overwrite a real config/cincai.yaml or a real broker.
smoke_prepare_config() {
  local port="${1:-19420}"
  SMOKE_DIR="$(mktemp -d "${TMPDIR:-/tmp}/cincai-smoke.XXXXXX")"
  trap smoke_cleanup EXIT
  mkdir -p "$SMOKE_DIR/config" "$SMOKE_DIR/data/.auth"
  cp config/cincai.yaml.example "$SMOKE_DIR/config/cincai.yaml"
  cp config/providers.yaml.example "$SMOKE_DIR/config/providers.yaml"
  CONFIG="$SMOKE_DIR/config/cincai.yaml"
  sed -i "s/:9420/:${port}/" "$CONFIG"
  BASE="http://127.0.0.1:${port}"
}

smoke_build() {
  go build -o bin/cincai ./cmd/cincai
}

smoke_create_gateway_key() {
  local name="$1"
  local out
  out="$(./bin/cincai keys create --config "$CONFIG" --name "$name" --static)"
  GATEWAY_KEY="$(echo "$out" | sed -n 's/.*key=\(sk-dg-[^ ]*\).*/\1/p')"
  if [[ -z "$GATEWAY_KEY" ]]; then
    echo "failed to parse gateway key" >&2
    exit 1
  fi
}

smoke_serve() {
  ./bin/cincai serve --config "$CONFIG" &
  SERVE_PID=$!
  for _ in $(seq 1 30); do
    curl -sf "$BASE/v1/healthz" >/dev/null && return 0
    sleep 0.2
  done
  echo "gateway did not become healthy" >&2
  exit 1
}

smoke_assert_catalog_routed() {
  local outfile="$1"
  if grep -qE 'no surface|unknown model|no modality' "$outfile" 2>/dev/null; then
    echo "catalog routing failed ($(basename "$outfile"))" >&2
    exit 1
  fi
}
