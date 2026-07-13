#!/usr/bin/env bash
set -euo pipefail

APISIX="${APISIX:-http://10.0.5.8:30180}"
TENANT="${TENANT:-default}"
LOG_DIR="${LOG_DIR:-doc/02_acceptance/runs/20260629-asset-discovery}"
RUN_ID="${RUN_ID:-$(date +%Y%m%d%H%M%S)-asset-discovery}"
STABLE_DIR="${STABLE_DIR:-doc/02_acceptance/02-regression}"
KUBECTL="${KUBECTL:-kubectl}"
JWT_SECRET_NAMESPACE="${JWT_SECRET_NAMESPACE:-traffic-analysis}"
JWT_SECRET_NAME="${JWT_SECRET_NAME:-traffic-credentials}"
JWT_SECRET_KEY="${JWT_SECRET_KEY:-JWT_SECRET}"
PG_NAMESPACE="${PG_NAMESPACE:-databases}"
PG_POD="${PG_POD:-postgres-primary-0}"
PG_SECRET_NAMESPACE="${PG_SECRET_NAMESPACE:-traffic-analysis}"
PG_SECRET_NAME="${PG_SECRET_NAME:-traffic-credentials}"
PG_SECRET_KEY="${PG_SECRET_KEY:-PG_PASSWORD}"

REPORT="$LOG_DIR/live-asset-discovery-$RUN_ID.ndjson"
SUMMARY="$LOG_DIR/live-asset-discovery-$RUN_ID-summary.json"
LOCAL_REPORT="$LOG_DIR/local-report.md"
STABLE_JSON="$STABLE_DIR/asset-discovery-latest.json"
STABLE_MD="$STABLE_DIR/asset-discovery-latest.md"

mkdir -p "$LOG_DIR" "$STABLE_DIR"
: >"$REPORT"

FAILURES=0
JWT_SECRET=""
PG_PASSWORD=""

need_cmd() {
  if ! command -v "$1" >/dev/null 2>&1; then
    echo "missing required command: $1" >&2
    exit 2
  fi
}

json_log() {
  local phase="$1" name="$2" ok="$3" status="$4" detail="${5:-}" artifact="${6:-}"
  jq -nc \
    --arg ts "$(date -Iseconds)" \
    --arg phase "$phase" \
    --arg name "$name" \
    --argjson ok "$ok" \
    --arg status "$status" \
    --arg detail "$detail" \
    --arg artifact "$artifact" \
    '{ts:$ts, phase:$phase, name:$name, ok:$ok, status:$status, detail:$detail, artifact:$artifact}' >>"$REPORT"
  if [[ "$ok" != "true" ]]; then
    FAILURES=$((FAILURES + 1))
  fi
}

trim_file() {
  local file="$1"
  if [[ -s "$file" ]]; then
    head -c 1200 "$file" \
      | tr '\n' ' ' \
      | sed -E 's/Bearer [A-Za-z0-9._-]+/Bearer <redacted>/g'
  fi
}

kctl() {
  env -u HTTP_PROXY -u HTTPS_PROXY -u ALL_PROXY -u http_proxy -u https_proxy -u all_proxy "$KUBECTL" "$@"
}

sql_escape() {
  printf "%s" "$1" | sed "s/'/''/g"
}

psql_exec() {
  local sql="$1"
  kctl -n "$PG_NAMESPACE" exec "$PG_POD" -- env PGPASSWORD="$PG_PASSWORD" \
    psql -q -U postgres -d traffic_platform -v ON_ERROR_STOP=1 -Atc "$sql"
}

make_uuid() {
  python3 - <<'PY'
import uuid
print(uuid.uuid4())
PY
}

