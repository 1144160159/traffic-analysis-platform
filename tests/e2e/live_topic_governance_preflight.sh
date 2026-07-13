#!/usr/bin/env bash
set -euo pipefail

APISIX="${APISIX:-http://10.0.5.8:30180}"
TENANT="${TENANT:-default}"
OTHER_TENANT="${OTHER_TENANT:-tenant-b}"
LOG_DIR="${LOG_DIR:-doc/02_acceptance/runs/$(date +%Y%m%d%H%M%S)-topic-governance-preflight}"
RUN_ID="${RUN_ID:-$(date +%Y%m%d%H%M%S)-topic-governance-preflight}"
REGRESSION_DIR="${REGRESSION_DIR:-doc/02_acceptance/02-regression}"
KUBECTL="${KUBECTL:-kubectl}"
ALLOW_BLOCKERS="${ALLOW_BLOCKERS:-false}"
JWT_SECRET_NAMESPACE="${JWT_SECRET_NAMESPACE:-traffic-analysis}"
JWT_SECRET_NAME="${JWT_SECRET_NAME:-traffic-credentials}"
JWT_SECRET_KEY="${JWT_SECRET_KEY:-JWT_SECRET}"
PG_NAMESPACE="${PG_NAMESPACE:-databases}"
PG_POD="${PG_POD:-postgres-primary-0}"
PG_SECRET_KEY="${PG_SECRET_KEY:-PG_PASSWORD}"

REPORT="$LOG_DIR/live-topic-governance-preflight-$RUN_ID.ndjson"
SUMMARY="$LOG_DIR/live-topic-governance-preflight-$RUN_ID-summary.json"
LOCAL_REPORT="$LOG_DIR/local-report.md"
RUN_SLUG="$(printf '%s' "$RUN_ID" | tr -c 'a-zA-Z0-9-' '-' | cut -c1-64 | sed 's/-*$//')"
VIEW_NAME="${VIEW_NAME:-codex topic view ${RUN_SLUG:-run}}"
SCOPE_NAME="${SCOPE_NAME:-codex scope ${RUN_SLUG:-run}}"
SUBSCRIPTION_RECIPIENT="${SUBSCRIPTION_RECIPIENT:-codex-topic-${RUN_SLUG:-run}}"

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
    "session_id": "codex-topic-governance-" + os.environ["RUN_ID"],
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

ADMIN_TOKEN="$(make_token codex-topic-admin "$TENANT" '["admin"]' '["*","admin:*","topic:write","topic:export","audit:read","user:read"]')"
VIEWER_TOKEN="$(make_token codex-topic-viewer "$TENANT" '["viewer"]' '["user:read","audit:read"]')"
OTHER_TOKEN="$(make_token codex-topic-other-admin "$OTHER_TENANT" '["admin"]' '["*","admin:*","topic:write","topic:export","audit:read","user:read"]')"

if grep -q "topic-view-save" web/ui/src/services/pageApiPlans.ts \
  && grep -q "TOPIC_REPORT_EXPORTED" web/ui/src/services/pageApiPlans.ts \
  && grep -q "adaptTopicsOverview" web/ui/src/services/pageSnapshotAdapters.ts; then
  json_log "contract" "frontend topic governance contract present" "info" true "ok" "topic actions, exports, and snapshot adapter declared" "web/ui/src/services/pageApiPlans.ts"
else
  json_log "contract" "frontend topic governance contract present" "blocker" false "missing" "topic action contracts or snapshot adapter missing" "web/ui/src/services/pageApiPlans.ts"
fi

curl_json "topic tunnel page readable" "GET" "/api/v1/topics/tunnel" "200" "$ADMIN_TOKEN" "$LOG_DIR/topic-tunnel.json"
assert_json "topic tunnel response shape" "$LOG_DIR/topic-tunnel.json" '.success == true and .data.topic == "tunnel"'
curl_json "topic exfil page readable" "GET" "/api/v1/topics/exfil" "200" "$ADMIN_TOKEN" "$LOG_DIR/topic-exfil.json"
assert_json "topic exfil response shape" "$LOG_DIR/topic-exfil.json" '.success == true and .data.topic == "exfil"'
curl_json "topic apt page readable" "GET" "/api/v1/topics/apt" "200" "$ADMIN_TOKEN" "$LOG_DIR/topic-apt.json"
assert_json "topic apt response shape" "$LOG_DIR/topic-apt.json" '.success == true and .data.topic == "apt"'

