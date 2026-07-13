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
WEB_IMAGE_EXPECTED="${WEB_IMAGE_EXPECTED:-traffic/web-ui:fusion-write-20260629-r1}"
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
  JWT_SECRET="$JWT_SECRET" TENANT="$TENANT" RUN_ID="$RUN_ID" USERNAME="$username" ROLES_JSON="$roles_json" PERMISSIONS_JSON="$permissions_json" python3 - <<'PY'
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
  local err_file code rc
  err_file="$output.err"
  set +e
  code="$(curl --noproxy '*' -sS -m 20 -o "$output" -w '%{http_code}' \
    -X "$method" \
    -H 'Content-Type: application/json' \
    -H "Authorization: Bearer $token" \
    -H "X-Tenant-ID: $TENANT" \
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

FUSION_STATS="$LOG_DIR/api-fusion-stats.json"
if curl_json "fusion stats through APISIX" "/api/v1/fusion/stats" "$FUSION_STATS"; then
  assert_json "fusion stats include threat_intel source" "$FUSION_STATS" '.success == true and (.data.data_source_stats.threat_intel.count // 0) >= 1'
fi

FUSION_ENTITIES="$LOG_DIR/api-fusion-entities.json"
if curl_json "fusion entities through APISIX" "/api/v1/fusion/entities?limit=3" "$FUSION_ENTITIES"; then
  assert_json "fusion entities returns aligned entities" "$FUSION_ENTITIES" '.success == true and ((.data.entities // .data.data.entities // []) | length) >= 1'
fi

THREAT_ENTRIES="$LOG_DIR/api-threat-intel-entries.json"
if curl_json "threat intel entries through APISIX" "/api/v1/threat-intel/entries?limit=5" "$THREAT_ENTRIES"; then
  assert_json "threat intel entries returns list" "$THREAT_ENTRIES" '.success == true and (.data | type) == "array" and (.data | length) >= 1'
fi

RUN_ID_SAFE="$(tr '[:upper:]_' '[:lower:]-' <<<"$RUN_ID" | sed -E 's/[^a-z0-9-]+/-/g; s/^-+|-+$//g')"
CONFLICT_ID="codex-fusion-conflict-${RUN_ID_SAFE}"
RULE_ID="codex-fusion-rule-${RUN_ID_SAFE}"

CONFLICT_BODY="$LOG_DIR/api-fusion-conflict-resolve-body.json"
jq -nc \
  --arg object_id "codex-fusion-object-${RUN_ID_SAFE}" \
  --arg rule_id "$RULE_ID" \
  '{object_id:$object_id, object_type:"threat_intel", field_name:"C2 情报命中", selected_source:"Threat Intel 威胁情报", selected_value:"185.130.5.253", strategy:"authoritative-source", note:"Codex live Fusion conflict resolution", rule_id:$rule_id, detail:{source_a:"Threat Intel 威胁情报", source_b:"Fusion 规则", confidence:"0.92"}}' >"$CONFLICT_BODY"

READER_CONFLICT_JSON="$LOG_DIR/api-fusion-conflict-reader-denied.json"
curl_json_body "read-only token cannot resolve fusion conflict" "POST" "/api/v1/fusion/conflicts/$CONFLICT_ID/resolve" "$READER_CONFLICT_JSON" "$CONFLICT_BODY" "$READER_TOKEN" "403" || true

CONFLICT_JSON="$LOG_DIR/api-fusion-conflict-resolve.json"
if curl_json_body "resolve fusion conflict through APISIX" "POST" "/api/v1/fusion/conflicts/$CONFLICT_ID/resolve" "$CONFLICT_JSON" "$CONFLICT_BODY"; then
  assert_json "fusion conflict resolution returns persisted version and audit flag" "$CONFLICT_JSON" --arg id "$CONFLICT_ID" '.success == true and .data.resolution.conflict_id == $id and (.data.resolution.state_version // 0) >= 1 and .data.audit_written == true'
fi

RULE_BODY="$LOG_DIR/api-fusion-rule-update-body.json"
jq -nc \
  '{rule_name:"Codex Fusion Threat Intel Rule", status:"draft", strategy:"authoritative-source", confidence_threshold:0.91, note:"Codex live Fusion rule edit", detail:{field_name:"C2 情报命中", selected_source:"Threat Intel 威胁情报"}}' >"$RULE_BODY"

