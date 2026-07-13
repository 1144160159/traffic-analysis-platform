#!/usr/bin/env bash
set -euo pipefail

APISIX="${APISIX:-http://10.0.5.8:30180}"
TENANT="${TENANT:-default}"
LOG_DIR="${LOG_DIR:-doc/02_acceptance/runs/20260630-fusion-value-report-preflight}"
RUN_ID="${RUN_ID:-$(date +%Y%m%d%H%M%S)-fusion-value-report}"
REGRESSION_DIR="${REGRESSION_DIR:-doc/02_acceptance/02-regression}"
KUBECTL="${KUBECTL:-kubectl}"
JWT_SECRET_NAMESPACE="${JWT_SECRET_NAMESPACE:-traffic-analysis}"
JWT_SECRET_NAME="${JWT_SECRET_NAME:-traffic-credentials}"
JWT_SECRET_KEY="${JWT_SECRET_KEY:-JWT_SECRET}"
ALLOW_BLOCKERS="${ALLOW_BLOCKERS:-false}"

REPORT="$LOG_DIR/live-fusion-value-report-$RUN_ID.ndjson"
SUMMARY="$LOG_DIR/live-fusion-value-report-$RUN_ID-summary.json"
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

run_check() {
  local name="$1" output="$2"
  shift 2
  local rc
  set +e
  "$@" >"$output" 2>&1
  rc=$?
  set -e
  if [[ "$rc" -eq 0 ]]; then
    json_log "local" "$name" "info" true "ok" "$*" "$(basename "$output")"
  else
    json_log "local" "$name" "blocker" false "rc=$rc" "$(trim_file "$output")" "$(basename "$output")"
  fi
}

