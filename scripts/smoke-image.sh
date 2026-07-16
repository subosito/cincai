#!/usr/bin/env bash
# Offline image routing: xai (fake upstream keys).
set -euo pipefail

# shellcheck source=scripts/smoke-common.sh
source "$(dirname "$0")/smoke-common.sh"
smoke_root
smoke_env
smoke_prepare_config "${CINCAI_SMOKE_PORT:-19420}"

smoke_image_route() {
  local label="$1" model="$2" body="$3"
  local outfile="/tmp/cincai-smoke-image-${model//\//-}.json"
  echo "== POST /v1/images/generations $label =="
  local code
  code=$(curl -sS -o "$outfile" -w '%{http_code}' \
    -H "Authorization: Bearer $GATEWAY_KEY" \
    -H 'Content-Type: application/json' \
    -d "$body" \
    "$BASE/v1/images/generations")
  echo "status=$code body=$(head -c 300 "$outfile")"
  smoke_assert_catalog_routed "$outfile"
  if [[ "$code" != "400" && "$code" != "401" && "$code" != "403" && "$code" != "415" && "$code" != "502" && "$code" != "200" ]]; then
    echo "unexpected status $code for $model" >&2
    exit 1
  fi
}

echo "== build =="
smoke_build
smoke_create_gateway_key smoke-image

echo "== credential import =="
./bin/cincai credential import xai --api-key "${SMOKE_XAI_API_KEY:-xai-smoke-fake}" --config "$CONFIG"

echo "== serve =="
smoke_serve

smoke_image_route xai grok-imagine-image-quality \
  '{"model":"grok-imagine-image-quality","prompt":"red circle","n":1}'

echo "== smoke-image OK =="