#!/usr/bin/env bash
set -euo pipefail

APISIX="${APISIX:-http://10.0.5.8:30180}"
TENANT="${TENANT:-default}"
LOOKBACK_MINUTES="${LOOKBACK_MINUTES:-1440}"
LOG_DIR="${LOG_DIR:-.artifacts/e2e}"
RUN_ID="${RUN_ID:-$(date +%Y%m%d%H%M%S)-$$}"
KUBECTL="${KUBECTL:-kubectl}"
JWT_SECRET_NAMESPACE="${JWT_SECRET_NAMESPACE:-traffic-analysis}"
JWT_SECRET_NAME="${JWT_SECRET_NAME:-traffic-credentials}"
JWT_SECRET_KEY="${JWT_SECRET_KEY:-JWT_SECRET}"
CH_NAMESPACE="${CH_NAMESPACE:-middleware}"
CH_POD="${CH_POD:-clickhouse-1-0}"
PLAYWRIGHT_NODE_MODULE="${PLAYWRIGHT_NODE_MODULE:-./web/ui/node_modules/playwright}"
PLAYWRIGHT_HEADLESS="${PLAYWRIGHT_HEADLESS:-true}"
FAIL_ON_GAP="${FAIL_ON_GAP:-0}"

REPORT="$LOG_DIR/live-latency-chain-$RUN_ID.ndjson"
SUMMARY="$LOG_DIR/live-latency-chain-$RUN_ID-summary.json"
LATENCY_API_FILE="$LOG_DIR/$RUN_ID-latency-chain-api.json"
DATA_QUALITY_FILE="$LOG_DIR/$RUN_ID-data-quality-api.json"
CH_COLUMNS_FILE="$LOG_DIR/$RUN_ID-clickhouse-latency-columns.jsonl"
CH_FLOWS_FILE="$LOG_DIR/$RUN_ID-clickhouse-flows-latency.jsonl"
CH_SESSIONS_FILE="$LOG_DIR/$RUN_ID-clickhouse-sessions-latency.jsonl"
CH_ALERTS_FILE="$LOG_DIR/$RUN_ID-clickhouse-alerts-latency.jsonl"
UI_FILE="$LOG_DIR/$RUN_ID-ui-data-quality.json"
UI_SCREENSHOT="$LOG_DIR/$RUN_ID-ui-data-quality.png"

FAILURES=0
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
  if [[ "$ok" != "true" ]]; then
    FAILURES=$((FAILURES + 1))
  fi
}

trim_file() {
  local file="$1"
  if [[ -s "$file" ]]; then
    head -c 500 "$file" | tr '\n' ' '
  fi
}

sql_escape() {
  printf "%s" "$1" | sed "s/'/''/g"
}

make_token() {
  JWT_SECRET="$JWT_SECRET" TENANT="$TENANT" python3 - <<'PY'
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
    "username": "codex-latency-chain",
    "email": "codex-latency-chain@local",
    "roles": ["admin"],
    "permissions": ["*", "admin:*", "alert:read", "audit:read", "screen:view"],
    "token_type": "access",
    "session_id": "codex-latency-chain-" + str(uuid.uuid4()),
    "iat": now,
    "exp": now + 3600,
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
  local name="$1" path="$2" outfile="$3" filter="${4:-.success == true}"
  local err_file curl_out rc code latency api_seen ok detail

  err_file="$(mktemp)"
  set +e
  curl_out="$(curl --noproxy '*' -sS -m 20 -o "$outfile" -w '%{http_code} %{time_total}' \
    -H "Authorization: Bearer $TOKEN" \
    -H "X-Tenant-ID: $TENANT" \
    -H "X-Request-ID: latency-chain-$RUN_ID-$name" \
    "$APISIX$path" 2>"$err_file")"
  rc=$?
  set -e
  api_seen="$(date +%s%3N)"
  printf "%s\n" "$api_seen" >"$outfile.api_seen_ts"

  if [[ "$rc" -ne 0 ]]; then
    ok=false
    code="000"
    latency="0"
    detail="curl rc=$rc err=$(trim_file "$err_file")"
  else
    read -r code latency <<<"$curl_out"
    printf "%s\n" "$latency" >"$outfile.latency_seconds"
    if [[ "$code" == "200" ]] && jq -e "$filter" "$outfile" >/dev/null 2>"$err_file"; then
      ok=true
      detail="$path api_seen_ts=$api_seen latency_seconds=$latency"
    else
      ok=false
      detail="code=$code path=$path body=$(trim_file "$outfile") err=$(trim_file "$err_file")"
    fi
  fi

  json_log "api" "$name" "$ok" "$code" "$detail"
  rm -f "$err_file"
}

