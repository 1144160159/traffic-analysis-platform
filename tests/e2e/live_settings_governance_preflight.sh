#!/usr/bin/env bash
set -euo pipefail

APISIX="${APISIX:-http://10.0.5.8:30180}"
TENANT="${TENANT:-default}"
OTHER_TENANT="${OTHER_TENANT:-tenant-b}"
LOG_DIR="${LOG_DIR:-doc/02_acceptance/runs/$(date +%Y%m%d%H%M%S)-settings-governance-preflight}"
RUN_ID="${RUN_ID:-$(date +%Y%m%d%H%M%S)-settings-governance-preflight}"
REGRESSION_DIR="${REGRESSION_DIR:-doc/02_acceptance/02-regression}"
KUBECTL="${KUBECTL:-kubectl}"
ALLOW_BLOCKERS="${ALLOW_BLOCKERS:-false}"
JWT_SECRET_NAMESPACE="${JWT_SECRET_NAMESPACE:-traffic-analysis}"
JWT_SECRET_NAME="${JWT_SECRET_NAME:-traffic-credentials}"
JWT_SECRET_KEY="${JWT_SECRET_KEY:-JWT_SECRET}"
PG_NAMESPACE="${PG_NAMESPACE:-databases}"
PG_POD="${PG_POD:-postgres-primary-0}"
PG_SECRET_NAMESPACE="${PG_SECRET_NAMESPACE:-traffic-analysis}"
PG_SECRET_NAME="${PG_SECRET_NAME:-traffic-credentials}"
PG_SECRET_KEY="${PG_SECRET_KEY:-PG_PASSWORD}"

REPORT="$LOG_DIR/live-settings-governance-preflight-$RUN_ID.ndjson"
SUMMARY="$LOG_DIR/live-settings-governance-preflight-$RUN_ID-summary.json"
LOCAL_REPORT="$LOG_DIR/local-report.md"

mkdir -p "$LOG_DIR" "$REGRESSION_DIR"
: >"$REPORT"

PG_PASSWORD=""
TOKEN_IDS=()
TEMP_FILES=()

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

trim_file() {
  local file="$1"
  if [[ -s "$file" ]]; then
    head -c 1200 "$file" \
      | tr '\n' ' ' \
      | sed -E 's/"token"[[:space:]]*:[[:space:]]*"[^"]+"/"token":"<redacted>"/g; s/Bearer [A-Za-z0-9._-]+/Bearer <redacted>/g'
  fi
}

sql_escape() {
  printf "%s" "$1" | sed "s/'/''/g"
}

json_log() {
  local phase="$1" name="$2" severity="$3" passed="$4" status="$5" detail="${6:-}" artifact="${7:-}"
  jq -nc \
    --arg ts "$(date -Iseconds)" \
    --arg phase "$phase" \
    --arg name "$name" \
    --arg severity "$severity" \
    --argjson passed "$passed" \
    --arg status "$status" \
    --arg detail "$detail" \
    --arg artifact "$artifact" \
    '{ts:$ts, phase:$phase, name:$name, severity:$severity, passed:$passed, status:$status, detail:$detail, artifact:$artifact}' >>"$REPORT"
}

psql_exec() {
  local sql="$1"
  kctl -n "$PG_NAMESPACE" exec "$PG_POD" -- env PGPASSWORD="$PG_PASSWORD" \
    psql -q -U postgres -d traffic_platform -v ON_ERROR_STOP=1 -Atc "$sql"
}

cleanup() {
  set +e
  if [[ -n "$PG_PASSWORD" && "${#TOKEN_IDS[@]}" -gt 0 ]]; then
    local ids_sql=""
    local token_id
    for token_id in "${TOKEN_IDS[@]}"; do
      [[ -n "$token_id" ]] || continue
      if [[ -z "$ids_sql" ]]; then
        ids_sql="'$(sql_escape "$token_id")'"
      else
        ids_sql="$ids_sql,'$(sql_escape "$token_id")'"
      fi
    done
    if [[ -n "$ids_sql" ]]; then
      psql_exec "DELETE FROM api_tokens WHERE token_id IN ($ids_sql);" >/dev/null 2>&1 || true
    fi
  fi
  rm -f "${TEMP_FILES[@]:-}"
}
trap cleanup EXIT