curl_json "topic saved views readable" "GET" "/api/v1/topics/views?limit=5" "200" "$ADMIN_TOKEN" "$LOG_DIR/topic-views-before.json"
assert_json "topic saved views response shape" "$LOG_DIR/topic-views-before.json" '.success == true and (.data.views | type == "array") and (.data.total | type == "number")'
curl_json "topic subscriptions readable" "GET" "/api/v1/topics/subscriptions?limit=5" "200" "$ADMIN_TOKEN" "$LOG_DIR/topic-subscriptions-before.json"
assert_json "topic subscriptions response shape" "$LOG_DIR/topic-subscriptions-before.json" '.success == true and (.data.subscriptions | type == "array") and (.data.total | type == "number")'

view_body="$(jq -nc --arg name "$VIEW_NAME" '{topic:"tunnel", name:$name, filters:{risk:"high", owner:"codex"}, visibility:"team", favorite:true}')"
curl_json "admin can save topic view" "POST" "/api/v1/topics/views" "201" "$ADMIN_TOKEN" "$LOG_DIR/topic-view-create.json" "$view_body"
assert_json "created topic view has identity" "$LOG_DIR/topic-view-create.json" \
  --arg name "$VIEW_NAME" '.success == true and (.data.view_id | type == "string" and length > 0) and .data.topic == "tunnel" and .data.name == $name'

VIEW_ID="$(jq -r '.data.view_id // ""' "$LOG_DIR/topic-view-create.json")"
if [[ -n "$VIEW_ID" ]]; then
  curl_json "admin can share favorite topic view" "PATCH" "/api/v1/topics/views/$VIEW_ID" "200" "$ADMIN_TOKEN" "$LOG_DIR/topic-view-share.json" '{"shared":true,"favorite":true}'
  assert_json "topic view share response" "$LOG_DIR/topic-view-share.json" '.success == true and .data.shared == true and (.data.share_token | type == "string" and length > 0)'

  curl_json "other tenant cannot update topic view" "PATCH" "/api/v1/topics/views/$VIEW_ID" "404" "$OTHER_TOKEN" "$LOG_DIR/topic-view-other-tenant.json" '{"favorite":false}' "$OTHER_TENANT" || true
  curl_json "viewer cannot save topic view" "POST" "/api/v1/topics/views" "403" "$VIEWER_TOKEN" "$LOG_DIR/topic-view-viewer.json" "$view_body" || true

  curl_json "audit log page API has topic view save event" "GET" "/api/v1/audit/logs?action=TOPIC_VIEW_SAVED&object_id=$VIEW_ID&limit=10" "200" "$ADMIN_TOKEN" "$LOG_DIR/topic-audit-view-saved.json"
  assert_json "audit log contains topic view save event" "$LOG_DIR/topic-audit-view-saved.json" \
    --arg view_id "$VIEW_ID" '.success == true and (.data.total >= 1) and ([.data.trails[] | select(.resource_id == $view_id and .action == "TOPIC_VIEW_SAVED")] | length) >= 1'

  assert_psql_count "topic saved view persisted in PostgreSQL" \
    "SELECT count(*) FROM topic_saved_views WHERE tenant_id = '$TENANT' AND view_id = '$VIEW_ID' AND shared = true AND favorite = true;" \
    "pg-topic-view-count.txt"
else
  json_log "assert" "created topic view id extracted" "blocker" false "missing" "create response did not return view_id" "topic-view-create.json"
fi

scope_body="$(jq -nc --arg scope "$SCOPE_NAME" '{scope_name:$scope, included_assets:["core-switch","border-fw"], risk_levels:["high","critical"], time_window:"24h", detail:{reason:"codex live topic governance preflight"}}')"
curl_json "admin can update topic scope" "PUT" "/api/v1/topics/scopes/tunnel" "200" "$ADMIN_TOKEN" "$LOG_DIR/topic-scope-update.json" "$scope_body"
assert_json "topic scope update response" "$LOG_DIR/topic-scope-update.json" \
  --arg scope "$SCOPE_NAME" '.success == true and .data.topic == "tunnel" and .data.scope_name == $scope and (.data.included_assets | length) >= 1'

subscription_body="$(jq -nc --arg recipient "$SUBSCRIPTION_RECIPIENT" '{topic:"exfil", channel:"webhook", threshold:"high", schedule:"realtime", recipients:[$recipient], enabled:true}')"
curl_json "admin can create topic subscription" "POST" "/api/v1/topics/subscriptions" "201" "$ADMIN_TOKEN" "$LOG_DIR/topic-subscription-create.json" "$subscription_body"
assert_json "created topic subscription has identity" "$LOG_DIR/topic-subscription-create.json" \
  --arg recipient "$SUBSCRIPTION_RECIPIENT" '.success == true and (.data.subscription_id | type == "string" and length > 0) and .data.topic == "exfil" and .data.enabled == true and (.data.recipients | index($recipient)) != null'

