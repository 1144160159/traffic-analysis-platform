#!/usr/bin/env bash
set -euo pipefail

APISIX="${APISIX:-http://10.0.5.8:30180}"
TENANT="${TENANT:-default}"
LOG_DIR="${LOG_DIR:-doc/02_acceptance/runs/20260629-business-flow-api-preflight}"
RUN_ID="${RUN_ID:-$(date +%Y%m%d%H%M%S)-business-flow-api-preflight}"
REGRESSION_DIR="${REGRESSION_DIR:-doc/02_acceptance/02-regression}"
KUBECTL="${KUBECTL:-kubectl}"
JWT_SECRET_NAMESPACE="${JWT_SECRET_NAMESPACE:-traffic-analysis}"
JWT_SECRET_NAME="${JWT_SECRET_NAME:-traffic-credentials}"
JWT_SECRET_KEY="${JWT_SECRET_KEY:-JWT_SECRET}"
ALLOW_BLOCKERS="${ALLOW_BLOCKERS:-false}"
FLOW_CONTRACT="${FLOW_CONTRACT:-doc/04_assets/ui_suite_gpt_v1/specs/business-flow-acceptance.json}"

REPORT="$LOG_DIR/live-business-flow-api-preflight-$RUN_ID.ndjson"
SUMMARY="$LOG_DIR/live-business-flow-api-preflight-$RUN_ID-summary.json"
LOCAL_REPORT="$LOG_DIR/local-report.md"
ENDPOINT_MATRIX="$LOG_DIR/business-flow-api-matrix.json"

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
    head -c 1000 "$file" | tr '\n' ' '
  fi
}

need_cmd git
need_cmd jq
need_cmd python3
need_cmd curl
need_cmd node
need_cmd "$KUBECTL"

git rev-parse HEAD >"$LOG_DIR/commit-sha.txt"
git branch --show-current >"$LOG_DIR/git-branch.txt"
git status --short >"$LOG_DIR/git-status.txt"
git diff --stat >"$LOG_DIR/git-diff-stat.txt" || true

if [[ -s "$FLOW_CONTRACT" ]]; then
  json_log "contract" "business flow acceptance contract present" "info" true "ok" "$FLOW_CONTRACT" "$FLOW_CONTRACT"
else
  json_log "contract" "business flow acceptance contract present" "blocker" false "missing" "$FLOW_CONTRACT" "$FLOW_CONTRACT"
fi

JWT_SECRET="$(kctl -n "$JWT_SECRET_NAMESPACE" get secret "$JWT_SECRET_NAME" -o "jsonpath={.data.$JWT_SECRET_KEY}" | base64 -d)"

ADMIN_TOKEN="$(JWT_SECRET="$JWT_SECRET" TENANT="$TENANT" RUN_ID="$RUN_ID" python3 - <<'PY'
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
    "username": "codex-business-flow-api",
    "email": "codex-business-flow-api@local",
    "roles": ["admin"],
    "permissions": [
        "*",
        "admin:*",
        "alert:read",
        "audit:read",
        "deploy:read",
        "graph:read",
        "model:read",
        "pcap:read",
        "probe:metrics",
        "rule:read",
        "screen:view",
        "token:read",
        "user:read",
    ],
    "token_type": "access",
    "session_id": "codex-business-flow-api-" + os.environ["RUN_ID"],
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
)"

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
    json_log "fixture" "$name" "blocker" false "curl-rc=$rc" "$(trim_file "$err_file")" "$(basename "$err_file")"
    echo ""
    return 0
  fi
  if [[ "$code" != 2* ]]; then
    json_log "fixture" "$name" "blocker" false "$code" "$(trim_file "$output")" "$(basename "$output")"
    echo ""
    return 0
  fi
  echo "$code"
}