make_token() {
  local username="$1" tenant="$2" user_id="$3" roles_json="$4" perms_json="$5" ttl="${6:-1800}"
  JWT_SECRET="$JWT_SECRET" TENANT="$tenant" USERNAME="$username" USER_ID="$user_id" ROLES_JSON="$roles_json" PERMS_JSON="$perms_json" RUN_ID="$RUN_ID" TTL="$ttl" python3 - <<'PY'
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
    "sub": os.environ["USER_ID"],
    "iat": now,
    "exp": now + int(os.environ["TTL"]),
    "jti": str(uuid.uuid4()),
    "user_id": os.environ["USER_ID"],
    "tenant_id": os.environ["TENANT"],
    "username": os.environ["USERNAME"],
    "roles": json.loads(os.environ["ROLES_JSON"]),
    "permissions": json.loads(os.environ["PERMS_JSON"]),
    "token_type": "access",
    "session_id": "codex-asset-discovery-" + os.environ["RUN_ID"],
}
header = {"alg": "HS256", "typ": "JWT"}
signing_input = b".".join([
    b64url(json.dumps(header, separators=(",", ":")).encode()).encode(),
    b64url(json.dumps(claims, separators=(",", ":")).encode()).encode(),
])
signature = hmac.new(os.environ["JWT_SECRET"].encode(), signing_input, hashlib.sha256).digest()
print(signing_input.decode() + "." + b64url(signature))
PY
}

api_post() {
  local path="$1" body="$2" out="$3" token="$4" expected="${5:-201}"
  local code
  code="$(curl --noproxy '*' -sS -m 20 -X POST "$APISIX$path" \
    -H 'Content-Type: application/json' \
    -H "Authorization: Bearer $token" \
    -H "X-Tenant-ID: $TENANT" \
    --data-binary @"$body" \
    -o "$out" \
    -w '%{http_code}')"
  if [[ "$code" != "$expected" ]]; then
    json_log "api" "POST $path returned HTTP $expected" false "http_$code" "$(trim_file "$out")" "$out"
    return 1
  fi
}

api_get() {
  local path="$1" out="$2" token="$3" expected="${4:-200}"
  local code
  code="$(curl --noproxy '*' -sS -m 20 "$APISIX$path" \
    -H "Authorization: Bearer $token" \
    -H "X-Tenant-ID: $TENANT" \
    -o "$out" \
    -w '%{http_code}')"
  if [[ "$code" != "$expected" ]]; then
    json_log "api" "GET $path returned HTTP $expected" false "http_$code" "$(trim_file "$out")" "$out"
    return 1
  fi
}

need_cmd curl
need_cmd jq
need_cmd python3
need_cmd "$KUBECTL"

JWT_SECRET="$(kctl -n "$JWT_SECRET_NAMESPACE" get secret "$JWT_SECRET_NAME" -o "jsonpath={.data.$JWT_SECRET_KEY}" | base64 -d)"
PG_PASSWORD="$(kctl -n "$PG_SECRET_NAMESPACE" get secret "$PG_SECRET_NAME" -o "jsonpath={.data.$PG_SECRET_KEY}" | base64 -d)"
ADMIN_USER_ID="$(make_uuid)"
VIEWER_USER_ID="$(make_uuid)"
ADMIN_TOKEN="$(make_token codex-asset-admin "$TENANT" "$ADMIN_USER_ID" '["admin"]' '["*","admin:*","asset:discover","asset:read","audit:read"]')"
VIEWER_TOKEN="$(make_token codex-asset-viewer "$TENANT" "$VIEWER_USER_ID" '["viewer"]' '["asset:read"]')"

suffix="$(date +%H%M%S)"
source_mac="02:cd:29:${suffix:0:2}:${suffix:2:2}:31"
neighbor_mac="02:cd:29:${suffix:0:2}:${suffix:2:2}:41"

api_get "/api/v1/tokens/scopes" "$LOG_DIR/token-scopes-response.json" "$ADMIN_TOKEN"
if jq -e '(.scopes // []) | map(.name) | index("asset:discover") != null' "$LOG_DIR/token-scopes-response.json" >/dev/null; then
  json_log "api" "Token scope catalog includes asset:discover" true "ok" "asset scope catalog synced" "token-scopes-response.json"
else
  json_log "api" "Token scope catalog includes asset:discover" false "missing" "asset:discover missing from auth-service scope catalog" "token-scopes-response.json"
fi

jq -nc \
  --arg tenant "$TENANT" \
  --arg name "codex-snmp-lldp-$suffix" \
  '{
    tenant_id: $tenant,
    name: $name,
    protocol: "snmp_lldp",
    endpoint: "10.12.0.0/24",
    secret_ref: "k8s://traffic-analysis/traffic-credentials#ASSET_DISCOVERY_SNMP",
    created_by: "codex-live-smoke"
  }' >"$LOG_DIR/credential-payload.json"

