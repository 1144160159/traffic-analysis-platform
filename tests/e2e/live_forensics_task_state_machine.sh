#!/usr/bin/env bash
set -euo pipefail

APISIX="${APISIX:-http://10.0.5.8:30180}"
TENANT="${TENANT:-default}"
OTHER_TENANT="${OTHER_TENANT:-campus-a}"
LOG_DIR="${LOG_DIR:-doc/02_acceptance/runs/20260629-forensics-task-state-machine}"
RUN_ID="${RUN_ID:-$(date +%Y%m%d%H%M%S)-$$}"
KUBECTL="${KUBECTL:-kubectl}"
JWT_SECRET_NAMESPACE="${JWT_SECRET_NAMESPACE:-traffic-analysis}"
JWT_SECRET_NAME="${JWT_SECRET_NAME:-traffic-credentials}"
JWT_SECRET_KEY="${JWT_SECRET_KEY:-JWT_SECRET}"
PG_SECRET_KEY="${PG_SECRET_KEY:-PG_PASSWORD}"

REPORT="$LOG_DIR/live-forensics-task-state-machine-$RUN_ID.ndjson"
SUMMARY="$LOG_DIR/live-forensics-task-state-machine-$RUN_ID-summary.json"
LATEST_JSON="doc/02_acceptance/02-regression/forensics-task-state-machine-latest.json"
LATEST_MD="doc/02_acceptance/02-regression/forensics-task-state-machine-latest.md"
mkdir -p "$LOG_DIR" "$(dirname "$LATEST_JSON")"

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
}

trim_file() {
  local file="$1"
  if [[ -s "$file" ]]; then
    head -c 800 "$file" | tr '\n' ' '
  fi
}

need_cmd curl
need_cmd jq
need_cmd python3
need_cmd "$KUBECTL"

RUN_SLUG="$(echo "$RUN_ID" | tr -c 'A-Za-z0-9-' '-' | sed 's/-$//')"
CANCEL_TASK_ID="$(python3 - <<'PY'
import uuid
print(uuid.uuid4())
PY
)"
COMPLETED_TASK_ID="$(python3 - <<'PY'
import uuid
print(uuid.uuid4())
PY
)"
CROSS_TASK_ID="$(python3 - <<'PY'
import uuid
print(uuid.uuid4())
PY
)"

JWT_SECRET="$(kctl -n "$JWT_SECRET_NAMESPACE" get secret "$JWT_SECRET_NAME" -o "jsonpath={.data.$JWT_SECRET_KEY}" | base64 -d)"
PGPASS="$(kctl -n "$JWT_SECRET_NAMESPACE" get secret "$JWT_SECRET_NAME" -o "jsonpath={.data.$PG_SECRET_KEY}" | base64 -d)"