SUB_ID="$(jq -r '.data.subscription_id // ""' "$LOG_DIR/topic-subscription-create.json")"
if [[ -n "$SUB_ID" ]]; then
  curl_json "admin can disable topic subscription" "PATCH" "/api/v1/topics/subscriptions/$SUB_ID" "200" "$ADMIN_TOKEN" "$LOG_DIR/topic-subscription-disable.json" '{"enabled":false}'
  assert_json "topic subscription disable response" "$LOG_DIR/topic-subscription-disable.json" '.success == true and .data.enabled == false'

  curl_json "other tenant cannot update topic subscription" "PATCH" "/api/v1/topics/subscriptions/$SUB_ID" "404" "$OTHER_TOKEN" "$LOG_DIR/topic-subscription-other-tenant.json" '{"enabled":true}' "$OTHER_TENANT" || true
  curl_json "viewer cannot create topic subscription" "POST" "/api/v1/topics/subscriptions" "403" "$VIEWER_TOKEN" "$LOG_DIR/topic-subscription-viewer.json" "$subscription_body" || true

  assert_psql_count "topic subscription persisted in PostgreSQL" \
    "SELECT count(*) FROM topic_subscriptions WHERE tenant_id = '$TENANT' AND subscription_id = '$SUB_ID' AND enabled = false;" \
    "pg-topic-subscription-count.txt"
else
  json_log "assert" "created topic subscription id extracted" "blocker" false "missing" "create response did not return subscription_id" "topic-subscription-create.json"
fi

curl_json "admin can export topic report" "POST" "/api/v1/topics/reports/export" "202" "$ADMIN_TOKEN" "$LOG_DIR/topic-report-export.json" '{"topic":"apt","format":"json"}'
assert_json "topic report export response" "$LOG_DIR/topic-report-export.json" '.success == true and .data.export_type == "report" and (.data.result.file_key | contains("topics/apt"))'
REPORT_EXPORT_ID="$(jq -r '.data.export_id // ""' "$LOG_DIR/topic-report-export.json")"

curl_json "admin can export topic evidence package" "POST" "/api/v1/topics/evidence-packages/export" "202" "$ADMIN_TOKEN" "$LOG_DIR/topic-evidence-package-export.json" '{"topic":"exfil","format":"json"}'
assert_json "topic evidence package export response" "$LOG_DIR/topic-evidence-package-export.json" '.success == true and .data.export_type == "evidence_package" and (.data.result.file_key | contains("topics/exfil"))'
EVIDENCE_EXPORT_ID="$(jq -r '.data.export_id // ""' "$LOG_DIR/topic-evidence-package-export.json")"

curl_json "viewer cannot export topic report" "POST" "/api/v1/topics/reports/export" "403" "$VIEWER_TOKEN" "$LOG_DIR/topic-report-export-viewer.json" '{"topic":"apt","format":"json"}' || true

curl_json "topic saved views list includes created view" "GET" "/api/v1/topics/views?topic=tunnel&limit=20" "200" "$ADMIN_TOKEN" "$LOG_DIR/topic-views-after.json"
if [[ -n "${VIEW_ID:-}" ]]; then
  assert_json "created topic view appears in list" "$LOG_DIR/topic-views-after.json" \
    --arg view_id "$VIEW_ID" '.success == true and ([.data.views[] | select(.view_id == $view_id)] | length) == 1'
fi

curl_json "topic subscriptions list includes created subscription" "GET" "/api/v1/topics/subscriptions?topic=exfil&limit=20" "200" "$ADMIN_TOKEN" "$LOG_DIR/topic-subscriptions-after.json"
if [[ -n "${SUB_ID:-}" ]]; then
  assert_json "created topic subscription appears in list" "$LOG_DIR/topic-subscriptions-after.json" \
    --arg subscription_id "$SUB_ID" '.success == true and ([.data.subscriptions[] | select(.subscription_id == $subscription_id)] | length) == 1'
fi

assert_psql_count "topic scope persisted in PostgreSQL" \
  "SELECT count(*) FROM topic_scope_overrides WHERE tenant_id = '$TENANT' AND topic = 'tunnel' AND scope_name = '$SCOPE_NAME';" \
  "pg-topic-scope-count.txt"