api_post "/api/v1/assets/discovery/credentials" "$LOG_DIR/credential-payload.json" "$LOG_DIR/credential-viewer-denied.json" "$VIEWER_TOKEN" "403" || true
if jq -e '((.message // "") | contains("asset:discover"))' "$LOG_DIR/credential-viewer-denied.json" >/dev/null; then
  json_log "api" "Viewer cannot register discovery credential" true "ok" "asset:discover required" "credential-viewer-denied.json"
else
  json_log "api" "Viewer cannot register discovery credential" false "invalid" "$(trim_file "$LOG_DIR/credential-viewer-denied.json")" "credential-viewer-denied.json"
fi

api_post "/api/v1/assets/discovery/credentials" "$LOG_DIR/credential-payload.json" "$LOG_DIR/credential-response.json" "$ADMIN_TOKEN"
credential_id="$(jq -r '.data.credential_id // empty' "$LOG_DIR/credential-response.json")"
secret_ref="$(jq -r '.data.secret_ref // empty' "$LOG_DIR/credential-response.json")"
created_at="$(jq -r '.data.created_at // empty' "$LOG_DIR/credential-response.json")"
if [[ -n "$credential_id" && "$secret_ref" == k8s://* && "$created_at" != "0001-01-01T00:00:00Z" ]]; then
  json_log "api" "SNMP/LLDP credential reference registered" true "ok" "credential_id=$credential_id" "credential-response.json"
else
  json_log "api" "SNMP/LLDP credential reference registered" false "invalid" "credential_id=$credential_id secret_ref=$secret_ref created_at=$created_at" "credential-response.json"
fi

credential_audit_count="$(psql_exec "SELECT count(*) FROM audit_logs WHERE tenant_id = '$(sql_escape "$TENANT")' AND action = 'ASSET_DISCOVERY_CREDENTIAL_REGISTER' AND object_id = '$(sql_escape "$credential_id")';")"
if [[ "$credential_audit_count" -ge 1 ]]; then
  json_log "postgres" "Credential registration audit log is queryable" true "ok" "count=$credential_audit_count" "audit_logs"
else
  json_log "postgres" "Credential registration audit log is queryable" false "missing" "count=$credential_audit_count credential_id=$credential_id" "audit_logs"
fi

jq -nc \
  --arg tenant "$TENANT" \
  --arg credential_id "$credential_id" \
  --arg suffix "$suffix" \
  --arg source_mac "$source_mac" \
  --arg neighbor_mac "$neighbor_mac" \
  '{
    tenant_id: $tenant,
    mode: "snmp_lldp",
    target_cidr: "10.12.0.0/24",
    credential_id: $credential_id,
    requested_by: "codex-live-smoke",
    observations: [
      {
        ip_address: "10.12.0.31",
        mac_address: $source_mac,
        hostname: ("codex-core-sw-" + $suffix),
        vendor: "Codex Test Switch",
        os_type: "switch",
        vlan_id: "310",
        switch_port: "Gi1/0/31",
        neighbors: [
          {
            ip_address: "10.12.0.41",
            mac_address: $neighbor_mac,
            hostname: ("codex-edge-sw-" + $suffix),
            interface: "Gi1/0/41",
            protocol: "lldp"
          }
        ]
      }
    ]
  }' >"$LOG_DIR/run-payload.json"

api_post "/api/v1/assets/discovery/runs" "$LOG_DIR/run-payload.json" "$LOG_DIR/run-response.json" "$ADMIN_TOKEN"
run_id="$(jq -r '.data.run.run_id // empty' "$LOG_DIR/run-response.json")"
accepted_assets="$(jq -r '.data.accepted_assets // -1' "$LOG_DIR/run-response.json")"
accepted_links="$(jq -r '.data.accepted_links // -1' "$LOG_DIR/run-response.json")"
rejected_records="$(jq -r '.data.rejected_records // -1' "$LOG_DIR/run-response.json")"
run_status="$(jq -r '.data.run.status // empty' "$LOG_DIR/run-response.json")"
if [[ -n "$run_id" && "$run_status" == "completed" && "$accepted_assets" -ge 1 && "$accepted_links" -ge 1 && "$rejected_records" -eq 0 ]]; then
  json_log "api" "SNMP/LLDP discovery run completed" true "ok" "run_id=$run_id assets=$accepted_assets links=$accepted_links" "run-response.json"
else
  json_log "api" "SNMP/LLDP discovery run completed" false "invalid" "run_id=$run_id status=$run_status assets=$accepted_assets links=$accepted_links rejected=$rejected_records" "run-response.json"
fi

api_get "/api/v1/assets/discovery/runs?tenant_id=$TENANT&limit=10" "$LOG_DIR/runs-response.json" "$ADMIN_TOKEN"
if jq -e --arg run_id "$run_id" '.data[]? | select(.run_id == $run_id and .status == "completed")' "$LOG_DIR/runs-response.json" >/dev/null; then
  json_log "api" "Discovery run is queryable" true "ok" "run_id=$run_id" "runs-response.json"
else
  json_log "api" "Discovery run is queryable" false "missing" "run_id=$run_id" "runs-response.json"
fi

jq -nc \
  --arg tenant "$TENANT" \
  --arg credential_id "$credential_id" \
  '{
    tenant_id: $tenant,
    mode: "snmp_lldp",
    target_cidr: "192.0.2.10/32",
    credential_id: $credential_id,
    requested_by: "codex-live-smoke-worker"
  }' >"$LOG_DIR/worker-run-payload.json"

