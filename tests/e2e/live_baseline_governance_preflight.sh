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

assert_psql_exec() {
  local name="$1" sql="$2" artifact="$3"
  local out="$LOG_DIR/$artifact"
  set +e
  psql_exec "$sql" >"$out" 2>"$out.err"
  local rc=$?
  set -e
  if [[ "$rc" -eq 0 ]]; then
    json_log "postgres" "$name" "info" true "ok" "$(trim_file "$out")" "$artifact"
  else
    json_log "postgres" "$name" "blocker" false "failed" "$(trim_file "$out.err")" "$(basename "$out.err")"
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

This gate covers five baseline dimensions, list/detail reads, the legacy reset,
audited governance command persistence, viewer write denial, PostgreSQL action,
version and outbox rows, audit-log queryability, and cross-tenant isolation.
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
assert_json "behavior baselines response shape" "$LOG_DIR/baselines-list.json" '.success == true and (.data.baselines | type == "array") and (.data.total | type == "number") and .data.summary.scope == "all_entities_in_window" and (.data.summary.learning | type == "number") and (.data.summary.active | type == "number")'

for baseline_type in asset account port protocol time; do
  curl_json "$baseline_type behavior baselines readable" "GET" "/api/v1/baselines?baseline_type=$baseline_type&limit=3" "200" "$ADMIN_TOKEN" "$LOG_DIR/baselines-$baseline_type.json"
  assert_json "$baseline_type baseline response is typed" "$LOG_DIR/baselines-$baseline_type.json" --arg baseline_type "$baseline_type" '.success == true and (.data.total | type == "number") and ([.data.baselines[] | select(.baseline_type != $baseline_type)] | length) == 0'
done

BASELINE_ID="$(jq -r '.data.baselines[0].baseline_id // ""' "$LOG_DIR/baselines-list.json")"
if [[ -z "$BASELINE_ID" ]]; then
  json_log "precondition" "behavior baseline sample exists" "blocker" false "missing" "GET /api/v1/baselines returned no baseline rows; seed traffic.sessions before reset validation" "baselines-list.json"
  finish
fi
json_log "precondition" "behavior baseline sample exists" "info" true "ok" "$BASELINE_ID" "baselines-list.json"

curl_json "behavior baseline detail readable" "GET" "/api/v1/baselines/$BASELINE_ID" "200" "$ADMIN_TOKEN" "$LOG_DIR/baseline-detail.json"
assert_json "behavior baseline detail response shape" "$LOG_DIR/baseline-detail.json" --arg baseline_id "$BASELINE_ID" '.success == true and .data.baseline_id == $baseline_id and (.data.metrics | type == "array")'

curl_json "behavior baseline real analytics readable" "GET" "/api/v1/baselines/$BASELINE_ID/analytics?window_days=30" "200" "$ADMIN_TOKEN" "$LOG_DIR/baseline-analytics.json"
assert_json "behavior analytics exposes real distributions and time buckets" "$LOG_DIR/baseline-analytics.json" --arg baseline_id "$BASELINE_ID" '.success == true and .data.baseline_id == $baseline_id and (.data.distributions | length) >= 1 and (.data.distributions[0].values | length) == 5 and (.data.series | length) >= 1 and (.data.series[0].samples | type == "array")'

curl_json "behavior baseline beyond old 200 row detail boundary is readable" "GET" "/api/v1/baselines?baseline_type=asset&limit=1&offset=300" "200" "$ADMIN_TOKEN" "$LOG_DIR/baseline-offset-300.json"
OFFSET_BASELINE_ID="$(jq -r '.data.baselines[0].baseline_id // ""' "$LOG_DIR/baseline-offset-300.json")"
if [[ -n "$OFFSET_BASELINE_ID" ]]; then
  curl_json "offset baseline detail readable" "GET" "/api/v1/baselines/$OFFSET_BASELINE_ID" "200" "$ADMIN_TOKEN" "$LOG_DIR/baseline-offset-300-detail.json"
  assert_json "offset baseline detail matches listed id" "$LOG_DIR/baseline-offset-300-detail.json" --arg baseline_id "$OFFSET_BASELINE_ID" '.success == true and .data.baseline_id == $baseline_id'
else
  json_log "precondition" "offset baseline sample not required for small fixture" "info" true "skipped" "asset total is below the old 200-row boundary" "baseline-offset-300.json"
fi

curl_json "90 day behavior baseline historical page is readable" "GET" "/api/v1/baselines?baseline_type=asset&window_days=90&limit=1&offset=1000" "200" "$ADMIN_TOKEN" "$LOG_DIR/baseline-90d-offset-1000.json"
HISTORICAL_BASELINE_ID="$(jq -r '.data.baselines[0].baseline_id // ""' "$LOG_DIR/baseline-90d-offset-1000.json")"
if [[ -n "$HISTORICAL_BASELINE_ID" ]]; then
  curl_json "90 day historical baseline detail inherits window" "GET" "/api/v1/baselines/$HISTORICAL_BASELINE_ID?window_days=90" "200" "$ADMIN_TOKEN" "$LOG_DIR/baseline-90d-offset-1000-detail.json"
  assert_json "90 day historical detail matches listed id" "$LOG_DIR/baseline-90d-offset-1000-detail.json" --arg baseline_id "$HISTORICAL_BASELINE_ID" '.success == true and .data.baseline_id == $baseline_id'
else
  json_log "precondition" "90 day historical sample not required for small fixture" "info" true "skipped" "90 day asset total is below 1000" "baseline-90d-offset-1000.json"
fi

curl_json "admin can queue audited threshold governance" "POST" "/api/v1/baselines/$BASELINE_ID/actions" "200" "$ADMIN_TOKEN" "$LOG_DIR/baseline-action-admin.json" '{"action":"adjust_threshold","reason":"live preflight verifies durable queued governance","warning_multiplier":2.1,"alert_multiplier":3.2}'
assert_json "threshold local state and downstream state are distinct" "$LOG_DIR/baseline-action-admin.json" --arg baseline_id "$BASELINE_ID" '.success == true and .data.action.baseline_id == $baseline_id and .data.action.action == "adjust_threshold" and .data.action.status == "applied" and .data.action.local_state_applied == true and .data.action.downstream_status == "queued" and .data.audit_written == true'
ACTION_ID="$(jq -r '.data.action.action_id // ""' "$LOG_DIR/baseline-action-admin.json")"

curl_json "persisted behavior baseline versions readable" "GET" "/api/v1/baselines/$BASELINE_ID/versions" "200" "$ADMIN_TOKEN" "$LOG_DIR/baseline-versions.json"
assert_json "version list exposes real rollback snapshots" "$LOG_DIR/baseline-versions.json" '.success == true and (.data.total >= 1) and (.data.versions | type == "array") and (.data.versions[0].snapshot | type == "object")'

curl_json "persisted behavior baseline actions readable" "GET" "/api/v1/baselines/$BASELINE_ID/actions" "200" "$ADMIN_TOKEN" "$LOG_DIR/baseline-actions.json"
assert_json "action list exposes local and downstream state" "$LOG_DIR/baseline-actions.json" '.success == true and (.data.total >= 1) and ([.data.actions[] | select(.action == "adjust_threshold" and .local_state_applied == true and .downstream_status == "queued")] | length) >= 1'

ACTION_ID_SQL="$(sql_escape "$ACTION_ID")"
assert_psql_exec "preflight marks one owned outbox row published" \
  "UPDATE behavior_baseline_outbox SET published=true, published_at=now(), attempts=1, last_error='' WHERE tenant_id='$TENANT' AND action_id='$ACTION_ID_SQL'::uuid RETURNING action_id;" \
  "pg-behavior-baseline-outbox-published.txt"
curl_json "published behavior baseline downstream state readable" "GET" "/api/v1/baselines/$BASELINE_ID/actions" "200" "$ADMIN_TOKEN" "$LOG_DIR/baseline-actions-published.json"
assert_json "published outbox state maps to published" "$LOG_DIR/baseline-actions-published.json" --arg action_id "$ACTION_ID" '([.data.actions[] | select(.action_id == $action_id and .downstream_status == "published" and .downstream_attempts == 1)] | length) == 1'

curl_json "admin can queue model feedback for failed-state mapping" "POST" "/api/v1/baselines/$BASELINE_ID/actions" "200" "$ADMIN_TOKEN" "$LOG_DIR/baseline-action-failed-seed.json" '{"action":"feedback_model","reason":"live preflight verifies persisted downstream failure mapping"}'
FAILED_ACTION_ID="$(jq -r '.data.action.action_id // ""' "$LOG_DIR/baseline-action-failed-seed.json")"
FAILED_ACTION_ID_SQL="$(sql_escape "$FAILED_ACTION_ID")"
assert_psql_exec "preflight records one owned outbox failure" \
  "UPDATE behavior_baseline_outbox SET published=false, attempts=2, last_error='preflight downstream failure' WHERE tenant_id='$TENANT' AND action_id='$FAILED_ACTION_ID_SQL'::uuid RETURNING action_id;" \
  "pg-behavior-baseline-outbox-failed.txt"
curl_json "failed behavior baseline downstream state readable" "GET" "/api/v1/baselines/$BASELINE_ID/actions" "200" "$ADMIN_TOKEN" "$LOG_DIR/baseline-actions-failed.json"
assert_json "outbox error maps to failed with attempts and reason" "$LOG_DIR/baseline-actions-failed.json" --arg action_id "$FAILED_ACTION_ID" '([.data.actions[] | select(.action_id == $action_id and .downstream_status == "failed" and .downstream_attempts == 2 and .downstream_error == "preflight downstream failure")] | length) == 1'

curl_json "viewer cannot queue behavior baseline governance" "POST" "/api/v1/baselines/$BASELINE_ID/actions" "403" "$VIEWER_TOKEN" "$LOG_DIR/baseline-action-viewer.json" '{"action":"freeze","reason":"viewer must be denied"}' || true
assert_json "viewer governance denial mentions alert write" "$LOG_DIR/baseline-action-viewer.json" '((.error.message // .message // "")) | contains("alert:write")'

curl_json "admin can reset behavior baseline" "POST" "/api/v1/baselines/$BASELINE_ID/reset" "200" "$ADMIN_TOKEN" "$LOG_DIR/baseline-reset-admin.json"
assert_json "baseline reset response is tenant scoped and learning" "$LOG_DIR/baseline-reset-admin.json" \
  --arg baseline_id "$BASELINE_ID" \
  --arg tenant "$TENANT" \
  '.success == true and .data.baseline_id == $baseline_id and .data.tenant_id == $tenant and .data.status == "learning"'

curl_json "zero-sample reset baseline remains directly readable" "GET" "/api/v1/baselines/$BASELINE_ID?window_days=30" "200" "$ADMIN_TOKEN" "$LOG_DIR/baseline-reset-detail.json"
assert_json "reset detail is a governable learning snapshot" "$LOG_DIR/baseline-reset-detail.json" --arg baseline_id "$BASELINE_ID" '.success == true and .data.baseline_id == $baseline_id and .data.status == "learning" and (.data.metrics | length) >= 1'
curl_json "zero-sample reset analytics remains readable" "GET" "/api/v1/baselines/$BASELINE_ID/analytics?window_days=30" "200" "$ADMIN_TOKEN" "$LOG_DIR/baseline-reset-analytics.json"
assert_json "reset analytics returns typed empty-or-new-sample series" "$LOG_DIR/baseline-reset-analytics.json" --arg baseline_id "$BASELINE_ID" '.success == true and .data.baseline_id == $baseline_id and (.data.distributions | type == "array") and (.data.series | type == "array")'
curl_json "zero-sample reset versions remain readable" "GET" "/api/v1/baselines/$BASELINE_ID/versions" "200" "$ADMIN_TOKEN" "$LOG_DIR/baseline-reset-versions.json"
assert_json "reset versions keep governance history" "$LOG_DIR/baseline-reset-versions.json" '.success == true and (.data.versions | type == "array") and (.data.total >= 1)'
curl_json "zero-sample reset actions remain readable" "GET" "/api/v1/baselines/$BASELINE_ID/actions" "200" "$ADMIN_TOKEN" "$LOG_DIR/baseline-reset-actions.json"
assert_json "reset actions keep published and failed downstream truth" "$LOG_DIR/baseline-reset-actions.json" --arg published_id "$ACTION_ID" --arg failed_id "$FAILED_ACTION_ID" '([.data.actions[] | select(.action_id == $published_id and .downstream_status == "published")] | length) == 1 and ([.data.actions[] | select(.action_id == $failed_id and .downstream_status == "failed")] | length) == 1'
curl_json "reset-aware all-window summary readable" "GET" "/api/v1/baselines?baseline_type=asset&window_days=30&limit=5" "200" "$ADMIN_TOKEN" "$LOG_DIR/baseline-reset-summary.json"
assert_json "summary is window-scoped reset-aware and exclusive" "$LOG_DIR/baseline-reset-summary.json" '.success == true and .data.summary.scope == "all_entities_in_window" and ((.data.summary.learning + .data.summary.active + .data.summary.drift + .data.summary.rebuild + .data.summary.frozen) == .data.summary.total) and (.data.summary.alerts <= .data.summary.total)'

curl_json "zero-sample reset baseline accepts subsequent governance" "POST" "/api/v1/baselines/$BASELINE_ID/actions" "200" "$ADMIN_TOKEN" "$LOG_DIR/baseline-reset-action-read-after-write.json" '{"action":"drift_watch","reason":"live preflight verifies post-reset governance remains available"}'
assert_json "post-reset governance is persisted and audited" "$LOG_DIR/baseline-reset-action-read-after-write.json" --arg baseline_id "$BASELINE_ID" '.success == true and .data.action.baseline_id == $baseline_id and .data.action.action == "drift_watch" and .data.action.local_state_applied == true and .data.audit_written == true'

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
assert_psql_count "behavior baseline action persisted in PostgreSQL" \
  "SELECT count(*) FROM behavior_baseline_actions WHERE tenant_id = '$TENANT' AND baseline_id = '$BASELINE_ID_SQL' AND action_type='adjust_threshold' AND status='applied';" \
  "pg-behavior-baseline-action-count.txt"
assert_psql_count "behavior baseline outbox persisted in PostgreSQL" \
  "SELECT count(*) FROM behavior_baseline_outbox o JOIN behavior_baseline_actions a ON a.action_id=o.action_id WHERE o.tenant_id = '$TENANT' AND o.baseline_id = '$BASELINE_ID_SQL' AND a.action_type='adjust_threshold' AND o.published=false;" \
  "pg-behavior-baseline-outbox-count.txt"
assert_psql_count "behavior baseline version snapshot persisted" \
  "SELECT count(*) FROM behavior_baseline_versions WHERE tenant_id = '$TENANT' AND baseline_id = '$BASELINE_ID_SQL' AND version >= 2;" \
  "pg-behavior-baseline-version-count.txt"
assert_psql_count "behavior baseline action audit persisted" \
  "SELECT count(*) FROM audit_logs WHERE tenant_id = '$TENANT' AND action = 'BEHAVIOR_BASELINE_ADJUST_THRESHOLD' AND object_type = 'baseline' AND object_id = '$BASELINE_ID_SQL';" \
  "pg-behavior-baseline-action-audit-count.txt"

finish
