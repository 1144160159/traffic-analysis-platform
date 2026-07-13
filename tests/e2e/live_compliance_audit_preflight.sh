#!/usr/bin/env bash
set -euo pipefail

APISIX="${APISIX:-http://10.0.5.8:30180}"
TENANT="${TENANT:-default}"
OTHER_TENANT="${OTHER_TENANT:-tenant-b}"
LOG_DIR="${LOG_DIR:-doc/02_acceptance/runs/$(date +%Y%m%d%H%M%S)-compliance-audit-preflight}"
RUN_ID="${RUN_ID:-$(date +%Y%m%d%H%M%S)-compliance-audit-preflight}"
REGRESSION_DIR="${REGRESSION_DIR:-doc/02_acceptance/02-regression}"
KUBECTL="${KUBECTL:-kubectl}"
ALLOW_BLOCKERS="${ALLOW_BLOCKERS:-false}"
JWT_SECRET_NAMESPACE="${JWT_SECRET_NAMESPACE:-traffic-analysis}"
JWT_SECRET_NAME="${JWT_SECRET_NAME:-traffic-credentials}"
JWT_SECRET_KEY="${JWT_SECRET_KEY:-JWT_SECRET}"
PG_NAMESPACE="${PG_NAMESPACE:-databases}"
PG_POD="${PG_POD:-postgres-primary-0}"
PG_SECRET_KEY="${PG_SECRET_KEY:-PG_PASSWORD}"

REPORT="$LOG_DIR/live-compliance-audit-preflight-$RUN_ID.ndjson"
SUMMARY="$LOG_DIR/live-compliance-audit-preflight-$RUN_ID-summary.json"
LOCAL_REPORT="$LOG_DIR/local-report.md"
RUN_SLUG="$(printf '%s' "$RUN_ID" | tr -c 'a-zA-Z0-9-' '-' | cut -c1-64 | sed 's/-*$//')"
REPORT_TYPE="${REPORT_TYPE:-codex-live-compliance-${RUN_SLUG:-run}}"

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
    head -c 1000 "$file" \
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
    "session_id": "codex-compliance-audit-" + os.environ["RUN_ID"],
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
  local name="$1" method="$2" path="$3" expected_code="$4" token="$5" output="$6" body="${7:-}"
  local err_file code rc
  err_file="$output.err"
  set +e
  if [[ -n "$body" ]]; then
    code="$(curl --noproxy '*' -sS -m 30 -o "$output" -w '%{http_code}' \
      -X "$method" \
      -H "Authorization: Bearer $token" \
      -H "X-Tenant-ID: $TENANT" \
      -H "Content-Type: application/json" \
      --data "$body" \
      "$APISIX$path" 2>"$err_file")"
  else
    code="$(curl --noproxy '*' -sS -m 30 -o "$output" -w '%{http_code}' \
      -X "$method" \
      -H "Authorization: Bearer $token" \
      -H "X-Tenant-ID: $TENANT" \
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

