#!/usr/bin/env bash
set -euo pipefail

APISIX="${APISIX:-http://10.0.5.8:30180}"
WEB_URL="${WEB_URL:-$APISIX}"
TENANT="${TENANT:-default}"
LOG_DIR="${LOG_DIR:-doc/02_acceptance/runs/20260629-fusion-threat-intel}"
RUN_ID="${RUN_ID:-$(date +%Y%m%d%H%M%S)-fusion-threat-intel}"
REGRESSION_DIR="${REGRESSION_DIR:-doc/02_acceptance/02-regression}"
KUBECTL="${KUBECTL:-kubectl}"
WEB_NAMESPACE="${WEB_NAMESPACE:-traffic-analysis}"
WEB_DEPLOYMENT="${WEB_DEPLOYMENT:-web-ui}"
ALERT_DEPLOYMENT="${ALERT_DEPLOYMENT:-alert-service}"
WEB_IMAGE_EXPECTED="${WEB_IMAGE_EXPECTED:-}"
JWT_SECRET_NAMESPACE="${JWT_SECRET_NAMESPACE:-traffic-analysis}"
JWT_SECRET_NAME="${JWT_SECRET_NAME:-traffic-credentials}"
JWT_SECRET_KEY="${JWT_SECRET_KEY:-JWT_SECRET}"
PG_NAMESPACE="${PG_NAMESPACE:-databases}"
PG_POD="${PG_POD:-postgres-primary-0}"
PG_SECRET_NAMESPACE="${PG_SECRET_NAMESPACE:-traffic-analysis}"
PG_SECRET_NAME="${PG_SECRET_NAME:-traffic-credentials}"
PG_SECRET_KEY="${PG_SECRET_KEY:-PG_PASSWORD}"

REPORT="$LOG_DIR/live-fusion-threat-intel-$RUN_ID.ndjson"
SUMMARY="$LOG_DIR/live-fusion-threat-intel-$RUN_ID-summary.json"
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
    head -c 1200 "$file" | tr '\n' ' '
  fi
}

check_grep() {
  local name="$1" pattern="$2" path="$3"
  if grep -qE "$pattern" "$path"; then
    json_log "contract" "$name" "info" true "ok" "$path" "$path"
  else
    json_log "contract" "$name" "blocker" false "missing" "$pattern in $path" "$path"
  fi
}

