#!/usr/bin/env bash
set -euo pipefail

APISIX="${APISIX:-http://10.0.5.8:30180}"
TENANT="${TENANT:-default}"
TENANT_B="${TENANT_B:-tenant-b}"
LOG_DIR="${LOG_DIR:-.artifacts/e2e}"
RUN_ID="${RUN_ID:-$(date +%Y%m%d%H%M%S)-$$}"
KUBECTL="${KUBECTL:-kubectl}"
JWT_SECRET_NAMESPACE="${JWT_SECRET_NAMESPACE:-traffic-analysis}"
JWT_SECRET_NAME="${JWT_SECRET_NAME:-traffic-credentials}"
JWT_SECRET_KEY="${JWT_SECRET_KEY:-JWT_SECRET}"
PG_NAMESPACE="${PG_NAMESPACE:-databases}"
PG_POD="${PG_POD:-postgres-primary-0}"
PG_SECRET_NAMESPACE="${PG_SECRET_NAMESPACE:-traffic-analysis}"
PG_SECRET_NAME="${PG_SECRET_NAME:-traffic-credentials}"
PG_SECRET_KEY="${PG_SECRET_KEY:-PG_PASSWORD}"

REPORT="$LOG_DIR/live-token-lifecycle-matrix-$RUN_ID.ndjson"
SUMMARY="$LOG_DIR/live-token-lifecycle-matrix-$RUN_ID-summary.json"
mkdir -p "$LOG_DIR"

FAILURES=0
TOKEN_IDS=()
TEMP_FILES=()
PG_PASSWORD=""

need_cmd() {
  if ! command -v "$1" >/dev/null 2>&1; then
    echo "missing required command: $1" >&2
    exit 2
  fi
}

kctl() {
  env -u HTTP_PROXY -u HTTPS_PROXY -u ALL_PROXY -u http_proxy -u https_proxy -u all_proxy "$KUBECTL" "$@"
}

new_tmp() {
  local file
  file="$(mktemp)"
  TEMP_FILES+=("$file")
  echo "$file"
}

cleanup() {
  set +e
  if [[ -n "$PG_PASSWORD" && "${#TOKEN_IDS[@]}" -gt 0 ]]; then
    local ids_sql=""
    local token_id
    for token_id in "${TOKEN_IDS[@]}"; do
      [[ -n "$token_id" ]] || continue
      if [[ -z "$ids_sql" ]]; then
        ids_sql="'$token_id'"
      else
        ids_sql="$ids_sql,'$token_id'"
      fi
    done
    if [[ -n "$ids_sql" ]]; then
      psql_exec "DELETE FROM api_tokens WHERE token_id IN ($ids_sql);" >/dev/null 2>&1 || true
    fi
  fi
  rm -f "${TEMP_FILES[@]:-}"
}
trap cleanup EXIT

json_log() {
  local phase="$1" name="$2" ok="$3" status="$4" detail="${5:-}"
  jq -nc \
    --arg ts "$(date -Iseconds)" \
    --arg phase "$phase" \
    --arg name "$name" \
    --argjson ok "$ok" \
    --arg status "$status" \
    --arg detail "$detail" \
    '{ts:$ts, phase:$phase, name:$name, ok:$ok, status:$status, detail:$detail}' >>"$REPORT"
  if [[ "$ok" != "true" ]]; then
    FAILURES=$((FAILURES + 1))
  fi
}

trim_file() {
  local file="$1"
  if [[ -s "$file" ]]; then
    head -c 500 "$file" \
      | tr '\n' ' ' \
      | sed -E 's/"token"[[:space:]]*:[[:space:]]*"[^"]+"/"token":"<redacted>"/g; s/Bearer [A-Za-z0-9._-]+/Bearer <redacted>/g'
  fi
}

