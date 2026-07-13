#!/usr/bin/env bash
set -u -o pipefail

ROUNDS="${ROUNDS:-100}"
APISIX="${APISIX:-http://10.0.5.8:30180}"
TENANT="${TENANT:-default}"
CURL_TIMEOUT="${CURL_TIMEOUT:-10}"
LOG_DIR="${LOG_DIR:-.artifacts/e2e}"
RUN_ID="${RUN_ID:-$(date +%Y%m%d%H%M%S)-$$}"
MIN_FLINK_RUNNING_JOBS="${MIN_FLINK_RUNNING_JOBS:-9}"
GRAPH_CHECK_EVERY="${GRAPH_CHECK_EVERY:-10}"
RUN_MUTATING_CHECKS="${RUN_MUTATING_CHECKS:-1}"
PG_NAMESPACE="${PG_NAMESPACE:-databases}"
PG_POD="${PG_POD:-postgres-primary-0}"
PG_DB="${PG_DB:-traffic_platform}"
PG_USER="${PG_USER:-postgres}"
CH_NAMESPACE="${CH_NAMESPACE:-middleware}"
CH_POD="${CH_POD:-clickhouse-1-0}"
FLINK_NAMESPACE="${FLINK_NAMESPACE:-flink}"
FLINK_JM_POD="${FLINK_JM_POD:-flink-jobmanager-0}"
KUBECTL="${KUBECTL:-kubectl}"

REPORT="$LOG_DIR/live-100-round-smoke-$RUN_ID.ndjson"
SUMMARY="$LOG_DIR/live-100-round-smoke-$RUN_ID-summary.json"
START_EPOCH="$(date +%s)"
TOTAL=0
PASSED=0
FAILED=0

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
  local round="$1"
  local name="$2"
  local ok="$3"
  local status="$4"
  local latency="$5"
  local detail="${6:-}"
  local latency_json="$latency"

  if ! [[ "$latency_json" =~ ^[0-9]+([.][0-9]+)?$ ]]; then
    latency_json=0
  fi

  TOTAL=$((TOTAL + 1))
  if [[ "$ok" == "true" ]]; then
    PASSED=$((PASSED + 1))
  else
    FAILED=$((FAILED + 1))
  fi

  jq -nc \
    --arg ts "$(date -Iseconds)" \
    --argjson round "$round" \
    --arg name "$name" \
    --argjson ok "$ok" \
    --arg status "$status" \
    --argjson latency "$latency_json" \
    --arg detail "$detail" \
    '{ts:$ts, round:$round, name:$name, ok:$ok, status:$status, latency_seconds:$latency, detail:$detail}' >>"$REPORT"
}

trim_file() {
  local file="$1"
  if [[ -s "$file" ]]; then
    head -c 500 "$file" | tr '\n' ' '
  fi
}

expect_status() {
  local code="$1"
  local expected="$2"

  if [[ "$expected" == "2xx" ]]; then
    [[ "$code" =~ ^2[0-9][0-9]$ ]]
  else
    [[ "$code" == "$expected" ]]
  fi
}

http_check() {
  local round="$1"
  local name="$2"
  local method="$3"
  local path="$4"
  local expected="$5"
  local jq_filter="${6:-}"
  local data="${7:-}"
  local auth_mode="${8:-auth}"
  local body_file err_file curl_out rc code latency detail ok

  body_file="$(mktemp)"
  err_file="$(mktemp)"
  local curl_args=(--noproxy '*' -sS -m "$CURL_TIMEOUT" -o "$body_file" -w "%{http_code} %{time_total}" -X "$method")
  curl_args+=(-H "X-Request-ID: live-100-$RUN_ID-$round-$name")

  if [[ "$auth_mode" == "auth" ]]; then
    curl_args+=(-H "Authorization: Bearer $TOKEN" -H "X-Tenant-ID: $TENANT")
  fi

  if [[ -n "$data" ]]; then
    curl_args+=(-H "Content-Type: application/json" --data "$data")
  fi

  curl_out="$(curl "${curl_args[@]}" "$APISIX$path" 2>"$err_file")"
  rc=$?
  if [[ "$rc" -ne 0 ]]; then
    code="000"
    latency="0"
    detail="curl rc=$rc path=$path err=$(trim_file "$err_file")"
    ok="false"
  else
    read -r code latency <<<"$curl_out"
    if expect_status "$code" "$expected"; then
      if [[ -n "$jq_filter" ]]; then
        if jq -e "$jq_filter" "$body_file" >/dev/null 2>"$err_file"; then
          ok="true"
          detail="$path"
        else
          ok="false"
          detail="jq failed path=$path filter=$jq_filter body=$(trim_file "$body_file") err=$(trim_file "$err_file")"
        fi
      else
        ok="true"
        detail="$path"
      fi
    else
      ok="false"
      detail="expected=$expected path=$path body=$(trim_file "$body_file") err=$(trim_file "$err_file")"
    fi
  fi

  json_log "$round" "$name" "$ok" "$code" "$latency" "$detail"
  rm -f "$body_file" "$err_file"
}

