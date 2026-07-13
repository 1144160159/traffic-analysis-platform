#!/usr/bin/env bash
set -euo pipefail

APISIX="${APISIX:-http://10.0.5.8:30180}"
TENANT="${TENANT:-default}"
LOG_DIR="${LOG_DIR:-doc/02_acceptance/runs/20260629-playbook-state-machine}"
RUN_ID="${RUN_ID:-$(date +%Y%m%d%H%M%S)-playbook-state-machine}"
REGRESSION_DIR="${REGRESSION_DIR:-doc/02_acceptance/02-regression}"
KUBECTL="${KUBECTL:-kubectl}"
PLAYBOOK_NAME="${PLAYBOOK_NAME:-log-lateral-movement}"
JWT_SECRET_NAMESPACE="${JWT_SECRET_NAMESPACE:-traffic-analysis}"
JWT_SECRET_NAME="${JWT_SECRET_NAME:-traffic-credentials}"
JWT_SECRET_KEY="${JWT_SECRET_KEY:-JWT_SECRET}"
PG_NAMESPACE="${PG_NAMESPACE:-databases}"
PG_POD="${PG_POD:-postgres-primary-0}"
PG_SECRET_KEY="${PG_SECRET_KEY:-PG_PASSWORD}"
PLAYWRIGHT_NODE_MODULE="${PLAYWRIGHT_NODE_MODULE:-./web/ui/node_modules/playwright}"
PLAYWRIGHT_HEADLESS="${PLAYWRIGHT_HEADLESS:-true}"

REPORT="$LOG_DIR/live-playbook-state-machine-$RUN_ID.ndjson"
SUMMARY="$LOG_DIR/live-playbook-state-machine-$RUN_ID-summary.json"
LOCAL_REPORT="$LOG_DIR/local-report.md"
SCREENSHOT="$LOG_DIR/live-playbook-state-machine-$RUN_ID.png"

mkdir -p "$LOG_DIR" "$REGRESSION_DIR"
: >"$REPORT"

ORIGINAL_ENABLED=""
ORIGINAL_MAX_RUNS=""
ORIGINAL_COOLDOWN_SECONDS=""
TOKEN=""

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