make_jwt() {
  local username="${1:-codex-fusion-threat-intel}"
  local roles_json="${2:-[\"admin\"]}"
  local permissions_json="${3:-[\"*\", \"admin:*\", \"alert:read\", \"graph:read\", \"rule:write\"]}"
  local token_tenant="${4:-$TENANT}"
  JWT_SECRET="$JWT_SECRET" TENANT="$token_tenant" RUN_ID="$RUN_ID" USERNAME="$username" ROLES_JSON="$roles_json" PERMISSIONS_JSON="$permissions_json" python3 - <<'PY'
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
    "permissions": json.loads(os.environ["PERMISSIONS_JSON"]),
    "token_type": "access",
    "session_id": "codex-fusion-threat-intel-" + os.environ["RUN_ID"] + "-" + os.environ["USERNAME"],
    "iat": now,
    "exp": now + 1800,
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

psql_exec() {
  local sql="$1"
  kctl -n "$PG_NAMESPACE" exec "$PG_POD" -- env PGPASSWORD="$PG_PASSWORD" \
    psql -U postgres -d traffic_platform -v ON_ERROR_STOP=1 -Atc "$sql"
}

curl_json() {
  local name="$1" path="$2" output="$3"
  local err_file code rc
  err_file="$output.err"
  set +e
  code="$(curl --noproxy '*' -sS -m 20 -o "$output" -w '%{http_code}' \
    -H "Authorization: Bearer $ADMIN_TOKEN" \
    -H "X-Tenant-ID: $TENANT" \
    "$APISIX$path" 2>"$err_file")"
  rc=$?
  set -e
  if [[ "$rc" -ne 0 ]]; then
    json_log "api" "$name" "blocker" false "curl-rc=$rc" "$(trim_file "$err_file")" "$(basename "$err_file")"
    return 1
  fi
  if [[ "$code" != 2* ]]; then
    json_log "api" "$name" "blocker" false "$code" "$(trim_file "$output")" "$(basename "$output")"
    return 1
  fi
  json_log "api" "$name" "info" true "$code" "$path" "$(basename "$output")"
  return 0
}

curl_json_body() {
  local name="$1" method="$2" path="$3" output="$4" body_file="$5" token="${6:-$ADMIN_TOKEN}" expected="${7:-2}"
  local request_tenant="${8:-$TENANT}"
  local err_file code rc
  err_file="$output.err"
  set +e
  code="$(curl --noproxy '*' -sS -m 20 -o "$output" -w '%{http_code}' \
    -X "$method" \
    -H 'Content-Type: application/json' \
    -H "Authorization: Bearer $token" \
    -H "X-Tenant-ID: $request_tenant" \
    --data-binary "@$body_file" \
    "$APISIX$path" 2>"$err_file")"
  rc=$?
  set -e
  if [[ "$rc" -ne 0 ]]; then
    json_log "api" "$name" "blocker" false "curl-rc=$rc" "$(trim_file "$err_file")" "$(basename "$err_file")"
    return 1
  fi
  if [[ "$expected" == "2" && "$code" == 2* ]] || [[ "$code" == "$expected" ]]; then
    json_log "api" "$name" "info" true "$code" "$path" "$(basename "$output")"
    return 0
  fi
  json_log "api" "$name" "blocker" false "$code" "$(trim_file "$output")" "$(basename "$output")"
  return 1
}

TEST_TENANT=""
FIXTURES_READY=false
cleanup_fixtures() {
  if [[ "$FIXTURES_READY" == true && -n "$TEST_TENANT" && -n "${PG_PASSWORD:-}" ]]; then
    psql_exec "DELETE FROM tenants WHERE tenant_id = '$TEST_TENANT';" >/dev/null 2>&1 || true
  fi
}
trap cleanup_fixtures EXIT

curl_text() {
  local name="$1" path="$2" output="$3"
  local err_file code rc
  err_file="$output.err"
  set +e
  code="$(curl --noproxy '*' -sS -m 20 -o "$output" -w '%{http_code}' "$WEB_URL$path" 2>"$err_file")"
  rc=$?
  set -e
  if [[ "$rc" -ne 0 ]]; then
    json_log "web" "$name" "blocker" false "curl-rc=$rc" "$(trim_file "$err_file")" "$(basename "$err_file")"
    return 1
  fi
  if [[ "$code" != 2* ]]; then
    json_log "web" "$name" "blocker" false "$code" "$(trim_file "$output")" "$(basename "$output")"
    return 1
  fi
  json_log "web" "$name" "info" true "$code" "$path" "$(basename "$output")"
  return 0
}

assert_json() {
  local name="$1" file="$2"
  shift 2
  if jq -e "$@" "$file" >/dev/null 2>&1; then
    json_log "assert" "$name" "info" true "ok" "" "$(basename "$file")"
  else
    json_log "assert" "$name" "blocker" false "failed" "$(trim_file "$file")" "$(basename "$file")"
  fi
}

need_cmd git
need_cmd jq
need_cmd python3
need_cmd curl
need_cmd grep
need_cmd npm
need_cmd "$KUBECTL"

git rev-parse HEAD >"$LOG_DIR/commit-sha.txt"
git branch --show-current >"$LOG_DIR/git-branch.txt"
git status --short >"$LOG_DIR/git-status.txt"
git diff --stat >"$LOG_DIR/git-diff-stat.txt" || true

check_grep "fusion page plan includes threat intel endpoint" "threat-intel/entries" "web/ui/src/services/pageApiPlans.ts"
check_grep "fusion route manifest advertises threat intel API" "threat-intel/entries" "web/ui/src/routes/routeManifest.tsx"
check_grep "fusion adapter maps threat intel entries" "Threat Intel API" "web/ui/src/services/pageSnapshotAdapters.ts"
check_grep "fusion workbench renders threat intel live evidence" "threatIntelEvidence" "web/ui/src/pages/FusionWorkbenchPage.tsx"
check_grep "fusion API plan registers conflict write" "fusion-conflict-resolve" "web/ui/src/services/pageApiPlans.ts"
check_grep "fusion workbench calls conflict write API" "resolveFusionConflict" "web/ui/src/pages/FusionWorkbenchPage.tsx"
check_grep "fusion workbench calls rule write API" "updateFusionRule" "web/ui/src/pages/FusionWorkbenchPage.tsx"
check_grep "canonical PostgreSQL schema declares fusion rule constraints" "fusion_rule_overrides_threshold_check" "common/sql/pg/00-init.sql"
check_grep "Kubernetes PostgreSQL init declares fusion rule constraints" "fusion_rule_overrides_threshold_check" "deployments/kubernetes/init-jobs/02-postgres-schema.yaml"
check_grep "merged PostgreSQL init declares fusion rule constraints" "fusion_rule_overrides_threshold_check" "go/control-plane/deployments/docker/init/postgres_merged.sql"

ICON_WORKFLOW_LOG="$LOG_DIR/fusion-ui-icon-workflow.json"
if node tests/e2e/fusion_ui_icon_workflow_check.mjs >"$ICON_WORKFLOW_LOG" 2>&1; then
  json_log "asset" "fusion icon candidates require visual approval before formal copy" "info" true "pass" "rejected candidates are quarantined; production uses reviewed code-native icons" "$(basename "$ICON_WORKFLOW_LOG")"
else
  json_log "asset" "fusion icon candidates require visual approval before formal copy" "blocker" false "failed" "$(trim_file "$ICON_WORKFLOW_LOG")" "$(basename "$ICON_WORKFLOW_LOG")"
fi

VITEST_LOG="$LOG_DIR/web-vitest-fusion-threat-intel.log"
if (cd web/ui && npm run test -- --run src/services/pageSnapshotAdapters.test.ts src/services/pageApiPlans.test.ts src/routes/routeManifest.test.ts) >"$VITEST_LOG" 2>&1; then
  json_log "web" "fusion threat intel Vitest contract" "info" true "pass" "pageSnapshotAdapters/pageApiPlans/routeManifest" "$(basename "$VITEST_LOG")"
else
  json_log "web" "fusion threat intel Vitest contract" "blocker" false "failed" "$(trim_file "$VITEST_LOG")" "$(basename "$VITEST_LOG")"
fi

JWT_SECRET="$(kctl -n "$JWT_SECRET_NAMESPACE" get secret "$JWT_SECRET_NAME" -o "jsonpath={.data.$JWT_SECRET_KEY}" | base64 -d)"
PG_PASSWORD="$(kctl -n "$PG_SECRET_NAMESPACE" get secret "$PG_SECRET_NAME" -o "jsonpath={.data.$PG_SECRET_KEY}" | base64 -d)"
ADMIN_TOKEN="$(make_jwt)"
READER_TOKEN="$(make_jwt "codex-fusion-threat-intel-reader" '["viewer"]' '["alert:read","graph:read","rule:read"]')"
PG_RULE_CONSTRAINTS_JSON="$LOG_DIR/pg-fusion-rule-constraints.json"
if psql_exec "SELECT json_object_agg(conname, pg_get_constraintdef(oid))::text FROM pg_constraint WHERE conrelid='fusion_rule_overrides'::regclass AND conname IN ('fusion_rule_overrides_status_check','fusion_rule_overrides_strategy_check','fusion_rule_overrides_threshold_check');" >"$PG_RULE_CONSTRAINTS_JSON" 2>"$PG_RULE_CONSTRAINTS_JSON.err"; then
  assert_json "fusion rule database constraints are present and canonical" "$PG_RULE_CONSTRAINTS_JSON" '.fusion_rule_overrides_status_check | contains("active") and contains("draft") and contains("disabled")'
  assert_json "fusion rule strategy database constraint is canonical" "$PG_RULE_CONSTRAINTS_JSON" '.fusion_rule_overrides_strategy_check | contains("authoritative-source") and contains("weighted-confidence") and contains("latest-observation") and contains("manual-review")'
  assert_json "fusion rule threshold database constraint is canonical" "$PG_RULE_CONSTRAINTS_JSON" '.fusion_rule_overrides_threshold_check | contains("confidence_threshold") and contains(">=") and contains("<=")'
else
  json_log "postgres" "fusion rule database constraints are present and canonical" "blocker" false "query-failed" "$(trim_file "$PG_RULE_CONSTRAINTS_JSON.err")" "$(basename "$PG_RULE_CONSTRAINTS_JSON.err")"
fi
RUN_ID_SAFE="$(tr '[:upper:]_' '[:lower:]-' <<<"$RUN_ID" | sed -E 's/[^a-z0-9-]+/-/g; s/^-+|-+$//g')"
TEST_TENANT="codex-fusion-${RUN_ID_SAFE:0:40}"
psql_exec "INSERT INTO tenants (tenant_id, tenant_name, name, status) VALUES ('$TEST_TENANT','Fusion isolated contract $RUN_ID_SAFE','Fusion isolated contract $RUN_ID_SAFE','active');" >/dev/null
FIXTURES_READY=true
TEST_ADMIN_TOKEN="$(make_jwt "codex-fusion-contract-admin" '["admin"]' '["*","admin:*","rule:write"]' "$TEST_TENANT")"
TEST_READER_TOKEN="$(make_jwt "codex-fusion-contract-reader" '["viewer"]' '["alert:read","graph:read","rule:read"]' "$TEST_TENANT")"

FUSION_STATS="$LOG_DIR/api-fusion-stats.json"
if curl_json "fusion stats through APISIX" "/api/v1/fusion/stats" "$FUSION_STATS"; then
  assert_json "fusion stats include threat_intel source" "$FUSION_STATS" '.success == true and (.data.data_source_stats.threat_intel.count // 0) >= 1'
fi

FUSION_WORKBENCH="$LOG_DIR/api-fusion-workbench.json"
FUSION_WORKBENCH_REPEAT="$LOG_DIR/api-fusion-workbench-repeat.json"
FUSION_WORKBENCH_PAGE1="$LOG_DIR/api-fusion-workbench-page1.json"
FUSION_WORKBENCH_PAGE2="$LOG_DIR/api-fusion-workbench-page2.json"
PG_WORKBENCH_BEFORE="$LOG_DIR/pg-fusion-readonly-before.txt"
PG_WORKBENCH_AFTER="$LOG_DIR/pg-fusion-readonly-after.txt"
psql_exec "SELECT
  (SELECT count(*) FROM fusion_rule_overrides WHERE tenant_id='$TENANT'),
  (SELECT count(*) FROM fusion_conflicts WHERE tenant_id='$TENANT'),
  (SELECT count(*) FROM audit_logs WHERE tenant_id='$TENANT' AND action LIKE 'FUSION_%');" >"$PG_WORKBENCH_BEFORE"
if curl_json "fusion workbench through APISIX" "/api/v1/fusion/workbench" "$FUSION_WORKBENCH"; then
  assert_json "fusion workbench has six live sources" "$FUSION_WORKBENCH" '.success == true and (.data.sources | length) == 6'
  assert_json "fusion sources disclose truthful threat intel and vulnerability storage" "$FUSION_WORKBENCH" '(.data.sources | map({key:.source_id,value:(.config.storage // "")}) | from_entries) as $storage | $storage.threat_intel == "postgres.threat_intel" and $storage.vulnerability == "postgres.assets.metadata.vulnerabilities"'
  assert_json "fusion source coverage never fabricates an unmeasured percentage" "$FUSION_WORKBENCH" '[.data.sources[].field_coverage] | all(. == null or (. >= 0 and . <= 1))'
  assert_json "fusion source error rate never fabricates an unmeasured zero" "$FUSION_WORKBENCH" '[.data.sources[].error_rate] | all(. == null or (. >= 0 and . <= 1))'
  assert_json "fusion workbench exposes canonical rule pipeline from persisted rules" "$FUSION_WORKBENCH" '(.data.pipeline_rules | map(.rule_id)) as $ids | ["IP_MAC_BIND_V3","ACCOUNT_HOST_LINK","ASSET_DEPT_COMPLETION","DOMAIN_IP_RESOLUTION","ALERT_ASSET_JOIN","VULN_SERVICE_MATCH"] | all(. as $id | $ids | index($id))'
  assert_json "fusion conflict totals match persisted state summary" "$FUSION_WORKBENCH" '.data.pending_count + .data.resolved_count == .data.conflict_total and ([.data.conflicts[] | select(.status == "resolved")] | length) == 0'
  assert_json "fusion audit response is bounded and contains no synthetic baseline events" "$FUSION_WORKBENCH" '(.data.audit_events | length) <= 50 and ([.data.audit_events[] | select(.details.baseline == "fusion-workbench-v1")] | length) == 0'
fi
if curl_json "fusion workbench first server page" "/api/v1/fusion/workbench?rule_limit=6&rule_offset=0&conflict_limit=3&conflict_offset=0&audit_limit=5&audit_offset=0" "$FUSION_WORKBENCH_PAGE1"; then
  assert_json "fusion workbench first page exposes totals and bounded rows" "$FUSION_WORKBENCH_PAGE1" '.success == true and .data.rule_total >= (.data.rules | length) and (.data.rules | length) <= 6 and .data.rule_limit == 6 and .data.rule_offset == 0 and .data.conflict_total >= (.data.conflicts | length) and (.data.conflicts | length) <= 3 and .data.conflict_limit == 3 and .data.conflict_offset == 0 and .data.audit_total >= (.data.audit_events | length) and (.data.audit_events | length) <= 5 and .data.audit_limit == 5 and .data.audit_offset == 0 and (.data.pending_risk_counts.high + .data.pending_risk_counts.medium + .data.pending_risk_counts.low) == .data.pending_count'
fi
if curl_json "fusion workbench second server page" "/api/v1/fusion/workbench?rule_limit=6&rule_offset=6&conflict_limit=3&conflict_offset=3&audit_limit=5&audit_offset=5" "$FUSION_WORKBENCH_PAGE2"; then
  assert_json "fusion workbench second page uses requested offsets" "$FUSION_WORKBENCH_PAGE2" '.success == true and .data.rule_limit == 6 and .data.rule_offset == 6 and .data.conflict_limit == 3 and .data.conflict_offset == 3 and .data.audit_limit == 5 and .data.audit_offset == 5'
  if jq -e --slurpfile first "$FUSION_WORKBENCH_PAGE1" '([.data.rules[].rule_id] as $second | [$first[0].data.rules[].rule_id] as $firstIDs | [$second[] | select(. as $id | $firstIDs | index($id))] | length) == 0' "$FUSION_WORKBENCH_PAGE2" >/dev/null; then
    json_log "api" "fusion rule server pages do not repeat rows" "info" true "disjoint" "offset page IDs differ from first page" "$(basename "$FUSION_WORKBENCH_PAGE2")"
  else
    json_log "api" "fusion rule server pages do not repeat rows" "blocker" false "overlap" "offset page repeated one or more rule IDs" "$(basename "$FUSION_WORKBENCH_PAGE2")"
  fi
  if jq -e --slurpfile first "$FUSION_WORKBENCH_PAGE1" '([.data.conflicts[].conflict_id] as $second | [$first[0].data.conflicts[].conflict_id] as $firstIDs | [$second[] | select(. as $id | $firstIDs | index($id))] | length) == 0' "$FUSION_WORKBENCH_PAGE2" >/dev/null; then
    json_log "api" "fusion conflict server pages do not repeat rows" "info" true "disjoint" "offset page IDs differ from first page" "$(basename "$FUSION_WORKBENCH_PAGE2")"
  else
    json_log "api" "fusion conflict server pages do not repeat rows" "blocker" false "overlap" "offset page repeated one or more conflict IDs" "$(basename "$FUSION_WORKBENCH_PAGE2")"
  fi
fi
curl_json "repeat fusion workbench read through APISIX" "/api/v1/fusion/workbench" "$FUSION_WORKBENCH_REPEAT" || true
psql_exec "SELECT
  (SELECT count(*) FROM fusion_rule_overrides WHERE tenant_id='$TENANT'),
  (SELECT count(*) FROM fusion_conflicts WHERE tenant_id='$TENANT'),
  (SELECT count(*) FROM audit_logs WHERE tenant_id='$TENANT' AND action LIKE 'FUSION_%');" >"$PG_WORKBENCH_AFTER"
if cmp -s "$PG_WORKBENCH_BEFORE" "$PG_WORKBENCH_AFTER"; then
  json_log "postgres" "fusion workbench GET is read only" "info" true "unchanged" "rules, conflicts and audit counts are stable across repeated GET" "$(basename "$PG_WORKBENCH_AFTER")"
else
  json_log "postgres" "fusion workbench GET is read only" "blocker" false "mutated" "before=$(tr '\n' ' ' <"$PG_WORKBENCH_BEFORE") after=$(tr '\n' ' ' <"$PG_WORKBENCH_AFTER")" "$(basename "$PG_WORKBENCH_AFTER")"
fi

FUSION_ENTITIES="$LOG_DIR/api-fusion-entities.json"
if curl_json "fusion entities through APISIX" "/api/v1/fusion/entities?limit=3" "$FUSION_ENTITIES"; then
  assert_json "fusion entities returns aligned entities" "$FUSION_ENTITIES" '.success == true and ((.data.entities // .data.data.entities // []) | length) >= 1'
fi

THREAT_ENTRIES="$LOG_DIR/api-threat-intel-entries.json"
if curl_json "threat intel entries through APISIX" "/api/v1/threat-intel/entries?limit=5" "$THREAT_ENTRIES"; then
  assert_json "threat intel entries returns list" "$THREAT_ENTRIES" '.success == true and (.data | type) == "array" and (.data | length) >= 1'
fi

SOURCE_SYNC_BODY="$LOG_DIR/api-fusion-source-sync-body.json"
printf '{}\n' >"$SOURCE_SYNC_BODY"
READER_SOURCE_SYNC_JSON="$LOG_DIR/api-fusion-source-sync-reader-denied.json"
curl_json_body "read-only token cannot request fusion source sync" "POST" "/api/v1/fusion/sources/traffic/sync" "$READER_SOURCE_SYNC_JSON" "$SOURCE_SYNC_BODY" "$TEST_READER_TOKEN" "403" "$TEST_TENANT" || true

CONFLICT_ID="codex-fusion-conflict-${RUN_ID_SAFE}"
RULE_ID="codex-fusion-rule-${RUN_ID_SAFE}"

psql_exec "INSERT INTO fusion_rule_overrides
  (tenant_id, rule_id, rule_name, version, status, strategy, confidence_threshold, note, updated_by, detail)
  VALUES ('$TEST_TENANT','$RULE_ID','Codex Fusion Threat Intel Rule',1,'active','authoritative-source',0.91,'isolated live contract fixture','codex-live','{\"source_a\":\"Threat Intel\",\"source_b\":\"Fusion\",\"field\":\"C2 情报命中\"}'::jsonb)
  ON CONFLICT (tenant_id, rule_id) DO NOTHING;" >/dev/null
psql_exec "INSERT INTO fusion_conflicts
  (tenant_id, conflict_id, object_id, object_type, field_name, source_values, source_count, confidence, severity, status, rule_id, state_version)
  VALUES ('$TEST_TENANT','$CONFLICT_ID','codex-fusion-object-${RUN_ID_SAFE}','threat_intel','C2 情报命中','[{\"source\":\"Threat Intel 威胁情报\",\"value\":\"185.130.5.253\",\"confidence\":0.92},{\"source\":\"Fusion 规则\",\"value\":\"C2-candidate\",\"confidence\":0.71}]'::jsonb,2,0.92,'high','pending','$RULE_ID',1)
  ON CONFLICT (tenant_id, conflict_id) DO NOTHING;" >/dev/null

CONFLICT_BODY="$LOG_DIR/api-fusion-conflict-resolve-body.json"
jq -nc \
  '{object_id:"client-forged-object", object_type:"client-forged-type", field_name:"client-forged-field", selected_source:"Threat Intel 威胁情报", selected_value:"185.130.5.253", strategy:"manual-repair-task", note:"Codex live Fusion repair task", rule_id:"client-forged-rule", expected_state_version:1, detail:{client_forged:true}}' >"$CONFLICT_BODY"

READER_CONFLICT_JSON="$LOG_DIR/api-fusion-conflict-reader-denied.json"
curl_json_body "read-only token cannot resolve fusion conflict" "POST" "/api/v1/fusion/conflicts/$CONFLICT_ID/resolve" "$READER_CONFLICT_JSON" "$CONFLICT_BODY" "$TEST_READER_TOKEN" "403" "$TEST_TENANT" || true

INVALID_SOURCE_BODY="$LOG_DIR/api-fusion-conflict-invalid-source-body.json"
jq '.selected_value = "185.130.5.253::client-forged"' "$CONFLICT_BODY" >"$INVALID_SOURCE_BODY"
INVALID_SOURCE_JSON="$LOG_DIR/api-fusion-conflict-invalid-source.json"
if curl_json_body "fusion conflict rejects a source value not stored in canonical facts" "POST" "/api/v1/fusion/conflicts/$CONFLICT_ID/resolve" "$INVALID_SOURCE_JSON" "$INVALID_SOURCE_BODY" "$TEST_ADMIN_TOKEN" "400" "$TEST_TENANT"; then
  assert_json "invalid fusion source value returns explicit code" "$INVALID_SOURCE_JSON" '.success == false and .error.code == "INVALID_SOURCE_VALUE"'
fi

INVALID_STRATEGY_BODY="$LOG_DIR/api-fusion-conflict-invalid-strategy-body.json"
jq '.strategy = "client-invented-strategy"' "$CONFLICT_BODY" >"$INVALID_STRATEGY_BODY"
INVALID_STRATEGY_JSON="$LOG_DIR/api-fusion-conflict-invalid-strategy.json"
if curl_json_body "fusion conflict rejects an unsupported strategy" "POST" "/api/v1/fusion/conflicts/$CONFLICT_ID/resolve" "$INVALID_STRATEGY_JSON" "$INVALID_STRATEGY_BODY" "$TEST_ADMIN_TOKEN" "400" "$TEST_TENANT"; then
  assert_json "invalid fusion strategy returns explicit code" "$INVALID_STRATEGY_JSON" '.success == false and .error.code == "INVALID_REQUEST"'
fi

CONFLICT_JSON="$LOG_DIR/api-fusion-conflict-resolve.json"
if curl_json_body "resolve fusion conflict through APISIX" "POST" "/api/v1/fusion/conflicts/$CONFLICT_ID/resolve" "$CONFLICT_JSON" "$CONFLICT_BODY" "$TEST_ADMIN_TOKEN" "2" "$TEST_TENANT"; then
  assert_json "fusion conflict resolution returns canonical facts, real repair task and audit flag" "$CONFLICT_JSON" --arg id "$CONFLICT_ID" --arg object_id "codex-fusion-object-${RUN_ID_SAFE}" --arg rule_id "$RULE_ID" '.success == true and .data.resolution.conflict_id == $id and .data.resolution.object_id == $object_id and .data.resolution.object_type == "threat_intel" and .data.resolution.field_name == "C2 情报命中" and .data.resolution.rule_id == $rule_id and (.data.resolution.detail.client_forged // false) == false and (.data.resolution.state_version // 0) >= 1 and .data.audit_written == true and .data.repair_task.conflict_id == $id and .data.repair_task.task_type == "fusion_conflict_repair" and (.data.repair_task.task_id | length) == 36'
fi
STALE_CONFLICT_JSON="$LOG_DIR/api-fusion-conflict-stale-version.json"
curl_json_body "stale fusion conflict state is rejected" "POST" "/api/v1/fusion/conflicts/$CONFLICT_ID/resolve" "$STALE_CONFLICT_JSON" "$CONFLICT_BODY" "$TEST_ADMIN_TOKEN" "409" "$TEST_TENANT" || true
REPEAT_REPAIR_BODY="$LOG_DIR/api-fusion-conflict-repeat-repair-body.json"
jq '.expected_state_version = 2' "$CONFLICT_BODY" >"$REPEAT_REPAIR_BODY"
REPEAT_REPAIR_JSON="$LOG_DIR/api-fusion-conflict-repeat-repair.json"
if curl_json_body "repair-pending fusion conflict rejects a duplicate repair task" "POST" "/api/v1/fusion/conflicts/$CONFLICT_ID/resolve" "$REPEAT_REPAIR_JSON" "$REPEAT_REPAIR_BODY" "$TEST_ADMIN_TOKEN" "409" "$TEST_TENANT"; then
  assert_json "duplicate fusion repair task returns explicit idempotency code" "$REPEAT_REPAIR_JSON" '.success == false and .error.code == "REPAIR_TASK_EXISTS"'
fi
PENDING_RESOLVE_BODY="$LOG_DIR/api-fusion-conflict-pending-resolve-body.json"
jq '.expected_state_version = 2 | .strategy = "authoritative-source"' "$CONFLICT_BODY" >"$PENDING_RESOLVE_BODY"
PENDING_RESOLVE_JSON="$LOG_DIR/api-fusion-conflict-pending-resolve.json"
if curl_json_body "repair-pending fusion conflict rejects another resolution strategy" "POST" "/api/v1/fusion/conflicts/$CONFLICT_ID/resolve" "$PENDING_RESOLVE_JSON" "$PENDING_RESOLVE_BODY" "$TEST_ADMIN_TOKEN" "409" "$TEST_TENANT"; then
  assert_json "repair-pending fusion conflict returns explicit lock code" "$PENDING_RESOLVE_JSON" '.success == false and .error.code == "CONFLICT_REPAIR_PENDING"'
fi

RULE_BODY="$LOG_DIR/api-fusion-rule-update-body.json"
jq -nc \
  '{rule_name:"CLIENT_FORGED_RULE_NAME", status:"draft", strategy:"authoritative-source", confidence_threshold:0.91, note:"Codex live Fusion rule edit", expected_version:1, detail:{client_forged:true, source_a:"forged", source_b:"forged", field:"forged"}}' >"$RULE_BODY"

RULE_JSON="$LOG_DIR/api-fusion-rule-update.json"
READER_RULE_JSON="$LOG_DIR/api-fusion-rule-reader-denied.json"
curl_json_body "read-only token cannot update fusion rule" "PATCH" "/api/v1/fusion/rules/$RULE_ID" "$READER_RULE_JSON" "$RULE_BODY" "$TEST_READER_TOKEN" "403" "$TEST_TENANT" || true
INVALID_RULE_STATUS_BODY="$LOG_DIR/api-fusion-rule-invalid-status-body.json"
jq '.status = "client-forged-status"' "$RULE_BODY" >"$INVALID_RULE_STATUS_BODY"
INVALID_RULE_STATUS_JSON="$LOG_DIR/api-fusion-rule-invalid-status.json"
if curl_json_body "fusion rule rejects an unsupported status" "PATCH" "/api/v1/fusion/rules/$RULE_ID" "$INVALID_RULE_STATUS_JSON" "$INVALID_RULE_STATUS_BODY" "$TEST_ADMIN_TOKEN" "400" "$TEST_TENANT"; then
  assert_json "invalid fusion rule status returns explicit code" "$INVALID_RULE_STATUS_JSON" '.success == false and .error.code == "INVALID_RULE_STATUS"'
fi
INVALID_RULE_STRATEGY_BODY="$LOG_DIR/api-fusion-rule-invalid-strategy-body.json"
jq '.strategy = "client-forged-strategy"' "$RULE_BODY" >"$INVALID_RULE_STRATEGY_BODY"
INVALID_RULE_STRATEGY_JSON="$LOG_DIR/api-fusion-rule-invalid-strategy.json"
if curl_json_body "fusion rule rejects an unsupported strategy" "PATCH" "/api/v1/fusion/rules/$RULE_ID" "$INVALID_RULE_STRATEGY_JSON" "$INVALID_RULE_STRATEGY_BODY" "$TEST_ADMIN_TOKEN" "400" "$TEST_TENANT"; then
  assert_json "invalid fusion rule strategy returns explicit code" "$INVALID_RULE_STRATEGY_JSON" '.success == false and .error.code == "INVALID_RULE_STRATEGY"'
fi
INVALID_RULE_THRESHOLD_BODY="$LOG_DIR/api-fusion-rule-invalid-threshold-body.json"
jq '.confidence_threshold = 1.4' "$RULE_BODY" >"$INVALID_RULE_THRESHOLD_BODY"
INVALID_RULE_THRESHOLD_JSON="$LOG_DIR/api-fusion-rule-invalid-threshold.json"
if curl_json_body "fusion rule rejects an out-of-range threshold" "PATCH" "/api/v1/fusion/rules/$RULE_ID" "$INVALID_RULE_THRESHOLD_JSON" "$INVALID_RULE_THRESHOLD_BODY" "$TEST_ADMIN_TOKEN" "400" "$TEST_TENANT"; then
  assert_json "invalid fusion rule threshold returns explicit code" "$INVALID_RULE_THRESHOLD_JSON" '.success == false and .error.code == "INVALID_RULE_THRESHOLD"'
fi
if curl_json_body "update fusion rule through APISIX" "PATCH" "/api/v1/fusion/rules/$RULE_ID" "$RULE_JSON" "$RULE_BODY" "$TEST_ADMIN_TOKEN" "2" "$TEST_TENANT"; then
  assert_json "fusion rule update preserves canonical name and detail" "$RULE_JSON" --arg id "$RULE_ID" '.success == true and .data.rule.rule_id == $id and .data.rule.rule_name == "Codex Fusion Threat Intel Rule" and .data.rule.detail.source_a == "Threat Intel" and .data.rule.detail.source_b == "Fusion" and .data.rule.detail.field == "C2 情报命中" and (.data.rule.detail.client_forged // false) == false and (.data.rule.version // 0) >= 1 and .data.audit_written == true'
fi
STALE_RULE_JSON="$LOG_DIR/api-fusion-rule-stale-version.json"
curl_json_body "stale fusion rule version is rejected" "PATCH" "/api/v1/fusion/rules/$RULE_ID" "$STALE_RULE_JSON" "$RULE_BODY" "$TEST_ADMIN_TOKEN" "409" "$TEST_TENANT" || true

EVIDENCE_BODY="$LOG_DIR/api-fusion-evidence-body.json"
jq -nc --arg conflict_id "$CONFLICT_ID" '{conflict_id:$conflict_id}' >"$EVIDENCE_BODY"
READER_EVIDENCE_JSON="$LOG_DIR/api-fusion-evidence-reader-denied.json"
curl_json_body "read-only token cannot export fusion evidence" "POST" "/api/v1/fusion/evidence-packages" "$READER_EVIDENCE_JSON" "$EVIDENCE_BODY" "$TEST_READER_TOKEN" "403" "$TEST_TENANT" || true
EVIDENCE_JSON="$LOG_DIR/api-fusion-evidence-export.json"
if curl_json_body "export fusion evidence through APISIX" "POST" "/api/v1/fusion/evidence-packages" "$EVIDENCE_JSON" "$EVIDENCE_BODY" "$TEST_ADMIN_TOKEN" "2" "$TEST_TENANT"; then
  assert_json "fusion evidence export returns complete evidence chain" "$EVIDENCE_JSON" --arg conflict_id "$CONFLICT_ID" --arg rule_id "$RULE_ID" '.success == true and (.data.sha256 | startswith("sha256:")) and (.data.content_base64 | length) > 0 and ((.data.content_base64 | @base64d | fromjson) as $doc | $doc.schema_version == 2 and $doc.conflict.conflict_id == $conflict_id and $doc.resolution.conflict_id == $conflict_id and $doc.resolution.selected_source == "Threat Intel 威胁情报" and $doc.resolution.selected_value == "185.130.5.253" and $doc.resolution.strategy == "manual-repair-task" and ($doc.resolution.resolved_by | length) > 0 and ($doc.resolution.resolved_at // 0) > 0 and ($doc.repair_tasks | length) == 1 and $doc.repair_tasks[0].conflict_id == $conflict_id and $doc.repair_tasks[0].status == "queued" and $doc.rule_snapshot.rule_id == $rule_id and ($doc.rule_snapshot.version // 0) >= 1 and ($doc.audit_events | map(select(.resource_id == $conflict_id and .action == "FUSION_CONFLICT_RESOLVED")) | length) >= 1)'
fi

PG_CONFLICT_TXT="$LOG_DIR/pg-fusion-conflict-resolution-count.txt"
PG_CONFLICT_JSON="$LOG_DIR/pg-fusion-conflict-resolution-count.json"
if psql_exec "SELECT count(*) FROM fusion_conflict_resolutions WHERE tenant_id = '$TEST_TENANT' AND conflict_id = '$CONFLICT_ID';" >"$PG_CONFLICT_TXT" 2>"$PG_CONFLICT_TXT.err"; then
  jq -Rn --arg count "$(tr -d '[:space:]' <"$PG_CONFLICT_TXT")" '{count:($count|tonumber)}' >"$PG_CONFLICT_JSON"
  assert_json "fusion conflict resolution row exists" "$PG_CONFLICT_JSON" '.count >= 1'
else
  json_log "postgres" "fusion conflict resolution row exists" "blocker" false "query-failed" "$(trim_file "$PG_CONFLICT_TXT.err")" "$(basename "$PG_CONFLICT_TXT.err")"
fi

PG_CANONICAL_JSON="$LOG_DIR/pg-fusion-conflict-resolution-canonical.json"
if psql_exec "SELECT json_build_object('object_id',object_id,'object_type',object_type,'field_name',field_name,'rule_id',rule_id,'detail',detail)::text FROM fusion_conflict_resolutions WHERE tenant_id = '$TEST_TENANT' AND conflict_id = '$CONFLICT_ID';" >"$PG_CANONICAL_JSON" 2>"$PG_CANONICAL_JSON.err"; then
  assert_json "fusion conflict resolution persisted server canonical facts" "$PG_CANONICAL_JSON" --arg object_id "codex-fusion-object-${RUN_ID_SAFE}" --arg rule_id "$RULE_ID" '.object_id == $object_id and .object_type == "threat_intel" and .field_name == "C2 情报命中" and .rule_id == $rule_id and (.detail.client_forged // false) == false'
else
  json_log "postgres" "fusion conflict resolution persisted server canonical facts" "blocker" false "query-failed" "$(trim_file "$PG_CANONICAL_JSON.err")" "$(basename "$PG_CANONICAL_JSON.err")"
fi

PG_RULE_TXT="$LOG_DIR/pg-fusion-rule-override-count.txt"
PG_RULE_JSON="$LOG_DIR/pg-fusion-rule-override-count.json"
if psql_exec "SELECT count(*) FROM fusion_rule_overrides WHERE tenant_id = '$TEST_TENANT' AND rule_id = '$RULE_ID';" >"$PG_RULE_TXT" 2>"$PG_RULE_TXT.err"; then
  jq -Rn --arg count "$(tr -d '[:space:]' <"$PG_RULE_TXT")" '{count:($count|tonumber)}' >"$PG_RULE_JSON"
  assert_json "fusion rule override row exists" "$PG_RULE_JSON" '.count >= 1'
else
  json_log "postgres" "fusion rule override row exists" "blocker" false "query-failed" "$(trim_file "$PG_RULE_TXT.err")" "$(basename "$PG_RULE_TXT.err")"
fi

PG_RULE_CANONICAL_JSON="$LOG_DIR/pg-fusion-rule-override-canonical.json"
if psql_exec "SELECT json_build_object('rule_name',rule_name,'status',status,'strategy',strategy,'detail',detail)::text FROM fusion_rule_overrides WHERE tenant_id = '$TEST_TENANT' AND rule_id = '$RULE_ID';" >"$PG_RULE_CANONICAL_JSON" 2>"$PG_RULE_CANONICAL_JSON.err"; then
  assert_json "fusion rule database row preserves canonical immutable fields" "$PG_RULE_CANONICAL_JSON" '.rule_name == "Codex Fusion Threat Intel Rule" and .status == "draft" and .strategy == "authoritative-source" and .detail.source_a == "Threat Intel" and .detail.source_b == "Fusion" and .detail.field == "C2 情报命中" and (.detail.client_forged // false) == false'
else
  json_log "postgres" "fusion rule database row preserves canonical immutable fields" "blocker" false "query-failed" "$(trim_file "$PG_RULE_CANONICAL_JSON.err")" "$(basename "$PG_RULE_CANONICAL_JSON.err")"
fi

PG_REPAIR_TASK_TXT="$LOG_DIR/pg-fusion-repair-task-count.txt"
PG_REPAIR_TASK_JSON="$LOG_DIR/pg-fusion-repair-task-count.json"
if psql_exec "SELECT count(*) FROM fusion_repair_tasks WHERE tenant_id = '$TEST_TENANT' AND conflict_id='$CONFLICT_ID' AND status='queued';" >"$PG_REPAIR_TASK_TXT" 2>"$PG_REPAIR_TASK_TXT.err"; then
  jq -Rn --arg count "$(tr -d '[:space:]' <"$PG_REPAIR_TASK_TXT")" '{count:($count|tonumber)}' >"$PG_REPAIR_TASK_JSON"
  assert_json "fusion repair task row exists" "$PG_REPAIR_TASK_JSON" '.count == 1'
else
  json_log "postgres" "fusion repair task row exists" "blocker" false "query-failed" "$(trim_file "$PG_REPAIR_TASK_TXT.err")" "$(basename "$PG_REPAIR_TASK_TXT.err")"
fi

PG_AUDIT_TXT="$LOG_DIR/pg-fusion-write-audit-count.txt"
PG_AUDIT_JSON="$LOG_DIR/pg-fusion-write-audit-count.json"
if psql_exec "SELECT count(*) FROM audit_logs WHERE tenant_id = '$TEST_TENANT' AND action IN ('FUSION_CONFLICT_RESOLVED','FUSION_RULE_UPDATED','FUSION_EVIDENCE_EXPORTED') AND object_id IN ('$CONFLICT_ID', '$RULE_ID');" >"$PG_AUDIT_TXT" 2>"$PG_AUDIT_TXT.err"; then
  jq -Rn --arg count "$(tr -d '[:space:]' <"$PG_AUDIT_TXT")" '{count:($count|tonumber)}' >"$PG_AUDIT_JSON"
  assert_json "fusion write audit rows exist" "$PG_AUDIT_JSON" '.count >= 3'
else
  json_log "postgres" "fusion write audit rows exist" "blocker" false "query-failed" "$(trim_file "$PG_AUDIT_TXT.err")" "$(basename "$PG_AUDIT_TXT.err")"
fi

ALERT_DEPLOY_JSON="$LOG_DIR/alert-service-deploy-live.json"
if kctl -n "$WEB_NAMESPACE" get deploy "$ALERT_DEPLOYMENT" -o json >"$ALERT_DEPLOY_JSON" 2>"$ALERT_DEPLOY_JSON.err"; then
  json_log "backend" "live alert-service deployment archived" "info" true "ok" "$(jq -r '.spec.template.spec.containers[0].image' "$ALERT_DEPLOY_JSON")" "$(basename "$ALERT_DEPLOY_JSON")"
else
  json_log "backend" "live alert-service deployment archived" "blocker" false "kubectl-failed" "$(trim_file "$ALERT_DEPLOY_JSON.err")" "$(basename "$ALERT_DEPLOY_JSON.err")"
fi

WEB_DEPLOY_JSON="$LOG_DIR/web-ui-deploy-live.json"
WEB_DEPLOY_IMAGE="$LOG_DIR/web-ui-deploy-image.txt"
if kctl -n "$WEB_NAMESPACE" get deploy "$WEB_DEPLOYMENT" -o json >"$WEB_DEPLOY_JSON" 2>"$WEB_DEPLOY_JSON.err"; then
  ACTUAL_WEB_IMAGE="$(jq -r '.spec.template.spec.containers[] | select(.name == "web-ui") | .image' "$WEB_DEPLOY_JSON")"
  echo "$ACTUAL_WEB_IMAGE" >"$WEB_DEPLOY_IMAGE"
  if [[ -z "$WEB_IMAGE_EXPECTED" || "$ACTUAL_WEB_IMAGE" == "$WEB_IMAGE_EXPECTED" ]]; then
    json_log "web" "live web-ui deployment image" "info" true "ok" "$ACTUAL_WEB_IMAGE" "$(basename "$WEB_DEPLOY_IMAGE")"
  else
    json_log "web" "live web-ui deployment image" "blocker" false "mismatch" "expected=$WEB_IMAGE_EXPECTED actual=$ACTUAL_WEB_IMAGE" "$(basename "$WEB_DEPLOY_IMAGE")"
  fi
else
  json_log "web" "live web-ui deployment image" "blocker" false "kubectl-failed" "$(trim_file "$WEB_DEPLOY_JSON.err")" "$(basename "$WEB_DEPLOY_JSON.err")"
fi

WEB_INDEX="$LOG_DIR/live-web-index.html"
if curl_text "live web index through APISIX" "/" "$WEB_INDEX"; then
  INDEX_ASSET="$(grep -o 'assets/index-[^" ]*\.js' "$WEB_INDEX" | head -n1 || true)"
  if [[ -z "$INDEX_ASSET" ]]; then
    json_log "web" "live web index asset discovery" "blocker" false "missing" "assets/index-*.js" "$(basename "$WEB_INDEX")"
  else
    WEB_INDEX_JS="$LOG_DIR/live-web-index.js"
    if curl_text "live web index bundle through APISIX" "/$INDEX_ASSET" "$WEB_INDEX_JS"; then
      FUSION_ASSET="$(grep -o 'FusionWorkbenchPage-[A-Za-z0-9_-]*\.js' "$WEB_INDEX_JS" | head -n1 || true)"
      if [[ -z "$FUSION_ASSET" ]]; then
        json_log "web" "live fusion chunk discovery" "blocker" false "missing" "FusionWorkbenchPage chunk" "$(basename "$WEB_INDEX_JS")"
      else
        WEB_FUSION_JS="$LOG_DIR/live-web-fusion.js"
        if curl_text "live fusion bundle through APISIX" "/assets/$FUSION_ASSET" "$WEB_FUSION_JS"; then
          {
            echo "index=$INDEX_ASSET"
            echo "fusion=$FUSION_ASSET"
            echo "markers=Threat Intel,threat-intel/entries?limit=12,/api/v1/threat-intel/entries,/v1/fusion/conflicts,/v1/fusion/rules"
          } >"$LOG_DIR/live-web-bundle-marker.txt"
          if grep -q 'Threat Intel' "$WEB_FUSION_JS" && grep -q 'threat-intel/entries?limit=12' "$WEB_FUSION_JS"; then
            json_log "web" "live fusion bundle includes threat intel UI markers" "info" true "ok" "$FUSION_ASSET" "live-web-bundle-marker.txt"
          else
            json_log "web" "live fusion bundle includes threat intel UI markers" "blocker" false "missing" "Threat Intel or threat-intel/entries?limit=12" "$(basename "$WEB_FUSION_JS")"
          fi
          if grep -q '/v1/fusion/conflicts/' "$WEB_FUSION_JS" && grep -q '/v1/fusion/rules/' "$WEB_FUSION_JS"; then
            json_log "web" "live fusion bundle includes write API markers" "info" true "ok" "$FUSION_ASSET" "live-web-bundle-marker.txt"
          else
            json_log "web" "live fusion bundle includes write API markers" "blocker" false "missing" "/v1/fusion/conflicts or /v1/fusion/rules" "$(basename "$WEB_FUSION_JS")"
          fi
          if grep -q '/api/v1/threat-intel/entries' "$WEB_INDEX_JS"; then
            json_log "web" "live index bundle advertises threat intel API endpoint" "info" true "ok" "$INDEX_ASSET" "live-web-bundle-marker.txt"
          else
            json_log "web" "live index bundle advertises threat intel API endpoint" "blocker" false "missing" "/api/v1/threat-intel/entries" "$(basename "$WEB_INDEX_JS")"
          fi
        fi
      fi
    fi
  fi