ui_check() {
  local round="$1"
  local route="$2"
  local body_file err_file curl_out rc code latency detail ok

  body_file="$(mktemp)"
  err_file="$(mktemp)"
  curl_out="$(curl --noproxy '*' -sS -m "$CURL_TIMEOUT" -o "$body_file" -w "%{http_code} %{time_total}" "$APISIX$route" 2>"$err_file")"
  rc=$?

  if [[ "$rc" -ne 0 ]]; then
    code="000"
    latency="0"
    ok="false"
    detail="curl rc=$rc route=$route err=$(trim_file "$err_file")"
  else
    read -r code latency <<<"$curl_out"
    if [[ "$code" == "200" ]] && grep -Eq '<div id="root"|<script[^>]+type="module"|/assets/' "$body_file"; then
      ok="true"
      detail="$route"
    else
      ok="false"
      detail="route=$route body=$(trim_file "$body_file") err=$(trim_file "$err_file")"
    fi
  fi

  json_log "$round" "ui:$route" "$ok" "$code" "$latency" "$detail"
  rm -f "$body_file" "$err_file"
}

asset_api_check() {
  local round="$1"
  local body_file err_file curl_out rc code latency detail ok

  body_file="$(mktemp)"
  err_file="$(mktemp)"
  curl_out="$(curl --noproxy '*' -sS -m "$CURL_TIMEOUT" -o "$body_file" -w "%{http_code} %{time_total}" \
    -H "Authorization: Bearer $TOKEN" \
    -H "X-Tenant-ID: $TENANT" \
    -H "X-Request-ID: live-100-$RUN_ID-$round-assets-seeded" \
    "$APISIX/api/v1/assets?limit=10" 2>"$err_file")"
  rc=$?

  if [[ "$rc" -ne 0 ]]; then
    code="000"
    latency="0"
    ok="false"
    detail="curl rc=$rc err=$(trim_file "$err_file")"
  else
    read -r code latency <<<"$curl_out"
    if [[ "$code" == "200" ]] && jq -e --arg host "$SMOKE_HOST" \
      '((.pagination.total // 0) >= 1) and (([.data[]?.hostname] | index($host)) != null)' "$body_file" >/dev/null 2>"$err_file"; then
      ok="true"
      detail="/api/v1/assets?limit=10 host=$SMOKE_HOST"
    else
      ok="false"
      detail="host=$SMOKE_HOST body=$(trim_file "$body_file") err=$(trim_file "$err_file")"
    fi
  fi

  json_log "$round" "api:assets-seeded" "$ok" "$code" "$latency" "$detail"
  rm -f "$body_file" "$err_file"
}

k8s_pod_check() {
  local round="$1"
  local out rc ok detail bad_pods attempt started elapsed

  started="$(date +%s)"
  bad_pods=""
  rc=0
  for attempt in 1 2 3; do
    out="$(kctl get pods -A --field-selector=status.phase!=Running,status.phase!=Succeeded --no-headers 2>&1)"
    rc=$?
    bad_pods="$(printf "%s\n" "$out" | awk 'NF && $0 !~ /No resources found/ && $4 != "Completed" {print}')"
    if [[ "$rc" -eq 0 ]] && [[ -z "$bad_pods" ]]; then
      break
    fi
    if [[ "$attempt" -lt 3 ]]; then
      sleep 3
    fi
  done
  elapsed=$(( $(date +%s) - started ))

  if [[ "$rc" -eq 0 ]] && [[ -z "$bad_pods" ]]; then
    ok="true"
    detail="all non-terminal pods running or succeeded"
  else
    ok="false"
    detail="$(echo "$bad_pods" | tr '\n' ' ' | cut -c1-500)"
  fi

  json_log "$round" "k8s:pods-running" "$ok" "$rc" "$elapsed" "$detail"
}

