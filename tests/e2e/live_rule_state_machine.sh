#!/usr/bin/env bash
set -euo pipefail

APISIX="${APISIX:-http://10.0.5.8:30180}"
TENANT="${TENANT:-default}"
OTHER_TENANT="${OTHER_TENANT:-campus-a}"
LOG_DIR="${LOG_DIR:-doc/02_acceptance/runs/20260629-rule-state-machine}"
RUN_ID="${RUN_ID:-$(date +%Y%m%d%H%M%S)-rule-state-machine}"
KUBECTL="${KUBECTL:-kubectl}"
JWT_SECRET_NAMESPACE="${JWT_SECRET_NAMESPACE:-traffic-analysis}"
JWT_SECRET_NAME="${JWT_SECRET_NAME:-traffic-credentials}"
JWT_SECRET_KEY="${JWT_SECRET_KEY:-JWT_SECRET}"
PG_NAMESPACE="${PG_NAMESPACE:-databases}"
PG_POD="${PG_POD:-postgres-primary-0}"
PG_SECRET_KEY="${PG_SECRET_KEY:-PG_PASSWORD}"
REGRESSION_DIR="${REGRESSION_DIR:-doc/02_acceptance/02-regression}"

REPORT="$LOG_DIR/live-rule-state-machine-$RUN_ID.ndjson"
SUMMARY="$LOG_DIR/live-rule-state-machine-$RUN_ID-summary.json"
LATEST_JSON="$REGRESSION_DIR/rule-state-machine-latest.json"
LATEST_MD="$REGRESSION_DIR/rule-state-machine-latest.md"

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
    head -c 1200 "$file" | tr '\n' ' '
  fi
}

make_uuid() {
  python3 - <<'PY'
import uuid
print(uuid.uuid4())
PY
}

