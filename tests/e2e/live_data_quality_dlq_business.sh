#!/usr/bin/env bash
set -euo pipefail

APISIX="${APISIX:-http://10.0.5.8:30180}"
TENANT="${TENANT:-tenant-a}"
LOG_DIR="${LOG_DIR:-.artifacts/e2e}"
RUN_ID="${RUN_ID:-$(date +%Y%m%d%H%M%S)-$$}"
KUBECTL="${KUBECTL:-kubectl}"
JWT_SECRET_NAMESPACE="${JWT_SECRET_NAMESPACE:-traffic-analysis}"
JWT_SECRET_NAME="${JWT_SECRET_NAME:-traffic-credentials}"
JWT_SECRET_KEY="${JWT_SECRET_KEY:-JWT_SECRET}"
PLAYWRIGHT_NODE_MODULE="${PLAYWRIGHT_NODE_MODULE:-./web/ui/node_modules/playwright}"
PLAYWRIGHT_HEADLESS="${PLAYWRIGHT_HEADLESS:-true}"

REPORT="$LOG_DIR/live-data-quality-dlq-business-$RUN_ID.ndjson"
SUMMARY="$LOG_DIR/live-data-quality-dlq-business-$RUN_ID-summary.json"
SCREENSHOT="$LOG_DIR/live-data-quality-dlq-business-$RUN_ID.png"
REPLAY_RESPONSE="$LOG_DIR/live-data-quality-dlq-business-$RUN_ID-replay-response.json"
mkdir -p "$LOG_DIR"

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
  local phase="$1" name="$2" ok="$3" status="$4" detail="${5:-}"
  jq -nc \
    --arg ts "$(date -Iseconds)" \
    --arg phase "$phase" \
    --arg name "$name" \
    --argjson ok "$ok" \
    --arg status "$status" \
    --arg detail "$detail" \
    '{ts:$ts, phase:$phase, name:$name, ok:$ok, status:$status, detail:$detail}' >>"$REPORT"
}

trim_file() {
  local file="$1"
  if [[ -s "$file" ]]; then
    head -c 700 "$file" | tr '\n' ' '
  fi
}

need_cmd curl
need_cmd jq
need_cmd python3
need_cmd node
need_cmd "$KUBECTL"

JWT_SECRET="$(kctl -n "$JWT_SECRET_NAMESPACE" get secret "$JWT_SECRET_NAME" -o "jsonpath={.data.$JWT_SECRET_KEY}" | base64 -d)"