finish() {
  local total failed passed
  total="$(wc -l <"$REPORT" | tr -d ' ')"
  failed="$(jq -s '[.[] | select(.ok == false)] | length' "$REPORT")"
  passed="$((total - failed))"
  jq -s \
    --arg run_id "$RUN_ID" \
    --arg apisix "$APISIX" \
    --arg tenant "$TENANT" \
    --arg tenant_b "$TENANT_B" \
    --arg report "$REPORT" \
    --argjson total "$total" \
    --argjson passed "$passed" \
    --argjson failed "$failed" \
    '{
      run_id:$run_id,
      apisix:$apisix,
      tenant:$tenant,
      tenant_b:$tenant_b,
      report:$report,
      result:(if $failed == 0 then "pass" else "fail" end),
      total:$total,
      passed:$passed,
      failed:$failed,
      checks:.
    }' "$REPORT" >"$SUMMARY"
  cat "$SUMMARY"
  if [[ "$failed" -ne 0 ]]; then
    exit 1
  fi
}

psql_exec() {
  local sql="$1"
  kctl -n "$PG_NAMESPACE" exec "$PG_POD" -- env PGPASSWORD="$PG_PASSWORD" \
    psql -U postgres -d traffic_platform -v ON_ERROR_STOP=1 -Atc "$sql"
}

make_token() {
  local username="$1"
  local tenant="$2"
  local roles_json="$3"
  local perms_json="$4"
  local ttl="${5:-1800}"
  JWT_SECRET="$JWT_SECRET" TENANT="$tenant" USERNAME="$username" ROLES_JSON="$roles_json" PERMS_JSON="$perms_json" TTL="$ttl" python3 - <<'PY'
import base64
import hashlib
import hmac
import json
import os
import time
import uuid

def b64url(raw: bytes) -> str:
    return base64.urlsafe_b64encode(raw).rstrip(b"=").decode("ascii")

now = int(time.time())
claims = {
    "iss": "traffic-auth-service",
    "sub": str(uuid.uuid4()),
    "jti": str(uuid.uuid4()),
    "user_id": str(uuid.uuid4()),
    "tenant_id": os.environ["TENANT"],
    "username": os.environ["USERNAME"],
    "email": f"{os.environ['USERNAME']}@local",
    "roles": json.loads(os.environ["ROLES_JSON"]),
    "permissions": json.loads(os.environ["PERMS_JSON"]),
    "token_type": "access",
    "session_id": "codex-token-lifecycle-" + str(uuid.uuid4()),
    "iat": now,
    "exp": now + int(os.environ["TTL"]),
}
header = {"alg": "HS256", "typ": "JWT"}
signing_input = b".".join([
    b64url(json.dumps(header, separators=(",", ":")).encode("utf-8")).encode("ascii"),
    b64url(json.dumps(claims, separators=(",", ":")).encode("utf-8")).encode("ascii"),
])
signature = hmac.new(os.environ["JWT_SECRET"].encode("utf-8"), signing_input, hashlib.sha256).digest()
print(signing_input.decode("ascii") + "." + b64url(signature))
PY
}

sha256_hex() {
  python3 - "$1" <<'PY'
import hashlib
import sys

print(hashlib.sha256(sys.argv[1].encode("utf-8")).hexdigest())
PY
}

http_request() {
  local method="$1" path="$2" token="$3" body="$4" output="$5"
  local err_file code rc
  err_file="$(new_tmp)"
  local args=(--noproxy '*' -sS -m 20 -o "$output" -w '%{http_code}' -X "$method" -H "X-Tenant-ID: $TENANT")
  if [[ -n "$token" ]]; then
    args+=(-H "Authorization: Bearer $token")
  fi
  if [[ -n "$body" ]]; then
    args+=(-H "Content-Type: application/json" --data "$body")
  fi

  set +e
  code="$(curl "${args[@]}" "$APISIX$path" 2>"$err_file")"
  rc=$?
  set -e
  if [[ "$rc" -ne 0 ]]; then
    echo "000"
    echo "curl rc=$rc err=$(trim_file "$err_file")" >"$output.err"
    TEMP_FILES+=("$output.err")
    return
  fi
  echo "$code"
}

