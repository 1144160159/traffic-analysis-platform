#!/usr/bin/env bash
set -euo pipefail

APISIX="${APISIX:-http://10.0.5.8:30180}"
TENANT="${TENANT:-default}"
LOG_DIR="${LOG_DIR:-.artifacts/e2e}"
RUN_ID="${RUN_ID:-$(date +%Y%m%d%H%M%S)-$$}"
KUBECTL="${KUBECTL:-kubectl}"
JWT_SECRET_NAMESPACE="${JWT_SECRET_NAMESPACE:-traffic-analysis}"
JWT_SECRET_NAME="${JWT_SECRET_NAME:-traffic-credentials}"
JWT_SECRET_KEY="${JWT_SECRET_KEY:-JWT_SECRET}"
PLAYWRIGHT_NODE_MODULE="${PLAYWRIGHT_NODE_MODULE:-./web/ui/node_modules/playwright}"
PLAYWRIGHT_HEADLESS="${PLAYWRIGHT_HEADLESS:-true}"

REPORT="$LOG_DIR/live-realtime-appshell-$RUN_ID.ndjson"
SUMMARY="$LOG_DIR/live-realtime-appshell-$RUN_ID-summary.json"
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

redact() {
  sed -E 's/token=[^&" ]+/token=<redacted>/g; s/Bearer [A-Za-z0-9._-]+/Bearer <redacted>/g'
}

need_cmd curl
need_cmd jq
need_cmd python3
need_cmd node
need_cmd "$KUBECTL"

CONFIG_JS="$(curl --noproxy '*' -sS -m 15 "$APISIX/config.js" | redact)"
if grep -q 'ENABLE_REALTIME: "true"' <<<"$CONFIG_JS"; then
  json_log "runtime" "config.js enables realtime" true "ok" "ENABLE_REALTIME=true"
else
  json_log "runtime" "config.js enables realtime" false "failed" "$CONFIG_JS"
fi

JWT_SECRET="$(kctl -n "$JWT_SECRET_NAMESPACE" get secret "$JWT_SECRET_NAME" -o "jsonpath={.data.$JWT_SECRET_KEY}" | base64 -d)"