ch_query() {
  local name="$1" query="$2" outfile="$3"
  local err_file rc ok detail

  err_file="$(mktemp)"
  set +e
  kctl -n "$CH_NAMESPACE" exec "$CH_POD" -c clickhouse -- clickhouse-client --query "$query" >"$outfile" 2>"$err_file"
  rc=$?
  set -e
  if [[ "$rc" -eq 0 ]]; then
    ok=true
    detail="$(trim_file "$outfile")"
  else
    ok=false
    detail="$(trim_file "$err_file")"
  fi
  json_log "clickhouse" "$name" "$ok" "$rc" "$detail"
  rm -f "$err_file"
}

need_cmd curl
need_cmd jq
need_cmd python3
need_cmd node
need_cmd "$KUBECTL"

JWT_SECRET="$(kctl -n "$JWT_SECRET_NAMESPACE" get secret "$JWT_SECRET_NAME" -o "jsonpath={.data.$JWT_SECRET_KEY}" | base64 -d)"
TOKEN="$(make_token)"
TENANT_SQL="$(sql_escape "$TENANT")"

ch_query "latency-chain-columns" \
  "SELECT table, groupArray(name) AS columns FROM system.columns WHERE database='traffic' AND table IN ('flows_raw','sessions','alerts','evidence') AND name IN ('event_ts','ingest_ts','kafka_ts','flink_out_ts','api_seen_ts','ui_seen_ts','first_seen','created_at','last_seen') GROUP BY table ORDER BY table FORMAT JSONEachRow" \
  "$CH_COLUMNS_FILE"

ch_query "flows-raw-event-to-ingest" \
  "SELECT count() AS sample_count, quantile(0.50)(toFloat64(greatest(ingest_ts-event_ts,0))) AS p50_ms, quantile(0.90)(toFloat64(greatest(ingest_ts-event_ts,0))) AS p90_ms, quantile(0.95)(toFloat64(greatest(ingest_ts-event_ts,0))) AS p95_ms, quantile(0.99)(toFloat64(greatest(ingest_ts-event_ts,0))) AS p99_ms FROM traffic.flows_raw WHERE tenant_id='$TENANT_SQL' AND ingest_ts>0 AND event_ts>0 AND ingest_ts >= toUnixTimestamp64Milli(now64(3) - INTERVAL $LOOKBACK_MINUTES MINUTE) FORMAT JSONEachRow" \
  "$CH_FLOWS_FILE"

ch_query "sessions-event-to-ingest" \
  "SELECT count() AS sample_count, quantile(0.50)(toFloat64(greatest(ingest_ts-event_ts,0))) AS p50_ms, quantile(0.90)(toFloat64(greatest(ingest_ts-event_ts,0))) AS p90_ms, quantile(0.95)(toFloat64(greatest(ingest_ts-event_ts,0))) AS p95_ms, quantile(0.99)(toFloat64(greatest(ingest_ts-event_ts,0))) AS p99_ms FROM traffic.sessions WHERE tenant_id='$TENANT_SQL' AND ingest_ts>0 AND event_ts>0 AND ingest_ts >= toUnixTimestamp64Milli(now64(3) - INTERVAL $LOOKBACK_MINUTES MINUTE) FORMAT JSONEachRow" \
  "$CH_SESSIONS_FILE"

ch_query "alerts-last-seen-to-created" \
  "SELECT count() AS sample_count, quantile(0.50)(toFloat64(greatest(created_at-last_seen,0))) AS p50_ms, quantile(0.90)(toFloat64(greatest(created_at-last_seen,0))) AS p90_ms, quantile(0.95)(toFloat64(greatest(created_at-last_seen,0))) AS p95_ms, quantile(0.99)(toFloat64(greatest(created_at-last_seen,0))) AS p99_ms FROM traffic.alerts WHERE tenant_id='$TENANT_SQL' AND created_at>0 AND last_seen>0 AND last_seen >= toUnixTimestamp64Milli(now64(3) - INTERVAL $LOOKBACK_MINUTES MINUTE) FORMAT JSONEachRow" \
  "$CH_ALERTS_FILE"

