#!/usr/bin/env bash
set -euo pipefail

BASE_URL="${BASE_URL:-http://10.0.5.8:30180}"
RUN_ID="${RUN_ID:-$(date -u +%Y%m%dT%H%M%SZ)-oidc-sso-preflight}"
OUT_DIR="${OUT_DIR:-doc/02_acceptance/02-regression/oidc-sso}"
RUN_DIR="doc/02_acceptance/runs/${RUN_ID}"

mkdir -p "$OUT_DIR" "$RUN_DIR"

DISCOVERY_JSON="$RUN_DIR/oidc-discovery.json"
LOGIN_HTML="$RUN_DIR/login.html"
INDEX_JS="$RUN_DIR/index.js"
LOGIN_PAGE_JS="$RUN_DIR/login-page.js"
CALLBACK_PAGE_JS="$RUN_DIR/oidc-callback-page.js"
OIDC_HEADERS="$RUN_DIR/oidc-login.headers"
KEYCLOAK_HEADERS="$RUN_DIR/keycloak-auth.headers"
KEYCLOAK_HTML="$RUN_DIR/keycloak-auth.html"
SUMMARY_JSON="$RUN_DIR/oidc-sso-preflight-summary.json"
SUMMARY_MD="$RUN_DIR/oidc-sso-preflight-summary.md"

failures=()

require() {
  local name="$1"
  local condition="$2"
  if [[ "$condition" != "true" ]]; then
    failures+=("$name")
  fi
}

curl --noproxy '*' -fsS "$BASE_URL/realms/master/.well-known/openid-configuration" -o "$DISCOVERY_JSON"
issuer="$(jq -r '.issuer // ""' "$DISCOVERY_JSON")"
auth_endpoint="$(jq -r '.authorization_endpoint // ""' "$DISCOVERY_JSON")"
token_endpoint="$(jq -r '.token_endpoint // ""' "$DISCOVERY_JSON")"

require "discovery issuer uses public gateway" "$([[ "$issuer" == "$BASE_URL/realms/master" ]] && echo true || echo false)"
require "authorization endpoint uses public gateway" "$([[ "$auth_endpoint" == "$BASE_URL/realms/master/protocol/openid-connect/auth" ]] && echo true || echo false)"
require "token endpoint uses public gateway" "$([[ "$token_endpoint" == "$BASE_URL/realms/master/protocol/openid-connect/token" ]] && echo true || echo false)"

curl --noproxy '*' -fsS "$BASE_URL/login" -o "$LOGIN_HTML"
index_asset="$(rg -o '/assets/index-[^" ]+\.js' "$LOGIN_HTML" | head -n1 || true)"
require "login index asset present" "$([[ -n "$index_asset" ]] && echo true || echo false)"
curl --noproxy '*' -fsS "$BASE_URL$index_asset" -o "$INDEX_JS"
login_chunk="$(rg -o 'LoginPage-[A-Za-z0-9_-]+\.js' "$INDEX_JS" | head -n1 || true)"
callback_chunk="$(rg -o 'OidcCallbackPage-[A-Za-z0-9_-]+\.js' "$INDEX_JS" | head -n1 || true)"
require "login lazy chunk present" "$([[ -n "$login_chunk" ]] && echo true || echo false)"
require "oidc callback lazy chunk present" "$([[ -n "$callback_chunk" ]] && echo true || echo false)"

if [[ -n "$login_chunk" ]]; then
  curl --noproxy '*' -fsS "$BASE_URL/assets/$login_chunk" -o "$LOGIN_PAGE_JS"
fi
if [[ -n "$callback_chunk" ]]; then
  curl --noproxy '*' -fsS "$BASE_URL/assets/$callback_chunk" -o "$CALLBACK_PAGE_JS"
fi

login_chunk_ok=false
if [[ -s "$LOGIN_PAGE_JS" ]] && rg -q 'OIDC / SSO|统一身份提供方|auth/oidc/login' "$LOGIN_PAGE_JS"; then
  login_chunk_ok=true
fi
callback_chunk_ok=false
if [[ -s "$CALLBACK_PAGE_JS" ]] && rg -q 'access_token|refresh_token|OIDC 登录态校验失败' "$CALLBACK_PAGE_JS"; then
  callback_chunk_ok=true
fi
require "login chunk contains sso tab" "$login_chunk_ok"
require "callback chunk consumes oidc tokens" "$callback_chunk_ok"

