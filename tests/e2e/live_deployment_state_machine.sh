#!/usr/bin/env bash
set -euo pipefail

APISIX="${APISIX:-http://10.0.5.8:30180}"
TENANT="${TENANT:-codex-deployment-state}"
OTHER_TENANT="${OTHER_TENANT:-codex-deployment-cross}"
LOG_DIR="${LOG_DIR:-doc/02_acceptance/runs/20260629-deployment-state-machine}"
RUN_ID="${RUN_ID:-$(date +%Y%m%d%H%M%S)-deployment-state-machine}"
KUBECTL="${KUBECTL:-kubectl}"
JWT_SECRET_NAMESPACE="${JWT_SECRET_NAMESPACE:-traffic-analysis}"
JWT_SECRET_NAME="${JWT_SECRET_NAME:-traffic-credentials}"
JWT_SECRET_KEY="${JWT_SECRET_KEY:-JWT_SECRET}"
PG_NAMESPACE="${PG_NAMESPACE:-databases}"
PG_POD="${PG_POD:-postgres-primary-0}"
PG_SECRET_KEY="${PG_SECRET_KEY:-PG_PASSWORD}"
REGRESSION_DIR="${REGRESSION_DIR:-doc/02_acceptance/02-regression}"

REPORT="$LOG_DIR/live-deployment-state-machine-$RUN_ID.ndjson"
SUMMARY="$LOG_DIR/live-deployment-state-machine-$RUN_ID-summary.json"
LATEST_JSON="$REGRESSION_DIR/deployment-state-machine-latest.json"
LATEST_MD="$REGRESSION_DIR/deployment-state-machine-latest.md"

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
    "session_id": "codex-deployment-state-" + os.environ["RUN_ID"],
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
  local token="${7:-$OPERATOR_TOKEN}" tenant="${8:-$TENANT}" user_id="${9:-$OPERATOR_USER_ID}" username="${10:-codex-deploy-operator}" roles="${11:-operator}" perms="${12:-deploy:read,deploy:create,deploy:gray,deploy:activate,deploy:rollback}"
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
    -H "User-Agent: codex-deployment-state-machine/$RUN_ID"
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
  elif [[ -n "$filter" ]] && ! DEPLOYMENT_ID="$DEPLOYMENT_ID" INVALID_DEPLOYMENT_ID="$INVALID_DEPLOYMENT_ID" CROSS_DEPLOYMENT_ID="$CROSS_DEPLOYMENT_ID" VIEWER_DEPLOYMENT_ID="$VIEWER_DEPLOYMENT_ID" jq -e "$filter" "$body_file" >/dev/null 2>"$err_file"; then
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
OPERATOR_USER_ID="$(make_uuid)"
VIEWER_USER_ID="$(make_uuid)"
AUDITOR_USER_ID="$(make_uuid)"
DEPLOYMENT_ID="$(make_uuid)"
PREVIOUS_ACTIVE_ID="$(make_uuid)"
INVALID_DEPLOYMENT_ID="$(make_uuid)"
CROSS_DEPLOYMENT_ID="$(make_uuid)"
VIEWER_DEPLOYMENT_ID="$(make_uuid)"
RULE_ID="$(make_uuid)"
CROSS_RULE_ID="$(make_uuid)"
MODEL_ID="$(make_uuid)"
CROSS_MODEL_ID="$(make_uuid)"
FEATURE_SET_ID="fs-$RUN_SLUG"
CROSS_FEATURE_SET_ID="fs-cross-$RUN_SLUG"
RULE_VERSION="$RULE_ID-v1"
CROSS_RULE_VERSION="$CROSS_RULE_ID-v1"
MODEL_VERSION="mv-$RUN_SLUG"
CROSS_MODEL_VERSION="mv-cross-$RUN_SLUG"

JWT_SECRET="$(kctl -n "$JWT_SECRET_NAMESPACE" get secret "$JWT_SECRET_NAME" -o "jsonpath={.data.$JWT_SECRET_KEY}" | base64 -d)"
PG_PASSWORD="$(kctl -n "$JWT_SECRET_NAMESPACE" get secret "$JWT_SECRET_NAME" -o "jsonpath={.data.$PG_SECRET_KEY}" | base64 -d)"

