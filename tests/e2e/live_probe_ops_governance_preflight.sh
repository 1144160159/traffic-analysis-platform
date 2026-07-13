#!/usr/bin/env bash
set -euo pipefail

APISIX="${APISIX:-http://10.0.5.8:30180}"
TENANT="${TENANT:-default}"
OTHER_TENANT="${OTHER_TENANT:-tenant-b}"
LOG_DIR="${LOG_DIR:-doc/02_acceptance/runs/$(date +%Y%m%d%H%M%S)-probe-ops-governance-preflight}"
RUN_ID="${RUN_ID:-$(date +%Y%m%d%H%M%S)-probe-ops-governance-preflight}"
REGRESSION_DIR="${REGRESSION_DIR:-doc/02_acceptance/02-regression}"
KUBECTL="${KUBECTL:-kubectl}"
ALLOW_BLOCKERS="${ALLOW_BLOCKERS:-false}"
JWT_SECRET_NAMESPACE="${JWT_SECRET_NAMESPACE:-traffic-analysis}"
JWT_SECRET_NAME="${JWT_SECRET_NAME:-traffic-credentials}"
JWT_SECRET_KEY="${JWT_SECRET_KEY:-JWT_SECRET}"
PG_NAMESPACE="${PG_NAMESPACE:-databases}"
PG_POD="${PG_POD:-postgres-primary-0}"
PG_SECRET_NAMESPACE="${PG_SECRET_NAMESPACE:-traffic-analysis}"
PG_SECRET_NAME="${PG_SECRET_NAME:-traffic-credentials}"
PG_SECRET_KEY="${PG_SECRET_KEY:-PG_PASSWORD}"

REPORT="$LOG_DIR/live-probe-ops-governance-preflight-$RUN_ID.ndjson"
SUMMARY="$LOG_DIR/live-probe-ops-governance-preflight-$RUN_ID-summary.json"
LOCAL_REPORT="$LOG_DIR/local-report.md"

mkdir -p "$LOG_DIR" "$REGRESSION_DIR"
: >"$REPORT"

PG_PASSWORD=""
TEMP_FILES=()
TEST_PROBE_IDS=()

need_cmd() {
  if ! command -v "$1" >/dev/null 2>&1; then
    echo "missing required command: $1" >&2
    exit 2
  fi
}

kctl() {
  env -u HTTP_PROXY -u HTTPS_PROXY -u ALL_PROXY -u http_proxy -u https_proxy -u all_proxy "$KUBECTL" "$@"
}

new_tmp() {
  local file
  file="$(mktemp)"
  TEMP_FILES+=("$file")
  echo "$file"
}

trim_file() {
  local file="$1"
  if [[ -s "$file" ]]; then
    head -c 1200 "$file" \
      | tr '\n' ' ' \
      | sed -E 's/"token"[[:space:]]*:[[:space:]]*"[^"]+"/"token":"<redacted>"/g; s/Bearer [A-Za-z0-9._-]+/Bearer <redacted>/g'
  fi
}