validate_raw_token() {
  local raw="$1" output="$2"
  http_request "POST" "/api/v1/tokens/validate" "" "$(jq -nc --arg token "$raw" '{token:$token}')" "$output"
}

require_http_json() {
  local phase="$1" name="$2" code="$3" expected="$4" body="$5"
  shift 5
  local ok detail
  if [[ "$code" != "$expected" ]]; then
    ok=false
    detail="expected=$expected actual=$code body=$(trim_file "$body")"
  elif [[ "$#" -gt 0 ]] && ! jq -e "$@" "$body" >/dev/null 2>"$body.jqerr"; then
    TEMP_FILES+=("$body.jqerr")
    ok=false
    detail="jq filter failed filter=$* body=$(trim_file "$body") err=$(trim_file "$body.jqerr")"
  else
    ok=true
    detail="http=$code"
  fi
  json_log "$phase" "$name" "$ok" "$code" "$detail"
}

require_pg_count() {
  local name="$1" sql="$2" min_count="$3"
  local out err count
  out="$(new_tmp)"
  err="$(new_tmp)"
  if psql_exec "$sql" >"$out" 2>"$err"; then
    count="$(tr -d '[:space:]' <"$out")"
    if [[ "$count" =~ ^[0-9]+$ && "$count" -ge "$min_count" ]]; then
      json_log "postgres" "$name" true "$count" "min=$min_count"
    else
      json_log "postgres" "$name" false "$count" "min=$min_count"
    fi
  else
    json_log "postgres" "$name" false "psql-failed" "$(trim_file "$err")"
  fi
}

wait_for_audit() {
  local name="$1" action="$2" object_id="$3" min_count="${4:-1}"
  local out err count
  out="$(new_tmp)"
  err="$(new_tmp)"
  for _ in $(seq 1 20); do
    if psql_exec "SELECT count(*) FROM audit_logs WHERE tenant_id = '$TENANT' AND action = '$action' AND object_id = '$object_id';" >"$out" 2>"$err"; then
      count="$(tr -d '[:space:]' <"$out")"
      if [[ "$count" =~ ^[0-9]+$ && "$count" -ge "$min_count" ]]; then
        json_log "audit" "$name" true "$count" "action=$action object_id=$object_id"
        return
      fi
    fi
    sleep 1
  done
  json_log "audit" "$name" false "${count:-missing}" "action=$action object_id=$object_id err=$(trim_file "$err")"
}

need_cmd curl
need_cmd jq
need_cmd python3
need_cmd "$KUBECTL"

JWT_SECRET="$(kctl -n "$JWT_SECRET_NAMESPACE" get secret "$JWT_SECRET_NAME" -o "jsonpath={.data.$JWT_SECRET_KEY}" | base64 -d)"
PG_PASSWORD="$(kctl -n "$PG_SECRET_NAMESPACE" get secret "$PG_SECRET_NAME" -o "jsonpath={.data.$PG_SECRET_KEY}" | base64 -d)"

ADMIN_TOKEN="$(make_token codex-token-admin "$TENANT" '["admin"]' '["*","admin:*","token:read","token:write"]')"
VIEWER_TOKEN="$(make_token codex-token-viewer "$TENANT" '["viewer"]' '["alert:read"]')"
TENANT_B_ADMIN_TOKEN="$(make_token codex-token-tenant-b "$TENANT_B" '["admin"]' '["*","admin:*","token:read","token:write"]')"

scopes_body="$(new_tmp)"
scopes_code="$(http_request "GET" "/api/v1/tokens/scopes" "$ADMIN_TOKEN" "" "$scopes_body")"
require_http_json "api" "scope catalog includes token lifecycle scopes" "$scopes_code" "200" "$scopes_body" '(.scopes // []) | map(.name) | (index("token:read") != null and index("token:write") != null)'