api_post "/api/v1/assets/discovery/runs" "$LOG_DIR/worker-run-payload.json" "$LOG_DIR/worker-run-response.json" "$ADMIN_TOKEN"
worker_run_id="$(jq -r '.data.run.run_id // empty' "$LOG_DIR/worker-run-response.json")"
worker_status="$(jq -r '.data.run.status // empty' "$LOG_DIR/worker-run-response.json")"
worker_error="$(jq -r '.data.run.error_message // empty' "$LOG_DIR/worker-run-response.json")"
if [[ -n "$worker_run_id" && "$worker_status" == "failed" && -n "$worker_error" ]]; then
  json_log "api" "SNMP/LLDP scanner worker records failed runs safely" true "ok" "run_id=$worker_run_id status=$worker_status" "worker-run-response.json"
else
  json_log "api" "SNMP/LLDP scanner worker records failed runs safely" false "invalid" "run_id=$worker_run_id status=$worker_status error=$worker_error" "worker-run-response.json"
fi

run_audit_count="$(psql_exec "SELECT count(*) FROM audit_logs WHERE tenant_id = '$(sql_escape "$TENANT")' AND action = 'ASSET_ACTIVE_DISCOVERY_RUN' AND object_id IN ('$(sql_escape "$run_id")','$(sql_escape "$worker_run_id")');")"
if [[ "$run_audit_count" -ge 2 ]]; then
  json_log "postgres" "Discovery run audit logs are queryable" true "ok" "count=$run_audit_count" "audit_logs"
else
  json_log "postgres" "Discovery run audit logs are queryable" false "missing" "count=$run_audit_count run_id=$run_id worker_run_id=$worker_run_id" "audit_logs"
fi

api_get "/api/v1/assets/discovery/neighbors?tenant_id=$TENANT&limit=10" "$LOG_DIR/neighbors-response.json" "$ADMIN_TOKEN"
source_asset_id="$(jq -r --arg run_id "$run_id" '.data[]? | select(.run_id == $run_id) | .source_asset_id' "$LOG_DIR/neighbors-response.json" | head -n1)"
neighbor_asset_id="$(jq -r --arg run_id "$run_id" '.data[]? | select(.run_id == $run_id) | .neighbor_asset_id' "$LOG_DIR/neighbors-response.json" | head -n1)"
if [[ -n "$source_asset_id" && -n "$neighbor_asset_id" ]]; then
  json_log "api" "LLDP topology link is queryable" true "ok" "source=$source_asset_id neighbor=$neighbor_asset_id" "neighbors-response.json"
else
  json_log "api" "LLDP topology link is queryable" false "missing" "run_id=$run_id" "neighbors-response.json"
fi