make_token() {
  local username="$1"
  local tenant="$2"
  local user_id="$3"
  local roles_json="$4"
  local perms_json="$5"
  JWT_SECRET="$JWT_SECRET" TENANT="$tenant" USER_ID="$user_id" USERNAME="$username" ROLES_JSON="$roles_json" PERMS_JSON="$perms_json" RUN_ID="$RUN_ID" python3 - <<'PY'
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
    "email": os.environ["USERNAME"] + "@local",
    "roles": json.loads(os.environ["ROLES_JSON"]),
    "permissions": json.loads(os.environ["PERMS_JSON"]),
    "token_type": "access",
    "session_id": "codex-rule-state-" + os.environ["RUN_ID"],
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

psql_exec() {
  local sql="$1"
  kctl -n "$PG_NAMESPACE" exec "$PG_POD" -- env PGPASSWORD="$PG_PASSWORD" \
    psql -U postgres -d traffic_platform -v ON_ERROR_STOP=1 -Atc "$sql"
}

curl_check() {
  local name="$1" method="$2" path="$3" expected="$4" data="${5:-}" filter="${6:-}"
  local token="${7:-$OPERATOR_TOKEN}" tenant="${8:-$TENANT}" user_id="${9:-$OPERATOR_USER_ID}" username="${10:-codex-rule-operator}" roles="${11:-operator}" perms="${12:-rule:read,rule:enable}"
  local body_file err_file code rc ok detail
  body_file="$LOG_DIR/$RUN_SLUG-${name// /-}.json"
  err_file="$body_file.err"
  local args=(
    --noproxy '*' -sS -m 20 -o "$body_file" -w '%{http_code}'
    -X "$method"
    -H "Authorization: Bearer $token"
    -H "X-Tenant-ID: $tenant"
    -H "X-User-ID: $user_id"
    -H "X-Username: $username"
    -H "X-Roles: $roles"
    -H "X-Permissions: $perms"
    -H "User-Agent: codex-rule-state-machine/$RUN_ID"
  )
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
  elif [[ -n "$filter" ]] && ! ENABLE_RULE_ID="$ENABLE_RULE_ID" DISABLE_RULE_ID="$DISABLE_RULE_ID" CROSS_RULE_ID="$CROSS_RULE_ID" VIEWER_RULE_ID="$VIEWER_RULE_ID" jq -e "$filter" "$body_file" >/dev/null 2>"$err_file"; then
    ok=false
    detail="jq filter failed filter=$filter body=$(trim_file "$body_file") err=$(trim_file "$err_file")"
  else
    ok=true
    detail="$method $path"
  fi
  json_log "api" "$name" "$ok" "$code" "$detail" "$body_file"
  [[ "$ok" == true ]]
}

assert_psql() {
  local name="$1" sql="$2" expected="$3"
  local out_file="$LOG_DIR/$RUN_SLUG-${name// /-}.txt"
  psql_exec "$sql" >"$out_file"
  if grep -qx "$expected" "$out_file"; then
    json_log "db" "$name" true "ok" "$(cat "$out_file")" "$out_file"
    return 0
  fi
  json_log "db" "$name" false "failed" "expected=$expected actual=$(trim_file "$out_file")" "$out_file"
  return 1
}

for cmd in curl jq python3 "$KUBECTL"; do
  need_cmd "$cmd"
done

RUN_SLUG="$(echo "$RUN_ID" | tr -c 'A-Za-z0-9-' '-' | sed 's/-$//')"
ENABLE_RULE_ID="$(make_uuid)"
DISABLE_RULE_ID="$(make_uuid)"
CROSS_RULE_ID="$(make_uuid)"
VIEWER_RULE_ID="$(make_uuid)"
OPERATOR_USER_ID="$(make_uuid)"
VIEWER_USER_ID="$(make_uuid)"
AUDITOR_USER_ID="$(make_uuid)"

JWT_SECRET="$(kctl -n "$JWT_SECRET_NAMESPACE" get secret "$JWT_SECRET_NAME" -o "jsonpath={.data.$JWT_SECRET_KEY}" | base64 -d)"
PG_PASSWORD="$(kctl -n "$JWT_SECRET_NAMESPACE" get secret "$JWT_SECRET_NAME" -o "jsonpath={.data.$PG_SECRET_KEY}" | base64 -d)"

OPERATOR_TOKEN="$(make_token codex-rule-operator "$TENANT" "$OPERATOR_USER_ID" '["operator"]' '["rule:read","rule:enable"]')"
VIEWER_TOKEN="$(make_token codex-rule-viewer "$TENANT" "$VIEWER_USER_ID" '["viewer"]' '["rule:read"]')"
AUDIT_TOKEN="$(make_token codex-rule-auditor "$TENANT" "$AUDITOR_USER_ID" '["admin"]' '["audit:read","rule:read","admin:*"]')"

psql_exec "
  INSERT INTO tenants (tenant_id, tenant_name, name, description, status) VALUES
    ('$TENANT', '$TENANT', '$TENANT', 'codex live rule state-machine tenant', 'active'),
    ('$OTHER_TENANT', '$OTHER_TENANT', '$OTHER_TENANT', 'codex live rule state-machine tenant', 'active')
  ON CONFLICT (tenant_id) DO UPDATE SET
    tenant_name = EXCLUDED.tenant_name,
    name = EXCLUDED.name,
    status = EXCLUDED.status,
    updated_at = now();

  INSERT INTO rules (
    rule_id, tenant_id, name, rule_type, engine, description,
    conditions, labels, severity, enabled, priority, version, status,
    created_by, updated_by, created_at, updated_at
  ) VALUES
    ('$ENABLE_RULE_ID', '$TENANT', 'codex rule enable $RUN_SLUG', 'custom', 'internal', 'live rule state-machine enable fixture',
      '{\"source\":\"live_rule_state_machine\",\"case\":\"enable\"}'::jsonb, ARRAY['codex','state-machine']::text[], 'medium', false, 50, 3, 'disabled',
      '$OPERATOR_USER_ID', '$OPERATOR_USER_ID', now(), now()),
    ('$DISABLE_RULE_ID', '$TENANT', 'codex rule disable $RUN_SLUG', 'custom', 'internal', 'live rule state-machine disable fixture',
      '{\"source\":\"live_rule_state_machine\",\"case\":\"disable\"}'::jsonb, ARRAY['codex','state-machine']::text[], 'high', true, 60, 5, 'active',
      '$OPERATOR_USER_ID', '$OPERATOR_USER_ID', now(), now()),
    ('$CROSS_RULE_ID', '$OTHER_TENANT', 'codex rule cross $RUN_SLUG', 'custom', 'internal', 'live rule state-machine cross-tenant fixture',
      '{\"source\":\"live_rule_state_machine\",\"case\":\"cross\"}'::jsonb, ARRAY['codex','state-machine']::text[], 'medium', true, 70, 7, 'active',
      '$OPERATOR_USER_ID', '$OPERATOR_USER_ID', now(), now()),
    ('$VIEWER_RULE_ID', '$TENANT', 'codex rule viewer $RUN_SLUG', 'custom', 'internal', 'live rule state-machine viewer fixture',
      '{\"source\":\"live_rule_state_machine\",\"case\":\"viewer\"}'::jsonb, ARRAY['codex','state-machine']::text[], 'low', false, 40, 2, 'disabled',
      '$OPERATOR_USER_ID', '$OPERATOR_USER_ID', now(), now())
  ON CONFLICT (rule_id) DO UPDATE SET
    tenant_id = EXCLUDED.tenant_id,
    name = EXCLUDED.name,
    rule_type = EXCLUDED.rule_type,
    engine = EXCLUDED.engine,
    description = EXCLUDED.description,
    conditions = EXCLUDED.conditions,
    labels = EXCLUDED.labels,
    severity = EXCLUDED.severity,
    enabled = EXCLUDED.enabled,
    priority = EXCLUDED.priority,
    version = EXCLUDED.version,
    status = EXCLUDED.status,
    updated_by = EXCLUDED.updated_by,
    updated_at = now();
" >/dev/null
json_log "fixture" "seed rules for enable disable cross viewer" true "ok" "$ENABLE_RULE_ID,$DISABLE_RULE_ID,$CROSS_RULE_ID,$VIEWER_RULE_ID"

curl_check "enable disabled rule" "POST" "/api/v1/rules/$ENABLE_RULE_ID/enable" "200" "" \
  '.success == true and .data.rule_id == env.ENABLE_RULE_ID and .data.enabled == true and .data.status == "active" and .data.version == 4'

assert_psql "enable rule persisted" \
  "SELECT enabled::text || '|' || status || '|' || version::text || '|' || (updated_by = '$OPERATOR_USER_ID')::text FROM rules WHERE rule_id = '$ENABLE_RULE_ID';" \
  "true|active|4|true"
assert_psql "enable version row persisted" \
  "SELECT count(*)::text FROM rule_versions WHERE rule_version = '$ENABLE_RULE_ID-v4' AND version = 4 AND rule_id = '$ENABLE_RULE_ID';" \
  "1"
assert_psql "enable outbox persisted" \
  "SELECT count(*)::text FROM rule_outbox WHERE rule_id = '$ENABLE_RULE_ID' AND event_type = 'enable';" \
  "1"

curl_check "disable active rule" "POST" "/api/v1/rules/$DISABLE_RULE_ID/disable" "200" "" \
  '.success == true and .data.rule_id == env.DISABLE_RULE_ID and .data.enabled == false and .data.status == "disabled" and .data.version == 6'

assert_psql "disable rule persisted" \
  "SELECT enabled::text || '|' || status || '|' || version::text || '|' || (updated_by = '$OPERATOR_USER_ID')::text FROM rules WHERE rule_id = '$DISABLE_RULE_ID';" \
  "false|disabled|6|true"
assert_psql "disable version row persisted" \
  "SELECT count(*)::text FROM rule_versions WHERE rule_version = '$DISABLE_RULE_ID-v6' AND version = 6 AND rule_id = '$DISABLE_RULE_ID';" \
  "1"
assert_psql "disable outbox persisted" \
  "SELECT count(*)::text FROM rule_outbox WHERE rule_id = '$DISABLE_RULE_ID' AND event_type = 'disable';" \
  "1"

curl_check "cross tenant disable rejected" "POST" "/api/v1/rules/$CROSS_RULE_ID/disable" "403" "" \
  '.code == "PERMISSION_DENIED" or .error.code == "PERMISSION_DENIED" or (.message // "" | contains("cross-tenant"))'
assert_psql "cross tenant rule unchanged" \
  "SELECT enabled::text || '|' || status || '|' || version::text FROM rules WHERE rule_id = '$CROSS_RULE_ID';" \
  "true|active|7"

curl_check "viewer enable rejected" "POST" "/api/v1/rules/$VIEWER_RULE_ID/enable" "403" "" \
  '.code == "PERMISSION_DENIED" or .error.code == "PERMISSION_DENIED" or (.message // "" | contains("permission denied"))' \
  "$VIEWER_TOKEN" "$TENANT" "$VIEWER_USER_ID" "codex-rule-viewer" "viewer" "rule:read"
assert_psql "viewer rejected rule unchanged" \
  "SELECT enabled::text || '|' || status || '|' || version::text FROM rules WHERE rule_id = '$VIEWER_RULE_ID';" \
  "false|disabled|2"

audit_file="$LOG_DIR/$RUN_SLUG-audit.json"
curl_check "audit trail query" "GET" "/api/v1/audit/logs?object_type=rule&limit=100" "200" "" \
  '.success == true' \
  "$AUDIT_TOKEN" "$TENANT" "$AUDITOR_USER_ID" "codex-rule-auditor" "admin" "audit:read,rule:read,admin:*"
cp "$LOG_DIR/$RUN_SLUG-audit-trail-query.json" "$audit_file" 2>/dev/null || true

if jq -e --arg enable_id "$ENABLE_RULE_ID" --arg disable_id "$DISABLE_RULE_ID" '
  .success == true
  and any(.data.trails[]; .resource_id == $enable_id and .action == "RULE_ENABLE" and .details.old_version == 3 and .details.new_version == 4 and .details.new_status == "active")
  and any(.data.trails[]; .resource_id == $disable_id and .action == "RULE_DISABLE" and .details.old_version == 5 and .details.new_version == 6 and .details.new_status == "disabled")
' "$audit_file" >/dev/null; then
  json_log "audit" "rule enable disable audit queryable" true "ok" "$ENABLE_RULE_ID,$DISABLE_RULE_ID" "$audit_file"
else
  json_log "audit" "rule enable disable audit queryable" false "failed" "body=$(trim_file "$audit_file")" "$audit_file"
  exit 1
fi

jq -s \
  --arg run_id "$RUN_ID" \
  --arg tenant "$TENANT" \
  --arg other_tenant "$OTHER_TENANT" \
  --arg enable_rule_id "$ENABLE_RULE_ID" \
  --arg disable_rule_id "$DISABLE_RULE_ID" \
  --arg cross_rule_id "$CROSS_RULE_ID" \
  --arg viewer_rule_id "$VIEWER_RULE_ID" \
  --arg report "$REPORT" \
  '{
    run_id: $run_id,
    result: (if all(.[]; .ok == true) then "pass" else "fail" end),
    tenant: $tenant,
    other_tenant: $other_tenant,
    enable_rule_id: $enable_rule_id,
    disable_rule_id: $disable_rule_id,
    cross_rule_id: $cross_rule_id,
    viewer_rule_id: $viewer_rule_id,
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
# Rule state-machine live preflight

- Run ID: \`$RUN_ID\`
- Result: \`$(jq -r '.result' "$SUMMARY")\`
- APISIX: \`$APISIX\`
- Enable rule: \`$ENABLE_RULE_ID\`
- Disable rule: \`$DISABLE_RULE_ID\`
- Checks: $(jq -r '.passed' "$SUMMARY")/$(jq -r '.total' "$SUMMARY") passed, blockers=$(jq -r '.blockers' "$SUMMARY"), warnings=$(jq -r '.warnings' "$SUMMARY")

## Evidence

- NDJSON: \`$REPORT\`
- Summary: \`$SUMMARY\`
- API/DB/Audit responses: \`$LOG_DIR/$RUN_SLUG-*.json\`, \`$LOG_DIR/$RUN_SLUG-*.txt\`

## Scope

This report validates the rule enable/disable state machine: a disabled rule can be enabled with \`rule:enable\`, an active rule can be disabled with \`rule:enable\`, both actions increment \`rules.version\`, create \`rule_versions\`, write \`rule_outbox\`, and persist queryable \`RULE_ENABLE\` / \`RULE_DISABLE\` audit rows. Cross-tenant and read-only requests are rejected and leave rule state unchanged.
EOF

cat "$SUMMARY"