fi

jq -s \
  --arg run_id "$RUN_ID" \
  --arg apisix "$APISIX" \
  --arg web_url "$WEB_URL" \
  --arg report "$REPORT" \
  '{
    run_id: $run_id,
    generated_at: now | todateiso8601,
    apisix: $apisix,
    web_url: $web_url,
    result: (if ([.[] | select(.severity == "blocker" and .passed == false)] | length) == 0 then "pass" else "blocked" end),
    totals: {
      total: length,
      passed: ([.[] | select(.passed == true)] | length),
      failed: ([.[] | select(.passed == false)] | length),
      blockers: ([.[] | select(.severity == "blocker" and .passed == false)] | length),
      warnings: ([.[] | select(.severity == "warning" and .passed == false)] | length)
    },
    artifacts: {
      ndjson: $report,
      vitest: "web-vitest-fusion-threat-intel.log",
      icon_workflow: "fusion-ui-icon-workflow.json",
      web_deployment: "web-ui-deploy-live.json",
      alert_deployment: "alert-service-deploy-live.json",
      web_bundle_marker: "live-web-bundle-marker.txt",
      fusion_stats: "api-fusion-stats.json",
      fusion_workbench: "api-fusion-workbench.json",
      fusion_workbench_page1: "api-fusion-workbench-page1.json",
      fusion_workbench_page2: "api-fusion-workbench-page2.json",
      fusion_entities: "api-fusion-entities.json",
      threat_entries: "api-threat-intel-entries.json",
      conflict_resolution: "api-fusion-conflict-resolve.json",
      rule_update: "api-fusion-rule-update.json",
      pg_conflict_resolution: "pg-fusion-conflict-resolution-count.txt",
      pg_rule_override: "pg-fusion-rule-override-count.txt",
      pg_repair_task: "pg-fusion-repair-task-count.txt",
      pg_audit: "pg-fusion-write-audit-count.txt"
    },
    checks: .
  }' "$REPORT" >"$SUMMARY"