make_jwt() {
  local username="$1"
  local roles_json="$2"
  local permissions_json="$3"
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
    "session_id": "codex-playbook-state-" + os.environ["RUN_ID"],
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

curl_json() {
  local name="$1" path="$2" output="$3" token="${4:-$TOKEN}" expected="${5:-2}"
  local err_file code rc
  err_file="$output.err"
  set +e
  code="$(curl --noproxy '*' -sS -m 20 -o "$output" -w '%{http_code}' \
    -H "Authorization: Bearer $token" \
    -H "X-Tenant-ID: $TENANT" \
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

curl_json_body() {
  local name="$1" method="$2" path="$3" output="$4" body_file="$5" token="${6:-$TOKEN}" expected="${7:-2}"
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

assert_json() {
  local name="$1" file="$2"
  shift 2
  local err_file="$file.assert.err"
  if jq -e "$@" "$file" >/dev/null 2>"$err_file"; then
    json_log "assert" "$name" "info" true "ok" "$*" "$(basename "$file")"
    return 0
  fi
  json_log "assert" "$name" "blocker" false "jq-failed" "filter=$* body=$(trim_file "$file") err=$(trim_file "$err_file")" "$(basename "$file")"
  return 1
}

psql_exec() {
  local sql="$1"
  kctl -n "$PG_NAMESPACE" exec "$PG_POD" -- env PGPASSWORD="$PG_PASSWORD" \
    psql -U postgres -d traffic_platform -v ON_ERROR_STOP=1 -Atc "$sql"
}

restore_playbook() {
  if [[ -z "$TOKEN" || -z "$ORIGINAL_ENABLED" || -z "$ORIGINAL_MAX_RUNS" || -z "$ORIGINAL_COOLDOWN_SECONDS" ]]; then
    return 0
  fi
  local restore_body="$LOG_DIR/playbook-restore-body.json"
  local restore_response="$LOG_DIR/playbook-restore-response.json"
  jq -nc \
    --argjson enabled "$ORIGINAL_ENABLED" \
    --argjson max_runs "$ORIGINAL_MAX_RUNS" \
    --argjson cooldown_seconds "$ORIGINAL_COOLDOWN_SECONDS" \
    '{enabled:$enabled,max_runs:$max_runs,cooldown_seconds:$cooldown_seconds}' >"$restore_body"
  curl --noproxy '*' -sS -m 20 -o "$restore_response" \
    -X PATCH \
    -H 'Content-Type: application/json' \
    -H "Authorization: Bearer $TOKEN" \
    -H "X-Tenant-ID: $TENANT" \
    --data-binary "@$restore_body" \
    "$APISIX/api/v1/playbooks/$PLAYBOOK_NAME" >/dev/null 2>"$restore_response.err" || true
}

trap restore_playbook EXIT

for cmd in curl jq python3 node "$KUBECTL"; do
  need_cmd "$cmd"
done

JWT_SECRET="$(kctl -n "$JWT_SECRET_NAMESPACE" get secret "$JWT_SECRET_NAME" -o "jsonpath={.data.$JWT_SECRET_KEY}" | base64 -d)"
PG_PASSWORD="$(kctl -n "$JWT_SECRET_NAMESPACE" get secret "$JWT_SECRET_NAME" -o "jsonpath={.data.$PG_SECRET_KEY}" | base64 -d)"
TOKEN="$(make_jwt codex-playbook-operator '["admin"]' '["*","admin:*","alert:write","alert:read"]')"

if grep -q "playbook-execute" web/ui/src/services/pageApiPlans.ts && grep -q "playbook-state-update" web/ui/src/services/pageApiPlans.ts; then
  json_log "contract" "frontend playbook action contract present" "info" true "ok" "pageApiPlans playbook execute/update actions" "web/ui/src/services/pageApiPlans.ts"
else
  json_log "contract" "frontend playbook action contract present" "blocker" false "missing" "playbook action contracts missing" "web/ui/src/services/pageApiPlans.ts"
fi
if grep -q "playbook max runs reached" go/control-plane/internal/alert/playbook/playbook_engine.go; then
  json_log "contract" "backend manual execute enforces max_runs" "info" true "ok" "ExecuteByName returns max-runs error" "go/control-plane/internal/alert/playbook/playbook_engine.go"
else
  json_log "contract" "backend manual execute enforces max_runs" "blocker" false "missing" "ExecuteByName max_runs guard missing" "go/control-plane/internal/alert/playbook/playbook_engine.go"
fi

AUTH_ME="$LOG_DIR/auth-me.json"
curl_json "auth/me exposes playbook operator token" "/api/v1/auth/me" "$AUTH_ME"
assert_json "operator token has write scope" "$AUTH_ME" '((.permissions // []) | index("alert:write") != null) or ((.permissions // []) | index("*") != null)'

CATALOG_BEFORE="$LOG_DIR/playbook-catalog-before.json"
curl_json "playbook catalog reachable" "/api/v1/playbooks/catalog" "$CATALOG_BEFORE"
assert_json "target playbook exists in catalog" "$CATALOG_BEFORE" --arg name "$PLAYBOOK_NAME" '.data.playbooks[] | select(.name == $name)'

ORIGINAL_ENABLED="$(jq -r --arg name "$PLAYBOOK_NAME" '.data.playbooks[] | select(.name == $name) | .enabled' "$CATALOG_BEFORE")"
ORIGINAL_MAX_RUNS="$(jq -r --arg name "$PLAYBOOK_NAME" '.data.playbooks[] | select(.name == $name) | (.max_runs // 0)' "$CATALOG_BEFORE")"
ORIGINAL_COOLDOWN_SECONDS="$(jq -r --arg name "$PLAYBOOK_NAME" '.data.playbooks[] | select(.name == $name) | (((.cooldown // 0) / 1000000000) | floor)' "$CATALOG_BEFORE")"
CURRENT_RUN_COUNT="$(jq -r --arg name "$PLAYBOOK_NAME" '.data.playbooks[] | select(.name == $name) | (.run_count // 0)' "$CATALOG_BEFORE")"
LIMIT_RUNS=$((CURRENT_RUN_COUNT + 1))
if [[ "$ORIGINAL_COOLDOWN_SECONDS" == "0" ]]; then
  ORIGINAL_COOLDOWN_SECONDS=300
fi

DISABLE_BODY="$LOG_DIR/playbook-disable-body.json"
DISABLE_RESPONSE="$LOG_DIR/playbook-disable-response.json"
jq -nc '{enabled:false}' >"$DISABLE_BODY"
curl_json_body "disable playbook through APISIX" "PATCH" "/api/v1/playbooks/$PLAYBOOK_NAME" "$DISABLE_RESPONSE" "$DISABLE_BODY"
assert_json "disable response persisted" "$DISABLE_RESPONSE" '.success == true and .data.enabled == false'

EXECUTE_BODY="$LOG_DIR/playbook-execute-body.json"
ALERT_ID="codex-playbook-$RUN_ID"
jq -nc \
  --arg alert_id "$ALERT_ID" \
  --arg tenant "$TENANT" \
  '{alert_id:$alert_id,alert_type:"lateral_movement",severity:"critical",score:0.97,source_ip:"192.0.2.45",dest_ip:"198.51.100.20",tenant_id:$tenant,related_alert_count:8,asset_risk:"high",asset_name:"codex-playbook-lab-host"}' >"$EXECUTE_BODY"

DISABLED_EXECUTE_RESPONSE="$LOG_DIR/playbook-disabled-execute-response.json"
curl_json_body "disabled playbook rejects execute" "POST" "/api/v1/playbooks/$PLAYBOOK_NAME/execute" "$DISABLED_EXECUTE_RESPONSE" "$EXECUTE_BODY" "$TOKEN" "404"
assert_json "disabled execute response names disabled guard" "$DISABLED_EXECUTE_RESPONSE" '.success == false and (.message | contains("disabled"))'

LIMIT_BODY="$LOG_DIR/playbook-limit-body.json"
LIMIT_RESPONSE="$LOG_DIR/playbook-limit-response.json"
jq -nc \
  --argjson max_runs "$LIMIT_RUNS" \
  --argjson cooldown_seconds "$ORIGINAL_COOLDOWN_SECONDS" \
  '{enabled:true,max_runs:$max_runs,cooldown_seconds:$cooldown_seconds}' >"$LIMIT_BODY"
curl_json_body "enable playbook with one remaining slot" "PATCH" "/api/v1/playbooks/$PLAYBOOK_NAME" "$LIMIT_RESPONSE" "$LIMIT_BODY"
assert_json "limit response persisted" "$LIMIT_RESPONSE" --argjson max_runs "$LIMIT_RUNS" '.success == true and .data.enabled == true and .data.max_runs == $max_runs'

FIRST_EXECUTE_RESPONSE="$LOG_DIR/playbook-first-execute-response.json"
curl_json_body "first playbook execute succeeds" "POST" "/api/v1/playbooks/$PLAYBOOK_NAME/execute" "$FIRST_EXECUTE_RESPONSE" "$EXECUTE_BODY"
assert_json "first execute returns actions" "$FIRST_EXECUTE_RESPONSE" --arg name "$PLAYBOOK_NAME" --arg alert "$ALERT_ID" '.success == true and .data.playbook == $name and .data.alert_id == $alert and (.data.success_actions // 0) >= 1 and (.execution.execution_id // "") != ""'

SECOND_EXECUTE_RESPONSE="$LOG_DIR/playbook-second-execute-response.json"
curl_json_body "second playbook execute hits max_runs" "POST" "/api/v1/playbooks/$PLAYBOOK_NAME/execute" "$SECOND_EXECUTE_RESPONSE" "$EXECUTE_BODY" "$TOKEN" "404"
assert_json "second execute response names max_runs guard" "$SECOND_EXECUTE_RESPONSE" '.success == false and (.message | contains("max runs"))'

EXECUTIONS_RESPONSE="$LOG_DIR/playbook-executions-response.json"
curl_json "playbook executions API lists run" "/api/v1/playbooks/executions?limit=10" "$EXECUTIONS_RESPONSE"
assert_json "executions API includes current alert" "$EXECUTIONS_RESPONSE" --arg alert "$ALERT_ID" --arg name "$PLAYBOOK_NAME" '.data.executions[] | select(.alert_id == $alert and .playbook == $name)'

PG_COUNT_FILE="$LOG_DIR/playbook-pg-row-count.txt"
if psql_exec "SELECT count(*) FROM alert_playbook_executions WHERE tenant_id = '$TENANT' AND alert_id = '$ALERT_ID' AND playbook_name = '$PLAYBOOK_NAME';" >"$PG_COUNT_FILE" 2>"$PG_COUNT_FILE.err"; then
  if [[ "$(tr -d '[:space:]' <"$PG_COUNT_FILE")" -ge 1 ]]; then
    json_log "postgres" "playbook execution row persisted" "info" true "ok" "$(trim_file "$PG_COUNT_FILE")" "$(basename "$PG_COUNT_FILE")"
  else
    json_log "postgres" "playbook execution row persisted" "blocker" false "missing" "$(trim_file "$PG_COUNT_FILE")" "$(basename "$PG_COUNT_FILE")"
  fi
else
  json_log "postgres" "playbook execution row persisted" "blocker" false "query-failed" "$(trim_file "$PG_COUNT_FILE.err")" "$(basename "$PG_COUNT_FILE.err")"
fi

TOKEN="$TOKEN" APISIX="$APISIX" REPORT="$REPORT" SCREENSHOT="$SCREENSHOT" PLAYWRIGHT_NODE_MODULE="$PLAYWRIGHT_NODE_MODULE" PLAYWRIGHT_HEADLESS="$PLAYWRIGHT_HEADLESS" node <<'JS'
const fs = require('node:fs');
const { chromium } = require(process.env.PLAYWRIGHT_NODE_MODULE);

const headless = process.env.PLAYWRIGHT_HEADLESS !== 'false';
const baseURL = process.env.APISIX;
const reportPath = process.env.REPORT;
const screenshotPath = process.env.SCREENSHOT;
const token = process.env.TOKEN;

function log(phase, name, severity, passed, status, detail, artifact = '') {
  fs.appendFileSync(reportPath, JSON.stringify({
    ts: new Date().toISOString(),
    phase,
    name,
    severity,
    passed,
    status,
    detail,
    artifact,
  }) + '\n');
}

(async () => {
  const browser = await chromium.launch({ headless });
  const context = await browser.newContext({ viewport: { width: 1440, height: 900 } });
  await context.addInitScript((authToken) => {
    localStorage.setItem('traffic-ui-token', authToken);
    localStorage.setItem('traffic-ui-refresh-token', 'codex-playbook-state-refresh');
  }, token);
  const page = await context.newPage();
  const consoleErrors = [];
  const pageErrors = [];
  const requestFailures = [];
  const httpErrors = [];
  page.on('console', (msg) => {
    if (msg.type() === 'error') consoleErrors.push(msg.text());
  });
  page.on('pageerror', (error) => pageErrors.push(error.message));
  page.on('requestfailed', (request) => requestFailures.push(`${request.method()} ${request.url()} ${request.failure()?.errorText ?? ''}`));
  page.on('response', (response) => {
    if (response.status() >= 400) httpErrors.push(`${response.status()} ${response.url()}`);
  });

  try {
    await page.goto(new URL('/playbooks', baseURL).toString(), { waitUntil: 'domcontentloaded', timeout: 30000 });
    await page.waitForTimeout(2500);
    const body = await page.locator('body').innerText({ timeout: 15000 });
    await page.screenshot({ path: screenshotPath, fullPage: true });
    const ok = page.url().includes('/playbooks') &&
      body.includes('SOAR 剧本') &&
      body.includes('执行历史') &&
      body.includes('Playbook Catalog API') &&
      body.includes('Executions API');
    log('browser', 'playbooks page renders catalog and executions', ok ? 'info' : 'blocker', ok, ok ? 'ok' : 'missing-markers', JSON.stringify({ url: page.url(), hasScreenshot: fs.existsSync(screenshotPath) }), screenshotPath.split('/').pop());
    const clean = consoleErrors.length === 0 && pageErrors.length === 0 && requestFailures.length === 0 && httpErrors.length === 0;
    log('browser', 'playbooks page has no runtime errors', clean ? 'info' : 'blocker', clean, clean ? 'ok' : 'runtime-errors', JSON.stringify({ consoleErrors, pageErrors, requestFailures, httpErrors }));
  } catch (error) {
    log('browser', 'playbooks page renders catalog and executions', 'blocker', false, 'exception', error.stack || error.message);
  } finally {
    await browser.close();
  }
})();
JS

jq -s \
  --arg run_id "$RUN_ID" \
  --arg apisix "$APISIX" \
  --arg report "$REPORT" \
  --arg local_report "$LOCAL_REPORT" \
  '{
    run_id: $run_id,
    generated_at: (now | todateiso8601),
    apisix: $apisix,
    result: (if ([.[] | select(.severity == "blocker" and .passed == false)] | length) > 0 then "blocked" else "pass" end),
    total: length,
    passed: ([.[] | select(.passed == true)] | length),
    blockers: ([.[] | select(.severity == "blocker" and .passed == false)] | length),
    warnings: ([.[] | select(.severity == "warn" and .passed == false)] | length),
    report: $report,
    local_report: $local_report,
    checks: .
  }' "$REPORT" >"$SUMMARY"

RESULT="$(jq -r '.result' "$SUMMARY")"
PASSED="$(jq -r '.passed' "$SUMMARY")"
TOTAL="$(jq -r '.total' "$SUMMARY")"
BLOCKERS="$(jq -r '.blockers' "$SUMMARY")"
WARNINGS="$(jq -r '.warnings' "$SUMMARY")"

{
  echo "# Playbook 状态机 live 预检"
  echo
  echo "- Run ID：\`$RUN_ID\`"
  echo "- 结果：\`$RESULT\`"
  echo "- APISIX：\`$APISIX\`"
  echo "- 目标剧本：\`$PLAYBOOK_NAME\`"
  echo "- 检查数：$PASSED/$TOTAL passed，blockers=$BLOCKERS，warnings=$WARNINGS"
  echo
  echo "## 证据"
  echo
  echo "- NDJSON：\`$REPORT\`"
  echo "- Summary：\`$SUMMARY\`"
  echo "- 截图：\`$SCREENSHOT\`"
  echo "- API 执行：\`playbook-first-execute-response.json\`、\`playbook-second-execute-response.json\`"
  echo "- PostgreSQL：\`playbook-pg-row-count.txt\`"
  echo
  echo "## 口径"
  echo
  echo "本报告验证 SOAR 剧本目录、禁用门禁、手动执行 max_runs 状态机、执行记录落库、执行历史 API 和 /playbooks 前端消费链路。脚本会临时 PATCH \`$PLAYBOOK_NAME\` 并在退出时恢复原 enabled/max_runs/cooldown_seconds 配置。"
} >"$LOCAL_REPORT"

cp "$SUMMARY" "$REGRESSION_DIR/playbook-state-machine-latest.json"
cp "$LOCAL_REPORT" "$REGRESSION_DIR/playbook-state-machine-latest.md"

echo "playbook state-machine result: $RESULT"
echo "summary: $SUMMARY"
echo "local report: $LOCAL_REPORT"

if [[ "$RESULT" == "blocked" ]]; then
  exit 1
fi