redirect_url="$BASE_URL/oidc/callback?next=%2Fdashboard"
oidc_start="$BASE_URL/api/v1/auth/oidc/login?redirect=$(python3 - <<PY
from urllib.parse import quote
print(quote("$redirect_url", safe=""))
PY
)"
oidc_status="$(curl --noproxy '*' -sS -o /dev/null -D "$OIDC_HEADERS" -w '%{http_code}' "$oidc_start")"
oidc_location="$(awk 'tolower($1)=="location:"{print $2}' "$OIDC_HEADERS" | tr -d '\r' | tail -n1)"
require "oidc login returns redirect" "$([[ "$oidc_status" == "302" && -n "$oidc_location" ]] && echo true || echo false)"
require "oidc redirect points to public keycloak" "$([[ "$oidc_location" == "$BASE_URL/realms/master/protocol/openid-connect/auth"* ]] && echo true || echo false)"
require "oidc redirect uses traffic-ui client" "$([[ "$oidc_location" == *"client_id=traffic-ui"* ]] && echo true || echo false)"

keycloak_status="$(curl --noproxy '*' -sS -o "$KEYCLOAK_HTML" -D "$KEYCLOAK_HEADERS" -w '%{http_code}' "$oidc_location")"
require "keycloak authorization page loads" "$([[ "$keycloak_status" == "200" ]] && echo true || echo false)"
keycloak_login_form=false
if rg -q 'Sign in to your account|Username or email|Password' "$KEYCLOAK_HTML"; then
  keycloak_login_form=true
fi
keycloak_client_exists=false
if ! rg -q 'Client not found|Invalid parameter' "$KEYCLOAK_HTML"; then
  keycloak_client_exists=true
fi
require "keycloak page is login form" "$keycloak_login_form"
require "keycloak client exists" "$keycloak_client_exists"

result="passed"
if ((${#failures[@]} > 0)); then
  result="blocked"
fi
failures_json="[]"
if ((${#failures[@]} > 0)); then
  failures_json="$(printf '%s\n' "${failures[@]}" | jq -R . | jq -s .)"
fi

jq -n \
  --arg run_id "$RUN_ID" \
  --arg result "$result" \
  --arg base_url "$BASE_URL" \
  --arg issuer "$issuer" \
  --arg authorization_endpoint "$auth_endpoint" \
  --arg token_endpoint "$token_endpoint" \
  --arg oidc_status "$oidc_status" \
  --arg oidc_location "$oidc_location" \
  --arg keycloak_status "$keycloak_status" \
  --arg login_index_asset "$index_asset" \
  --arg login_chunk "$login_chunk" \
  --arg callback_chunk "$callback_chunk" \
  --argjson failures "$failures_json" \
  '{
    run_id: $run_id,
    result: $result,
    base_url: $base_url,
    discovery: {
      issuer: $issuer,
      authorization_endpoint: $authorization_endpoint,
      token_endpoint: $token_endpoint
    },
    frontend: {
      login_index_asset: $login_index_asset,
      login_chunk: $login_chunk,
      oidc_callback_chunk: $callback_chunk
    },
    oidc_login: {
      status: $oidc_status,
      location: $oidc_location
    },
    keycloak_authorization_page: {
      status: $keycloak_status
    },
    failures: $failures
  }' >"$SUMMARY_JSON"

{
  printf '# OIDC/SSO Live Preflight\n\n'
  printf -- '- run_id: `%s`\n' "$RUN_ID"
  printf -- '- result: `%s`\n' "$result"
  printf -- '- base_url: `%s`\n' "$BASE_URL"
  printf -- '- authorization_endpoint: `%s`\n' "$auth_endpoint"
  printf -- '- oidc_login_status: `%s`\n' "$oidc_status"
  printf -- '- keycloak_page_status: `%s`\n' "$keycloak_status"
  if ((${#failures[@]} > 0)); then
    printf '\n## Failures\n'
    printf -- '- %s\n' "${failures[@]}"
  fi
} >"$SUMMARY_MD"

cp "$SUMMARY_JSON" "$OUT_DIR/oidc-sso-preflight-latest.json"
cp "$SUMMARY_MD" "$OUT_DIR/oidc-sso-preflight-latest.md"

if [[ "$result" != "passed" ]]; then
  cat "$SUMMARY_JSON"
  exit 1
fi

cat "$SUMMARY_JSON"