make_token() {
  local username="$1"
  local roles_json="$2"
  local perms_json="$3"
  JWT_SECRET="$JWT_SECRET" TENANT="$TENANT" USERNAME="$username" ROLES_JSON="$roles_json" PERMS_JSON="$perms_json" python3 - <<'PY'
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
    "email": f"{os.environ['USERNAME']}@local",
    "roles": json.loads(os.environ["ROLES_JSON"]),
    "permissions": json.loads(os.environ["PERMS_JSON"]),
    "token_type": "access",
    "session_id": "codex-realtime-appshell-" + str(uuid.uuid4()),
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

ADMIN_TOKEN="$(make_token codex-realtime-visual '["admin"]' '["*","admin:*"]')"

ADMIN_TOKEN="$ADMIN_TOKEN" APISIX="$APISIX" TENANT="$TENANT" REPORT="$REPORT" PLAYWRIGHT_NODE_MODULE="$PLAYWRIGHT_NODE_MODULE" PLAYWRIGHT_HEADLESS="$PLAYWRIGHT_HEADLESS" node <<'JS'
const fs = require('node:fs');
const { chromium } = require(process.env.PLAYWRIGHT_NODE_MODULE);

const baseURL = process.env.APISIX;
const tenant = process.env.TENANT;
const reportPath = process.env.REPORT;
const headless = process.env.PLAYWRIGHT_HEADLESS !== 'false';

function sanitize(value) {
  return String(value ?? '')
    .replace(/token=[^&" ]+/g, 'token=<redacted>')
    .replace(/Bearer [A-Za-z0-9._-]+/g, 'Bearer <redacted>');
}

function log(name, ok, status, detail) {
  fs.appendFileSync(reportPath, JSON.stringify({
    ts: new Date().toISOString(),
    phase: 'browser',
    name,
    ok,
    status,
    detail: sanitize(typeof detail === 'string' ? detail : JSON.stringify(detail)),
  }) + '\n');
}

(async () => {
  const browser = await chromium.launch({ headless });
  const context = await browser.newContext({ viewport: { width: 1440, height: 900 } });
  await context.addInitScript((token) => {
    localStorage.setItem('traffic-ui-token', token);
    localStorage.setItem('traffic-ui-refresh-token', 'codex-realtime-appshell-refresh');
  }, process.env.ADMIN_TOKEN);

  const page = await context.newPage();
  const consoleErrors = [];
  const pageErrors = [];
  const requestFailures = [];
  const serverErrors = [];
  const sockets = [];
  const socketFrames = [];

  page.on('console', (msg) => {
    if (msg.type() === 'error') consoleErrors.push(sanitize(msg.text()));
  });
  page.on('pageerror', (error) => pageErrors.push(sanitize(error.message)));
  page.on('requestfailed', (request) => requestFailures.push(sanitize(`${request.method()} ${request.url()} ${request.failure()?.errorText ?? ''}`)));
  page.on('response', (response) => {
    if (response.status() >= 500) serverErrors.push(`${response.status()} ${sanitize(response.url())}`);
  });
  page.on('websocket', (ws) => {
    const parsed = new URL(ws.url());
    sockets.push({
      protocol: parsed.protocol,
      path: parsed.pathname,
      hasToken: parsed.searchParams.has('token'),
      tenantId: parsed.searchParams.get('tenant_id'),
    });
    ws.on('framereceived', (event) => {
      try {
        const parsedFrame = JSON.parse(event.payload);
        socketFrames.push({
          type: parsedFrame.type,
          tenant_id: parsedFrame.tenant_id,
          username: parsedFrame.username,
        });
      } catch {
        socketFrames.push({ type: 'non-json' });
      }
    });
  });

  let ok = false;
  let detail = {};
  try {
    await page.goto(new URL('/dashboard', baseURL).toString(), { waitUntil: 'domcontentloaded', timeout: 30_000 });
    await page.waitForFunction(() => {
      const text = document.body.innerText;
      return text.includes('实时通道') && text.includes('已连接');
    }, { timeout: 20_000 });
    await page.waitForTimeout(1000);
    const body = await page.locator('body').innerText({ timeout: 10_000 });
    const wsReady = socketFrames.some((frame) => frame.type === 'ready' && frame.tenant_id === tenant);
    const wsEndpoint = sockets.some((socket) => socket.path === '/ws/events' && socket.hasToken && socket.tenantId === tenant);
    const hasDashboard = body.includes('仪表盘');
    const hasRealtimeConnected = body.includes('实时通道') && body.includes('已连接');
    ok = wsReady && wsEndpoint && hasDashboard && hasRealtimeConnected &&
      consoleErrors.length === 0 && pageErrors.length === 0 && requestFailures.length === 0 && serverErrors.length === 0;
    detail = {
      url: page.url(),
      hasDashboard,
      hasRealtimeConnected,
      wsEndpoint,
      wsReady,
      sockets,
      socketFrames,
      consoleErrors,
      pageErrors,
      requestFailures,
      serverErrors,
    };
  } catch (error) {
    ok = false;
    detail = {
      error: sanitize(error.message),
      sockets,
      socketFrames,
      consoleErrors,
      pageErrors,
      requestFailures,
      serverErrors,
    };
  } finally {
    await context.close();
    await browser.close();
  }

  log('authorized dashboard opens realtime websocket', ok, ok ? 'ok' : 'failed', detail);
  if (!ok) process.exit(1);
})().catch((error) => {
  console.error(sanitize(error.message));
  process.exit(1);
});
JS

total="$(wc -l <"$REPORT" | tr -d ' ')"
failed="$(jq -s '[.[] | select(.ok == false)] | length' "$REPORT")"
passed="$((total - failed))"
jq -s \
  --arg run_id "$RUN_ID" \
  --arg apisix "$APISIX" \
  --arg report "$REPORT" \
  --argjson total "$total" \
  --argjson passed "$passed" \
  --argjson failed "$failed" \
  '{run_id:$run_id, apisix:$apisix, report:$report, total:$total, passed:$passed, failed:$failed, checks:.}' \
  "$REPORT" >"$SUMMARY"

cat "$SUMMARY"
if [[ "$failed" -ne 0 ]]; then
  exit 1
fi