ALERTS_BODY="$LOG_DIR/fixture-alerts.json"
curl_json "load live alert fixture" "/api/v1/alerts?limit=1&page_size=1" "$ALERTS_BODY" >/dev/null
ALERT_ID="$(jq -r '
  ([.. | objects | .alert_id? | select(type == "string" and length > 0)][0] // empty)
' "$ALERTS_BODY" 2>/dev/null || true)"
if [[ -n "$ALERT_ID" ]]; then
  json_log "fixture" "live alert id resolved" "info" true "ok" "$ALERT_ID" "fixture-alerts.json"
else
  json_log "fixture" "live alert id resolved" "blocker" false "missing" "no alert_id in /api/v1/alerts?limit=1" "fixture-alerts.json"
fi

CAMPAIGNS_BODY="$LOG_DIR/fixture-campaigns.json"
curl_json "load live campaign fixture" "/api/v1/campaigns?limit=1&page_size=1" "$CAMPAIGNS_BODY" >/dev/null
CAMPAIGN_ID="$(jq -r '
  ([.. | objects | .campaign_id? | select(type == "string" and length > 0)][0] //
   [.. | objects | .id? | select(type == "string" and length > 0)][0] //
   empty)
' "$CAMPAIGNS_BODY" 2>/dev/null || true)"
if [[ -n "$CAMPAIGN_ID" ]]; then
  json_log "fixture" "live campaign id resolved" "info" true "ok" "$CAMPAIGN_ID" "fixture-campaigns.json"
else
  json_log "fixture" "live campaign id resolved" "blocker" false "missing" "no campaign id in /api/v1/campaigns?limit=1" "fixture-campaigns.json"
fi

ALERT_ID="$ALERT_ID" CAMPAIGN_ID="$CAMPAIGN_ID" FLOW_CONTRACT="$FLOW_CONTRACT" node >"$ENDPOINT_MATRIX" <<'JS'
const fs = require('node:fs');
const flows = JSON.parse(fs.readFileSync(process.env.FLOW_CONTRACT, 'utf8'));
const endpointToFlows = new Map();
for (const flow of flows) {
  for (const endpoint of flow.apiEndpoints ?? []) {
    if (!endpointToFlows.has(endpoint)) endpointToFlows.set(endpoint, new Set());
    endpointToFlows.get(endpoint).add(flow.id);
  }
}

const queryFor = (endpoint) => {
  const params = new URLSearchParams();
  params.set('limit', '8');
  params.set('page_size', '8');
  if (endpoint === '/api/v1/graph/explore') {
    params.set('ip', '10.20.4.18');
    params.set('depth', '2');
    params.set('run_id', 'realtime');
  }
  return params.toString();
};

const resolve = (endpoint) => {
  if (endpoint.includes('{id}')) {
    if (endpoint.startsWith('/api/v1/alerts/')) return endpoint.replace('{id}', process.env.ALERT_ID || '__missing_alert_id__');
    if (endpoint.startsWith('/api/v1/campaigns/')) return endpoint.replace('{id}', process.env.CAMPAIGN_ID || '__missing_campaign_id__');
  }
  return endpoint;
};

const endpoints = [...endpointToFlows.entries()]
  .sort(([a], [b]) => a.localeCompare(b))
  .map(([endpoint, flowIds]) => {
    const resolvedPath = resolve(endpoint);
    const query = queryFor(endpoint);
    return {
      endpoint,
      resolvedPath,
      urlPath: `${resolvedPath}${query ? `?${query}` : ''}`,
      flows: [...flowIds].sort(),
      fixtureMissing: resolvedPath.includes('__missing_'),
    };
  });

console.log(JSON.stringify({
  generatedAt: new Date().toISOString(),
  summary: {
    flows: flows.length,
    endpoints: endpoints.length,
    dynamicEndpoints: endpoints.filter((item) => item.endpoint.includes('{id}')).length,
    missingFixtures: endpoints.filter((item) => item.fixtureMissing).length,
  },
  endpoints,
}, null, 2));
JS

jq -c '.endpoints[]' "$ENDPOINT_MATRIX" | while IFS= read -r endpoint; do
  RAW_ENDPOINT="$(jq -r '.endpoint' <<<"$endpoint")"
  URL_PATH="$(jq -r '.urlPath' <<<"$endpoint")"
  FIXTURE_MISSING="$(jq -r '.fixtureMissing' <<<"$endpoint")"
  SAFE_NAME="endpoint-$(sed -E 's#[^A-Za-z0-9]+#-#g; s#^-|-$##g' <<<"$RAW_ENDPOINT")"
  BODY_FILE="$LOG_DIR/$SAFE_NAME.json"
  ERR_FILE="$LOG_DIR/$SAFE_NAME.err"
  HEADER_FILE="$LOG_DIR/$SAFE_NAME.headers"

  if [[ "$FIXTURE_MISSING" == "true" ]]; then
    json_log "api" "GET $RAW_ENDPOINT" "blocker" false "missing-fixture" "$URL_PATH" "$(basename "$BODY_FILE")"
    continue
  fi

  set +e
  CODE="$(curl --noproxy '*' -sS -m 20 -D "$HEADER_FILE" -o "$BODY_FILE" -w '%{http_code}' \
    -H "Authorization: Bearer $ADMIN_TOKEN" \
    -H "X-Tenant-ID: $TENANT" \
    "$APISIX$URL_PATH" 2>"$ERR_FILE")"
  RC=$?
  set -e

  if [[ "$RC" -ne 0 ]]; then
    json_log "api" "GET $RAW_ENDPOINT" "blocker" false "curl-rc=$RC" "$(trim_file "$ERR_FILE")" "$(basename "$ERR_FILE")"
    continue
  fi

  if [[ "$CODE" == 2* ]]; then
    if jq -e 'type' "$BODY_FILE" >/dev/null 2>"$ERR_FILE"; then
      DETAIL="$(jq -c '{type:type, top_keys:(if type=="object" then keys else [] end), array_len:(if type=="array" then length else null end)}' "$BODY_FILE" 2>/dev/null || true)"
      json_log "api" "GET $RAW_ENDPOINT" "info" true "$CODE" "$DETAIL" "$(basename "$BODY_FILE")"
    else
      json_log "api" "GET $RAW_ENDPOINT" "blocker" false "non-json-$CODE" "$(trim_file "$BODY_FILE")" "$(basename "$BODY_FILE")"
    fi
  elif [[ "$CODE" == "404" || "$CODE" == "405" || "$CODE" == 5* || "$CODE" == "401" || "$CODE" == "403" ]]; then
    json_log "api" "GET $RAW_ENDPOINT" "blocker" false "$CODE" "$(trim_file "$BODY_FILE")" "$(basename "$BODY_FILE")"
  else
    json_log "api" "GET $RAW_ENDPOINT" "warn" false "$CODE" "$(trim_file "$BODY_FILE")" "$(basename "$BODY_FILE")"
  fi
done

jq -s \
  --arg run_id "$RUN_ID" \
  --arg apisix "$APISIX" \
  --arg report "$REPORT" \
  --arg endpoint_matrix "$ENDPOINT_MATRIX" \
  --arg local_report "$LOCAL_REPORT" \
  '{
    run_id:$run_id,
    generated_at: now | todateiso8601,
    apisix:$apisix,
    result: (if ([.[] | select(.severity == "blocker" and .passed == false)] | length) > 0 then "blocked" elif ([.[] | select(.severity == "warn" and .passed == false)] | length) > 0 then "warn" else "pass" end),
    total: length,
    passed: ([.[] | select(.passed == true)] | length),
    blockers: ([.[] | select(.severity == "blocker" and .passed == false)] | length),
    warnings: ([.[] | select(.severity == "warn" and .passed == false)] | length),
    api_checks: ([.[] | select(.phase == "api")] | length),
    api_passed: ([.[] | select(.phase == "api" and .passed == true)] | length),
    report:$report,
    endpoint_matrix:$endpoint_matrix,
    local_report:$local_report,
    checks: .
  }' "$REPORT" >"$SUMMARY"

