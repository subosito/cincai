# Shared helpers for offline smoke scripts. Source only — do not execute.
set -euo pipefail

smoke_root() {
  ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
  cd "$ROOT"
  unset GOFLAGS
}

smoke_env() {
  export CINCAI_BROKER_KEY="${CINCAI_BROKER_KEY:-$(openssl rand -base64 32)}"
  export CINCAI_AUTH_ADMIN_TOKEN="${CINCAI_AUTH_ADMIN_TOKEN:-smoke-admin-token}"
  export CINCAI_AUTH_PROVISION_TOKEN="${CINCAI_AUTH_PROVISION_TOKEN:-smoke-provision-token}"
}

smoke_prepare_config() {
  local port="${1:-19420}"
  mkdir -p data/.auth config
  cp -f config/cincai.yaml.example config/cincai.yaml
  cp -f config/providers.yaml.example config/providers.yaml
  sed -i "s/:9420/:${port}/" config/cincai.yaml
  sed -i "s/:9421/:$((port + 1))/" config/cincai.yaml
  BASE="http://127.0.0.1:${port}"
}

smoke_build() {
  go build -o bin/cincai ./cmd/cincai
}

smoke_create_gateway_key() {
  local name="$1"
  local out
  out="$(./bin/cincai keys create --config config/cincai.yaml --name "$name" --static)"
  GATEWAY_KEY="$(echo "$out" | sed -n 's/.*key=\(sk-dg-[^ ]*\).*/\1/p')"
  if [[ -z "$GATEWAY_KEY" ]]; then
    echo "failed to parse gateway key" >&2
    exit 1
  fi
}

smoke_serve() {
  ./bin/cincai serve --config config/cincai.yaml &
  SERVE_PID=$!
  trap 'kill "$SERVE_PID" 2>/dev/null || true' EXIT
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