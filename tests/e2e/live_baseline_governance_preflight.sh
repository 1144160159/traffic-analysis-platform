#!/usr/bin/env bash
set -euo pipefail

APISIX="${APISIX:-http://10.0.5.8:30180}"
TENANT="${TENANT:-default}"
OTHER_TENANT="${OTHER_TENANT:-tenant-b}"
LOG_DIR="${LOG_DIR:-doc/02_acceptance/runs/$(date +%Y%m%d%H%M%S)-baseline-governance-preflight}"
RUN_ID="${RUN_ID:-$(date +%Y%m%d%H%M%S)-baseline-governance-preflight}"
REGRESSION_DIR="${REGRESSION_DIR:-doc/02_acceptance/02-regression}"
KUBECTL="${KUBECTL:-kubectl}"
ALLOW_BLOCKERS="${ALLOW_BLOCKERS:-false}"
JWT_SECRET_NAMESPACE="${JWT_SECRET_NAMESPACE:-traffic-analysis}"
JWT_SECRET_NAME="${JWT_SECRET_NAME:-traffic-credentials}"
JWT_SECRET_KEY="${JWT_SECRET_KEY:-JWT_SECRET}"
PG_NAMESPACE="${PG_NAMESPACE:-databases}"
PG_POD="${PG_POD:-postgres-primary-0}"
PG_SECRET_KEY="${PG_SECRET_KEY:-PG_PASSWORD}"

REPORT="$LOG_DIR/live-baseline-governance-preflight-$RUN_ID.ndjson"
SUMMARY="$LOG_DIR/live-baseline-governance-preflight-$RUN_ID-summary.json"
LOCAL_REPORT="$LOG_DIR/local-report.md"

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

urlencode() {
  jq -rn --arg value "$1" '$value | @uri'
}

sql_escape() {
  printf "%s" "$1" | sed "s/'/''/g"
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
    "session_id": "codex-baseline-governance-" + os.environ["RUN_ID"],
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
    --arg baseline_id "${BASELINE_ID:-}" \
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
      baseline_id:$baseline_id,
      report:$report,
      local_report:$local_report,
      total:$total,
      passed:$passed,
      blockers:$blockers,
      warnings:$warnings,
      checks:$checks
    }' >"$SUMMARY"

  cat >"$LOCAL_REPORT" <<MD
# Baseline Governance Live Preflight

