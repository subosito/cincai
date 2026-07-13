#!/usr/bin/env bash
# Offline chat routing: passthrough + wire-translate (fake upstream keys).
set -euo pipefail

# shellcheck source=scripts/smoke-common.sh
source "$(dirname "$0")/smoke-common.sh"
smoke_root
smoke_env
smoke_prepare_config "${CINCAI_SMOKE_PORT:-19420}"

echo "== build =="
smoke_build

echo "== gateway key =="
smoke_create_gateway_key smoke-chat

echo "== credential import =="
./bin/cincai credential import deepseek-api \
  --api-key "${SMOKE_UPSTREAM_API_KEY:-sk-smoke-fake-upstream}" \
  --config config/cincai.yaml

echo "== serve =="
smoke_serve

echo "== GET /v1/healthz =="
curl -sf "$BASE/v1/healthz" | head -c 200
echo

echo "== GET /v1/models =="
curl -sf -H "Authorization: Bearer $GATEWAY_KEY" "$BASE/v1/models" | head -c 400
echo

echo "== POST /v1/chat/completions deepseek-v4-flash =="
CHAT_OUT=/tmp/cincai-smoke-chat.json
CHAT_CODE=$(curl -sS -o "$CHAT_OUT" -w '%{http_code}' \
  -H "Authorization: Bearer $GATEWAY_KEY" \
  -H 'Content-Type: application/json' \
  -d '{"model":"deepseek-v4-flash","messages":[{"role":"user","content":"hi"}],"max_tokens":8,"stream":false}' \
  "$BASE/v1/chat/completions")
echo "status=$CHAT_CODE body=$(head -c 300 "$CHAT_OUT")"
smoke_assert_catalog_routed "$CHAT_OUT"
if [[ "$CHAT_CODE" != "401" && "$CHAT_CODE" != "200" && "$CHAT_CODE" != "402" && "$CHAT_CODE" != "403" ]]; then
  echo "unexpected chat status $CHAT_CODE" >&2
  exit 1
fi

echo "== POST /v1/messages deepseek-v4-flash (wire-translate a2o) =="
MSG_OUT=/tmp/cincai-smoke-messages.json
MSG_CODE=$(curl -sS -o "$MSG_OUT" -w '%{http_code}' \
  -H "Authorization: Bearer $GATEWAY_KEY" \
  -H 'Content-Type: application/json' \
  -d '{"model":"deepseek-v4-flash","max_tokens":8,"messages":[{"role":"user","content":"hi"}]}' \
  "$BASE/v1/messages")
echo "status=$MSG_CODE body=$(head -c 300 "$MSG_OUT")"
smoke_assert_catalog_routed "$MSG_OUT"

echo "== smoke-chat OK =="