psql_exec() {
  local sql="$1"
  kctl -n "$PG_NAMESPACE" exec "$PG_POD" -- env PGPASSWORD="$PG_PASSWORD" \
    psql -U postgres -d traffic_platform -v ON_ERROR_STOP=1 -Atc "$sql"
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

need_cmd git
need_cmd jq
need_cmd curl
need_cmd python3
need_cmd "$KUBECTL"

git rev-parse HEAD >"$LOG_DIR/commit-sha.txt"
git status --short >"$LOG_DIR/git-status.txt"

JWT_SECRET="$(kctl -n "$JWT_SECRET_NAMESPACE" get secret "$JWT_SECRET_NAME" -o "jsonpath={.data.$JWT_SECRET_KEY}" | base64 -d)"
PG_PASSWORD="$(kctl -n "$JWT_SECRET_NAMESPACE" get secret "$JWT_SECRET_NAME" -o "jsonpath={.data.$PG_SECRET_KEY}" | base64 -d)"

ADMIN_TOKEN="$(make_token codex-compliance-admin "$TENANT" '["admin"]' '["*","admin:*","audit:read","user:read"]')"
VIEWER_TOKEN="$(make_token codex-compliance-viewer "$TENANT" '["viewer"]' '["user:read","audit:read"]')"
OTHER_TOKEN="$(make_token codex-compliance-other-admin "$OTHER_TENANT" '["admin"]' '["*","admin:*","audit:read","user:read"]')"

curl_json "compliance reports list is readable" "GET" "/api/v1/compliance/reports?limit=5" "200" "$ADMIN_TOKEN" "$LOG_DIR/compliance-reports-before.json"
assert_json "compliance reports response shape" "$LOG_DIR/compliance-reports-before.json" '.success == true and (.data.reports | type == "array") and (.data.total | type == "number")'

curl_json "compliance audit trail is readable" "GET" "/api/v1/compliance/audit-trail?limit=5" "200" "$ADMIN_TOKEN" "$LOG_DIR/compliance-audit-trail-before.json"
assert_json "compliance audit trail response shape" "$LOG_DIR/compliance-audit-trail-before.json" '.success == true and (.data.trails | type == "array") and (.data.total | type == "number")'

curl_json "audit log page API is readable" "GET" "/api/v1/audit/logs?limit=5" "200" "$ADMIN_TOKEN" "$LOG_DIR/audit-logs-before.json"
assert_json "audit log response shape" "$LOG_DIR/audit-logs-before.json" '.success == true and (.data.trails | type == "array") and (.data.total | type == "number")'

generate_body="$(jq -nc --arg report_type "$REPORT_TYPE" '{report_type:$report_type}')"
curl_json "admin can generate compliance report" "POST" "/api/v1/compliance/reports/generate" "200" "$ADMIN_TOKEN" "$LOG_DIR/compliance-generate-admin.json" "$generate_body"
assert_json "generated compliance report has persisted identity" "$LOG_DIR/compliance-generate-admin.json" \
  --arg report_type "$REPORT_TYPE" '.success == true and (.data.report_id | type == "string" and length > 0) and .data.report_type == $report_type and .data.status == "completed"'

REPORT_ID="$(jq -r '.data.report_id // ""' "$LOG_DIR/compliance-generate-admin.json")"
if [[ -n "$REPORT_ID" ]]; then
  curl_json "generated compliance report is queryable" "GET" "/api/v1/compliance/reports?report_type=$REPORT_TYPE&limit=10" "200" "$ADMIN_TOKEN" "$LOG_DIR/compliance-reports-after.json"
  assert_json "generated report appears in compliance report list" "$LOG_DIR/compliance-reports-after.json" \
    --arg report_id "$REPORT_ID" '.success == true and ([.data.reports[] | select(.report_id == $report_id)] | length) == 1'

  curl_json "compliance audit trail has generated report event" "GET" "/api/v1/compliance/audit-trail?action=COMPLIANCE_REPORT_GENERATED&object_id=$REPORT_ID&limit=10" "200" "$ADMIN_TOKEN" "$LOG_DIR/compliance-audit-trail-generated.json"
  assert_json "compliance audit trail contains generated event" "$LOG_DIR/compliance-audit-trail-generated.json" \
    --arg report_id "$REPORT_ID" '.success == true and (.data.total >= 1) and ([.data.trails[] | select(.resource_id == $report_id and .action == "COMPLIANCE_REPORT_GENERATED")] | length) >= 1'

  curl_json "audit log page API has generated report event" "GET" "/api/v1/audit/logs?action=COMPLIANCE_REPORT_GENERATED&object_id=$REPORT_ID&limit=10" "200" "$ADMIN_TOKEN" "$LOG_DIR/audit-logs-generated.json"
  assert_json "audit log page API contains generated event" "$LOG_DIR/audit-logs-generated.json" \
    --arg report_id "$REPORT_ID" '.success == true and (.data.total >= 1) and ([.data.trails[] | select(.resource_id == $report_id and .action == "COMPLIANCE_REPORT_GENERATED")] | length) >= 1'

  set +e
  psql_exec "SELECT count(*) FROM compliance_reports WHERE tenant_id = '$TENANT' AND report_id::text = '$REPORT_ID' AND report_type = '$REPORT_TYPE';" >"$LOG_DIR/pg-compliance-report-count.txt" 2>"$LOG_DIR/pg-compliance-report-count.err"
  report_count_rc=$?
  psql_exec "SELECT count(*) FROM audit_logs WHERE tenant_id = '$TENANT' AND action = 'COMPLIANCE_REPORT_GENERATED' AND object_type = 'compliance_report' AND object_id = '$REPORT_ID';" >"$LOG_DIR/pg-compliance-audit-count.txt" 2>"$LOG_DIR/pg-compliance-audit-count.err"
  audit_count_rc=$?
  set -e
  report_count="$(tr -d '[:space:]' <"$LOG_DIR/pg-compliance-report-count.txt")"
  audit_count="$(tr -d '[:space:]' <"$LOG_DIR/pg-compliance-audit-count.txt")"
  if [[ "$report_count_rc" -eq 0 && "$report_count" =~ ^[0-9]+$ && "$report_count" -ge 1 ]]; then
    json_log "postgres" "generated compliance report persisted in PostgreSQL" "info" true "ok" "$REPORT_ID" "pg-compliance-report-count.txt"
  else
    json_log "postgres" "generated compliance report persisted in PostgreSQL" "blocker" false "missing" "$(trim_file "$LOG_DIR/pg-compliance-report-count.err")" "pg-compliance-report-count.err"
  fi
  if [[ "$audit_count_rc" -eq 0 && "$audit_count" =~ ^[0-9]+$ && "$audit_count" -ge 1 ]]; then
    json_log "postgres" "generated compliance audit row persisted in PostgreSQL" "info" true "ok" "$REPORT_ID" "pg-compliance-audit-count.txt"
  else
    json_log "postgres" "generated compliance audit row persisted in PostgreSQL" "blocker" false "missing" "$(trim_file "$LOG_DIR/pg-compliance-audit-count.err")" "pg-compliance-audit-count.err"
  fi

  curl_json "other tenant cannot see generated audit row" "GET" "/api/v1/audit/logs?action=COMPLIANCE_REPORT_GENERATED&object_id=$REPORT_ID&limit=10" "200" "$OTHER_TOKEN" "$LOG_DIR/other-tenant-audit-logs-generated.json"
  assert_json "audit log query is tenant isolated" "$LOG_DIR/other-tenant-audit-logs-generated.json" '.success == true and .data.total == 0'
else
  json_log "assert" "generated compliance report id extracted" "blocker" false "missing" "admin generation did not return report_id" "compliance-generate-admin.json"
fi

curl_json "viewer cannot generate compliance report" "POST" "/api/v1/compliance/reports/generate" "403" "$VIEWER_TOKEN" "$LOG_DIR/compliance-generate-viewer.json" "$generate_body" || true

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
  --arg report_type "$REPORT_TYPE" \
  --arg report_id "${REPORT_ID:-}" \
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
    report_type:$report_type,
    report_id:$report_id,
    report:$report,
    local_report:$local_report,
    total:$total,
    passed:$passed,
    blockers:$blockers,
    warnings:$warnings,
    checks:$checks
  }' >"$SUMMARY"