probe_daemonset_check() {
  local round="$1"
  local out rc desired ready ok detail

  out="$(kctl -n traffic-analysis get ds probe-agent -o jsonpath='{.status.desiredNumberScheduled} {.status.numberReady}' 2>&1)"
  rc=$?

  if [[ "$rc" -eq 0 ]]; then
    read -r desired ready <<<"$out"
    if [[ -n "$desired" && "$desired" == "$ready" ]]; then
      ok="true"
      detail="probe-agent ready $ready/$desired"
    else
      ok="false"
      detail="probe-agent ready ${ready:-unknown}/${desired:-unknown}"
    fi
  else
    ok="false"
    detail="$(echo "$out" | tr '\n' ' ' | cut -c1-500)"
  fi

  json_log "$round" "k8s:probe-agent-daemonset" "$ok" "$rc" "0" "$detail"
}

flink_check() {
  local round="$1"
  local body_file err_file rc ok detail

  body_file="$(mktemp)"
  err_file="$(mktemp)"
  kctl -n "$FLINK_NAMESPACE" exec "$FLINK_JM_POD" -- curl -fsS http://localhost:8081/jobs/overview >"$body_file" 2>"$err_file"
  rc=$?

  if [[ "$rc" -eq 0 ]] && jq -e --argjson min "$MIN_FLINK_RUNNING_JOBS" \
    '(.jobs | length) >= $min and ([.jobs[] | select(.state != "RUNNING" or ((.tasks.running // 0) != (.tasks.total // 0)))] | length == 0)' "$body_file" >/dev/null 2>>"$err_file"; then
    ok="true"
    detail="flink jobs running >= $MIN_FLINK_RUNNING_JOBS"
  else
    ok="false"
    detail="body=$(trim_file "$body_file") err=$(trim_file "$err_file")"
  fi

  json_log "$round" "k8s:flink-jobs-running" "$ok" "$rc" "0" "$detail"
  rm -f "$body_file" "$err_file"
}

clickhouse_check() {
  local round="$1"
  local out rc ok detail

  out="$(kctl -n "$CH_NAMESPACE" exec "$CH_POD" -c clickhouse -- clickhouse-client --query "SELECT count() FROM traffic.flows_raw" 2>&1)"
  rc=$?

  if [[ "$rc" -eq 0 ]] && [[ "$out" =~ ^[0-9]+$ ]]; then
    ok="true"
    detail="traffic.flows_raw count=$out"
  else
    ok="false"
    detail="$(echo "$out" | tr '\n' ' ' | cut -c1-500)"
  fi

  json_log "$round" "db:clickhouse-flows-raw" "$ok" "$rc" "0" "$detail"
}

pg_exec() {
  kctl -n "$PG_NAMESPACE" exec "$PG_POD" -- env PGPASSWORD="$PG_PASSWORD" \
    psql -U "$PG_USER" -d "$PG_DB" -v ON_ERROR_STOP=1 -tAc "$1"
}

pg_asset_check() {
  local round="$1"
  local out rc ok detail

  out="$(pg_exec "SELECT count(*) FROM assets WHERE tenant_id = '$TENANT_SQL' AND hostname = '$SMOKE_HOST_SQL';" 2>&1)"
  rc=$?

  if [[ "$rc" -eq 0 ]] && [[ "$out" =~ ^[[:space:]]*[1-9][0-9]*[[:space:]]*$ ]]; then
    ok="true"
    detail="asset hostname=$SMOKE_HOST count=$(echo "$out" | tr -d '[:space:]')"
  else
    ok="false"
    detail="$(echo "$out" | tr '\n' ' ' | cut -c1-500)"
  fi

  json_log "$round" "db:postgres-seeded-asset" "$ok" "$rc" "0" "$detail"
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
tenant = os.environ["TENANT"]
secret = os.environ["JWT_SECRET"].encode("utf-8")
header = {"alg": "HS256", "typ": "JWT"}
claims = {
    "iss": "traffic-auth-service",
    "sub": str(uuid.uuid4()),
    "jti": str(uuid.uuid4()),
    "user_id": str(uuid.uuid4()),
    "tenant_id": tenant,
    "username": "codex-live-100",
    "email": "codex-live-100@local",
    "roles": ["admin"],
    "permissions": ["*", "admin:*", "model:read", "token:read"],
    "token_type": "access",
    "session_id": "codex-live-100-" + str(uuid.uuid4()),
    "iat": now,
    "exp": now + 14400,
}
signing_input = b".".join([
    b64url(json.dumps(header, separators=(",", ":")).encode("utf-8")).encode("ascii"),
    b64url(json.dumps(claims, separators=(",", ":")).encode("utf-8")).encode("ascii"),
])
signature = hmac.new(secret, signing_input, hashlib.sha256).digest()
print(signing_input.decode("ascii") + "." + b64url(signature))
PY
}

