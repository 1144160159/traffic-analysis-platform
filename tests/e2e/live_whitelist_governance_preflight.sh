#!/usr/bin/env bash
set -euo pipefail

APISIX="${APISIX:-http://10.0.5.8:30180}"
TENANT="${TENANT:-default}"
OTHER_TENANT="${OTHER_TENANT:-tenant-b}"
LOG_DIR="${LOG_DIR:-doc/02_acceptance/runs/$(date +%Y%m%d%H%M%S)-whitelist-governance-preflight}"
RUN_ID="${RUN_ID:-$(date +%Y%m%d%H%M%S)-whitelist-governance-preflight}"
REGRESSION_DIR="${REGRESSION_DIR:-doc/02_acceptance/02-regression}"
KUBECTL="${KUBECTL:-kubectl}"
ALLOW_BLOCKERS="${ALLOW_BLOCKERS:-false}"
JWT_SECRET_NAMESPACE="${JWT_SECRET_NAMESPACE:-traffic-analysis}"
JWT_SECRET_NAME="${JWT_SECRET_NAME:-traffic-credentials}"
JWT_SECRET_KEY="${JWT_SECRET_KEY:-JWT_SECRET}"
PG_NAMESPACE="${PG_NAMESPACE:-databases}"
PG_POD="${PG_POD:-postgres-primary-0}"
PG_SECRET_KEY="${PG_SECRET_KEY:-PG_PASSWORD}"

REPORT="$LOG_DIR/live-whitelist-governance-preflight-$RUN_ID.ndjson"
SUMMARY="$LOG_DIR/live-whitelist-governance-preflight-$RUN_ID-summary.json"
LOCAL_REPORT="$LOG_DIR/local-report.md"
RUN_SLUG="$(printf '%s' "$RUN_ID" | tr -c 'a-zA-Z0-9-' '-' | cut -c1-64 | sed 's/-*$//')"
ENTRY_VALUE="${ENTRY_VALUE:-codex-${RUN_SLUG:-run}.example.test}"
SOURCE_ALERT_ID="${SOURCE_ALERT_ID:-codex-alert-${RUN_SLUG:-run}}"

mkdir -p "$LOG_DIR" "$REGRESSION_DIR"
: >"$REPORT"

need_cmd() {
  if ! command -v "$1" >/dev/null 2>&1; then
    echo "missing required command: $1" >&2
    exit 2
  fi
}

