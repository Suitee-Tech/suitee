#!/usr/bin/env bash
set -euo pipefail

GATEWAY_URL="${GATEWAY_URL:-http://localhost:8080}"
API_BASE="${GATEWAY_URL%/}/api/v1"

echo "== TOCA Smoke Test =="
echo "Gateway: $GATEWAY_URL"

echo ""
echo "[1/4] Health check (gateway)"
curl -fsS "$GATEWAY_URL/health" >/dev/null
echo "OK"

EMAIL="e2e-$(date +%s)@test.com"
PASSWORD="Test12345!"

echo ""
echo "[2/4] Register (auth) -> $EMAIL"
REGISTER_PAYLOAD=$(cat <<JSON
{
  "email": "$EMAIL",
  "password": "$PASSWORD",
  "confirm_password": "$PASSWORD",
  "type": "individual",
  "firstname": "E2E",
  "lastname": "Tester"
}
JSON
)

# Alguns ambientes podem já ter o usuário; não falhar por conflito.
REGISTER_CODE="$(curl -sS -o /tmp/toca-register.json -w "%{http_code}" \
  -H "Content-Type: application/json" \
  -H "X-Debug: true" \
  -d "$REGISTER_PAYLOAD" \
  "$API_BASE/auth/register" || true)"

if [[ "$REGISTER_CODE" != "201" && "$REGISTER_CODE" != "409" ]]; then
  echo "Register falhou (HTTP $REGISTER_CODE):"
  cat /tmp/toca-register.json || true
  exit 1
fi
ACCESS_TOKEN="$(python3 - <<'PY'
import json
with open("/tmp/toca-register.json", "r", encoding="utf-8") as f:
    body = json.load(f)
data = body.get("data") or {}
print(data.get("access_token") or "")
PY
)"

if [[ -z "$ACCESS_TOKEN" ]]; then
  echo "Não foi possível extrair access_token de /auth/register:"
  cat /tmp/toca-register.json
  exit 1
fi
echo "OK"



echo ""
echo "[4/4] /auth/me (token)"
curl -fsS \
  -H "Authorization: Bearer $ACCESS_TOKEN" \
  "$API_BASE/auth/me" > /tmp/toca-me.json
echo "OK"

echo ""
echo "Smoke test finalizado com sucesso."