OPERATOR_TOKEN="$(make_token codex-deploy-operator "$TENANT" "$OPERATOR_USER_ID" '["operator"]' '["deploy:read","deploy:create","deploy:gray","deploy:activate","deploy:rollback"]')"
VIEWER_TOKEN="$(make_token codex-deploy-viewer "$TENANT" "$VIEWER_USER_ID" '["viewer"]' '["deploy:read"]')"
AUDIT_TOKEN="$(make_token codex-deploy-auditor "$TENANT" "$AUDITOR_USER_ID" '["admin"]' '["audit:read","deploy:read","admin:*"]')"

psql_exec "
  INSERT INTO tenants (tenant_id, tenant_name, name, description, status) VALUES
    ('$TENANT', '$TENANT', '$TENANT', 'codex deployment state-machine tenant', 'active'),
    ('$OTHER_TENANT', '$OTHER_TENANT', '$OTHER_TENANT', 'codex deployment state-machine tenant', 'active')
  ON CONFLICT (tenant_id) DO UPDATE SET
    tenant_name = EXCLUDED.tenant_name,
    name = EXCLUDED.name,
    status = EXCLUDED.status,
    updated_at = now();

  INSERT INTO users (user_id, tenant_id, username, email, status) VALUES
    ('$OPERATOR_USER_ID', '$TENANT', 'codex-deploy-operator-$RUN_SLUG', 'codex-deploy-operator-$RUN_SLUG@local', 'active'),
    ('$VIEWER_USER_ID', '$TENANT', 'codex-deploy-viewer-$RUN_SLUG', 'codex-deploy-viewer-$RUN_SLUG@local', 'active'),
    ('$AUDITOR_USER_ID', '$TENANT', 'codex-deploy-auditor-$RUN_SLUG', 'codex-deploy-auditor-$RUN_SLUG@local', 'active')
  ON CONFLICT (user_id) DO UPDATE SET updated_at = now();

  INSERT INTO feature_sets (feature_set_id, tenant_id, name, params, schema_version, status)
  VALUES
    ('$FEATURE_SET_ID', '$TENANT', 'codex deployment state features $RUN_SLUG', '{\"source\":\"live_deployment_state_machine\"}'::jsonb, 'v1', 'active'),
    ('$CROSS_FEATURE_SET_ID', '$OTHER_TENANT', 'codex deployment cross features $RUN_SLUG', '{\"source\":\"live_deployment_state_machine\"}'::jsonb, 'v1', 'active')
  ON CONFLICT (feature_set_id) DO UPDATE SET updated_at = now();

  INSERT INTO models (model_id, tenant_id, name, model_type, description, metadata)
  VALUES
    ('$MODEL_ID', '$TENANT', 'codex deployment model $RUN_SLUG', 'classification', 'deployment state-machine fixture', '{\"source\":\"live_deployment_state_machine\"}'::jsonb),
    ('$CROSS_MODEL_ID', '$OTHER_TENANT', 'codex deployment cross model $RUN_SLUG', 'classification', 'deployment state-machine fixture', '{\"source\":\"live_deployment_state_machine\"}'::jsonb)
  ON CONFLICT (model_id) DO UPDATE SET updated_at = now();

  INSERT INTO model_versions (model_version, model_id, tenant_id, feature_set_id, artifact_uri, metrics, status, created_by)
  VALUES
    ('$MODEL_VERSION', '$MODEL_ID', '$TENANT', '$FEATURE_SET_ID', 's3://codex/deployment-state/$RUN_SLUG/model.bin', '{\"f1\":0.91}'::jsonb, 'registered', '$OPERATOR_USER_ID'),
    ('$CROSS_MODEL_VERSION', '$CROSS_MODEL_ID', '$OTHER_TENANT', '$CROSS_FEATURE_SET_ID', 's3://codex/deployment-state/$RUN_SLUG/cross-model.bin', '{\"f1\":0.91}'::jsonb, 'registered', NULL)
  ON CONFLICT (model_version) DO UPDATE SET updated_at = now();

  INSERT INTO rules (
    rule_id, tenant_id, name, rule_type, engine, description,
    conditions, labels, severity, enabled, priority, version, status,
    created_by, updated_by, created_at, updated_at
  ) VALUES
    ('$RULE_ID', '$TENANT', 'codex deploy rule $RUN_SLUG', 'custom', 'internal', 'deployment state-machine fixture',
      '{\"source\":\"live_deployment_state_machine\"}'::jsonb, ARRAY['codex','deployment-state']::text[], 'medium', true, 50, 1, 'active',
      '$OPERATOR_USER_ID', '$OPERATOR_USER_ID', now(), now()),
    ('$CROSS_RULE_ID', '$OTHER_TENANT', 'codex deploy cross rule $RUN_SLUG', 'custom', 'internal', 'deployment state-machine fixture',
      '{\"source\":\"live_deployment_state_machine\"}'::jsonb, ARRAY['codex','deployment-state']::text[], 'medium', true, 50, 1, 'active',
      '$OPERATOR_USER_ID', '$OPERATOR_USER_ID', now(), now())
  ON CONFLICT (rule_id) DO UPDATE SET updated_at = now();

  INSERT INTO rule_versions (rule_version, rule_id, tenant_id, version, content_uri, status, created_by)
  VALUES
    ('$RULE_VERSION', '$RULE_ID', '$TENANT', 1, 'inline:{\"source\":\"live_deployment_state_machine\"}', 'active', '$OPERATOR_USER_ID'),
    ('$CROSS_RULE_VERSION', '$CROSS_RULE_ID', '$OTHER_TENANT', 1, 'inline:{\"source\":\"live_deployment_state_machine\"}', 'active', '$OPERATOR_USER_ID')
  ON CONFLICT (rule_version) DO UPDATE SET status = EXCLUDED.status;

  INSERT INTO deployments (
    deployment_id, tenant_id, name, description, model_version, rule_version, feature_set_id,
    scope, status, created_by, created_at, updated_at, metadata
  ) VALUES
    ('$DEPLOYMENT_ID', '$TENANT', 'codex deployment main $RUN_SLUG', 'planned -> gray -> active -> pause -> resume -> rollback',
      '$MODEL_VERSION', '$RULE_VERSION', '$FEATURE_SET_ID', '{\"percentage\":10,\"source\":\"live_deployment_state_machine\"}'::jsonb, 'planned', '$OPERATOR_USER_ID', now(), now(), '{\"case\":\"main\"}'::jsonb),
    ('$PREVIOUS_ACTIVE_ID', '$TENANT', 'codex deployment previous active $RUN_SLUG', 'previous active should be superseded',
      '$MODEL_VERSION', '$RULE_VERSION', '$FEATURE_SET_ID', '{\"percentage\":100,\"source\":\"live_deployment_state_machine\"}'::jsonb, 'active', '$OPERATOR_USER_ID', now() - interval '5 minutes', now() - interval '5 minutes', '{\"case\":\"previous-active\"}'::jsonb),
    ('$INVALID_DEPLOYMENT_ID', '$TENANT', 'codex deployment invalid $RUN_SLUG', 'planned rollback should fail',
      '$MODEL_VERSION', '$RULE_VERSION', '$FEATURE_SET_ID', '{\"percentage\":5,\"source\":\"live_deployment_state_machine\"}'::jsonb, 'planned', '$OPERATOR_USER_ID', now(), now(), '{\"case\":\"invalid\"}'::jsonb),
    ('$CROSS_DEPLOYMENT_ID', '$OTHER_TENANT', 'codex deployment cross $RUN_SLUG', 'cross tenant rollback should fail',
      '$CROSS_MODEL_VERSION', '$CROSS_RULE_VERSION', '$CROSS_FEATURE_SET_ID', '{\"percentage\":100,\"source\":\"live_deployment_state_machine\"}'::jsonb, 'active', NULL, now(), now(), '{\"case\":\"cross\"}'::jsonb),
    ('$VIEWER_DEPLOYMENT_ID', '$TENANT', 'codex deployment viewer $RUN_SLUG', 'viewer gray should fail',
      '$MODEL_VERSION', '$RULE_VERSION', '$FEATURE_SET_ID', '{\"percentage\":5,\"source\":\"live_deployment_state_machine\"}'::jsonb, 'planned', '$OPERATOR_USER_ID', now(), now(), '{\"case\":\"viewer\"}'::jsonb)
  ON CONFLICT (deployment_id) DO UPDATE SET
    status = EXCLUDED.status,
    updated_at = now(),
    gray_started_at = NULL,
    gray_expired_at = NULL,
    activated_at = NULL,
    rolled_back_at = NULL,
    rollback_from = NULL,
    metadata = EXCLUDED.metadata;