- Run: \`$RUN_ID\`
- Result: \`$result\`
- Checks: \`$passed/$total\` passed, \`$blockers\` blockers, \`$warnings\` warnings
- Baseline: \`${BASELINE_ID:-missing}\`

This gate closes the behavior-baseline reset loop: baseline list/detail read,
frontend action contract, admin reset, viewer write denial, PostgreSQL
\`behavior_baseline_resets\` persistence, audit-log queryability, and
cross-tenant audit isolation.
MD

  cp "$SUMMARY" "$REGRESSION_DIR/baseline-governance-preflight-latest.json"
  cp "$LOCAL_REPORT" "$REGRESSION_DIR/baseline-governance-preflight-latest.md"
  cp "$LOG_DIR/baseline-reset-admin.json" "$REGRESSION_DIR/baseline-reset-latest.json" 2>/dev/null || true
  cp "$LOG_DIR/baseline-audit-reset.json" "$REGRESSION_DIR/baseline-audit-latest.json" 2>/dev/null || true

  echo "baseline governance preflight result: $result"
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
PG_PASSWORD="$(kctl -n "$JWT_SECRET_NAMESPACE" get secret "$JWT_SECRET_NAME" -o "jsonpath={.data.$PG_SECRET_KEY}" | base64 -d)"

ADMIN_TOKEN="$(make_token codex-baseline-admin "$TENANT" '["admin"]' '["*","admin:*","alert:write","audit:read","user:read"]')"
VIEWER_TOKEN="$(make_token codex-baseline-viewer "$TENANT" '["viewer"]' '["alert:read","audit:read"]')"
OTHER_TOKEN="$(make_token codex-baseline-other-admin "$OTHER_TENANT" '["admin"]' '["*","admin:*","alert:write","audit:read","user:read"]')"

if grep -q "baseline-reset" web/ui/src/services/pageApiPlans.ts && grep -q "BEHAVIOR_BASELINE_RESET" web/ui/src/services/pageApiPlans.ts; then
  json_log "contract" "frontend baseline action contract present" "info" true "ok" "baseline reset action declared" "web/ui/src/services/pageApiPlans.ts"
else
  json_log "contract" "frontend baseline action contract present" "blocker" false "missing" "baseline action contract missing" "web/ui/src/services/pageApiPlans.ts"
fi

curl_json "behavior baselines readable" "GET" "/api/v1/baselines?limit=5" "200" "$ADMIN_TOKEN" "$LOG_DIR/baselines-list.json"
assert_json "behavior baselines response shape" "$LOG_DIR/baselines-list.json" '.success == true and (.data.baselines | type == "array") and (.data.total | type == "number")'

BASELINE_ID="$(jq -r '.data.baselines[0].baseline_id // ""' "$LOG_DIR/baselines-list.json")"
if [[ -z "$BASELINE_ID" ]]; then
  json_log "precondition" "behavior baseline sample exists" "blocker" false "missing" "GET /api/v1/baselines returned no baseline rows; seed traffic.sessions before reset validation" "baselines-list.json"
  finish
fi
json_log "precondition" "behavior baseline sample exists" "info" true "ok" "$BASELINE_ID" "baselines-list.json"

curl_json "behavior baseline detail readable" "GET" "/api/v1/baselines/$BASELINE_ID" "200" "$ADMIN_TOKEN" "$LOG_DIR/baseline-detail.json"
assert_json "behavior baseline detail response shape" "$LOG_DIR/baseline-detail.json" --arg baseline_id "$BASELINE_ID" '.success == true and .data.baseline_id == $baseline_id and (.data.metrics | type == "array")'

curl_json "admin can reset behavior baseline" "POST" "/api/v1/baselines/$BASELINE_ID/reset" "200" "$ADMIN_TOKEN" "$LOG_DIR/baseline-reset-admin.json"
assert_json "baseline reset response is tenant scoped and learning" "$LOG_DIR/baseline-reset-admin.json" \
  --arg baseline_id "$BASELINE_ID" \
  --arg tenant "$TENANT" \
  '.success == true and .data.baseline_id == $baseline_id and .data.tenant_id == $tenant and .data.status == "learning"'

curl_json "viewer cannot reset behavior baseline" "POST" "/api/v1/baselines/$BASELINE_ID/reset" "403" "$VIEWER_TOKEN" "$LOG_DIR/baseline-reset-viewer.json" || true
assert_json "viewer reset denial mentions alert write" "$LOG_DIR/baseline-reset-viewer.json" '((.error.message // .message // "")) | contains("alert:write")'

ENCODED_BASELINE_ID="$(urlencode "$BASELINE_ID")"
curl_json "audit log page API has baseline reset event" "GET" "/api/v1/audit/logs?action=BEHAVIOR_BASELINE_RESET&object_id=$ENCODED_BASELINE_ID&limit=10" "200" "$ADMIN_TOKEN" "$LOG_DIR/baseline-audit-reset.json"
assert_json "audit log contains baseline reset event" "$LOG_DIR/baseline-audit-reset.json" \
  --arg baseline_id "$BASELINE_ID" \
  '.success == true and (.data.total >= 1) and ([.data.trails[] | select(.resource_id == $baseline_id and .action == "BEHAVIOR_BASELINE_RESET")] | length) >= 1'

curl_json "other tenant cannot see baseline reset audit" "GET" "/api/v1/audit/logs?action=BEHAVIOR_BASELINE_RESET&object_id=$ENCODED_BASELINE_ID&limit=10" "200" "$OTHER_TOKEN" "$LOG_DIR/baseline-audit-other-tenant.json" "" "$OTHER_TENANT"
assert_json "baseline reset audit is tenant isolated" "$LOG_DIR/baseline-audit-other-tenant.json" '.success == true and .data.total == 0'

BASELINE_ID_SQL="$(sql_escape "$BASELINE_ID")"
assert_psql_count "behavior baseline reset persisted in PostgreSQL" \
  "SELECT count(*) FROM behavior_baseline_resets WHERE tenant_id = '$TENANT' AND baseline_id = '$BASELINE_ID_SQL' AND requested_by <> '';" \
  "pg-behavior-baseline-reset-count.txt"
assert_psql_count "behavior baseline reset audit persisted" \
  "SELECT count(*) FROM audit_logs WHERE tenant_id = '$TENANT' AND action = 'BEHAVIOR_BASELINE_RESET' AND object_type = 'baseline' AND object_id = '$BASELINE_ID_SQL';" \
  "pg-behavior-baseline-audit-count.txt"

finish