sql_escape() {
  printf "%s" "$1" | sed "s/'/''/g"
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

psql_exec() {
  local sql="$1"
  kctl -n "$PG_NAMESPACE" exec "$PG_POD" -- env PGPASSWORD="$PG_PASSWORD" \
    psql -q -U postgres -d traffic_platform -v ON_ERROR_STOP=1 -Atc "$sql"
}

cleanup() {
  set +e
  if [[ -n "$PG_PASSWORD" && "${#TEST_PROBE_IDS[@]}" -gt 0 ]]; then
    local ids_sql=""
    local probe_id
    for probe_id in "${TEST_PROBE_IDS[@]}"; do
      [[ -n "$probe_id" ]] || continue
      if [[ -z "$ids_sql" ]]; then
        ids_sql="'$(sql_escape "$probe_id")'"
      else
        ids_sql="$ids_sql,'$(sql_escape "$probe_id")'"
      fi
    done
    if [[ -n "$ids_sql" ]]; then
      psql_exec "DELETE FROM probe_operations WHERE probe_id IN ($ids_sql); DELETE FROM probes WHERE probe_id IN ($ids_sql);" >/dev/null 2>&1 || true
    fi
  fi
  rm -f "${TEMP_FILES[@]:-}"
}
trap cleanup EXIT

ensure_user() {
  local tenant="$1" username="$2"
  local tenant_sql username_sql
  tenant_sql="$(sql_escape "$tenant")"
  username_sql="$(sql_escape "$username")"
  psql_exec "
    INSERT INTO tenants (tenant_id, tenant_name, name, status)
    VALUES ('$tenant_sql', '$tenant_sql', '$tenant_sql', 'active')
    ON CONFLICT (tenant_id) DO UPDATE
      SET tenant_name = COALESCE(NULLIF(tenants.tenant_name, ''), EXCLUDED.tenant_name),
          name = COALESCE(NULLIF(tenants.name, ''), EXCLUDED.name),
          status = 'active',
          updated_at = now();
    INSERT INTO users (user_id, tenant_id, username, email, status)
    VALUES (uuid_generate_v4(), '$tenant_sql', '$username_sql', '$username_sql@local', 'active')
    ON CONFLICT (tenant_id, username)
    DO UPDATE SET status = 'active', updated_at = now()
    RETURNING user_id;
  "
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
    "jti": str(uuid.uuid4()),
    "user_id": os.environ["USER_ID"],
    "tenant_id": os.environ["TENANT"],
    "username": os.environ["USERNAME"],
    "email": f"{os.environ['USERNAME']}@local",
    "roles": json.loads(os.environ["ROLES_JSON"]),
    "permissions": json.loads(os.environ["PERMS_JSON"]),
    "token_type": "access",
    "session_id": "codex-probe-ops-" + os.environ["RUN_ID"],
    "iat": now,
    "exp": now + int(os.environ["TTL"]),
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

ensure_probe_schema() {
  psql_exec "
    CREATE EXTENSION IF NOT EXISTS \"uuid-ossp\";
    ALTER TABLE probes ADD COLUMN IF NOT EXISTS hardware_info JSONB;
    ALTER TABLE probes ADD COLUMN IF NOT EXISTS software_version TEXT;
    ALTER TABLE probes ADD COLUMN IF NOT EXISTS updated_at TIMESTAMPTZ NOT NULL DEFAULT now();
    CREATE TABLE IF NOT EXISTS probe_operations (
      operation_id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
      tenant_id TEXT NOT NULL,
      probe_id TEXT NOT NULL,
      operation_type TEXT NOT NULL,
      status TEXT NOT NULL DEFAULT 'completed',
      requested_by TEXT NOT NULL DEFAULT '',
      request JSONB NOT NULL DEFAULT '{}'::jsonb,
      result JSONB NOT NULL DEFAULT '{}'::jsonb,
      created_at TIMESTAMPTZ NOT NULL DEFAULT now()
    );
    CREATE INDEX IF NOT EXISTS idx_probe_operations_tenant_probe_time ON probe_operations (tenant_id, probe_id, created_at DESC);
    CREATE INDEX IF NOT EXISTS idx_probe_operations_tenant_type_time ON probe_operations (tenant_id, operation_type, created_at DESC);
  " >/dev/null
}

upsert_probe() {
  local tenant="$1" probe_id="$2" name="$3"
  local tenant_sql probe_sql name_sql hardware hardware_sql
  tenant_sql="$(sql_escape "$tenant")"
  probe_sql="$(sql_escape "$probe_id")"
  name_sql="$(sql_escape "$name")"
  hardware="$(jq -nc --arg host "$name" --arg ip "10.66.30.10" '{hostname:$host,ip_address:$ip,capture_mode:"af_packet",interfaces:["eth2","eth3"],cpu_usage:12.4,drop_rate:0.01,bandwidth_mbps:9800,health_score:99}')"
  hardware_sql="$(sql_escape "$hardware")"
  psql_exec "
    INSERT INTO probes (probe_id, tenant_id, name, status, hardware_info, software_version, last_heartbeat, updated_at)
    VALUES ('$probe_sql', '$tenant_sql', '$name_sql', 'active', '$hardware_sql'::jsonb, 'v3.4.7', now(), now())
    ON CONFLICT (probe_id) DO UPDATE
      SET tenant_id = EXCLUDED.tenant_id,
          name = EXCLUDED.name,
          status = 'active',
          hardware_info = EXCLUDED.hardware_info,
          software_version = EXCLUDED.software_version,
          last_heartbeat = now(),
          updated_at = now();
  " >/dev/null
}

curl_json() {
  local name="$1" method="$2" path="$3" expected_code="$4" token="$5" output="$6" body="${7:-}" tenant_header="${8:-$TENANT}"
  local err_file code rc
  err_file="$output.err"
  set +e
  if [[ -n "$body" ]]; then
    code="$(curl --noproxy '*' -sS -m 30 -o "$output" -w '%{http_code}' \
      -X "$method" \
      -H "Authorization: Bearer $token" \
      -H "X-Tenant-ID: $tenant_header" \
      -H "Content-Type: application/json" \
      --data "$body" \
      "$APISIX$path" 2>"$err_file")"
  else
    code="$(curl --noproxy '*' -sS -m 30 -o "$output" -w '%{http_code}' \
      -X "$method" \
      -H "Authorization: Bearer $token" \
      -H "X-Tenant-ID: $tenant_header" \
      "$APISIX$path" 2>"$err_file")"
  fi
  rc=$?
  set -e
  if [[ "$rc" -ne 0 ]]; then
    json_log "api" "$name" "blocker" false "curl-rc=$rc" "$(trim_file "$err_file")" "$(basename "$err_file")"
    return 1
  fi
  if [[ "$code" == "$expected_code" ]]; then
    json_log "api" "$name" "info" true "$code" "$(trim_file "$output")" "$(basename "$output")"
    return 0
  fi
  json_log "api" "$name" "blocker" false "http-$code" "expected=$expected_code body=$(trim_file "$output")" "$(basename "$output")"
  return 1
}

assert_json() {
  local name="$1" file="$2"
  shift 2
  local jq_args=()
  while [[ "$#" -gt 1 ]]; do
    jq_args+=("$1")
    shift
  done
  local filter="$1"
  if jq -e "${jq_args[@]}" "$filter" "$file" >/dev/null 2>&1; then
    json_log "assert" "$name" "info" true "ok" "$filter" "$(basename "$file")"
  else
    json_log "assert" "$name" "blocker" false "failed" "$filter body=$(trim_file "$file")" "$(basename "$file")"
  fi
}

assert_psql_count() {
  local name="$1" sql="$2" artifact="$3"
  local out="$LOG_DIR/$artifact"
  set +e
  psql_exec "$sql" >"$out" 2>"$out.err"
  local rc=$?
  set -e
  local count
  count="$(tr -d '[:space:]' <"$out" 2>/dev/null || true)"
  if [[ "$rc" -eq 0 && "$count" =~ ^[0-9]+$ && "$count" -ge 1 ]]; then
    json_log "postgres" "$name" "info" true "ok" "$count" "$artifact"
  else
    json_log "postgres" "$name" "blocker" false "missing" "$(trim_file "$out.err")" "$(basename "$out.err")"
  fi
}

wait_for_audit() {
  local name="$1" action="$2" object_id="$3" artifact="$4" min_count="${5:-1}"
  local out="$LOG_DIR/$artifact"
  local object_id_sql
  object_id_sql="$(sql_escape "$object_id")"
  for _ in $(seq 1 20); do
    set +e
    psql_exec "SELECT count(*) FROM audit_logs WHERE tenant_id = '$TENANT' AND action = '$action' AND object_id = '$object_id_sql';" >"$out" 2>"$out.err"
    local rc=$?
    set -e
    local count
    count="$(tr -d '[:space:]' <"$out" 2>/dev/null || true)"
    if [[ "$rc" -eq 0 && "$count" =~ ^[0-9]+$ && "$count" -ge "$min_count" ]]; then
      json_log "audit" "$name" "info" true "ok" "action=$action object_id=$object_id count=$count" "$artifact"
      return
    fi
    sleep 1
  done
  json_log "audit" "$name" "blocker" false "missing" "action=$action object_id=$object_id err=$(trim_file "$out.err")" "$(basename "$out.err")"
}

finish() {
  local total passed blockers warnings result
  total="$(jq -s 'length' "$REPORT")"
  passed="$(jq -s '[.[] | select(.passed == true)] | length' "$REPORT")"
  blockers="$(jq -s '[.[] | select(.passed == false and .severity == "blocker")] | length' "$REPORT")"
  warnings="$(jq -s '[.[] | select(.severity == "warn")] | length' "$REPORT")"
  result="pass"
  if [[ "$blockers" -gt 0 ]]; then
    result="blocked"
  elif [[ "$warnings" -gt 0 ]]; then
    result="warn"
  fi

  jq -n \
    --arg run_id "$RUN_ID" \
    --arg result "$result" \
    --arg apisix "$APISIX" \
    --arg tenant "$TENANT" \
    --arg other_tenant "$OTHER_TENANT" \
    --arg probe_id "${PROBE_ID:-}" \
    --arg other_probe_id "${OTHER_PROBE_ID:-}" \
    --arg config_operation_id "${CONFIG_OPERATION_ID:-}" \
    --arg cert_operation_id "${CERT_OPERATION_ID:-}" \
    --arg batch_id "${BATCH_ID:-}" \
    --arg report "$REPORT" \
    --arg local_report "$LOCAL_REPORT" \
    --argjson total "$total" \
    --argjson passed "$passed" \
    --argjson blockers "$blockers" \
    --argjson warnings "$warnings" \
    --slurpfile checks "$REPORT" \
    '{
      run_id:$run_id,
      result:$result,
      apisix:$apisix,
      tenant:$tenant,
      other_tenant:$other_tenant,
      probe_id:$probe_id,
      other_probe_id:$other_probe_id,
      config_operation_id:$config_operation_id,
      cert_operation_id:$cert_operation_id,
      batch_id:$batch_id,
      report:$report,
      local_report:$local_report,
      total:$total,
      passed:$passed,
      blockers:$blockers,
      warnings:$warnings,
      checks:$checks
    }' >"$SUMMARY"

  cat >"$LOCAL_REPORT" <<MD
# Probe Operations Governance Live Preflight

- Run: \`$RUN_ID\`
- Result: \`$result\`
- Checks: \`$passed/$total\` passed, \`$blockers\` blockers, \`$warnings\` warnings
- Probe: \`${PROBE_ID:-missing}\`
- Config operation: \`${CONFIG_OPERATION_ID:-missing}\`
- Cert operation: \`${CERT_OPERATION_ID:-missing}\`
- Upgrade batch: \`${BATCH_ID:-missing}\`

This gate closes the probe-management loop for config push, connectivity
test, mTLS certificate rotation by Kubernetes secret reference, batch upgrade,
viewer denial, tenant isolation, PostgreSQL persistence and audit-log evidence.
MD

  cp "$SUMMARY" "$REGRESSION_DIR/probe-ops-governance-preflight-latest.json"
  cp "$LOCAL_REPORT" "$REGRESSION_DIR/probe-ops-governance-preflight-latest.md"
  cp "$LOG_DIR/probe-config-push.json" "$REGRESSION_DIR/probe-ops-config-push-latest.json" 2>/dev/null || true
  cp "$LOG_DIR/probe-cert-rotate.json" "$REGRESSION_DIR/probe-ops-cert-rotate-latest.json" 2>/dev/null || true
  cp "$LOG_DIR/probe-batch-upgrade.json" "$REGRESSION_DIR/probe-ops-batch-upgrade-latest.json" 2>/dev/null || true

  echo "probe ops governance preflight result: $result"
  echo "summary: $SUMMARY"
  echo "local report: $LOCAL_REPORT"

  if [[ "$result" == "blocked" && "$ALLOW_BLOCKERS" != "true" ]]; then
    exit 1
  fi
}

for cmd in git jq curl python3 "$KUBECTL"; do
  need_cmd "$cmd"
done

git rev-parse HEAD >"$LOG_DIR/commit-sha.txt"
git status --short >"$LOG_DIR/git-status.txt"

JWT_SECRET="$(kctl -n "$JWT_SECRET_NAMESPACE" get secret "$JWT_SECRET_NAME" -o "jsonpath={.data.$JWT_SECRET_KEY}" | base64 -d)"
PG_PASSWORD="$(kctl -n "$PG_SECRET_NAMESPACE" get secret "$PG_SECRET_NAME" -o "jsonpath={.data.$PG_SECRET_KEY}" | base64 -d)"

OPERATOR_USERNAME="codex-probe-operator"
VIEWER_USERNAME="codex-probe-viewer"
OTHER_USERNAME="codex-probe-other-admin"
OPERATOR_USER_ID="$(ensure_user "$TENANT" "$OPERATOR_USERNAME" | grep -E '^[0-9a-fA-F-]{36}$' | tail -n 1 | tr -d '[:space:]')"
VIEWER_USER_ID="$(ensure_user "$TENANT" "$VIEWER_USERNAME" | grep -E '^[0-9a-fA-F-]{36}$' | tail -n 1 | tr -d '[:space:]')"
OTHER_USER_ID="$(ensure_user "$OTHER_TENANT" "$OTHER_USERNAME" | grep -E '^[0-9a-fA-F-]{36}$' | tail -n 1 | tr -d '[:space:]')"

OPERATOR_TOKEN="$(make_token "$OPERATOR_USERNAME" "$TENANT" "$OPERATOR_USER_ID" '["operator"]' '["probe:write","probe:metrics","alert:read"]')"
VIEWER_TOKEN="$(make_token "$VIEWER_USERNAME" "$TENANT" "$VIEWER_USER_ID" '["viewer"]' '["probe:metrics","alert:read"]')"
OTHER_TOKEN="$(make_token "$OTHER_USERNAME" "$OTHER_TENANT" "$OTHER_USER_ID" '["admin"]' '["*","admin:*","probe:write"]')"

ensure_probe_schema

PROBE_ID="codex-probe-ops-$RUN_ID"
OTHER_PROBE_ID="codex-probe-ops-other-$RUN_ID"
TEST_PROBE_IDS+=("$PROBE_ID" "$OTHER_PROBE_ID")
upsert_probe "$TENANT" "$PROBE_ID" "Codex Probe Ops"
upsert_probe "$OTHER_TENANT" "$OTHER_PROBE_ID" "Codex Probe Ops Other"

if grep -q "probe-batch-upgrade" web/ui/src/services/pageApiPlans.ts \
  && grep -q "probe-config-push" web/ui/src/services/pageApiPlans.ts \
  && grep -q "probe-cert-rotate" web/ui/src/services/pageApiPlans.ts; then
  json_log "contract" "frontend probe operation contract present" "info" true "ok" "probe actions declared" "web/ui/src/services/pageApiPlans.ts"
else
  json_log "contract" "frontend probe operation contract present" "blocker" false "missing" "probe action contract missing" "web/ui/src/services/pageApiPlans.ts"
fi

curl_json "probe list readable" "GET" "/api/v1/probes?limit=10" "200" "$OPERATOR_TOKEN" "$LOG_DIR/probe-list.json"
assert_json "probe list includes test probe" "$LOG_DIR/probe-list.json" --arg probe "$PROBE_ID" '.success == true and ((.data.probes // []) | map(.probe_id) | index($probe) != null)'

config_version="codex-probe-config-$RUN_ID"
config_payload="$(jq -nc --arg version "$config_version" '{config_version:$version,capture_mode:"af_packet",interfaces:["eth2","eth3"],archive_path:"s3://pcap-archive/codex/",batch_send_mbps:2000,reason:"codex live probe ops"}')"
curl_json "operator pushes probe config" "POST" "/api/v1/probes/$PROBE_ID/config" "200" "$OPERATOR_TOKEN" "$LOG_DIR/probe-config-push.json" "$config_payload"
assert_json "probe config push response shape" "$LOG_DIR/probe-config-push.json" --arg version "$config_version" '.success == true and .data.operation_id and .data.applied == true and .data.config_version == $version'
CONFIG_OPERATION_ID="$(jq -r '.data.operation_id // empty' "$LOG_DIR/probe-config-push.json")"

viewer_body="$LOG_DIR/probe-config-viewer-denied.json"
curl_json "viewer cannot push probe config" "POST" "/api/v1/probes/$PROBE_ID/config" "403" "$VIEWER_TOKEN" "$viewer_body" "$config_payload" || true
assert_json "viewer probe config denial mentions permission" "$viewer_body" '((.error.message // .message // "")) | contains("probe:write")'

other_body="$LOG_DIR/probe-config-cross-tenant-denied.json"
curl_json "other tenant cannot operate default probe" "POST" "/api/v1/probes/$PROBE_ID/config" "404" "$OTHER_TOKEN" "$other_body" "$config_payload" "$OTHER_TENANT" || true
assert_json "cross tenant probe operation is isolated" "$other_body" '(.error.code // .code // "") == "NOT_FOUND"'

connectivity_payload="$(jq -nc '{targets:["ingest-gateway","kafka","clickhouse"],reason:"codex live connectivity"}')"
curl_json "operator runs connectivity test" "POST" "/api/v1/probes/$PROBE_ID/connectivity-test" "200" "$OPERATOR_TOKEN" "$LOG_DIR/probe-connectivity-test.json" "$connectivity_payload"
assert_json "connectivity response has passing checks" "$LOG_DIR/probe-connectivity-test.json" '.success == true and .data.operation_id and (.data.checks | type == "array") and (.data.checks | length >= 3) and all(.data.checks[]; .status == "pass")'

cert_payload="$(jq -nc '{secret_ref:"k8s://traffic-analysis/traffic-credentials#PROBE_MTLS_CERT",rotation_window:"immediate",reason:"codex live cert rotation"}')"
curl_json "operator rotates probe certificate" "POST" "/api/v1/probes/$PROBE_ID/certificates/rotate" "200" "$OPERATOR_TOKEN" "$LOG_DIR/probe-cert-rotate.json" "$cert_payload"
assert_json "cert rotation response uses secret ref only" "$LOG_DIR/probe-cert-rotate.json" '.success == true and .data.operation_id and (.data.secret_ref | startswith("k8s://")) and (.data.private_key? == null) and (.data.certificate? == null)'
CERT_OPERATION_ID="$(jq -r '.data.operation_id // empty' "$LOG_DIR/probe-cert-rotate.json")"

plaintext_cert_payload="$(jq -nc '{secret_ref:"k8s://traffic-analysis/traffic-credentials#PROBE_MTLS_CERT",private_key:"not-for-storage"}')"
plaintext_body="$LOG_DIR/probe-cert-plaintext-rejected.json"
curl_json "plaintext certificate material rejected" "POST" "/api/v1/probes/$PROBE_ID/certificates/rotate" "400" "$OPERATOR_TOKEN" "$plaintext_body" "$plaintext_cert_payload" || true
assert_json "plaintext rejection mentions secret material" "$plaintext_body" '((.error.message // .message // "")) | contains("plaintext")'

target_version="v3.4.8-codex-$RUN_ID"
batch_payload="$(jq -nc --arg probe "$PROBE_ID" --arg version "$target_version" '{probe_ids:[$probe],target_version:$version,rollout_strategy:"canary",reason:"codex live batch upgrade"}')"
curl_json "operator runs batch upgrade" "POST" "/api/v1/probes/batch-upgrade" "200" "$OPERATOR_TOKEN" "$LOG_DIR/probe-batch-upgrade.json" "$batch_payload"
assert_json "batch upgrade response shape" "$LOG_DIR/probe-batch-upgrade.json" --arg version "$target_version" '.success == true and .data.batch_id and .data.upgraded_count == 1 and .data.target_version == $version'
BATCH_ID="$(jq -r '.data.batch_id // empty' "$LOG_DIR/probe-batch-upgrade.json")"

PROBE_ID_SQL="$(sql_escape "$PROBE_ID")"
CONFIG_VERSION_SQL="$(sql_escape "$config_version")"
TARGET_VERSION_SQL="$(sql_escape "$target_version")"
assert_psql_count "config operation persisted" \
  "SELECT count(*) FROM probe_operations WHERE tenant_id = '$TENANT' AND probe_id = '$PROBE_ID_SQL' AND operation_type = 'config_push' AND request->>'config_version' = '$CONFIG_VERSION_SQL';" \
  "pg-probe-config-operation-count.txt"
assert_psql_count "connectivity operation persisted" \
  "SELECT count(*) FROM probe_operations WHERE tenant_id = '$TENANT' AND probe_id = '$PROBE_ID_SQL' AND operation_type = 'connectivity_test';" \
  "pg-probe-connectivity-operation-count.txt"
assert_psql_count "cert rotation operation persisted without plaintext" \
  "SELECT count(*) FROM probe_operations WHERE tenant_id = '$TENANT' AND probe_id = '$PROBE_ID_SQL' AND operation_type = 'cert_rotate' AND request->>'secret_ref' LIKE 'k8s://%' AND request::text NOT LIKE '%not-for-storage%';" \
  "pg-probe-cert-operation-count.txt"
assert_psql_count "batch upgrade operation persisted" \
  "SELECT count(*) FROM probe_operations WHERE tenant_id = '$TENANT' AND probe_id = '$PROBE_ID_SQL' AND operation_type = 'batch_upgrade' AND result->>'target_version' = '$TARGET_VERSION_SQL';" \
  "pg-probe-batch-operation-count.txt"
assert_psql_count "probe software version updated" \
  "SELECT count(*) FROM probes WHERE tenant_id = '$TENANT' AND probe_id = '$PROBE_ID_SQL' AND software_version = '$TARGET_VERSION_SQL';" \
  "pg-probe-version-updated-count.txt"

wait_for_audit "config push audit row exists" "PROBE_CONFIG_PUSH" "$PROBE_ID" "pg-probe-config-audit-count.txt"
wait_for_audit "connectivity test audit row exists" "PROBE_CONNECTIVITY_TEST" "$PROBE_ID" "pg-probe-connectivity-audit-count.txt"
wait_for_audit "cert rotate audit row exists" "PROBE_CERT_ROTATE" "$PROBE_ID" "pg-probe-cert-audit-count.txt"
wait_for_audit "batch upgrade audit row exists" "PROBE_BATCH_UPGRADE" "$BATCH_ID" "pg-probe-batch-audit-count.txt"

if grep -R -E 'not-for-storage|Bearer [A-Za-z0-9._-]+' "$LOG_DIR" >/dev/null 2>&1; then
  json_log "security" "probe artifacts do not contain plaintext secrets" "blocker" false "leak-detected" "plaintext secret marker found in log dir" "$LOG_DIR"
else
  json_log "security" "probe artifacts do not contain plaintext secrets" "info" true "ok" "no plaintext secret marker found" "$LOG_DIR"
fi

finish