kctl() {
  env -u HTTP_PROXY -u HTTPS_PROXY -u ALL_PROXY -u http_proxy -u https_proxy -u all_proxy "$KUBECTL" "$@"
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

trim_file() {
  local file="$1"
  if [[ -s "$file" ]]; then
    head -c 1200 "$file" \
      | tr '\n' ' ' \
      | sed -E 's/Bearer [A-Za-z0-9._-]+/Bearer <redacted>/g'
  fi
}

make_token() {
  local username="$1" tenant="$2" roles_json="$3" perms_json="$4" ttl="${5:-1800}"
  JWT_SECRET="$JWT_SECRET" TENANT="$tenant" USERNAME="$username" ROLES_JSON="$roles_json" PERMS_JSON="$perms_json" RUN_ID="$RUN_ID" TTL="$ttl" python3 - <<'PY'
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
    "email": os.environ["USERNAME"] + "@local",
    "roles": json.loads(os.environ["ROLES_JSON"]),
    "permissions": json.loads(os.environ["PERMS_JSON"]),
    "token_type": "access",
    "session_id": "codex-whitelist-governance-" + os.environ["RUN_ID"],
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

psql_exec() {
  local sql="$1"
  kctl -n "$PG_NAMESPACE" exec "$PG_POD" -- env PGPASSWORD="$PG_PASSWORD" \
    psql -U postgres -d traffic_platform -v ON_ERROR_STOP=1 -Atc "$sql"
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

for cmd in git jq curl python3 "$KUBECTL"; do
  need_cmd "$cmd"
done

git rev-parse HEAD >"$LOG_DIR/commit-sha.txt"
git status --short >"$LOG_DIR/git-status.txt"

JWT_SECRET="$(kctl -n "$JWT_SECRET_NAMESPACE" get secret "$JWT_SECRET_NAME" -o "jsonpath={.data.$JWT_SECRET_KEY}" | base64 -d)"
PG_PASSWORD="$(kctl -n "$JWT_SECRET_NAMESPACE" get secret "$JWT_SECRET_NAME" -o "jsonpath={.data.$PG_SECRET_KEY}" | base64 -d)"

ADMIN_TOKEN="$(make_token codex-whitelist-admin "$TENANT" '["admin"]' '["*","admin:*","alert:write","alert:read","audit:read","user:read"]')"
VIEWER_TOKEN="$(make_token codex-whitelist-viewer "$TENANT" '["viewer"]' '["user:read","audit:read","alert:read"]')"
OTHER_TOKEN="$(make_token codex-whitelist-other-admin "$OTHER_TENANT" '["admin"]' '["*","admin:*","alert:write","alert:read","audit:read","user:read"]')"

if grep -q "whitelist-create" web/ui/src/services/pageApiPlans.ts \
  && grep -q "WHITELIST_DISABLED" web/ui/src/services/pageApiPlans.ts \
  && grep -q "drawer-whitelist-approval" web/ui/src/pages/WhitelistGovernancePage.tsx; then
  json_log "contract" "frontend whitelist governance contract present" "info" true "ok" "action plan and overlay contract declared" "web/ui/src/services/pageApiPlans.ts"
else
  json_log "contract" "frontend whitelist governance contract present" "blocker" false "missing" "whitelist action plan or approval overlay missing" "web/ui/src/services/pageApiPlans.ts"
fi

EXPIRES_AT="$(date -u -d '+2 days' '+%Y-%m-%dT%H:%M:%SZ')"
EXTENDED_AT="$(date -u -d '+30 days' '+%Y-%m-%dT%H:%M:%SZ')"

curl_json "whitelist list is readable" "GET" "/api/v1/whitelist?limit=5" "200" "$ADMIN_TOKEN" "$LOG_DIR/whitelist-before.json"
assert_json "whitelist response shape" "$LOG_DIR/whitelist-before.json" '.success == true and (.data.entries | type == "array") and (.data.total | type == "number")'

create_body="$(jq -nc \
  --arg value "$ENTRY_VALUE" \
  --arg source_alert "$SOURCE_ALERT_ID" \
  --arg expires_at "$EXPIRES_AT" \
  '{type:"domain", value:$value, reason:"false_positive", description:"codex live whitelist governance preflight", status:"draft", approval_status:"draft", source_alert_id:$source_alert, owner_role:"soc-duty", expires_at:$expires_at}')"
curl_json "admin can create whitelist draft" "POST" "/api/v1/whitelist" "201" "$ADMIN_TOKEN" "$LOG_DIR/whitelist-create.json" "$create_body"
assert_json "created whitelist entry has identity" "$LOG_DIR/whitelist-create.json" \
  --arg value "$ENTRY_VALUE" '.success == true and (.data.id | type == "string" and length > 0) and .data.value == $value and .data.tenant_id == "default" and .data.status == "draft"'

ENTRY_ID="$(jq -r '.data.id // ""' "$LOG_DIR/whitelist-create.json")"
if [[ -n "$ENTRY_ID" ]]; then
  curl_json "viewer cannot create whitelist entry" "POST" "/api/v1/whitelist" "403" "$VIEWER_TOKEN" "$LOG_DIR/whitelist-create-viewer.json" "$create_body" || true

  curl_json "admin can submit whitelist approval" "PATCH" "/api/v1/whitelist/$ENTRY_ID" "200" "$ADMIN_TOKEN" "$LOG_DIR/whitelist-submit-approval.json" '{"status":"pending","approval_status":"pending"}'
  assert_json "whitelist approval submission response" "$LOG_DIR/whitelist-submit-approval.json" '.success == true and .data.status == "pending" and .data.approval_status == "pending"'

  curl_json "admin can approve and activate whitelist entry" "PATCH" "/api/v1/whitelist/$ENTRY_ID" "200" "$ADMIN_TOKEN" "$LOG_DIR/whitelist-approve.json" '{"status":"active","approval_status":"approved"}'
  assert_json "whitelist approval activation response" "$LOG_DIR/whitelist-approve.json" '.success == true and .data.status == "active" and .data.approval_status == "approved"'

  check_body="$(jq -nc --arg value "$ENTRY_VALUE" '{type:"domain", value:$value, tenant_id:"tenant-spoof"}')"
  curl_json "active whitelist entry matches check API" "POST" "/api/v1/whitelist/check" "200" "$ADMIN_TOKEN" "$LOG_DIR/whitelist-check-active.json" "$check_body"
  assert_json "active whitelist check result" "$LOG_DIR/whitelist-check-active.json" '.success == true and .data.whitelisted == true'

  extend_body="$(jq -nc --arg expires_at "$EXTENDED_AT" '{expires_at:$expires_at, reason:"codex expiry governance extension"}')"
  curl_json "admin can extend whitelist expiry" "PATCH" "/api/v1/whitelist/$ENTRY_ID" "200" "$ADMIN_TOKEN" "$LOG_DIR/whitelist-extend.json" "$extend_body"
  assert_json "whitelist expiry extension response" "$LOG_DIR/whitelist-extend.json" \
    --arg expires_at "$EXTENDED_AT" '.success == true and (.data.expires_at | startswith($expires_at[:19]))'

  curl_json "other tenant cannot update whitelist entry" "PATCH" "/api/v1/whitelist/$ENTRY_ID" "404" "$OTHER_TOKEN" "$LOG_DIR/whitelist-other-tenant.json" '{"status":"disabled"}' "$OTHER_TENANT" || true
  curl_json "viewer cannot disable whitelist entry" "PATCH" "/api/v1/whitelist/$ENTRY_ID" "403" "$VIEWER_TOKEN" "$LOG_DIR/whitelist-disable-viewer.json" '{"status":"disabled"}' || true

  curl_json "admin can disable whitelist entry" "PATCH" "/api/v1/whitelist/$ENTRY_ID" "200" "$ADMIN_TOKEN" "$LOG_DIR/whitelist-disable.json" '{"status":"disabled"}'
  assert_json "whitelist disable response" "$LOG_DIR/whitelist-disable.json" '.success == true and .data.status == "disabled" and (.data.disabled_at | type == "string")'

  curl_json "disabled whitelist entry stops matching check API" "POST" "/api/v1/whitelist/check" "200" "$ADMIN_TOKEN" "$LOG_DIR/whitelist-check-disabled.json" "$check_body"
  assert_json "disabled whitelist check result" "$LOG_DIR/whitelist-check-disabled.json" '.success == true and .data.whitelisted == false'

  curl_json "audit log page API has whitelist create event" "GET" "/api/v1/audit/logs?action=WHITELIST_CREATED&object_id=$ENTRY_ID&limit=10" "200" "$ADMIN_TOKEN" "$LOG_DIR/whitelist-audit-created.json"
  assert_json "audit log contains whitelist create event" "$LOG_DIR/whitelist-audit-created.json" \
    --arg entry_id "$ENTRY_ID" '.success == true and (.data.total >= 1) and ([.data.trails[] | select(.resource_id == $entry_id and .action == "WHITELIST_CREATED")] | length) >= 1'

  curl_json "audit log page API has whitelist disable event" "GET" "/api/v1/audit/logs?action=WHITELIST_DISABLED&object_id=$ENTRY_ID&limit=10" "200" "$ADMIN_TOKEN" "$LOG_DIR/whitelist-audit-disabled.json"
  assert_json "audit log contains whitelist disable event" "$LOG_DIR/whitelist-audit-disabled.json" \
    --arg entry_id "$ENTRY_ID" '.success == true and (.data.total >= 1) and ([.data.trails[] | select(.resource_id == $entry_id and .action == "WHITELIST_DISABLED")] | length) >= 1'

  assert_psql_count "whitelist entry persisted as disabled in PostgreSQL" \
    "SELECT count(*) FROM whitelist WHERE tenant_id = '$TENANT' AND id::text = '$ENTRY_ID' AND value = '$ENTRY_VALUE' AND status = 'disabled' AND approval_status = 'approved';" \
    "pg-whitelist-entry-count.txt"
  assert_psql_count "whitelist governance audit rows persisted in PostgreSQL" \
    "SELECT count(*) FROM audit_logs WHERE tenant_id = '$TENANT' AND object_type = 'whitelist' AND object_id = '$ENTRY_ID' AND action IN ('WHITELIST_CREATED','WHITELIST_APPROVAL_SUBMITTED','WHITELIST_APPROVED','WHITELIST_EXTENDED','WHITELIST_DISABLED');" \
    "pg-whitelist-audit-count.txt"
else
  json_log "assert" "created whitelist entry id extracted" "blocker" false "missing" "create response did not return id" "whitelist-create.json"
fi

TOTAL="$(jq -s 'length' "$REPORT")"
PASSED="$(jq -s '[.[] | select(.passed == true)] | length' "$REPORT")"
BLOCKERS="$(jq -s '[.[] | select(.passed == false and .severity == "blocker")] | length' "$REPORT")"
WARNINGS="$(jq -s '[.[] | select(.severity == "warn")] | length' "$REPORT")"
RESULT="pass"
if [[ "$BLOCKERS" -gt 0 ]]; then
  RESULT="blocked"
elif [[ "$WARNINGS" -gt 0 ]]; then
  RESULT="warn"
fi

jq -n \
  --arg run_id "$RUN_ID" \
  --arg result "$RESULT" \
  --arg apisix "$APISIX" \
  --arg tenant "$TENANT" \
  --arg other_tenant "$OTHER_TENANT" \
  --arg entry_id "${ENTRY_ID:-}" \
  --arg entry_value "$ENTRY_VALUE" \
  --arg source_alert_id "$SOURCE_ALERT_ID" \
  --arg report "$REPORT" \
  --arg local_report "$LOCAL_REPORT" \
  --argjson total "$TOTAL" \
  --argjson passed "$PASSED" \
  --argjson blockers "$BLOCKERS" \
  --argjson warnings "$WARNINGS" \
  --slurpfile checks "$REPORT" \
  '{
    run_id:$run_id,
    result:$result,
    apisix:$apisix,
    tenant:$tenant,
    other_tenant:$other_tenant,
    entry_id:$entry_id,
    entry_value:$entry_value,
    source_alert_id:$source_alert_id,
    report:$report,
    local_report:$local_report,
    total:$total,
    passed:$passed,
    blockers:$blockers,
    warnings:$warnings,
    checks:$checks
  }' >"$SUMMARY"

cat >"$LOCAL_REPORT" <<MD
# Whitelist Governance Live Preflight

- Run: \`$RUN_ID\`
- Result: \`$RESULT\`
- Checks: \`$PASSED/$TOTAL\` passed, \`$BLOCKERS\` blockers, \`$WARNINGS\` warnings
- Whitelist entry: \`${ENTRY_ID:-missing}\`
- Match value: \`$ENTRY_VALUE\`

This gate closes the whitelist governance business loop:
draft creation, approval submission, activation, expiry extension, disable,
match-check behavior, viewer write denial, cross-tenant isolation, PostgreSQL
persistence, and audit-log queryability.
MD

cp "$SUMMARY" "$REGRESSION_DIR/whitelist-governance-preflight-latest.json"
cp "$LOCAL_REPORT" "$REGRESSION_DIR/whitelist-governance-preflight-latest.md"
cp "$LOG_DIR/whitelist-before.json" "$REGRESSION_DIR/whitelist-latest.json" 2>/dev/null || true
cp "$LOG_DIR/whitelist-disable.json" "$REGRESSION_DIR/whitelist-disable-latest.json" 2>/dev/null || true
cp "$LOG_DIR/whitelist-audit-disabled.json" "$REGRESSION_DIR/whitelist-audit-latest.json" 2>/dev/null || true

echo "whitelist governance preflight result: $RESULT"
echo "summary: $SUMMARY"
echo "local report: $LOCAL_REPORT"

if [[ "$RESULT" == "blocked" && "$ALLOW_BLOCKERS" != "true" ]]; then
  exit 1
fi