" >/dev/null
json_log "fixture" "seed deployment state-machine fixtures" true "ok" "$DEPLOYMENT_ID,$PREVIOUS_ACTIVE_ID,$INVALID_DEPLOYMENT_ID,$CROSS_DEPLOYMENT_ID,$VIEWER_DEPLOYMENT_ID"

curl_check "start gray deployment" "POST" "/api/v1/deployments/$DEPLOYMENT_ID/gray" "200" "" '.success == true'
assert_psql "gray status persisted" \
  "SELECT status || '|' || (gray_started_at IS NOT NULL)::text || '|' || (gray_expired_at IS NOT NULL)::text FROM deployments WHERE deployment_id = '$DEPLOYMENT_ID';" \
  "gray|true|true"
assert_psql "gray history persisted" \
  "SELECT count(*)::text FROM deployment_history WHERE deployment_id = '$DEPLOYMENT_ID' AND action = 'gray_started';" \
  "1"

curl_check "activate deployment" "POST" "/api/v1/deployments/$DEPLOYMENT_ID/activate" "200" "" '.success == true'
assert_psql "active status persisted" \
  "SELECT status || '|' || (activated_at IS NOT NULL)::text FROM deployments WHERE deployment_id = '$DEPLOYMENT_ID';" \
  "active|true"