make_token() {
  JWT_SECRET="$JWT_SECRET" TENANT="$TENANT" RUN_ID="$RUN_ID" python3 - <<'PY'
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
    "username": "codex-data-quality-operator",
    "email": "codex-data-quality-operator@local",
    "roles": ["admin"],
    "permissions": ["*", "admin:*", "dlq:replay"],
    "token_type": "access",
    "session_id": "codex-data-quality-dlq-" + os.environ["RUN_ID"],
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

TOKEN="$(make_token)"

curl_check() {
  local name="$1" method="$2" path="$3" expected="$4" data="${5:-}" filter="${6:-}"
  local body_file err_file code rc ok detail
  body_file="$(mktemp)"
  err_file="$(mktemp)"
  local args=(--noproxy '*' -sS -m 20 -o "$body_file" -w '%{http_code}' -X "$method" -H "X-Tenant-ID: $TENANT" -H "Authorization: Bearer $TOKEN")
  if [[ -n "$data" ]]; then
    args+=(-H "Content-Type: application/json" --data "$data")
  fi
  set +e
  code="$(curl "${args[@]}" "$APISIX$path" 2>"$err_file")"
  rc=$?
  set -e
  if [[ "$rc" -ne 0 ]]; then
    ok=false
    detail="curl rc=$rc err=$(trim_file "$err_file")"
  elif [[ "$code" != "$expected" ]]; then
    ok=false
    detail="expected=$expected actual=$code body=$(trim_file "$body_file")"
  elif [[ -n "$filter" ]] && ! jq -e "$filter" "$body_file" >/dev/null 2>"$err_file"; then
    ok=false
    detail="jq filter failed filter=$filter body=$(trim_file "$body_file") err=$(trim_file "$err_file")"
  else
    ok=true
    detail="$method $path"
  fi
  json_log "api" "$name" "$ok" "$code" "$detail"
  rm -f "$body_file" "$err_file"
  [[ "$ok" == true ]]
}

curl_check "auth/me exposes dlq replay user token" "GET" "/api/v1/auth/me" "200" "" '((.permissions // []) | index("dlq:replay") != null) or ((.permissions // []) | index("*") != null)'
curl_check "data-quality API reachable" "GET" "/api/v1/data-quality" "200" "" '.success == true'

TOKEN="$TOKEN" APISIX="$APISIX" TENANT="$TENANT" RUN_ID="$RUN_ID" REPORT="$REPORT" SUMMARY="$SUMMARY" SCREENSHOT="$SCREENSHOT" REPLAY_RESPONSE="$REPLAY_RESPONSE" PLAYWRIGHT_NODE_MODULE="$PLAYWRIGHT_NODE_MODULE" PLAYWRIGHT_HEADLESS="$PLAYWRIGHT_HEADLESS" node <<'JS'
const fs = require('node:fs');
const { chromium } = require(process.env.PLAYWRIGHT_NODE_MODULE);

const baseURL = process.env.APISIX;
const reportPath = process.env.REPORT;
const screenshotPath = process.env.SCREENSHOT;
const replayResponsePath = process.env.REPLAY_RESPONSE;
const token = process.env.TOKEN;
const tenant = process.env.TENANT;
const runId = process.env.RUN_ID;
const headless = process.env.PLAYWRIGHT_HEADLESS !== 'false';

function log(phase, name, ok, status, detail) {
  fs.appendFileSync(reportPath, JSON.stringify({
    ts: new Date().toISOString(),
    phase,
    name,
    ok,
    status,
    detail,
  }) + '\n');
}

function redact(value) {
  return String(value ?? '')
    .replace(/token=[^&" ]+/g, 'token=<redacted>')
    .replace(/Bearer [A-Za-z0-9._-]+/g, 'Bearer <redacted>');
}

(async () => {
  const browser = await chromium.launch({ headless });
  const context = await browser.newContext({ viewport: { width: 1440, height: 900 } });
  await context.addInitScript((authToken) => {
    localStorage.setItem('traffic-ui-token', authToken);
    localStorage.setItem('traffic-ui-refresh-token', 'codex-data-quality-dlq-business-refresh');
  }, token);

  const page = await context.newPage();
  const consoleErrors = [];
  const pageErrors = [];
  const requestFailures = [];
  const httpErrors = [];

  page.on('console', (msg) => {
    if (msg.type() === 'error') consoleErrors.push(redact(msg.text()));
  });
  page.on('pageerror', (error) => pageErrors.push(redact(error.message)));
  page.on('requestfailed', (request) => requestFailures.push(redact(`${request.method()} ${request.url()} ${request.failure()?.errorText ?? ''}`)));
  page.on('response', (response) => {
    if (response.status() >= 400) httpErrors.push(redact(`${response.status()} ${response.url()}`));
  });

  const detail = {};
  try {
    await page.goto(new URL('/data-quality?tab=replay-reconcile', baseURL).toString(), { waitUntil: 'domcontentloaded', timeout: 30_000 });
    await page.waitForTimeout(2500);

    const bodyBefore = await page.locator('body').innerText({ timeout: 15_000 });
    const routeOk = /\/data-quality(\?|$)/.test(new URL(page.url()).pathname + new URL(page.url()).search);
    const initialContains = {
      dataQuality: bodyBefore.includes('数据质量'),
      replayQueue: bodyBefore.includes('DLQ 重放队列'),
      replayContract: bodyBefore.includes('DLQ Replay API 契约'),
    };
    log('browser', 'data-quality replay tab renders', routeOk && Object.values(initialContains).every(Boolean), routeOk ? 'ok' : 'bad-url', JSON.stringify({ url: page.url(), initialContains }));

    await page.getByRole('button', { name: /Dry-run/ }).first().click();
    await page.getByText('DLQ fallback replay dry-run', { exact: true }).waitFor({ timeout: 10_000 });

    const approvalId = `APPROVAL-${runId}-DQ-DLQ`;
    const idempotencyKey = `${tenant}:${approvalId}:dry-run`;
    await page.getByLabel('审批单号').fill(approvalId);
    await page.getByLabel('幂等键').fill(idempotencyKey);
    await page.getByLabel('重放原因').fill(`Codex live business page dry-run ${runId}`);
    await page.getByLabel('修复摘要').fill(`Codex verified DataQuality modal and backend JWT replay contract ${runId}`);

    const replayResponsePromise = page.waitForResponse((response) =>
      response.url().includes('/api/v1/dlq/replay/fallback') &&
      response.request().method() === 'POST',
      { timeout: 30_000 },
    );
    await page.getByRole('button', { name: '执行 dry-run 预检' }).click();
    const replayResponse = await replayResponsePromise;
    const replayBody = await replayResponse.json().catch(async () => ({ raw: await replayResponse.text().catch(() => '') }));
    fs.writeFileSync(replayResponsePath, JSON.stringify(replayBody, null, 2));
    await page.getByText(/Replay dry_run|Replay completed|Replay partial/).waitFor({ timeout: 15_000 });

    const bodyAfter = await page.locator('body').innerText({ timeout: 10_000 });
    await page.screenshot({ path: screenshotPath, fullPage: true });

    detail.url = page.url();
    detail.replayStatus = replayResponse.status();
    detail.replayBody = replayBody;
    detail.contains = {
      modalTitle: bodyAfter.includes('DLQ fallback replay dry-run'),
      dryRunResult: bodyAfter.includes('Replay dry_run'),
      replayId: bodyAfter.includes('replay_id='),
    };
    detail.consoleErrors = consoleErrors;
    detail.pageErrors = pageErrors;
    detail.requestFailures = requestFailures;
    detail.httpErrors = httpErrors;

    const ok = routeOk &&
      replayResponse.status() === 200 &&
      replayBody.status === 'dry_run' &&
      Object.values(detail.contains).every(Boolean) &&
      consoleErrors.length === 0 &&
      pageErrors.length === 0 &&
      requestFailures.length === 0 &&
      httpErrors.length === 0;
    log('browser', 'data-quality DLQ modal dry-run submit', ok, ok ? 'ok' : 'failed', JSON.stringify(detail));
    if (!ok) throw new Error(JSON.stringify(detail));
  } finally {
    await context.close();
    await browser.close();
  }
})().catch((error) => {
  log('browser', 'data-quality DLQ modal dry-run submit', false, 'exception', redact(error.stack || error.message));
  process.exit(1);
});
JS

total="$(wc -l <"$REPORT" | tr -d ' ')"
failed="$(jq -s '[.[] | select(.ok == false)] | length' "$REPORT")"
passed="$((total - failed))"
jq -s \
  --arg run_id "$RUN_ID" \
  --arg apisix "$APISIX" \
  --arg tenant "$TENANT" \
  --arg report "$REPORT" \
  --arg summary "$SUMMARY" \
  --arg screenshot "$SCREENSHOT" \
  --arg replay_response "$REPLAY_RESPONSE" \
  --argjson total "$total" \
  --argjson passed "$passed" \
  --argjson failed "$failed" \
  '{run_id:$run_id, apisix:$apisix, tenant:$tenant, report:$report, summary:$summary, screenshot:$screenshot, replay_response:$replay_response, total:$total, passed:$passed, failed:$failed, checks:.}' \
  "$REPORT" >"$SUMMARY"

cat "$SUMMARY"
if [[ "$failed" -ne 0 ]]; then
  exit 1
fi