RULE_JSON="$LOG_DIR/api-fusion-rule-update.json"
if curl_json_body "update fusion rule through APISIX" "PATCH" "/api/v1/fusion/rules/$RULE_ID" "$RULE_JSON" "$RULE_BODY"; then
  assert_json "fusion rule update returns version and audit flag" "$RULE_JSON" --arg id "$RULE_ID" '.success == true and .data.rule.rule_id == $id and (.data.rule.version // 0) >= 1 and .data.audit_written == true'
fi

PG_CONFLICT_TXT="$LOG_DIR/pg-fusion-conflict-resolution-count.txt"
if psql_exec "SELECT count(*) FROM fusion_conflict_resolutions WHERE tenant_id = '$TENANT' AND conflict_id = '$CONFLICT_ID';" >"$PG_CONFLICT_TXT" 2>"$PG_CONFLICT_TXT.err"; then
  assert_json "fusion conflict resolution row exists" <(jq -Rn --arg count "$(tr -d '[:space:]' <"$PG_CONFLICT_TXT")" '{count:($count|tonumber)}') '.count >= 1'
else
  json_log "postgres" "fusion conflict resolution row exists" "blocker" false "query-failed" "$(trim_file "$PG_CONFLICT_TXT.err")" "$(basename "$PG_CONFLICT_TXT.err")"
fi

PG_RULE_TXT="$LOG_DIR/pg-fusion-rule-override-count.txt"
if psql_exec "SELECT count(*) FROM fusion_rule_overrides WHERE tenant_id = '$TENANT' AND rule_id = '$RULE_ID';" >"$PG_RULE_TXT" 2>"$PG_RULE_TXT.err"; then
  assert_json "fusion rule override row exists" <(jq -Rn --arg count "$(tr -d '[:space:]' <"$PG_RULE_TXT")" '{count:($count|tonumber)}') '.count >= 1'
else
  json_log "postgres" "fusion rule override row exists" "blocker" false "query-failed" "$(trim_file "$PG_RULE_TXT.err")" "$(basename "$PG_RULE_TXT.err")"
fi

PG_AUDIT_TXT="$LOG_DIR/pg-fusion-write-audit-count.txt"
if psql_exec "SELECT count(*) FROM audit_logs WHERE tenant_id = '$TENANT' AND action IN ('FUSION_CONFLICT_RESOLVED','FUSION_RULE_UPDATED') AND object_id IN ('$CONFLICT_ID', '$RULE_ID');" >"$PG_AUDIT_TXT" 2>"$PG_AUDIT_TXT.err"; then
  assert_json "fusion write audit rows exist" <(jq -Rn --arg count "$(tr -d '[:space:]' <"$PG_AUDIT_TXT")" '{count:($count|tonumber)}') '.count >= 2'
else
  json_log "postgres" "fusion write audit rows exist" "blocker" false "query-failed" "$(trim_file "$PG_AUDIT_TXT.err")" "$(basename "$PG_AUDIT_TXT.err")"
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
            echo "markers=Threat Intel API,情报命中,/api/v1/threat-intel/entries,/v1/fusion/conflicts,/v1/fusion/rules"
          } >"$LOG_DIR/live-web-bundle-marker.txt"
          if grep -q 'Threat Intel API' "$WEB_FUSION_JS" && grep -q '情报命中' "$WEB_FUSION_JS"; then
            json_log "web" "live fusion bundle includes threat intel UI markers" "info" true "ok" "$FUSION_ASSET" "live-web-bundle-marker.txt"
          else
            json_log "web" "live fusion bundle includes threat intel UI markers" "blocker" false "missing" "Threat Intel API or 情报命中" "$(basename "$WEB_FUSION_JS")"
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
      web_deployment: "web-ui-deploy-live.json",
      web_bundle_marker: "live-web-bundle-marker.txt",
      fusion_stats: "api-fusion-stats.json",
      fusion_entities: "api-fusion-entities.json",
      threat_entries: "api-threat-intel-entries.json",
      conflict_resolution: "api-fusion-conflict-resolve.json",
      rule_update: "api-fusion-rule-update.json",
      pg_conflict_resolution: "pg-fusion-conflict-resolution-count.txt",
      pg_rule_override: "pg-fusion-rule-override-count.txt",
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