api_get "/api/v1/assets/$source_asset_id?tenant_id=$TENANT" "$LOG_DIR/source-asset-response.json" "$ADMIN_TOKEN"
api_get "/api/v1/assets/$neighbor_asset_id?tenant_id=$TENANT" "$LOG_DIR/neighbor-asset-response.json" "$ADMIN_TOKEN"
if jq -e --arg mac "$source_mac" '.data.mac_address == $mac and (.data.source | startswith("active:"))' "$LOG_DIR/source-asset-response.json" >/dev/null; then
  json_log "api" "Source asset was upserted from active discovery" true "ok" "$source_mac" "source-asset-response.json"
else
  json_log "api" "Source asset was upserted from active discovery" false "invalid" "$source_mac" "source-asset-response.json"
fi
if jq -e --arg mac "$neighbor_mac" '.data.mac_address == $mac and (.data.source | startswith("active:"))' "$LOG_DIR/neighbor-asset-response.json" >/dev/null; then
  json_log "api" "Neighbor asset was upserted from LLDP discovery" true "ok" "$neighbor_mac" "neighbor-asset-response.json"
else
  json_log "api" "Neighbor asset was upserted from LLDP discovery" false "invalid" "$neighbor_mac" "neighbor-asset-response.json"
fi

TOTAL="$(jq -s 'length' "$REPORT")"
PASSED="$(jq -s '[.[] | select(.ok == true)] | length' "$REPORT")"
BLOCKERS="$(jq -s '[.[] | select(.ok != true)] | length' "$REPORT")"
RESULT="pass"
if [[ "$FAILURES" -ne 0 ]]; then
  RESULT="blocked"
fi

jq -n \
  --arg run_id "$RUN_ID" \
  --arg result "$RESULT" \
  --arg apisix "$APISIX" \
  --arg tenant "$TENANT" \
  --arg discovery_run_id "$run_id" \
  --arg worker_run_id "$worker_run_id" \
  --arg credential_id "$credential_id" \
  --argjson passed "$PASSED" \
  --argjson total "$TOTAL" \
  --argjson blockers "$BLOCKERS" \
  --slurpfile checks "$REPORT" \
  '{
    run_id: $run_id,
    result: $result,
    apisix: $apisix,
    tenant_id: $tenant,
    discovery_run_id: $discovery_run_id,
    worker_run_id: $worker_run_id,
    credential_id: $credential_id,
    passed: $passed,
    total: $total,
    blockers: $blockers,
    warnings: 0,
    checks: $checks
  }' >"$SUMMARY"

cp "$SUMMARY" "$STABLE_JSON"

{
  echo "# SNMP/LLDP 主动资产发现回归报告"
  echo
  echo "- Run ID：\`$RUN_ID\`"
  echo "- 结果：\`$RESULT\`"
  echo "- APISIX：\`$APISIX\`"
  echo "- 检查数：$PASSED/$TOTAL passed，blockers=$BLOCKERS，warnings=0"
  echo "- Discovery Run：\`$run_id\`"
  echo "- Worker Run：\`$worker_run_id\`"
  echo "- Credential：\`$credential_id\`"
  echo
  echo "## 证据"
  echo
  echo "- Summary：\`$SUMMARY\`"
  echo "- NDJSON：\`$REPORT\`"
  echo "- Credential response：\`$LOG_DIR/credential-response.json\`"
  echo "- Run response：\`$LOG_DIR/run-response.json\`"
  echo "- Worker run response：\`$LOG_DIR/worker-run-response.json\`"
  echo "- Neighbor response：\`$LOG_DIR/neighbors-response.json\`"
  echo
  echo "## 口径"
  echo
  echo "本报告通过真实 APISIX、auth-service、asset-service 和 PostgreSQL 验证 SNMP/LLDP 主动发现控制面：auth scope catalog 包含 asset:discover；viewer 仅 asset:read 时写接口返回 403；凭据只登记 Secret 引用、不接收明文；发现任务写入 asset_discovery_runs；成功写操作同步进入 audit_logs；观测资产写入 assets；LLDP 邻居关系写入 asset_topology_links；无 observations 的 scanner worker 路径会创建 failed run 并记录错误，不会静默停留 queued。"
} >"$LOCAL_REPORT"
cp "$LOCAL_REPORT" "$STABLE_MD"

echo "asset discovery preflight result: $RESULT"
echo "summary: $SUMMARY"
if [[ "$RESULT" != "pass" ]]; then
  exit 1
fi