seed_asset() {
  local sql out rc

  sql="
INSERT INTO assets (
  asset_id, tenant_id, ip_address, mac_address, hostname, vendor, os_type,
  source, vlan_id, switch_port, tags, criticality, metadata,
  first_seen, last_seen, created_at, updated_at
) VALUES (
  '$SMOKE_ASSET_ID_SQL', '$TENANT_SQL', '$SMOKE_IP_SQL', '$SMOKE_MAC_SQL',
  '$SMOKE_HOST_SQL', 'Codex', 'linux', 'live_100_round_smoke',
  'smoke', 'codex-100', '{\"test\":\"live_100_round_smoke\"}'::jsonb,
  8, '{\"run_id\":\"$RUN_ID_SQL\"}'::jsonb, NOW(), NOW(), NOW(), NOW()
)
ON CONFLICT (tenant_id, mac_address) WHERE mac_address IS NOT NULL
DO UPDATE SET
  ip_address = EXCLUDED.ip_address,
  hostname = EXCLUDED.hostname,
  vendor = EXCLUDED.vendor,
  os_type = EXCLUDED.os_type,
  source = EXCLUDED.source,
  tags = EXCLUDED.tags,
  criticality = EXCLUDED.criticality,
  metadata = EXCLUDED.metadata,
  last_seen = NOW(),
  updated_at = NOW()
RETURNING asset_id;
"

  out="$(pg_exec "$sql" 2>&1)"
  rc=$?
  if [[ "$rc" -ne 0 ]]; then
    echo "failed to seed asset in PostgreSQL: $out" >&2
    exit 3
  fi
  echo "seeded PostgreSQL asset: $(echo "$out" | tr -d '[:space:]') host=$SMOKE_HOST ip=$SMOKE_IP"
}

seed_notification_settings() {
  local payload

  payload="$(jq -nc \
    --arg ref "k8s://traffic-analysis/live-100-round-smoke" \
    '{enabled:true,min_severity:"medium",rate_limit_per_min:30,channels:{email:false,slack:false,webhook:false,wechat:false,dingtalk:false,feishu:false},secret_ref:$ref}')"
  http_check 0 "api:notifications-settings-put" "PUT" "/api/v1/notifications/settings" "200" "" "$payload"
}

run_once_mutating_checks() {
  local playbook_payload

  if [[ "$RUN_MUTATING_CHECKS" != "1" ]]; then
    echo "skipping one-time mutating API checks (RUN_MUTATING_CHECKS=$RUN_MUTATING_CHECKS)"
    return
  fi

  http_check 0 "api:data-quality-baseline-post" "POST" "/api/v1/data-quality/baseline" "200" ".success == true"

  playbook_payload="$(jq -nc \
    --arg alert_id "codex-live-100-$RUN_ID" \
    --arg tenant "$TENANT" \
    '{alert_id:$alert_id,alert_type:"scan",severity:"critical",score:0.96,source_ip:"192.0.2.100",dest_ip:"198.51.100.10",tenant_id:$tenant,related_alert_count:9,asset_risk:"high"}')"
  http_check 0 "api:playbooks-execute-post" "POST" "/api/v1/playbooks/block-scanner/execute" "200" \
    '.success == true and (.data.playbook == "block-scanner") and ((.data.success_actions // 0) >= 1)' "$playbook_payload"

  http_check 0 "api:notifications-test-post" "POST" "/api/v1/notifications/test" "200" ".success == true"
}