make_jwt() {
  local username="${1:-codex-fusion-value-report}"
  local roles_json="${2:-[\"admin\"]}"
  local permissions_json="${3:-[\"*\", \"admin:*\", \"alert:read\", \"graph:read\", \"rule:read\", \"audit:read\", \"user:read\"]}"
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
    "session_id": "codex-fusion-value-report-" + os.environ["RUN_ID"] + "-" + os.environ["USERNAME"],
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

fetch_jwt_secret() {
  local encoded_secret decoded_secret rc secret_err decode_err
  secret_err="$LOG_DIR/jwt-secret.err"
  decode_err="$LOG_DIR/jwt-secret-decode.err"
  set +e
  encoded_secret="$(kctl -n "$JWT_SECRET_NAMESPACE" get secret "$JWT_SECRET_NAME" -o "jsonpath={.data.$JWT_SECRET_KEY}" 2>"$secret_err")"
  rc=$?
  set -e
  if [[ "$rc" -ne 0 || -z "$encoded_secret" ]]; then
    json_log "secret" "jwt signing secret available" "blocker" false "missing" "$(trim_file "$secret_err")" "$(basename "$secret_err")"
    return 1
  fi

  set +e
  decoded_secret="$(printf '%s' "$encoded_secret" | base64 -d 2>"$decode_err")"
  rc=$?
  set -e
  if [[ "$rc" -ne 0 || -z "$decoded_secret" ]]; then
    json_log "secret" "jwt signing secret decodable" "blocker" false "invalid" "$(trim_file "$decode_err")" "$(basename "$decode_err")"
    return 1
  fi

  JWT_SECRET="$decoded_secret"
  json_log "secret" "jwt signing secret available" "info" true "ok" "$JWT_SECRET_NAMESPACE/$JWT_SECRET_NAME#$JWT_SECRET_KEY" ""
  return 0
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

assert_json() {
  local name="$1" file="$2"
  shift 2
  if jq -e "$@" "$file" >/dev/null 2>&1; then
    json_log "assert" "$name" "info" true "ok" "" "$(basename "$file")"
  else
    json_log "assert" "$name" "blocker" false "failed" "$(trim_file "$file")" "$(basename "$file")"
  fi
}

finalize() {
  local passed failed blockers warnings result
  passed="$(jq -s '[.[] | select(.passed == true)] | length' "$REPORT")"
  failed="$(jq -s '[.[] | select(.passed == false)] | length' "$REPORT")"
  blockers="$(jq -s '[.[] | select(.passed == false and .severity == "blocker")] | length' "$REPORT")"
  warnings="$(jq -s '[.[] | select(.passed == false and .severity == "warning")] | length' "$REPORT")"
  result="pass"
  if [[ "$blockers" -gt 0 ]]; then
    result="blocked"
  fi
  jq -n \
    --arg run_id "$RUN_ID" \
    --arg result "$result" \
    --arg report "$REPORT" \
    --arg generated_at "$(date -Iseconds)" \
    --argjson passed "$passed" \
    --argjson failed "$failed" \
    --argjson blockers "$blockers" \
    --argjson warnings "$warnings" \
    '{run_id:$run_id, result:$result, generated_at:$generated_at, passed:$passed, failed:$failed, blockers:$blockers, warnings:$warnings, report:$report}' >"$SUMMARY"
  cat >"$LOCAL_REPORT" <<EOF
# Fusion 价值量化 live preflight

- Run ID: \`$RUN_ID\`
- Result: \`$result\`
- Passed: $passed
- Failed: $failed
- Blockers: $blockers
- Warnings: $warnings
- NDJSON: \`$REPORT\`
- Summary: \`$SUMMARY\`

本脚本验证 \`/api/v1/fusion/value-report\` 是否已通过真实 APISIX/JWT/K8s 链路返回可复核的单源/多源消融报告结构。
EOF
  cp "$SUMMARY" "$REGRESSION_DIR/fusion-value-report-preflight-latest.json"
  cp "$LOCAL_REPORT" "$REGRESSION_DIR/fusion-value-report-preflight-latest.md"
  if [[ "$blockers" -gt 0 && "$ALLOW_BLOCKERS" != "true" ]]; then
    exit 1
  fi
}

trap finalize EXIT

need_cmd git
need_cmd jq
need_cmd python3
need_cmd curl
need_cmd npm
need_cmd go
need_cmd "$KUBECTL"

git rev-parse HEAD >"$LOG_DIR/commit-sha.txt"
git branch --show-current >"$LOG_DIR/git-branch.txt"
git status --short >"$LOG_DIR/git-status.txt"
git diff --stat >"$LOG_DIR/git-diff-stat.txt" || true

check_grep "fusion value route registered" "/fusion/value-report" "go/control-plane/internal/alert/api/handler_system.go"
check_grep "fusion value formula version present" "fusion-value-ablation-v1" "go/control-plane/internal/alert/api/handler_product_pages.go"
check_grep "fusion API plan includes value report" "fusion/value-report" "web/ui/src/services/pageApiPlans.ts"
check_grep "fusion adapter exposes value evidence" "Fusion Value API" "web/ui/src/services/pageSnapshotAdapters.ts"
check_grep "route manifest advertises value report" "fusion/value-report" "web/ui/src/routes/routeManifest.tsx"

run_check "go fusion value report tests" "$LOG_DIR/go-test-fusion-value.log" \
  bash -lc "cd go/control-plane && go test ./internal/alert/api -run 'Test(FusionValueReportNoDependenciesReturnsGatedReport|TopicGovernanceRoutesAreRegisteredUnderAPIV1)$'"
run_check "frontend fusion adapter tests" "$LOG_DIR/web-vitest-fusion-value.log" \
  npm --prefix web/ui run test -- --run src/services/pageSnapshotAdapters.test.ts src/routes/routeManifest.test.ts
run_check "ui contract validation" "$LOG_DIR/ui-contract-validation.log" \
  node doc/04_assets/ui_suite_gpt_v1/validate_frontend_contracts.mjs

VALUE_RESPONSE="$LOG_DIR/fusion-value-report-response.json"
STATS_RESPONSE="$LOG_DIR/fusion-stats-response.json"
ENTITIES_RESPONSE="$LOG_DIR/fusion-entities-response.json"

if fetch_jwt_secret; then
  ADMIN_TOKEN="$(make_jwt)"

  curl_json "fusion stats reachable" "/api/v1/fusion/stats" "$STATS_RESPONSE" || true
  curl_json "fusion entities reachable" "/api/v1/fusion/entities?limit=8&page_size=8" "$ENTITIES_RESPONSE" || true
  curl_json "fusion value report reachable" "/api/v1/fusion/value-report?window_hours=168" "$VALUE_RESPONSE" || true

  if [[ -s "$VALUE_RESPONSE" ]]; then
    assert_json "value report success envelope" "$VALUE_RESPONSE" '.success == true and (.data | type == "object")'
    assert_json "value report formula version" "$VALUE_RESPONSE" '.data.formula_version == "fusion-value-ablation-v1"'
    assert_json "single and multi source objects" "$VALUE_RESPONSE" '(.data.single_source_baseline | type == "object") and (.data.multi_source | type == "object")'
    assert_json "delta contains numeric value metrics" "$VALUE_RESPONSE" '
      (.data.delta.lead_time_minutes | type == "number") and
      (.data.delta.false_positive_reduction_pct | type == "number") and
      (.data.delta.mttr_reduction_pct | type == "number")'
    assert_json "quality gates include reproducibility" "$VALUE_RESPONSE" '([.data.quality_gates[]?.gate] | index("formula_reproducibility") != null)'
    assert_json "evidence references fusion stats and alert MTTR" "$VALUE_RESPONSE" '
      ([.data.evidence[]?.label] | index("Fusion Stats API") != null) and
      ([.data.evidence[]?.label] | index("Alert MTTR") != null)'
    assert_json "window hours is bounded and positive" "$VALUE_RESPONSE" '(.data.window_hours | type == "number") and (.data.window_hours >= 1) and (.data.window_hours <= 2160)'
  fi
fi