assert_psql "previous active superseded" \
  "SELECT status FROM deployments WHERE deployment_id = '$PREVIOUS_ACTIVE_ID';" \
  "superseded"
assert_psql "activate history persisted" \
  "SELECT count(*)::text FROM deployment_history WHERE deployment_id = '$DEPLOYMENT_ID' AND action = 'activated';" \
  "1"

curl_check "pause active deployment" "POST" "/api/v1/deployments/$DEPLOYMENT_ID/pause" "200" "" '.success == true'
assert_psql "pause status persisted" \
  "SELECT status FROM deployments WHERE deployment_id = '$DEPLOYMENT_ID';" \
  "paused"
assert_psql "pause history persisted" \
  "SELECT count(*)::text FROM deployment_history WHERE deployment_id = '$DEPLOYMENT_ID' AND action = 'paused';" \
  "1"

curl_check "resume paused deployment" "POST" "/api/v1/deployments/$DEPLOYMENT_ID/resume" "200" "" '.success == true'
assert_psql "resume status persisted" \
  "SELECT status FROM deployments WHERE deployment_id = '$DEPLOYMENT_ID';" \
  "active"
assert_psql "resume history persisted" \
  "SELECT count(*)::text FROM deployment_history WHERE deployment_id = '$DEPLOYMENT_ID' AND action = 'resumed';" \
  "1"

curl_check "rollback active deployment" "POST" "/api/v1/deployments/$DEPLOYMENT_ID/rollback" "200" "" '.success == true'
assert_psql "rollback status persisted" \
  "SELECT status || '|' || (rolled_back_at IS NOT NULL)::text FROM deployments WHERE deployment_id = '$DEPLOYMENT_ID';" \
  "rolled_back|true"
assert_psql "rollback history persisted" \
  "SELECT count(*)::text FROM deployment_history WHERE deployment_id = '$DEPLOYMENT_ID' AND action = 'rolled_back';" \
  "1"

curl_check "planned rollback rejected" "POST" "/api/v1/deployments/$INVALID_DEPLOYMENT_ID/rollback" "409" "" \
  '.code == "BIZ_3004" or .error.code == "BIZ_3004" or (.message // "" | contains("cannot transition"))'
assert_psql "planned rollback unchanged" \
  "SELECT status FROM deployments WHERE deployment_id = '$INVALID_DEPLOYMENT_ID';" \
  "planned"

curl_check "cross tenant rollback rejected" "POST" "/api/v1/deployments/$CROSS_DEPLOYMENT_ID/rollback" "403" "" \
  '.code == "AUTH_1004" or .error.code == "AUTH_1004" or (.message // "" | contains("cross-tenant"))'
assert_psql "cross tenant deployment unchanged" \
  "SELECT status FROM deployments WHERE deployment_id = '$CROSS_DEPLOYMENT_ID';" \
  "active"