ensure_user() {
  local tenant="$1" username="$2"
  local tenant_sql username_sql
  tenant_sql="$(sql_escape "$tenant")"
  username_sql="$(sql_escape "$username")"
  psql_exec "
    INSERT INTO tenants (tenant_id, tenant_name, name, status)
    VALUES ('$tenant_sql', '$tenant_sql', '$tenant_sql', 'active')
    ON CONFLICT (tenant_id) DO UPDATE
      SET tenant_name = COALESCE(NULLIF(tenants.tenant_name, ''), EXCLUDED.tenant_name),
          name = COALESCE(NULLIF(tenants.name, ''), EXCLUDED.name),
          status = 'active',
          updated_at = now();
    INSERT INTO users (user_id, tenant_id, username, email, status)
    VALUES (uuid_generate_v4(), '$tenant_sql', '$username_sql', '$username_sql@local', 'active')
    ON CONFLICT (tenant_id, username)
    DO UPDATE SET status = 'active', updated_at = now()
    RETURNING user_id;
  "
}

make_token() {
  local username="$1" tenant="$2" user_id="$3" roles_json="$4" perms_json="$5" ttl="${6:-1800}"
  JWT_SECRET="$JWT_SECRET" TENANT="$tenant" USERNAME="$username" USER_ID="$user_id" ROLES_JSON="$roles_json" PERMS_JSON="$perms_json" RUN_ID="$RUN_ID" TTL="$ttl" python3 - <<'PY'
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
    "sub": os.environ["USER_ID"],
    "jti": str(uuid.uuid4()),
    "user_id": os.environ["USER_ID"],
    "tenant_id": os.environ["TENANT"],
    "username": os.environ["USERNAME"],
    "email": f"{os.environ['USERNAME']}@local",
    "roles": json.loads(os.environ["ROLES_JSON"]),
    "permissions": json.loads(os.environ["PERMS_JSON"]),
    "token_type": "access",
    "session_id": "codex-settings-governance-" + os.environ["RUN_ID"],
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

curl_json() {
  local name="$1" method="$2" path="$3" expected_code="$4" token="$5" output="$6" body="${7:-}" tenant_header="${8:-$TENANT}"
  local err_file code rc
  err_file="$output.err"
  set +e
  if [[ -n "$body" ]]; then
    code="$(curl --noproxy '*' -sS -m 30 -o "$output" -w '%{http_code}' \
      -X "$method" \
      -H "Authorization: Bearer $token" \
      -H "X-Tenant-ID: $tenant_header" \
      -H "Content-Type: application/json" \
      --data "$body" \
      "$APISIX$path" 2>"$err_file")"
  else
    code="$(curl --noproxy '*' -sS -m 30 -o "$output" -w '%{http_code}' \
      -X "$method" \
      -H "Authorization: Bearer $token" \
      -H "X-Tenant-ID: $tenant_header" \
      "$APISIX$path" 2>"$err_file")"
  fi
  rc=$?
  set -e
  if [[ "$rc" -ne 0 ]]; then
    json_log "api" "$name" "blocker" false "curl-rc=$rc" "$(trim_file "$err_file")" "$(basename "$err_file")"
    return 1
  fi
  if [[ "$code" == "$expected_code" ]]; then
    json_log "api" "$name" "info" true "$code" "$(trim_file "$output")" "$(basename "$output")"
    return 0
  fi
  json_log "api" "$name" "blocker" false "http-$code" "expected=$expected_code body=$(trim_file "$output")" "$(basename "$output")"
  return 1
}

validate_raw_token() {
  local name="$1" raw="$2" expected_code="$3" output="$4"
  curl_json "$name" "POST" "/api/v1/tokens/validate" "$expected_code" "" "$output" "$(jq -nc --arg token "$raw" '{token:$token}')"
}

assert_json() {
  local name="$1" file="$2"
  shift 2
  local jq_args=()
  while [[ "$#" -gt 1 ]]; do
    jq_args+=("$1")
    shift
  done
  local filter="$1"
  if jq -e "${jq_args[@]}" "$filter" "$file" >/dev/null 2>&1; then
    json_log "assert" "$name" "info" true "ok" "$filter" "$(basename "$file")"
  else
    json_log "assert" "$name" "blocker" false "failed" "$filter body=$(trim_file "$file")" "$(basename "$file")"
  fi
}

assert_psql_count() {
  local name="$1" sql="$2" artifact="$3"
  local out="$LOG_DIR/$artifact"
  set +e
  psql_exec "$sql" >"$out" 2>"$out.err"
  local rc=$?
  set -e
  local count
  count="$(tr -d '[:space:]' <"$out" 2>/dev/null || true)"
  if [[ "$rc" -eq 0 && "$count" =~ ^[0-9]+$ && "$count" -ge 1 ]]; then
    json_log "postgres" "$name" "info" true "ok" "$count" "$artifact"
  else
    json_log "postgres" "$name" "blocker" false "missing" "$(trim_file "$out.err")" "$(basename "$out.err")"
  fi
}

wait_for_audit() {
  local name="$1" action="$2" object_id="$3" artifact="$4" min_count="${5:-1}"
  local out="$LOG_DIR/$artifact"
  local object_id_sql
  object_id_sql="$(sql_escape "$object_id")"
  for _ in $(seq 1 20); do
    set +e
    psql_exec "SELECT count(*) FROM audit_logs WHERE tenant_id = '$TENANT' AND action = '$action' AND object_id = '$object_id_sql';" >"$out" 2>"$out.err"
    local rc=$?
    set -e
    local count
    count="$(tr -d '[:space:]' <"$out" 2>/dev/null || true)"
    if [[ "$rc" -eq 0 && "$count" =~ ^[0-9]+$ && "$count" -ge "$min_count" ]]; then
      json_log "audit" "$name" "info" true "ok" "action=$action object_id=$object_id count=$count" "$artifact"
      return
    fi
    sleep 1
  done
  json_log "audit" "$name" "blocker" false "missing" "action=$action object_id=$object_id err=$(trim_file "$out.err")" "$(basename "$out.err")"
}

redact_token_json() {
  local src="$1" dest="$2"
  jq 'if type == "object" then ((if has("token") then .token = "<redacted>" else . end) | del(.token_hash)) else . end' "$src" >"$dest"
}

finish() {
  local total passed blockers warnings result
  total="$(jq -s 'length' "$REPORT")"
  passed="$(jq -s '[.[] | select(.passed == true)] | length' "$REPORT")"
  blockers="$(jq -s '[.[] | select(.passed == false and .severity == "blocker")] | length' "$REPORT")"
  warnings="$(jq -s '[.[] | select(.severity == "warn")] | length' "$REPORT")"
  result="pass"
  if [[ "$blockers" -gt 0 ]]; then
    result="blocked"
  elif [[ "$warnings" -gt 0 ]]; then
    result="warn"
  fi

  jq -n \
    --arg run_id "$RUN_ID" \
    --arg result "$result" \
    --arg apisix "$APISIX" \
    --arg tenant "$TENANT" \
    --arg other_tenant "$OTHER_TENANT" \
    --arg admin_user_id "${ADMIN_USER_ID:-}" \
    --arg created_token_id "${CREATED_TOKEN_ID:-}" \
    --arg regenerated_token_id "${REGENERATED_TOKEN_ID:-}" \
    --arg report "$REPORT" \
    --arg local_report "$LOCAL_REPORT" \
    --argjson total "$total" \
    --argjson passed "$passed" \
    --argjson blockers "$blockers" \
    --argjson warnings "$warnings" \
    --slurpfile checks "$REPORT" \
    '{
      run_id:$run_id,
      result:$result,
      apisix:$apisix,
      tenant:$tenant,
      other_tenant:$other_tenant,
      admin_user_id:$admin_user_id,
      created_token_id:$created_token_id,
      regenerated_token_id:$regenerated_token_id,
      report:$report,
      local_report:$local_report,
      total:$total,
      passed:$passed,
      blockers:$blockers,
      warnings:$warnings,
      checks:$checks
    }' >"$SUMMARY"

  cat >"$LOCAL_REPORT" <<MD
# Settings Governance Live Preflight

- Run: \`$RUN_ID\`
- Result: \`$result\`
- Checks: \`$passed/$total\` passed, \`$blockers\` blockers, \`$warnings\` warnings
- Settings user: \`${ADMIN_USER_ID:-missing}\`
- Created token: \`${CREATED_TOKEN_ID:-missing}\`
- Regenerated token: \`${REGENERATED_TOKEN_ID:-missing}\`

This gate closes the system settings loop: frontend settings action contract,
display preferences save/read through auth-service, API token create/scope
update/regenerate/revoke/validate, viewer write denial, tenant isolation,
PostgreSQL persistence, and token audit-log queryability. Token-bearing API
responses are stored only in temporary files; regression artifacts are redacted.
MD

  cp "$SUMMARY" "$REGRESSION_DIR/settings-governance-preflight-latest.json"
  cp "$LOCAL_REPORT" "$REGRESSION_DIR/settings-governance-preflight-latest.md"
  cp "$LOG_DIR/settings-display-get.json" "$REGRESSION_DIR/settings-display-latest.json" 2>/dev/null || true
  cp "$LOG_DIR/settings-token-create-redacted.json" "$REGRESSION_DIR/settings-token-create-redacted-latest.json" 2>/dev/null || true
  cp "$LOG_DIR/settings-token-regenerate-redacted.json" "$REGRESSION_DIR/settings-token-regenerate-redacted-latest.json" 2>/dev/null || true

  echo "settings governance preflight result: $result"
  echo "summary: $SUMMARY"
  echo "local report: $LOCAL_REPORT"

  if [[ "$result" == "blocked" && "$ALLOW_BLOCKERS" != "true" ]]; then
    exit 1
  fi
}

for cmd in git jq curl python3 "$KUBECTL"; do
  need_cmd "$cmd"
done

git rev-parse HEAD >"$LOG_DIR/commit-sha.txt"
git status --short >"$LOG_DIR/git-status.txt"

JWT_SECRET="$(kctl -n "$JWT_SECRET_NAMESPACE" get secret "$JWT_SECRET_NAME" -o "jsonpath={.data.$JWT_SECRET_KEY}" | base64 -d)"
PG_PASSWORD="$(kctl -n "$PG_SECRET_NAMESPACE" get secret "$PG_SECRET_NAME" -o "jsonpath={.data.$PG_SECRET_KEY}" | base64 -d)"

ADMIN_USERNAME="codex-settings-admin"
VIEWER_USERNAME="codex-settings-viewer"
OTHER_USERNAME="codex-settings-other-admin"
ADMIN_USER_ID="$(ensure_user "$TENANT" "$ADMIN_USERNAME" | grep -E '^[0-9a-fA-F-]{36}$' | tail -n 1 | tr -d '[:space:]')"
VIEWER_USER_ID="$(ensure_user "$TENANT" "$VIEWER_USERNAME" | grep -E '^[0-9a-fA-F-]{36}$' | tail -n 1 | tr -d '[:space:]')"
OTHER_USER_ID="$(ensure_user "$OTHER_TENANT" "$OTHER_USERNAME" | grep -E '^[0-9a-fA-F-]{36}$' | tail -n 1 | tr -d '[:space:]')"

ADMIN_TOKEN="$(make_token "$ADMIN_USERNAME" "$TENANT" "$ADMIN_USER_ID" '["admin"]' '["*","admin:*","admin:read","admin:write","token:read","token:write","audit:read","user:read"]')"
VIEWER_TOKEN="$(make_token "$VIEWER_USERNAME" "$TENANT" "$VIEWER_USER_ID" '["viewer"]' '["alert:read"]')"
OTHER_TOKEN="$(make_token "$OTHER_USERNAME" "$OTHER_TENANT" "$OTHER_USER_ID" '["admin"]' '["*","admin:*","admin:read","admin:write","token:read","token:write","audit:read","user:read"]')"

if grep -q "settings-token-create" web/ui/src/services/pageApiPlans.ts \
  && grep -q "settings-preferences-save" web/ui/src/services/pageApiPlans.ts \
  && grep -q "settings-token-regenerate" web/ui/src/services/pageApiPlans.ts; then
  json_log "contract" "frontend settings action contract present" "info" true "ok" "settings actions declared" "web/ui/src/services/pageApiPlans.ts"
else
  json_log "contract" "frontend settings action contract present" "blocker" false "missing" "settings action contract missing" "web/ui/src/services/pageApiPlans.ts"
fi

curl_json "scope catalog readable" "GET" "/api/v1/tokens/scopes" "200" "$ADMIN_TOKEN" "$LOG_DIR/token-scopes.json"
assert_json "scope catalog includes token scopes" "$LOG_DIR/token-scopes.json" '(.scopes // []) | map(.name) | (index("token:read") != null and index("token:write") != null)'

curl_json "probe scope catalog readable" "GET" "/api/v1/tokens/scopes/probe" "200" "$ADMIN_TOKEN" "$LOG_DIR/probe-token-scopes.json"
assert_json "probe scope catalog includes defaults" "$LOG_DIR/probe-token-scopes.json" '(.default_scopes // []) | index("probe:ingest") != null'

curl_json "token list readable" "GET" "/api/v1/tokens?limit=5" "200" "$ADMIN_TOKEN" "$LOG_DIR/tokens-list.json"
assert_json "token list response shape" "$LOG_DIR/tokens-list.json" '(.tokens | type == "array") and (.total | type == "number")'

settings_payload="$(jq -nc '{page_size:50,refresh_interval:30,default_time_range:"last_24h",timezone:"Asia/Shanghai",show_ws_status:true}')"
curl_json "display settings saved" "PUT" "/api/v1/auth/settings/display" "200" "$ADMIN_TOKEN" "$LOG_DIR/settings-display-save.json" "$settings_payload"
assert_json "display settings save response shape" "$LOG_DIR/settings-display-save.json" '.category == "display" and .settings.page_size == 50 and .settings.refresh_interval == 30 and .settings.show_ws_status == true'

curl_json "display settings readable" "GET" "/api/v1/auth/settings/display" "200" "$ADMIN_TOKEN" "$LOG_DIR/settings-display-get.json"
assert_json "display settings read response includes saved values" "$LOG_DIR/settings-display-get.json" '.category == "display" and .settings.page_size == 50 and .settings.timezone == "Asia/Shanghai"'

curl_json "invalid settings category rejected" "PUT" "/api/v1/auth/settings/security" "400" "$ADMIN_TOKEN" "$LOG_DIR/settings-invalid-category.json" "$settings_payload" || true
assert_json "invalid settings category mentions validation" "$LOG_DIR/settings-invalid-category.json" '((.error.message // .message // "")) | contains("invalid settings category")'

ADMIN_USER_ID_SQL="$(sql_escape "$ADMIN_USER_ID")"
assert_psql_count "display settings persisted in PostgreSQL" \
  "SELECT count(*) FROM user_settings WHERE tenant_id = '$TENANT' AND user_id = '$ADMIN_USER_ID_SQL' AND category = 'display' AND settings->>'timezone' = 'Asia/Shanghai' AND settings->>'page_size' = '50';" \
  "pg-user-settings-display-count.txt"

viewer_body="$LOG_DIR/token-create-viewer-denied.json"
viewer_payload="$(jq -nc --arg name "codex-settings-denied-$RUN_ID" '{name:$name,scopes:["alert:read"],expires_in_sec:60}')"
curl_json "viewer cannot create token" "POST" "/api/v1/tokens" "403" "$VIEWER_TOKEN" "$viewer_body" "$viewer_payload" || true
assert_json "viewer token create denial mentions permission" "$viewer_body" '((.error.message // .message // "")) | contains("Permission denied")'

create_body="$(new_tmp)"
created_name="codex-settings-token-$RUN_ID"
create_payload="$(jq -nc --arg name "$created_name" '{name:$name,description:"Codex live settings governance token",scopes:["alert:read","token:read"],expires_in_sec:300}')"
curl_json "admin creates settings API token" "POST" "/api/v1/tokens" "201" "$ADMIN_TOKEN" "$create_body" "$create_payload"
assert_json "created token response hides hash" "$create_body" '.token_id and .token and .token_prefix and (.token_hash? == null) and ((.scopes // []) | index("alert:read") != null)'

if jq -e '.token_id and .token' "$create_body" >/dev/null 2>&1; then
  CREATED_TOKEN_ID="$(jq -r '.token_id' "$create_body")"
  CREATED_RAW_TOKEN="$(jq -r '.token' "$create_body")"
  CREATED_PREFIX="$(jq -r '.token_prefix' "$create_body")"
  TOKEN_IDS+=("$CREATED_TOKEN_ID")
  redact_token_json "$create_body" "$LOG_DIR/settings-token-create-redacted.json"
  created_hash="$(sha256_hex "$CREATED_RAW_TOKEN")"
  expected_prefix="${CREATED_RAW_TOKEN:0:18}"
  if [[ "$CREATED_PREFIX" == "$expected_prefix" ]]; then
    json_log "assert" "created token exposes expected safe prefix" "info" true "ok" "token_id=$CREATED_TOKEN_ID prefix=$CREATED_PREFIX" "settings-token-create-redacted.json"
  else
    json_log "assert" "created token exposes expected safe prefix" "blocker" false "mismatch" "token_id=$CREATED_TOKEN_ID prefix=$CREATED_PREFIX" "settings-token-create-redacted.json"
  fi
  CREATED_TOKEN_ID_SQL="$(sql_escape "$CREATED_TOKEN_ID")"
  assert_psql_count "created token hash and scopes persisted" \
    "SELECT count(*) FROM api_tokens WHERE token_id = '$CREATED_TOKEN_ID_SQL' AND tenant_id = '$TENANT' AND token_hash = '$created_hash' AND token_prefix = '$CREATED_PREFIX' AND status = 'active' AND scopes ? 'alert:read' AND scopes ? 'token:read';" \
    "pg-settings-token-create-count.txt"
  wait_for_audit "create token audit row exists" "create_token" "$CREATED_TOKEN_ID" "pg-settings-token-create-audit-count.txt"
else
  finish
fi

tenant_b_body="$LOG_DIR/token-read-other-tenant.json"
curl_json "other tenant cannot read settings token" "GET" "/api/v1/tokens/$CREATED_TOKEN_ID" "404" "$OTHER_TOKEN" "$tenant_b_body" "" "$OTHER_TENANT" || true
assert_json "other tenant token read is isolated" "$tenant_b_body" '(.code // "") == "BIZ_3010"'

invalid_scope_body="$LOG_DIR/token-scope-invalid.json"
invalid_scope_payload="$(jq -nc '{scopes:["alert:read","invalid:scope"]}')"
curl_json "invalid token scope rejected" "PUT" "/api/v1/tokens/$CREATED_TOKEN_ID/scopes" "400" "$ADMIN_TOKEN" "$invalid_scope_body" "$invalid_scope_payload" || true
assert_json "invalid scope response names invalid scope" "$invalid_scope_body" '((.error.message // .message // "")) | contains("invalid scopes")'

scope_body="$LOG_DIR/token-scope-update.json"
scope_payload="$(jq -nc '{scopes:["alert:read"]}')"
curl_json "admin updates token scopes" "PUT" "/api/v1/tokens/$CREATED_TOKEN_ID/scopes" "200" "$ADMIN_TOKEN" "$scope_body" "$scope_payload"
assert_json "scope update response confirms success" "$scope_body" '(.message // "") | contains("Scopes updated")'
assert_psql_count "updated token scopes persisted" \
  "SELECT count(*) FROM api_tokens WHERE token_id = '$CREATED_TOKEN_ID_SQL' AND tenant_id = '$TENANT' AND status = 'active' AND scopes ? 'alert:read' AND NOT scopes ? 'token:read';" \
  "pg-settings-token-scope-update-count.txt"

validate_body="$LOG_DIR/token-validate-created.json"
validate_raw_token "created raw token validates after scope update" "$CREATED_RAW_TOKEN" "200" "$validate_body"
assert_json "created token validate response reflects updated scope" "$validate_body" --arg tenant "$TENANT" '.valid == true and .tenant_id == $tenant and ((.scopes // []) | index("alert:read") != null) and ((.scopes // []) | index("token:read") == null)'

regen_body="$(new_tmp)"
curl_json "admin regenerates settings token" "POST" "/api/v1/tokens/$CREATED_TOKEN_ID/regenerate" "201" "$ADMIN_TOKEN" "$regen_body"
assert_json "regenerated token response hides hash" "$regen_body" '.token_id and .token and .token_prefix and (.token_hash? == null)'

if jq -e '.token_id and .token' "$regen_body" >/dev/null 2>&1; then
  REGENERATED_TOKEN_ID="$(jq -r '.token_id' "$regen_body")"
  REGENERATED_RAW_TOKEN="$(jq -r '.token' "$regen_body")"
  REGENERATED_PREFIX="$(jq -r '.token_prefix' "$regen_body")"
  TOKEN_IDS+=("$REGENERATED_TOKEN_ID")
  redact_token_json "$regen_body" "$LOG_DIR/settings-token-regenerate-redacted.json"
  regenerated_hash="$(sha256_hex "$REGENERATED_RAW_TOKEN")"
  REGENERATED_TOKEN_ID_SQL="$(sql_escape "$REGENERATED_TOKEN_ID")"
  assert_psql_count "old token revoked by regenerate" \
    "SELECT count(*) FROM api_tokens WHERE token_id = '$CREATED_TOKEN_ID_SQL' AND status = 'revoked' AND revoked_at IS NOT NULL;" \
    "pg-settings-token-old-revoked-count.txt"
  assert_psql_count "regenerated token hash persisted" \
    "SELECT count(*) FROM api_tokens WHERE token_id = '$REGENERATED_TOKEN_ID_SQL' AND tenant_id = '$TENANT' AND token_hash = '$regenerated_hash' AND token_prefix = '$REGENERATED_PREFIX' AND status = 'active';" \
    "pg-settings-token-regenerate-count.txt"
  wait_for_audit "regenerate token audit row exists" "regenerate_token" "$REGENERATED_TOKEN_ID" "pg-settings-token-regenerate-audit-count.txt"
else
  finish
fi

old_after_regen_body="$LOG_DIR/token-validate-old-after-regenerate.json"
validate_raw_token "old raw token rejected after regenerate" "$CREATED_RAW_TOKEN" "401" "$old_after_regen_body" || true
assert_json "old token rejected response is invalid token" "$old_after_regen_body" '(.code // "") == "AUTH_1003"'

new_validate_body="$LOG_DIR/token-validate-regenerated.json"
validate_raw_token "regenerated raw token validates" "$REGENERATED_RAW_TOKEN" "200" "$new_validate_body"
assert_json "regenerated token validate response is tenant scoped" "$new_validate_body" --arg tenant "$TENANT" '.valid == true and .tenant_id == $tenant and ((.scopes // []) | index("alert:read") != null)'

revoke_body="$LOG_DIR/token-revoke-regenerated.json"
curl_json "admin revokes regenerated token" "POST" "/api/v1/tokens/$REGENERATED_TOKEN_ID/revoke" "200" "$ADMIN_TOKEN" "$revoke_body"
assert_json "revoke response confirms success" "$revoke_body" '(.message // "") | contains("revoked")'

new_after_revoke_body="$LOG_DIR/token-validate-after-revoke.json"
validate_raw_token "revoked raw token rejected" "$REGENERATED_RAW_TOKEN" "401" "$new_after_revoke_body" || true
assert_json "revoked token rejected response is invalid token" "$new_after_revoke_body" '(.code // "") == "AUTH_1003"'
assert_psql_count "revoked token state persisted" \
  "SELECT count(*) FROM api_tokens WHERE token_id = '$REGENERATED_TOKEN_ID_SQL' AND status = 'revoked' AND revoked_at IS NOT NULL;" \
  "pg-settings-token-revoke-count.txt"
wait_for_audit "revoke token audit row exists" "revoke_token" "$REGENERATED_TOKEN_ID" "pg-settings-token-revoke-audit-count.txt"

finish