created_name="codex-token-lifecycle-$RUN_ID"
create_body="$(new_tmp)"
create_payload="$(jq -nc --arg name "$created_name" '{name:$name,description:"Codex live token lifecycle matrix",scopes:["alert:read","token:read"],expires_in_sec:300}')"
create_code="$(http_request "POST" "/api/v1/tokens" "$ADMIN_TOKEN" "$create_payload" "$create_body")"
require_http_json "api" "admin creates API token" "$create_code" "201" "$create_body" '.token_id and .token and .token_prefix and (.scopes | index("alert:read") != null) and (.scopes | index("token:read") != null)'
if [[ "$create_code" != "201" ]]; then
  finish
fi

created_id="$(jq -r '.token_id' "$create_body")"
created_raw="$(jq -r '.token' "$create_body")"
created_prefix="$(jq -r '.token_prefix' "$create_body")"
TOKEN_IDS+=("$created_id")
created_expected_prefix="${created_raw:0:18}"
created_hash="$(sha256_hex "$created_raw")"
if [[ "$created_prefix" == "$created_expected_prefix" ]]; then
  json_log "api" "created token response exposes safe prefix only" true "ok" "token_id=$created_id prefix=$created_prefix"
else
  json_log "api" "created token response exposes safe prefix only" false "mismatch" "token_id=$created_id prefix=$created_prefix"
fi

require_pg_count "created token hash/prefix/scopes persisted" \
  "SELECT count(*) FROM api_tokens WHERE token_id = '$created_id' AND tenant_id = '$TENANT' AND token_hash = '$created_hash' AND token_prefix = '$created_prefix' AND status = 'active' AND scopes ? 'alert:read' AND scopes ? 'token:read';" \
  1

get_body="$(new_tmp)"
get_code="$(http_request "GET" "/api/v1/tokens/$created_id" "$ADMIN_TOKEN" "" "$get_body")"
require_http_json "api" "admin reads token metadata without secret hash" "$get_code" "200" "$get_body" --arg created_id "$created_id" '.token_id == $created_id and (.token_hash? == null) and (.token? == null)'

viewer_body="$(new_tmp)"
viewer_payload="$(jq -nc --arg name "codex-token-denied-$RUN_ID" '{name:$name,scopes:["alert:read"],expires_in_sec:60}')"
viewer_code="$(http_request "POST" "/api/v1/tokens" "$VIEWER_TOKEN" "$viewer_payload" "$viewer_body")"
require_http_json "api" "viewer cannot create token" "$viewer_code" "403" "$viewer_body" '(.message // "") | contains("Permission denied")'

tenant_b_body="$(new_tmp)"
tenant_b_code="$(http_request "GET" "/api/v1/tokens/$created_id" "$TENANT_B_ADMIN_TOKEN" "" "$tenant_b_body")"
require_http_json "api" "cross tenant admin cannot read tenant token" "$tenant_b_code" "404" "$tenant_b_body" '(.code // "") == "BIZ_3010"'

validate_body="$(new_tmp)"
validate_code="$(validate_raw_token "$created_raw" "$validate_body")"
require_http_json "api" "created raw token validates" "$validate_code" "200" "$validate_body" --arg tenant "$TENANT" '.valid == true and .tenant_id == $tenant and ((.scopes // []) | index("alert:read") != null)'

wait_for_audit "create token audit row exists" "create_token" "$created_id" 1

regen_body="$(new_tmp)"
regen_code="$(http_request "POST" "/api/v1/tokens/$created_id/regenerate" "$ADMIN_TOKEN" "" "$regen_body")"
require_http_json "api" "admin regenerates token" "$regen_code" "201" "$regen_body" '.token_id and .token and .token_prefix'
if [[ "$regen_code" != "201" ]]; then
  finish
fi