run_api_checks() {
  local round="$1"

  http_check "$round" "public:captcha" "GET" "/api/v1/auth/captcha" "200" "" "" "none"
  http_check "$round" "auth:anonymous-models-401" "GET" "/api/v1/models?limit=1" "401" "" "" "none"
  http_check "$round" "api:auth-me" "GET" "/api/v1/auth/me" "200" ".tenant_id == \"$TENANT\""
  http_check "$round" "api:dashboard-stats" "GET" "/api/v1/dashboard/stats" "200"
  http_check "$round" "api:dashboard-alert-trend" "GET" "/api/v1/dashboard/alerts/trend?hours=24&interval=hour" "200"
  http_check "$round" "api:dashboard-top-src-ips" "GET" "/api/v1/dashboard/top-ips/src?limit=5" "200"
  http_check "$round" "api:dashboard-encrypted-trend" "GET" "/api/v1/dashboard/encrypted/trend" "200"
  http_check "$round" "api:dashboard-attack-phases" "GET" "/api/v1/dashboard/attack-phases" "200"
  asset_api_check "$round"
  http_check "$round" "api:risk-assets" "GET" "/api/v1/risk/assets" "200"
  http_check "$round" "api:alerts" "GET" "/api/v1/alerts?limit=1" "200"
  http_check "$round" "api:campaigns" "GET" "/api/v1/campaigns?limit=1" "200"
  http_check "$round" "api:attack-chains" "GET" "/api/v1/attack-chains?limit=1" "200"
  http_check "$round" "api:encrypted-stats" "GET" "/api/v1/encrypted-traffic/stats" "200"
  http_check "$round" "api:encrypted-sessions" "GET" "/api/v1/encrypted-traffic/sessions?limit=1" "200"
  http_check "$round" "api:encrypted-ja3" "GET" "/api/v1/encrypted-traffic/ja3" "200"
  http_check "$round" "api:encrypted-tunnels" "GET" "/api/v1/encrypted-traffic/tunnels" "200"
  http_check "$round" "api:encrypted-exfiltration" "GET" "/api/v1/encrypted-traffic/exfiltration?limit=1" "200"
  http_check "$round" "api:topics-tunnel" "GET" "/api/v1/topics/tunnel" "200"
  http_check "$round" "api:topics-exfil" "GET" "/api/v1/topics/exfil" "200"
  http_check "$round" "api:topics-apt" "GET" "/api/v1/topics/apt" "200"
  http_check "$round" "api:rules" "GET" "/api/v1/rules?limit=1" "200"
  http_check "$round" "api:deployments" "GET" "/api/v1/deployments?limit=1" "200"
  http_check "$round" "api:models" "GET" "/api/v1/models?limit=1" "200"
  http_check "$round" "api:mlops-status" "GET" "/api/v1/mlops/status" "200"
  http_check "$round" "api:mlops-conditions" "GET" "/api/v1/mlops/conditions" "200"
  http_check "$round" "api:data-quality" "GET" "/api/v1/data-quality" "200"
  http_check "$round" "api:playbooks-catalog" "GET" "/api/v1/playbooks/catalog" "200"
  http_check "$round" "api:playbooks-executions" "GET" "/api/v1/playbooks/executions?limit=3" "200"
  http_check "$round" "api:pcap-jobs" "GET" "/api/v1/pcap/jobs?limit=1" "200"
  if [[ "$GRAPH_CHECK_EVERY" -gt 0 ]] && (( round % GRAPH_CHECK_EVERY == 0 )); then
    http_check "$round" "api:graph-explore" "GET" "/api/v1/graph/explore?ip=10.0.0.1&depth=1&limit=5" "200"
  fi
  http_check "$round" "api:fusion-sources" "GET" "/api/v1/fusion/sources" "200"
  http_check "$round" "api:fusion-stats" "GET" "/api/v1/fusion/stats" "200"
  http_check "$round" "api:fusion-entities" "GET" "/api/v1/fusion/entities?limit=1" "200"
  http_check "$round" "api:baselines" "GET" "/api/v1/baselines?limit=1" "200"
  http_check "$round" "api:probes" "GET" "/api/v1/probes?limit=1" "200"
  http_check "$round" "api:whitelist" "GET" "/api/v1/whitelist?limit=1" "200"
  http_check "$round" "api:compliance-reports" "GET" "/api/v1/compliance/reports?limit=1" "200"
  http_check "$round" "api:compliance-audit-trail" "GET" "/api/v1/compliance/audit-trail?limit=1" "200"
  http_check "$round" "api:audit-logs" "GET" "/api/v1/audit/logs?limit=1" "200"
  http_check "$round" "api:notifications-settings" "GET" "/api/v1/notifications/settings" "200"
}

