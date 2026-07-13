#!/usr/bin/env bash
# Offline video routing: xai passthrough (fake upstream key).
set -euo pipefail

# shellcheck source=scripts/smoke-common.sh
source "$(dirname "$0")/smoke-common.sh"
smoke_root
smoke_env
smoke_prepare_config "${CINCAI_SMOKE_PORT:-19422}"

echo "== build =="
smoke_build
smoke_create_gateway_key smoke-video

echo "== credential import =="
./bin/cincai credential import xai \
  --api-key "${SMOKE_XAI_API_KEY:-xai-smoke-fake}" \
  --config config/cincai.yaml

echo "== serve =="
smoke_serve

echo "== POST /v1/videos/generations grok-imagine-video =="
OUTFILE=/tmp/cincai-smoke-video.json
code=$(curl -sS -o "$OUTFILE" -w '%{http_code}' \
  -H "Authorization: Bearer $GATEWAY_KEY" \
  -H 'Content-Type: application/json' \
  -d '{"model":"grok-imagine-video","prompt":"ocean wave","poll_interval_sec":1,"timeout_sec":5}' \
  "$BASE/v1/videos/generations")
echo "status=$code body=$(head -c 300 "$OUTFILE" 2>/dev/null || true)"
smoke_assert_catalog_routed "$OUTFILE"
if [[ "$code" != "400" && "$code" != "401" && "$code" != "403" && "$code" != "502" && "$code" != "200" && "$code" != "504" ]]; then
  echo "unexpected status $code" >&2
  exit 1
fi

echo "== smoke-video OK =="