cat >"$LOCAL_REPORT" <<MD
# Compliance Audit Live Preflight

- Run: \`$RUN_ID\`
- Result: \`$RESULT\`
- Checks: \`$PASSED/$TOTAL\` passed, \`$BLOCKERS\` blockers, \`$WARNINGS\` warnings
- Report type: \`$REPORT_TYPE\`
- Report id: \`${REPORT_ID:-missing}\`

This gate closes the compliance/audit business loop for the audit-config menu:
admin report generation, report query, audit trail query, audit-log page API
query, PostgreSQL persistence, tenant isolation, and viewer write denial.
MD

cp "$SUMMARY" "$REGRESSION_DIR/compliance-audit-preflight-latest.json"
cp "$LOCAL_REPORT" "$REGRESSION_DIR/compliance-audit-preflight-latest.md"
cp "$LOG_DIR/compliance-reports-after.json" "$REGRESSION_DIR/compliance-reports-latest.json" 2>/dev/null || true
cp "$LOG_DIR/compliance-audit-trail-generated.json" "$REGRESSION_DIR/compliance-audit-trail-latest.json" 2>/dev/null || true
cp "$LOG_DIR/audit-logs-generated.json" "$REGRESSION_DIR/audit-log-generated-latest.json" 2>/dev/null || true

echo "compliance audit preflight result: $RESULT"
echo "summary: $SUMMARY"
echo "local report: $LOCAL_REPORT"

if [[ "$RESULT" == "blocked" && "$ALLOW_BLOCKERS" != "true" ]]; then
  exit 1
fi