run_ui_checks() {
  local round="$1"

  ui_check "$round" "/login"
  ui_check "$round" "/screen"
  ui_check "$round" "/dashboard"
  ui_check "$round" "/alerts"
  ui_check "$round" "/campaigns"
  ui_check "$round" "/attack-chains"
  ui_check "$round" "/encrypted-traffic"
  ui_check "$round" "/topics"
  ui_check "$round" "/assets"
  ui_check "$round" "/rules"
  ui_check "$round" "/deployments"
  ui_check "$round" "/models"
  ui_check "$round" "/mlops"
  ui_check "$round" "/data-quality"
  ui_check "$round" "/playbooks"
  ui_check "$round" "/forensics"
  ui_check "$round" "/graph"
  ui_check "$round" "/fusion"
  ui_check "$round" "/baselines"
  ui_check "$round" "/probes"
  ui_check "$round" "/whitelist"
  ui_check "$round" "/compliance"
  ui_check "$round" "/audit-log"
  ui_check "$round" "/notifications"
  ui_check "$round" "/settings"
}

write_summary() {
  local end_epoch elapsed

  end_epoch="$(date +%s)"
  elapsed=$((end_epoch - START_EPOCH))
  jq -nc \
    --arg run_id "$RUN_ID" \
    --arg apisix "$APISIX" \
    --arg tenant "$TENANT" \
    --arg smoke_host "$SMOKE_HOST" \
    --arg report "$REPORT" \
    --argjson rounds "$ROUNDS" \
    --argjson total "$TOTAL" \
    --argjson passed "$PASSED" \
    --argjson failed "$FAILED" \
    --argjson elapsed "$elapsed" \
    '{run_id:$run_id, apisix:$apisix, tenant:$tenant, smoke_host:$smoke_host, rounds:$rounds, checks:{total:$total,passed:$passed,failed:$failed}, elapsed_seconds:$elapsed, report:$report}' >"$SUMMARY"
}

need_cmd curl
need_cmd jq
need_cmd python3
need_cmd base64
need_cmd sed
need_cmd grep
need_cmd "$KUBECTL"

JWT_SECRET="$(kctl -n traffic-analysis get secret traffic-credentials -o jsonpath='{.data.JWT_SECRET}' | base64 -d)"
PG_PASSWORD="$(kctl -n traffic-analysis get secret traffic-credentials -o jsonpath='{.data.PG_PASSWORD}' | base64 -d)"
TOKEN="$(make_token)"

SMOKE_ASSET_ID="$(python3 - <<'PY'
import uuid
print(uuid.uuid4())
PY
)"
SMOKE_HOST="codex-smoke-$(echo "$RUN_ID" | tr -cd '[:alnum:]-' | tr '[:upper:]' '[:lower:]')"
SMOKE_IP="$(python3 - <<'PY'
import random
print(f"10.66.{random.randint(1, 220)}.{random.randint(1, 220)}")
PY
)"
SMOKE_MAC="$(python3 - <<'PY'
import random
print("02:42:ac:%02x:%02x:%02x" % tuple(random.randint(0, 255) for _ in range(3)))
PY
)"

TENANT_SQL="$(sql_escape "$TENANT")"
RUN_ID_SQL="$(sql_escape "$RUN_ID")"
SMOKE_ASSET_ID_SQL="$(sql_escape "$SMOKE_ASSET_ID")"
SMOKE_HOST_SQL="$(sql_escape "$SMOKE_HOST")"
SMOKE_IP_SQL="$(sql_escape "$SMOKE_IP")"
SMOKE_MAC_SQL="$(sql_escape "$SMOKE_MAC")"

echo "live 100-round smoke"
echo "apisix=$APISIX tenant=$TENANT rounds=$ROUNDS"
echo "report=$REPORT"
seed_asset
seed_notification_settings
run_once_mutating_checks

for round in $(seq 1 "$ROUNDS"); do
  before_total="$TOTAL"
  before_failed="$FAILED"

  run_api_checks "$round"
  run_ui_checks "$round"
  k8s_pod_check "$round"
  probe_daemonset_check "$round"
  flink_check "$round"
  pg_asset_check "$round"
  clickhouse_check "$round"

  round_total=$((TOTAL - before_total))
  round_failed=$((FAILED - before_failed))
  echo "round $round/$ROUNDS checks=$round_total failed=$round_failed total_failed=$FAILED"
done

write_summary

echo "summary=$SUMMARY"
jq . "$SUMMARY"

if [[ "$FAILED" -ne 0 ]]; then
  echo "failed checks (first 20):" >&2
  jq -r 'select(.ok == false) | "[round \(.round)] \(.name) status=\(.status) detail=\(.detail)"' "$REPORT" | head -20 >&2
  exit 1
fi

echo "all live smoke checks passed"