curl_json "latency-chain" "/api/v1/data-quality/latency-chain?lookback_minutes=$LOOKBACK_MINUTES" "$LATENCY_API_FILE"
curl_json "data-quality" "/api/v1/data-quality" "$DATA_QUALITY_FILE"

set +e
TOKEN="$TOKEN" APISIX="$APISIX" TENANT="$TENANT" UI_FILE="$UI_FILE" UI_SCREENSHOT="$UI_SCREENSHOT" \
  PLAYWRIGHT_NODE_MODULE="$PLAYWRIGHT_NODE_MODULE" PLAYWRIGHT_HEADLESS="$PLAYWRIGHT_HEADLESS" node <<'JS'
const fs = require('node:fs');
const { chromium } = require(process.env.PLAYWRIGHT_NODE_MODULE);

const baseURL = process.env.APISIX;
const token = process.env.TOKEN;
const uiFile = process.env.UI_FILE;
const screenshotPath = process.env.UI_SCREENSHOT;
const headless = process.env.PLAYWRIGHT_HEADLESS !== 'false';

(async () => {
  const browser = await chromium.launch({ headless });
  const context = await browser.newContext({ viewport: { width: 1440, height: 900 } });
  await context.addInitScript((authToken) => {
    localStorage.setItem('traffic-ui-token', authToken);
    localStorage.setItem('traffic-ui-refresh-token', 'codex-latency-chain-refresh');
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

  const started = Date.now();
  let result;
  try {
    await page.goto(new URL('/data-quality?tab=topic-health', baseURL).toString(), { waitUntil: 'domcontentloaded', timeout: 30_000 });
    await page.waitForTimeout(3500);
    const body = await page.locator('body').innerText({ timeout: 10_000 });
    const perf = await page.evaluate(() => {
      const nav = performance.getEntriesByType('navigation')[0];
      return {
        navigation_duration_ms: nav ? nav.duration : 0,
        dom_content_loaded_ms: nav ? nav.domContentLoadedEventEnd : 0,
        load_event_ms: nav ? nav.loadEventEnd : 0,
      };
    });
    await page.screenshot({ path: screenshotPath, fullPage: true });
    const uiSeenTs = Date.now();
    const contains = {
      dataQuality: body.includes('数据质量'),
      p95: body.includes('P95'),
      topicHealth: body.includes('Topic'),
    };
    const ok = Object.values(contains).every(Boolean) &&
      consoleErrors.length === 0 &&
      pageErrors.length === 0 &&
      requestFailures.length === 0 &&
      httpErrors.length === 0;
    result = {
      ok,
      path: '/data-quality?tab=topic-health',
      url: page.url(),
      tenant: process.env.TENANT,
      ui_seen_ts: uiSeenTs,
      wall_latency_ms: uiSeenTs - started,
      contains,
      perf,
      consoleErrors,
      pageErrors,
      requestFailures,
      httpErrors,
      screenshot: screenshotPath,
    };
  } catch (error) {
    result = {
      ok: false,
      path: '/data-quality?tab=topic-health',
      tenant: process.env.TENANT,
      ui_seen_ts: Date.now(),
      error: error.message,
      consoleErrors,
      pageErrors,
      requestFailures,
      httpErrors,
      screenshot: screenshotPath,
    };
  } finally {
    await context.close();
    await browser.close();
  }
  fs.writeFileSync(uiFile, JSON.stringify(result, null, 2));
  if (!result.ok) process.exit(1);
})().catch((error) => {
  fs.writeFileSync(uiFile, JSON.stringify({ ok: false, error: error.message, ui_seen_ts: Date.now() }, null, 2));
  process.exit(1);
});
JS
ui_rc=$?
set -e
if [[ "$ui_rc" -eq 0 ]]; then
  json_log "browser" "data-quality-ui-seen" true "ok" "$(trim_file "$UI_FILE")"
else
  json_log "browser" "data-quality-ui-seen" false "$ui_rc" "$(trim_file "$UI_FILE")"
fi

SUMMARY="$SUMMARY" REPORT="$REPORT" RUN_ID="$RUN_ID" TENANT="$TENANT" APISIX="$APISIX" LOOKBACK_MINUTES="$LOOKBACK_MINUTES" \
  LATENCY_API_FILE="$LATENCY_API_FILE" DATA_QUALITY_FILE="$DATA_QUALITY_FILE" UI_FILE="$UI_FILE" UI_SCREENSHOT="$UI_SCREENSHOT" \
  CH_COLUMNS_FILE="$CH_COLUMNS_FILE" CH_FLOWS_FILE="$CH_FLOWS_FILE" CH_SESSIONS_FILE="$CH_SESSIONS_FILE" CH_ALERTS_FILE="$CH_ALERTS_FILE" \
  FAILURES="$FAILURES" python3 - <<'PY'
import json
import os
from pathlib import Path

def read_json(path):
    try:
        return json.loads(Path(path).read_text())
    except Exception as exc:
        return {"_error": str(exc), "_path": path}

def read_jsonl(path):
    rows = []
    try:
        for line in Path(path).read_text().splitlines():
            if line.strip():
                rows.append(json.loads(line))
    except Exception as exc:
        rows.append({"_error": str(exc), "_path": path})
    return rows

latency_api = read_json(os.environ["LATENCY_API_FILE"])
data_quality = read_json(os.environ["DATA_QUALITY_FILE"])
ui = read_json(os.environ["UI_FILE"])
latency_data = latency_api.get("data") if isinstance(latency_api.get("data"), dict) else {}
stages = list(latency_data.get("stages") or [])
if ui.get("ok"):
    for stage in stages:
        if stage.get("name") == "ui_seen_ts":
            stage["status"] = "measured"
            stage["source"] = "/data-quality?tab=topic-health"
            stage["detail"] = str(ui.get("ui_seen_ts"))

gaps = list(latency_data.get("gaps") or [])
if ui.get("ok"):
    gaps = [gap for gap in gaps if not str(gap).startswith("ui_seen_ts is missing")]
else:
    gaps.append("ui_seen_ts browser measurement failed")
if any(stage.get("status") == "missing" for stage in stages):
    for stage in stages:
        if stage.get("status") == "missing":
            msg = f"{stage.get('name')} is missing ({stage.get('source')})"
            if msg not in gaps:
                gaps.append(msg)

segments = list(latency_data.get("segments") or [])
has_segment_gap = any(segment.get("status") == "gap" for segment in segments)
has_segment_fail = any(segment.get("status") == "fail" for segment in segments)
if gaps or has_segment_gap:
    result = "gap"
elif has_segment_fail:
    result = "fail"
else:
    result = "pass"
summary = {
    "run_id": os.environ["RUN_ID"],
    "tenant": os.environ["TENANT"],
    "apisix": os.environ["APISIX"],
    "lookback_minutes": int(os.environ["LOOKBACK_MINUTES"]),
    "result": result,
    "full_chain_closed": not gaps and not has_segment_gap,
    "command_failures": int(os.environ["FAILURES"]),
    "threshold_ms": latency_data.get("threshold_ms"),
    "stage_coverage": stages,
    "api_latency_chain": latency_data,
    "data_quality_api": data_quality.get("data", data_quality),
    "browser_ui": ui,
    "clickhouse": {
        "columns": read_jsonl(os.environ["CH_COLUMNS_FILE"]),
        "flows_raw": read_jsonl(os.environ["CH_FLOWS_FILE"]),
        "sessions": read_jsonl(os.environ["CH_SESSIONS_FILE"]),
        "alerts": read_jsonl(os.environ["CH_ALERTS_FILE"]),
    },
    "gaps": gaps,
    "evidence_files": {
        "report": os.environ["REPORT"],
        "latency_api": os.environ["LATENCY_API_FILE"],
        "data_quality_api": os.environ["DATA_QUALITY_FILE"],
        "ui": os.environ["UI_FILE"],
        "ui_screenshot": os.environ["UI_SCREENSHOT"],
        "clickhouse_columns": os.environ["CH_COLUMNS_FILE"],
        "clickhouse_flows": os.environ["CH_FLOWS_FILE"],
        "clickhouse_sessions": os.environ["CH_SESSIONS_FILE"],
        "clickhouse_alerts": os.environ["CH_ALERTS_FILE"],
    },
}
Path(os.environ["SUMMARY"]).write_text(json.dumps(summary, ensure_ascii=False, indent=2) + "\n")
PY

echo "latency chain summary: $SUMMARY"
jq '{run_id, result, full_chain_closed, command_failures, gaps, stage_coverage, evidence_files}' "$SUMMARY"

if [[ "$FAILURES" -gt 0 ]]; then
  exit 1
fi
if [[ "$FAIL_ON_GAP" == "1" ]] && [[ "$(jq -r '.result' "$SUMMARY")" != "pass" ]]; then
  exit 1
fi