new_id="$(jq -r '.token_id' "$regen_body")"
new_raw="$(jq -r '.token' "$regen_body")"
new_prefix="$(jq -r '.token_prefix' "$regen_body")"
TOKEN_IDS+=("$new_id")
new_hash="$(sha256_hex "$new_raw")"

old_after_regen_body="$(new_tmp)"
old_after_regen_code="$(validate_raw_token "$created_raw" "$old_after_regen_body")"
require_http_json "api" "old raw token rejected after regenerate" "$old_after_regen_code" "401" "$old_after_regen_body" '(.code // "") == "AUTH_1003"'

new_validate_body="$(new_tmp)"
new_validate_code="$(validate_raw_token "$new_raw" "$new_validate_body")"
require_http_json "api" "new raw token validates after regenerate" "$new_validate_code" "200" "$new_validate_body" --arg tenant "$TENANT" '.valid == true and .tenant_id == $tenant and ((.scopes // []) | index("alert:read") != null)'

require_pg_count "old token revoked by regenerate" \
  "SELECT count(*) FROM api_tokens WHERE token_id = '$created_id' AND status = 'revoked' AND revoked_at IS NOT NULL;" \
  1
require_pg_count "regenerated token hash/prefix persisted" \
  "SELECT count(*) FROM api_tokens WHERE token_id = '$new_id' AND tenant_id = '$TENANT' AND token_hash = '$new_hash' AND token_prefix = '$new_prefix' AND status = 'active';" \
  1
wait_for_audit "regenerate token audit row exists" "regenerate_token" "$new_id" 1

revoke_body="$(new_tmp)"
revoke_code="$(http_request "POST" "/api/v1/tokens/$new_id/revoke" "$ADMIN_TOKEN" "" "$revoke_body")"
require_http_json "api" "admin revokes regenerated token" "$revoke_code" "200" "$revoke_body" '(.message // "") | contains("revoked")'

new_after_revoke_body="$(new_tmp)"
new_after_revoke_code="$(validate_raw_token "$new_raw" "$new_after_revoke_body")"
require_http_json "api" "revoked raw token rejected" "$new_after_revoke_code" "401" "$new_after_revoke_body" '(.code // "") == "AUTH_1003"'
require_pg_count "revoked token state persisted" \
  "SELECT count(*) FROM api_tokens WHERE token_id = '$new_id' AND status = 'revoked' AND revoked_at IS NOT NULL;" \
  1
wait_for_audit "revoke token audit row exists" "revoke_token" "$new_id" 1

short_body="$(new_tmp)"
short_payload="$(jq -nc --arg name "codex-token-expiring-$RUN_ID" '{name:$name,description:"Codex live token expiry matrix",scopes:["alert:read"],expires_in_sec:1}')"
short_code="$(http_request "POST" "/api/v1/tokens" "$ADMIN_TOKEN" "$short_payload" "$short_body")"
require_http_json "api" "admin creates short lived token" "$short_code" "201" "$short_body" '.token_id and .token and .expires_at'
if [[ "$short_code" != "201" ]]; then
  finish
fi
short_id="$(jq -r '.token_id' "$short_body")"
short_raw="$(jq -r '.token' "$short_body")"
TOKEN_IDS+=("$short_id")

short_initial_body="$(new_tmp)"
short_initial_code="$(validate_raw_token "$short_raw" "$short_initial_body")"
require_http_json "api" "short lived token initially validates" "$short_initial_code" "200" "$short_initial_body" '.valid == true'

sleep 2
short_expired_body="$(new_tmp)"
short_expired_code="$(validate_raw_token "$short_raw" "$short_expired_body")"
require_http_json "api" "expired raw token rejected" "$short_expired_code" "401" "$short_expired_body" '(.code // "") == "AUTH_1003"'
require_pg_count "expired token has past expiry" \
  "SELECT count(*) FROM api_tokens WHERE token_id = '$short_id' AND expires_at <= now();" \
  1

finish