node - "$SUMMARY" "$LOCAL_REPORT" <<'JS'
const fs = require('node:fs');
const [summaryFile, reportFile] = process.argv.slice(2);
const summary = JSON.parse(fs.readFileSync(summaryFile, 'utf8'));
const failed = summary.checks.filter((check) => !check.passed);
const blockers = failed.filter((check) => check.severity === 'blocker');
const warnings = failed.filter((check) => check.severity === 'warn');
const row = (cells) => `| ${cells.join(' | ')} |`;
const table = (items) => {
  if (!items.length) return '- 无';
  return [
    row(['阶段', '检查', '等级', '状态', '证据']),
    row(['---', '---', '---', '---', '---']),
    ...items.map((item) => row([item.phase, item.name, item.severity, item.status, item.artifact || '-'])),
  ].join('\n');
};
const md = `# 业务流 API 契约预检报告

- Run ID：\`${summary.run_id}\`
- 结果：\`${summary.result}\`
- APISIX：\`${summary.apisix}\`
- 检查数：${summary.passed}/${summary.total} passed，API=${summary.api_passed}/${summary.api_checks} passed，blockers=${summary.blockers}，warnings=${summary.warnings}

## Blockers

${table(blockers)}

## Warnings

${table(warnings)}

## 证据

- NDJSON：\`${summary.report}\`
- Summary：\`${summaryFile}\`
- Endpoint matrix：\`${summary.endpoint_matrix}\`

## 口径

本报告从 \`doc/04_assets/ui_suite_gpt_v1/specs/business-flow-acceptance.json\` 抽取所有唯一 API，经 APISIX 使用短期 admin JWT 做只读 GET 验证。动态详情接口使用 live \`/alerts\` 和 \`/campaigns\` 解析真实 ID；未解析到 ID 时按 blocker 记录。
`;
fs.writeFileSync(reportFile, md);
JS

cp "$SUMMARY" "$REGRESSION_DIR/business-flow-api-preflight-latest.json"
cp "$LOCAL_REPORT" "$REGRESSION_DIR/business-flow-api-preflight-latest.md"
cp "$ENDPOINT_MATRIX" "$REGRESSION_DIR/business-flow-api-matrix-latest.json"

RESULT="$(jq -r '.result' "$SUMMARY")"
echo "business flow api preflight result: $RESULT"
echo "summary: $SUMMARY"
echo "local report: $LOCAL_REPORT"

if [[ "$RESULT" == "blocked" && "$ALLOW_BLOCKERS" != "true" ]]; then
  exit 1
fi
