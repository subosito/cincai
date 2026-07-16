#!/usr/bin/env bash
# Offline speech routing: elevenlabs (fake upstream key).
set -euo pipefail

# shellcheck source=scripts/smoke-common.sh
source "$(dirname "$0")/smoke-common.sh"
smoke_root
smoke_env
smoke_prepare_config "${CINCAI_SMOKE_PORT:-19421}"

echo "== build =="
smoke_build
smoke_create_gateway_key smoke-speech

echo "== credential import =="
./bin/cincai credential import elevenlabs-api \
  --api-key "${SMOKE_ELEVENLABS_API_KEY:-el-smoke-fake}" \
  --config "$CONFIG"

echo "== serve =="
smoke_serve

echo "== POST /v1/audio/speech eleven_v3 =="
OUTFILE=/tmp/cincai-smoke-speech.json
code=$(curl -sS -o "$OUTFILE" -w '%{http_code}' \
  -H "Authorization: Bearer $GATEWAY_KEY" \
  -H 'Content-Type: application/json' \
  -d '{"model":"eleven_v3","input":"hello","voice":"voice123"}' \
  "$BASE/v1/audio/speech")
echo "status=$code body=$(head -c 300 "$OUTFILE" 2>/dev/null || true)"
smoke_assert_catalog_routed "$OUTFILE"
if [[ "$code" != "400" && "$code" != "401" && "$code" != "403" && "$code" != "502" && "$code" != "200" ]]; then
  echo "unexpected status $code" >&2
  exit 1
fi

echo "== smoke-speech OK =="