make_token() {
  local username="$1"
  local tenant="$2"
  local roles_json="$3"
  local perms_json="$4"
  JWT_SECRET="$JWT_SECRET" TENANT="$tenant" USERNAME="$username" ROLES_JSON="$roles_json" PERMS_JSON="$perms_json" python3 - <<'PY'
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
    "session_id": "codex-forensics-state-" + str(uuid.uuid4()),
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

FORENSICS_TOKEN="$(make_token codex-forensics-operator "$TENANT" '["analyst"]' '["pcap:read","pcap:write","pcap:download"]')"
AUDIT_TOKEN="$(make_token codex-forensics-auditor "$TENANT" '["admin"]' '["audit:read","pcap:read","admin:*"]')"

psql_exec() {
  local sql="$1"
  kctl -n databases exec postgres-primary-0 -- env PGPASSWORD="$PGPASS" psql -U postgres -d traffic_platform -v ON_ERROR_STOP=1 -Atc "$sql"
}

psql_exec "
  INSERT INTO tasks (task_id, tenant_id, task_type, params, status, progress, created_by, created_at, updated_at)
  VALUES
    ('$CANCEL_TASK_ID', '$TENANT', 'pcap_cut', '{\"source\":\"live_forensics_task_state_machine\",\"case\":\"cancel\"}'::jsonb, 'processing', 37, 'codex-live-e2e', now(), now()),
    ('$COMPLETED_TASK_ID', '$TENANT', 'pcap_cut', '{\"source\":\"live_forensics_task_state_machine\",\"case\":\"completed\"}'::jsonb, 'completed', 100, 'codex-live-e2e', now(), now()),
    ('$CROSS_TASK_ID', '$OTHER_TENANT', 'pcap_cut', '{\"source\":\"live_forensics_task_state_machine\",\"case\":\"cross\"}'::jsonb, 'processing', 42, 'codex-live-e2e', now(), now())
  ON CONFLICT (task_id) DO UPDATE SET
    tenant_id = EXCLUDED.tenant_id,
    task_type = EXCLUDED.task_type,
    params = EXCLUDED.params,
    status = EXCLUDED.status,
    progress = EXCLUDED.progress,
    updated_at = now();" >/dev/null
json_log "fixture" "seed processing/completed/cross-tenant tasks" true "ok" "$CANCEL_TASK_ID,$COMPLETED_TASK_ID,$CROSS_TASK_ID"

curl_check() {
  local name="$1" method="$2" path="$3" expected="$4" data="${5:-}" filter="${6:-}" token="${7:-$FORENSICS_TOKEN}" tenant="${8:-$TENANT}"
  local body_file err_file code rc ok detail
  body_file="$LOG_DIR/$RUN_SLUG-${name// /-}.json"
  err_file="$(mktemp)"
  local args=(--noproxy '*' -sS -m 20 -o "$body_file" -w '%{http_code}' -X "$method" -H "X-Tenant-ID: $tenant")
  if [[ -n "$token" ]]; then
    args+=(-H "Authorization: Bearer $token")
  fi
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
  elif [[ -n "$filter" ]] && ! CANCEL_TASK_ID="$CANCEL_TASK_ID" COMPLETED_TASK_ID="$COMPLETED_TASK_ID" CROSS_TASK_ID="$CROSS_TASK_ID" jq -e "$filter" "$body_file" >/dev/null 2>"$err_file"; then
    ok=false
    detail="jq filter failed filter=$filter body=$(trim_file "$body_file") err=$(trim_file "$err_file")"
  else
    ok=true
    detail="$method $path"
  fi
  json_log "api" "$name" "$ok" "$code" "$detail" "$body_file"
  rm -f "$err_file"
  [[ "$ok" == true ]]
}

curl_check "cancel processing task" "POST" "/api/v1/pcap/jobs/$CANCEL_TASK_ID/cancel" "200" "" \
  '.success == true and .data.job_id == env.CANCEL_TASK_ID and .data.status == "cancelled"'

cancel_db_file="$LOG_DIR/$RUN_SLUG-cancel-db-status.txt"
psql_exec "SELECT status || '|' || (completed_at IS NOT NULL)::text FROM tasks WHERE task_id = '$CANCEL_TASK_ID';" >"$cancel_db_file"
if grep -qx 'cancelled|true' "$cancel_db_file"; then
  json_log "db" "cancelled task persisted with completed_at" true "ok" "$(cat "$cancel_db_file")" "$cancel_db_file"
else
  json_log "db" "cancelled task persisted with completed_at" false "failed" "$(cat "$cancel_db_file")" "$cancel_db_file"
  exit 1
fi

curl_check "completed task cannot be cancelled" "POST" "/api/v1/pcap/jobs/$COMPLETED_TASK_ID/cancel" "409" "" \
  '.code == "INVALID_STATE" or .error.code == "INVALID_STATE" or (.message // "" | contains("cannot cancel"))'

curl_check "cross tenant cancel rejected" "POST" "/api/v1/pcap/jobs/$CROSS_TASK_ID/cancel" "403" "" \
  '.code == "FORBIDDEN" or .error.code == "FORBIDDEN" or (.message // "" | contains("Access denied"))'

cross_db_file="$LOG_DIR/$RUN_SLUG-cross-db-status.txt"
psql_exec "SELECT status FROM tasks WHERE task_id = '$CROSS_TASK_ID';" >"$cross_db_file"
if grep -qx 'processing' "$cross_db_file"; then
  json_log "db" "cross tenant task remains processing" true "ok" "$(cat "$cross_db_file")" "$cross_db_file"
else
  json_log "db" "cross tenant task remains processing" false "failed" "$(cat "$cross_db_file")" "$cross_db_file"
  exit 1
fi

audit_file="$LOG_DIR/$RUN_SLUG-audit.json"
curl --noproxy '*' -sS -m 20 -H "Authorization: Bearer $AUDIT_TOKEN" -H "X-Tenant-ID: $TENANT" \
  "$APISIX/api/v1/audit/logs?object_type=pcap&limit=50" >"$audit_file"
if jq -e --arg id "$CANCEL_TASK_ID" '.success == true and any(.data.trails[]; .resource_id == $id and .action == "PCAP_CANCEL" and .details.mode == "cancel" and .details.previous_status == "processing")' "$audit_file" >/dev/null; then
  json_log "audit" "pcap cancel audit queryable" true "200" "$CANCEL_TASK_ID" "$audit_file"
else
  json_log "audit" "pcap cancel audit queryable" false "200" "body=$(trim_file "$audit_file")" "$audit_file"
  exit 1
fi

jq -s \
  --arg run_id "$RUN_ID" \
  --arg tenant "$TENANT" \
  --arg other_tenant "$OTHER_TENANT" \
  --arg cancel_task_id "$CANCEL_TASK_ID" \
  --arg completed_task_id "$COMPLETED_TASK_ID" \
  --arg cross_task_id "$CROSS_TASK_ID" \
  --arg report "$REPORT" \
  '{
    run_id: $run_id,
    result: (if all(.[]; .ok == true) then "pass" else "fail" end),
    tenant: $tenant,
    other_tenant: $other_tenant,
    cancel_task_id: $cancel_task_id,
    completed_task_id: $completed_task_id,
    cross_task_id: $cross_task_id,
    total: length,
    passed: map(select(.ok == true)) | length,
    failed: map(select(.ok != true)) | length,
    blockers: map(select(.ok != true)) | length,
    warnings: 0,
    report: $report,
    checks: .
  }' "$REPORT" >"$SUMMARY"

cp "$SUMMARY" "$LATEST_JSON"

cat >"$LATEST_MD" <<EOF
# Forensics task state-machine live 预检

- Run ID：\`$RUN_ID\`
- 结果：\`$(jq -r '.result' "$SUMMARY")\`
- APISIX：\`$APISIX\`
- 取消任务：\`$CANCEL_TASK_ID\`
- 检查数：$(jq -r '.passed' "$SUMMARY")/$(jq -r '.total' "$SUMMARY") passed，blockers=$(jq -r '.blockers' "$SUMMARY")，warnings=$(jq -r '.warnings' "$SUMMARY")

## 证据

- NDJSON：\`$REPORT\`
- Summary：\`$SUMMARY\`
- API/DB/Audit 响应：\`$LOG_DIR/$RUN_SLUG-*.json\`、\`$LOG_DIR/$RUN_SLUG-*.txt\`

## 口径

本报告验证取证任务取消状态机：processing 任务可取消并持久化为 cancelled，completed 任务取消返回 409，跨租户取消返回 403 且原任务状态不变，成功取消同步写入 \`audit_logs\` 的 \`PCAP_CANCEL\` 事件。
EOF

cat "$SUMMARY"