RESULT="$(jq -r '.result' "$SUMMARY")"
TOTAL="$(jq -r '.totals.total' "$SUMMARY")"
PASSED="$(jq -r '.totals.passed' "$SUMMARY")"
BLOCKERS="$(jq -r '.totals.blockers' "$SUMMARY")"
WARNINGS="$(jq -r '.totals.warnings' "$SUMMARY")"

{
  echo "# Fusion x Threat Intel Contract Report"
  echo
  echo "- Run ID: \`$RUN_ID\`"
  echo "- Result: \`$RESULT\`"
  echo "- APISIX: \`$APISIX\`"
  echo "- Web URL: \`$WEB_URL\`"
  echo "- Web image expected: \`$WEB_IMAGE_EXPECTED\`"
  echo "- Checks: $PASSED/$TOTAL passed, blockers=$BLOCKERS, warnings=$WARNINGS"
  echo
  echo "## Blockers"
  jq -r '.checks[] | select(.severity == "blocker" and .passed == false) | "- " + .name + ": " + .status + " " + .detail' "$SUMMARY"
  if [[ "$BLOCKERS" == "0" ]]; then
    echo "- None"
  fi
  echo
  echo "## Warnings"
  jq -r '.checks[] | select(.severity == "warning" and .passed == false) | "- " + .name + ": " + .status + " " + .detail' "$SUMMARY"
  if [[ "$WARNINGS" == "0" ]]; then
    echo "- None"
  fi
  echo
  echo "## Evidence"
  echo "- NDJSON: \`$REPORT\`"
  echo "- Summary: \`$SUMMARY\`"
  echo "- Vitest: \`$LOG_DIR/web-vitest-fusion-threat-intel.log\`"
  echo "- Web deployment: \`$LOG_DIR/web-ui-deploy-live.json\`"
  echo "- Web bundle marker: \`$LOG_DIR/live-web-bundle-marker.txt\`"
  echo "- Fusion stats: \`$LOG_DIR/api-fusion-stats.json\`"
  echo "- Fusion entities: \`$LOG_DIR/api-fusion-entities.json\`"
  echo "- Threat Intel entries: \`$LOG_DIR/api-threat-intel-entries.json\`"
  echo "- Fusion conflict resolution: \`$LOG_DIR/api-fusion-conflict-resolve.json\`"
  echo "- Fusion rule update: \`$LOG_DIR/api-fusion-rule-update.json\`"
  echo "- PG conflict row count: \`$LOG_DIR/pg-fusion-conflict-resolution-count.txt\`"
  echo "- PG rule row count: \`$LOG_DIR/pg-fusion-rule-override-count.txt\`"
  echo "- PG audit row count: \`$LOG_DIR/pg-fusion-write-audit-count.txt\`"
  echo
  echo "## Scope"
  echo
  echo "This report verifies that the Fusion page contract consumes the Threat Intel service through APISIX, maps live intelligence into Fusion source status, metrics, rows, timeline, and evidence, and writes Fusion conflict/rule actions through APISIX into PostgreSQL and audit_logs."
} >"$LOCAL_REPORT"

cp "$SUMMARY" "$REGRESSION_DIR/fusion-threat-intel-latest.json"
cp "$LOCAL_REPORT" "$REGRESSION_DIR/fusion-threat-intel-latest.md"

if [[ "$RESULT" != "pass" ]]; then
  exit 1
fi
