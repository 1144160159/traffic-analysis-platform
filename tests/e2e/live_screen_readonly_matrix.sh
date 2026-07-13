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

REPORT="$LOG_DIR/live-screen-readonly-matrix-$RUN_ID.ndjson"
SUMMARY="$LOG_DIR/live-screen-readonly-matrix-$RUN_ID-summary.json"
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
    head -c 500 "$file" | tr '\n' ' '
  fi
}

need_cmd curl
need_cmd jq
need_cmd python3
need_cmd node
need_cmd "$KUBECTL"

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
    "session_id": "codex-screen-readonly-" + str(uuid.uuid4()),
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

ADMIN_TOKEN="$(make_token codex-screen-admin '["admin"]' '["*","admin:*","token:read"]')"
SCREEN_TOKEN="$(make_token codex-screen-readonly '["screen-viewer"]' '["screen:view"]')"

curl_check() {
  local name="$1" method="$2" path="$3" mode="$4" expected="$5" data="${6:-}" filter="${7:-}"
  local body_file err_file code rc detail ok
  body_file="$(mktemp)"
  err_file="$(mktemp)"
  local args=(--noproxy '*' -sS -m 15 -o "$body_file" -w '%{http_code}' -X "$method" -H "X-Tenant-ID: $TENANT")
  case "$mode" in
    admin) args+=(-H "Authorization: Bearer $ADMIN_TOKEN") ;;
    screen) args+=(-H "Authorization: Bearer $SCREEN_TOKEN") ;;
    none) ;;
    *) echo "unknown auth mode: $mode" >&2; return 2 ;;
  esac
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

curl_check "scope catalog includes screen:view" "GET" "/api/v1/tokens/scopes" "admin" "200" "" '(.scopes // []) | map(.name) | index("screen:view") != null'
curl_check "screen token auth/me exposes only screen scope" "GET" "/api/v1/auth/me" "screen" "200" "" '((.permissions // []) | index("screen:view") != null) and (((.permissions // []) | index("alert:write")) == null) and (((.permissions // []) | index("token:read")) == null)'
curl_check "screen token can read dashboard stats for screen data" "GET" "/api/v1/dashboard/stats" "screen" "200" "" '.success == true'
curl_check "screen token cannot list API tokens" "GET" "/api/v1/tokens" "screen" "403" "" ""

ALERT_ID="$(curl --noproxy '*' -sS -m 15 -H "Authorization: Bearer $ADMIN_TOKEN" -H "X-Tenant-ID: $TENANT" "$APISIX/api/v1/alerts?limit=1" | jq -r '.data[0].alert_id // empty')"
if [[ -z "$ALERT_ID" ]]; then
  json_log "api" "load alert id for readonly write-deny check" false "000" "no live alert available"
  echo "no live alert available for screen readonly matrix" >&2
  exit 1
fi
curl_check "screen token cannot update alert status" "PUT" "/api/v1/alerts/$ALERT_ID/status" "screen" "403" '{"status":"assigned","reason":"screen readonly token should be denied"}' '(.message // "") | contains("alert:write")'

SCREEN_TOKEN="$SCREEN_TOKEN" APISIX="$APISIX" TENANT="$TENANT" REPORT="$REPORT" PLAYWRIGHT_NODE_MODULE="$PLAYWRIGHT_NODE_MODULE" PLAYWRIGHT_HEADLESS="$PLAYWRIGHT_HEADLESS" node <<'JS'
const fs = require('node:fs');
const { chromium } = require(process.env.PLAYWRIGHT_NODE_MODULE);

const baseURL = process.env.APISIX;
const reportPath = process.env.REPORT;
const headless = process.env.PLAYWRIGHT_HEADLESS !== 'false';

function sanitize(value) {
  return String(value ?? '').replace(/token=[^&" ]+/g, 'token=<redacted>');
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

async function newScreenContext(browser) {
  const context = await browser.newContext({ viewport: { width: 1440, height: 900 } });
  await context.addInitScript((token) => {
    localStorage.setItem('traffic-ui-token', token);
    localStorage.setItem('traffic-ui-refresh-token', 'codex-screen-readonly-refresh');
  }, process.env.SCREEN_TOKEN);
  return context;
}

async function runCase(browser, testCase) {
  const context = await newScreenContext(browser);
  const page = await context.newPage();
  const consoleErrors = [];
  const pageErrors = [];
  const requestFailures = [];
  const serverErrors = [];
  page.on('console', (msg) => {
    if (msg.type() === 'error') consoleErrors.push(msg.text());
  });
  page.on('pageerror', (error) => pageErrors.push(error.message));
  page.on('requestfailed', (request) => requestFailures.push(`${request.method()} ${request.url()} ${request.failure()?.errorText ?? ''}`));
  page.on('response', (response) => {
    if (response.status() >= 500) serverErrors.push(`${response.status()} ${response.url()}`);
  });

  let ok = false;
  let detail = {};
  try {
    await page.goto(new URL(testCase.path, baseURL).toString(), { waitUntil: 'domcontentloaded', timeout: 30_000 });
    await page.waitForTimeout(testCase.waitMs ?? 3000);
    const body = await page.locator('body').innerText({ timeout: 10_000 }).catch((error) => `BODY_READ_ERROR: ${error.message}`);
    const contains = Object.fromEntries((testCase.contains ?? []).map((text) => [text, body.includes(text)]));
    const excludes = Object.fromEntries((testCase.excludes ?? []).map((text) => [text, !body.includes(text)]));
    ok = Object.values(contains).every(Boolean) && Object.values(excludes).every(Boolean) &&
      consoleErrors.length === 0 && pageErrors.length === 0 && requestFailures.length === 0 && serverErrors.length === 0;
    detail = { url: page.url(), contains, excludes, consoleErrors, pageErrors, requestFailures, serverErrors };
  } catch (error) {
    ok = false;
    detail = { error: error.message, consoleErrors, pageErrors, requestFailures, serverErrors };
  } finally {
    await context.close();
  }
  log(testCase.name, ok, ok ? 'ok' : 'failed', detail);
  if (!ok) throw new Error(`${testCase.name} failed: ${JSON.stringify(detail)}`);
}

(async () => {
  const browser = await chromium.launch({ headless });
  const cases = [
    {
      name: 'screen readonly token renders screen route',
      path: '/screen',
      contains: ['园区数字孪生拓扑', '真实 API'],
      excludes: ['权限不足', '脱敏公开演示'],
      waitMs: 3500,
    },
    {
      name: 'screen readonly token is denied settings route',
      path: '/settings',
      contains: ['权限不足', '系统设置', 'admin:*', 'token:read'],
      waitMs: 2500,
    },
    {
      name: 'screen readonly token hides write-oriented navigation',
      path: '/screen',
      contains: ['态势大屏'],
      excludes: ['系统设置', '规则管理', '模型管理'],
      waitMs: 2500,
    },
  ];
  try {
    for (const testCase of cases) await runCase(browser, testCase);
  } finally {
    await browser.close();
  }
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