if [[ -n "${REPORT_EXPORT_ID:-}" ]]; then
  assert_psql_count "topic report export persisted in PostgreSQL" \
    "SELECT count(*) FROM topic_exports WHERE tenant_id = '$TENANT' AND export_id = '$REPORT_EXPORT_ID' AND export_type = 'report';" \
    "pg-topic-report-export-count.txt"
else
  json_log "assert" "topic report export id extracted" "blocker" false "missing" "export response did not return export_id" "topic-report-export.json"
fi

if [[ -n "${EVIDENCE_EXPORT_ID:-}" ]]; then
  assert_psql_count "topic evidence package export persisted in PostgreSQL" \
    "SELECT count(*) FROM topic_exports WHERE tenant_id = '$TENANT' AND export_id = '$EVIDENCE_EXPORT_ID' AND export_type = 'evidence_package';" \
    "pg-topic-evidence-export-count.txt"
else
  json_log "assert" "topic evidence package export id extracted" "blocker" false "missing" "export response did not return export_id" "topic-evidence-package-export.json"
fi

assert_psql_count "topic governance audit persisted" \
  "SELECT count(*) FROM audit_logs WHERE tenant_id = '$TENANT' AND action IN ('TOPIC_VIEW_SAVED','TOPIC_VIEW_UPDATED','TOPIC_VIEW_SHARED','TOPIC_VIEW_FAVORITE_UPDATED','TOPIC_SCOPE_UPDATED','TOPIC_SUBSCRIPTION_CREATED','TOPIC_SUBSCRIPTION_UPDATED','TOPIC_REPORT_EXPORTED','TOPIC_EVIDENCE_PACKAGE_EXPORTED');" \
  "pg-topic-audit-count.txt"

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
  --arg view_id "${VIEW_ID:-}" \
  --arg subscription_id "${SUB_ID:-}" \
  --arg report_export_id "${REPORT_EXPORT_ID:-}" \
  --arg evidence_export_id "${EVIDENCE_EXPORT_ID:-}" \
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
    view_id:$view_id,
    subscription_id:$subscription_id,
    report_export_id:$report_export_id,
    evidence_export_id:$evidence_export_id,
    report:$report,
    local_report:$local_report,
    total:$total,
    passed:$passed,
    blockers:$blockers,
    warnings:$warnings,
    checks:$checks
  }' >"$SUMMARY"

cat >"$LOCAL_REPORT" <<MD
# Topic Governance Live Preflight

- Run: \`$RUN_ID\`
- Result: \`$RESULT\`
- Checks: \`$PASSED/$TOTAL\` passed, \`$BLOCKERS\` blockers, \`$WARNINGS\` warnings
- Saved view: \`${VIEW_ID:-missing}\`
- Subscription: \`${SUB_ID:-missing}\`
- Exports: report \`${REPORT_EXPORT_ID:-missing}\`, evidence package \`${EVIDENCE_EXPORT_ID:-missing}\`

This gate closes the topic-panel governance loop:
readable tunnel/exfil/APT topic pages, saved view create/share/favorite,
topic scope update, subscription create/disable, report and evidence package
exports, viewer write denial, cross-tenant isolation, PostgreSQL persistence,
and audit-log queryability.
MD

cp "$SUMMARY" "$REGRESSION_DIR/topic-governance-preflight-latest.json"
cp "$LOCAL_REPORT" "$REGRESSION_DIR/topic-governance-preflight-latest.md"
cp "$LOG_DIR/topic-views-after.json" "$REGRESSION_DIR/topic-views-latest.json" 2>/dev/null || true
cp "$LOG_DIR/topic-subscriptions-after.json" "$REGRESSION_DIR/topic-subscriptions-latest.json" 2>/dev/null || true
cp "$LOG_DIR/topic-report-export.json" "$REGRESSION_DIR/topic-report-export-latest.json" 2>/dev/null || true
cp "$LOG_DIR/topic-evidence-package-export.json" "$REGRESSION_DIR/topic-evidence-package-export-latest.json" 2>/dev/null || true
cp "$LOG_DIR/topic-audit-view-saved.json" "$REGRESSION_DIR/topic-audit-latest.json" 2>/dev/null || true

echo "topic governance preflight result: $RESULT"
echo "summary: $SUMMARY"
echo "local report: $LOCAL_REPORT"

if [[ "$RESULT" == "blocked" && "$ALLOW_BLOCKERS" != "true" ]]; then
  exit 1
fi