curl_check "viewer gray rejected" "POST" "/api/v1/deployments/$VIEWER_DEPLOYMENT_ID/gray" "403" "" \
  '.code == "AUTH_1004" or .error.code == "AUTH_1004" or (.message // "" | contains("permission denied"))' \
  "$VIEWER_TOKEN" "$TENANT" "$VIEWER_USER_ID" "codex-deploy-viewer" "viewer" "deploy:read"
assert_psql "viewer deployment unchanged" \
  "SELECT status FROM deployments WHERE deployment_id = '$VIEWER_DEPLOYMENT_ID';" \
  "planned"

audit_file="$LOG_DIR/$RUN_SLUG-audit.json"
curl_check "audit trail query" "GET" "/api/v1/audit/logs?object_type=deployment&limit=200" "200" "" \
  '.success == true' \
  "$AUDIT_TOKEN" "$TENANT" "$AUDITOR_USER_ID" "codex-deploy-auditor" "admin" "audit:read,deploy:read,admin:*"
cp "$LOG_DIR/$RUN_SLUG-audit-trail-query.json" "$audit_file" 2>/dev/null || true

if jq -e --arg id "$DEPLOYMENT_ID" '
  .success == true
  and any(.data.trails[]; .resource_id == $id and .action == "DEPLOY_GRAY" and .details.new_status == "gray")
  and any(.data.trails[]; .resource_id == $id and .action == "DEPLOY_ACTIVATE" and .details.new_status == "active")
  and any(.data.trails[]; .resource_id == $id and .action == "DEPLOY_PAUSE" and .details.new_status == "paused")
  and any(.data.trails[]; .resource_id == $id and .action == "DEPLOY_RESUME" and .details.new_status == "active")
  and any(.data.trails[]; .resource_id == $id and .action == "DEPLOY_ROLLBACK" and .details.new_status == "rolled_back")
' "$audit_file" >/dev/null; then
  json_log "audit" "deployment action audit queryable" true "ok" "$DEPLOYMENT_ID" "$audit_file"
else
  json_log "audit" "deployment action audit queryable" false "failed" "body=$(trim_file "$audit_file")" "$audit_file"
  exit 1
fi

jq -s \
  --arg run_id "$RUN_ID" \
  --arg tenant "$TENANT" \
  --arg other_tenant "$OTHER_TENANT" \
  --arg deployment_id "$DEPLOYMENT_ID" \
  --arg invalid_deployment_id "$INVALID_DEPLOYMENT_ID" \
  --arg cross_deployment_id "$CROSS_DEPLOYMENT_ID" \
  --arg viewer_deployment_id "$VIEWER_DEPLOYMENT_ID" \
  --arg report "$REPORT" \
  '{
    run_id: $run_id,
    result: (if all(.[]; .ok == true) then "pass" else "fail" end),
    tenant: $tenant,
    other_tenant: $other_tenant,
    deployment_id: $deployment_id,
    invalid_deployment_id: $invalid_deployment_id,
    cross_deployment_id: $cross_deployment_id,
    viewer_deployment_id: $viewer_deployment_id,
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
# Deployment state-machine live preflight

- Run ID: \`$RUN_ID\`
- Result: \`$(jq -r '.result' "$SUMMARY")\`
- APISIX: \`$APISIX\`
- Deployment: \`$DEPLOYMENT_ID\`
- Checks: $(jq -r '.passed' "$SUMMARY")/$(jq -r '.total' "$SUMMARY") passed, blockers=$(jq -r '.blockers' "$SUMMARY"), warnings=$(jq -r '.warnings' "$SUMMARY")

## Evidence

- NDJSON: \`$REPORT\`
- Summary: \`$SUMMARY\`
- API/DB/Audit responses: \`$LOG_DIR/$RUN_SLUG-*.json\`, \`$LOG_DIR/$RUN_SLUG-*.txt\`

## Scope

This report validates the deployment action state machine: planned deployments can enter gray, gray deployments can activate, active deployments can pause/resume/rollback, activation supersedes previous active deployments, invalid planned rollback returns 409, cross-tenant and read-only requests return 403, deployment history is persisted, and \`DEPLOY_GRAY\` / \`DEPLOY_ACTIVATE\` / \`DEPLOY_PAUSE\` / \`DEPLOY_RESUME\` / \`DEPLOY_ROLLBACK\` are queryable through \`audit_logs\`.
EOF

cat "$SUMMARY"
