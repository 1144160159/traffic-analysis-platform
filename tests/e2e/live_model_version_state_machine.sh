#!/usr/bin/env bash
set -euo pipefail

APISIX="${APISIX:-http://10.0.5.8:30180}"
TENANT="${TENANT:-codex-model-state}"
OTHER_TENANT="${OTHER_TENANT:-codex-model-cross}"
LOG_DIR="${LOG_DIR:-doc/02_acceptance/runs/20260629-model-version-state-machine}"
RUN_ID="${RUN_ID:-$(date +%Y%m%d%H%M%S)-model-version-state-machine}"
KUBECTL="${KUBECTL:-kubectl}"
JWT_SECRET_NAMESPACE="${JWT_SECRET_NAMESPACE:-traffic-analysis}"
JWT_SECRET_NAME="${JWT_SECRET_NAME:-traffic-credentials}"
JWT_SECRET_KEY="${JWT_SECRET_KEY:-JWT_SECRET}"
PG_NAMESPACE="${PG_NAMESPACE:-databases}"
PG_POD="${PG_POD:-postgres-primary-0}"
PG_SECRET_KEY="${PG_SECRET_KEY:-PG_PASSWORD}"
REGRESSION_DIR="${REGRESSION_DIR:-doc/02_acceptance/02-regression}"

REPORT="$LOG_DIR/live-model-version-state-machine-$RUN_ID.ndjson"
SUMMARY="$LOG_DIR/live-model-version-state-machine-$RUN_ID-summary.json"
LATEST_JSON="$REGRESSION_DIR/model-version-state-machine-latest.json"
LATEST_MD="$REGRESSION_DIR/model-version-state-machine-latest.md"

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
    "session_id": "codex-model-state-" + os.environ["RUN_ID"],
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
  local token="${7:-$OPERATOR_TOKEN}" tenant="${8:-$TENANT}" user_id="${9:-$OPERATOR_USER_ID}" username="${10:-codex-model-operator}" roles="${11:-operator}" perms="${12:-model:read,model:create,model:activate}"
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
    -H "User-Agent: codex-model-version-state-machine/$RUN_ID"
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
  elif [[ -n "$filter" ]] && ! jq -e "$filter" "$body_file" >/dev/null 2>"$err_file"; then
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
MODEL_ID="$(make_uuid)"
CROSS_MODEL_ID="$(make_uuid)"
FEATURE_SET_ID="fs-model-$RUN_SLUG"
CROSS_FEATURE_SET_ID="fs-model-cross-$RUN_SLUG"
PREVIOUS_ACTIVE_VERSION="mv-$RUN_SLUG-previous-active"
REGISTERED_VERSION="mv-$RUN_SLUG-registered"
DEPRECATE_INVALID_VERSION="mv-$RUN_SLUG-invalid-registered"
CROSS_ACTIVE_VERSION="mv-$RUN_SLUG-cross-active"
VIEWER_VERSION="mv-$RUN_SLUG-viewer-registered"

JWT_SECRET="$(kctl -n "$JWT_SECRET_NAMESPACE" get secret "$JWT_SECRET_NAME" -o "jsonpath={.data.$JWT_SECRET_KEY}" | base64 -d)"
PG_PASSWORD="$(kctl -n "$JWT_SECRET_NAMESPACE" get secret "$JWT_SECRET_NAME" -o "jsonpath={.data.$PG_SECRET_KEY}" | base64 -d)"

OPERATOR_TOKEN="$(make_token codex-model-operator "$TENANT" "$OPERATOR_USER_ID" '["operator"]' '["model:read","model:create","model:activate"]')"
VIEWER_TOKEN="$(make_token codex-model-viewer "$TENANT" "$VIEWER_USER_ID" '["viewer"]' '["model:read"]')"
AUDIT_TOKEN="$(make_token codex-model-auditor "$TENANT" "$AUDITOR_USER_ID" '["admin"]' '["audit:read","model:read","admin:*"]')"

psql_exec "
  INSERT INTO tenants (tenant_id, tenant_name, name, description, status) VALUES
    ('$TENANT', '$TENANT', '$TENANT', 'codex model state-machine tenant', 'active'),
    ('$OTHER_TENANT', '$OTHER_TENANT', '$OTHER_TENANT', 'codex model cross tenant', 'active')
  ON CONFLICT (tenant_id) DO UPDATE SET
    tenant_name = EXCLUDED.tenant_name,
    name = EXCLUDED.name,
    status = EXCLUDED.status,
    updated_at = now();

  INSERT INTO users (user_id, tenant_id, username, email, status) VALUES
    ('$OPERATOR_USER_ID', '$TENANT', 'codex-model-operator-$RUN_SLUG', 'codex-model-operator-$RUN_SLUG@local', 'active'),
    ('$VIEWER_USER_ID', '$TENANT', 'codex-model-viewer-$RUN_SLUG', 'codex-model-viewer-$RUN_SLUG@local', 'active'),
    ('$AUDITOR_USER_ID', '$TENANT', 'codex-model-auditor-$RUN_SLUG', 'codex-model-auditor-$RUN_SLUG@local', 'active')
  ON CONFLICT (user_id) DO UPDATE SET updated_at = now();

  INSERT INTO feature_sets (feature_set_id, tenant_id, name, params, schema_version, status)
  VALUES
    ('$FEATURE_SET_ID', '$TENANT', 'codex model state features $RUN_SLUG', '{\"source\":\"live_model_version_state_machine\"}'::jsonb, 'v1', 'active'),
    ('$CROSS_FEATURE_SET_ID', '$OTHER_TENANT', 'codex model cross features $RUN_SLUG', '{\"source\":\"live_model_version_state_machine\"}'::jsonb, 'v1', 'active')
  ON CONFLICT (feature_set_id) DO UPDATE SET updated_at = now();

  INSERT INTO models (model_id, tenant_id, name, model_type, description, metadata)
  VALUES
    ('$MODEL_ID', '$TENANT', 'codex model state $RUN_SLUG', 'xgboost', 'model version state-machine fixture', '{\"source\":\"live_model_version_state_machine\"}'::jsonb),
    ('$CROSS_MODEL_ID', '$OTHER_TENANT', 'codex model cross $RUN_SLUG', 'xgboost', 'model version state-machine fixture', '{\"source\":\"live_model_version_state_machine\"}'::jsonb)
  ON CONFLICT (tenant_id, name) DO UPDATE SET updated_at = now();

  INSERT INTO model_versions (model_version, model_id, tenant_id, feature_set_id, artifact_uri, metrics, status, created_by, updated_at)
  VALUES
    ('$PREVIOUS_ACTIVE_VERSION', '$MODEL_ID', '$TENANT', '$FEATURE_SET_ID', 's3://codex/model-state/$RUN_SLUG/previous/model.bin', '{\"f1_score\":0.82}'::jsonb, 'active', '$OPERATOR_USER_ID', now()),
    ('$DEPRECATE_INVALID_VERSION', '$MODEL_ID', '$TENANT', '$FEATURE_SET_ID', 's3://codex/model-state/$RUN_SLUG/invalid/model.bin', '{\"f1_score\":0.75}'::jsonb, 'registered', '$OPERATOR_USER_ID', now()),
    ('$VIEWER_VERSION', '$MODEL_ID', '$TENANT', '$FEATURE_SET_ID', 's3://codex/model-state/$RUN_SLUG/viewer/model.bin', '{\"f1_score\":0.76}'::jsonb, 'registered', '$OPERATOR_USER_ID', now()),
    ('$CROSS_ACTIVE_VERSION', '$CROSS_MODEL_ID', '$OTHER_TENANT', '$CROSS_FEATURE_SET_ID', 's3://codex/model-state/$RUN_SLUG/cross/model.bin', '{\"f1_score\":0.84}'::jsonb, 'active', NULL, now())
  ON CONFLICT (model_version) DO UPDATE SET
    status = EXCLUDED.status,
    artifact_uri = EXCLUDED.artifact_uri,
    metrics = EXCLUDED.metrics,
    updated_at = now();
" >/dev/null
json_log "fixture" "seed model version state-machine fixtures" true "ok" "$MODEL_ID,$PREVIOUS_ACTIVE_VERSION,$DEPRECATE_INVALID_VERSION,$VIEWER_VERSION,$CROSS_ACTIVE_VERSION"

REGISTER_BODY="$(jq -nc \
  --arg version "$REGISTERED_VERSION" \
  --arg artifact "s3://codex/model-state/$RUN_SLUG/registered/model.bin" \
  --arg feature "$FEATURE_SET_ID" \
  '{version:$version, artifact_uri:$artifact, feature_set_id:$feature, model_type:"xgboost", metrics:{f1_score:0.93, precision:0.91, recall:0.95}}')"

curl_check "register model version" "POST" "/api/v1/models/$MODEL_ID/versions" "201" "$REGISTER_BODY" '.success == true and .data.status == "registered"'
assert_psql "registered version persisted" \
  "SELECT status || '|' || (created_by::text = '$OPERATOR_USER_ID')::text FROM model_versions WHERE model_version = '$REGISTERED_VERSION';" \
  "registered|true"

curl_check "activate registered model version" "POST" "/api/v1/models/$MODEL_ID/versions/$REGISTERED_VERSION/activate" "200" "" '.success == true'
assert_psql "registered version activated" \
  "SELECT status FROM model_versions WHERE model_version = '$REGISTERED_VERSION';" \
  "active"
assert_psql "previous active version deprecated" \
  "SELECT status FROM model_versions WHERE model_version = '$PREVIOUS_ACTIVE_VERSION';" \
  "deprecated"
assert_psql "only one active version remains after activation" \
  "SELECT COUNT(*)::text FROM model_versions WHERE model_id = '$MODEL_ID' AND status = 'active';" \
  "1"

curl_check "deprecate active model version" "POST" "/api/v1/models/$MODEL_ID/versions/$REGISTERED_VERSION/deprecate" "200" "" '.success == true'
assert_psql "active version deprecated" \
  "SELECT status FROM model_versions WHERE model_version = '$REGISTERED_VERSION';" \
  "deprecated"

curl_check "registered deprecate rejected" "POST" "/api/v1/models/$MODEL_ID/versions/$DEPRECATE_INVALID_VERSION/deprecate" "409" "" ""
assert_psql "registered deprecate unchanged" \
  "SELECT status FROM model_versions WHERE model_version = '$DEPRECATE_INVALID_VERSION';" \
  "registered"

curl_check "cross tenant activate rejected" "POST" "/api/v1/models/$CROSS_MODEL_ID/versions/$CROSS_ACTIVE_VERSION/activate" "403" "" "" \
  "$OPERATOR_TOKEN" "$TENANT" "$OPERATOR_USER_ID" "codex-model-operator" "operator" "model:read,model:create,model:activate"
assert_psql "cross tenant model version unchanged" \
  "SELECT status FROM model_versions WHERE model_version = '$CROSS_ACTIVE_VERSION';" \
  "active"

curl_check "viewer activate rejected" "POST" "/api/v1/models/$MODEL_ID/versions/$VIEWER_VERSION/activate" "403" "" "" \
  "$VIEWER_TOKEN" "$TENANT" "$VIEWER_USER_ID" "codex-model-viewer" "viewer" "model:read"
assert_psql "viewer rejected model version unchanged" \
  "SELECT status FROM model_versions WHERE model_version = '$VIEWER_VERSION';" \
  "registered"

curl_check "audit trail query" "GET" "/api/v1/audit/logs?object_type=model_version&limit=200" "200" "" '.success == true' \
  "$AUDIT_TOKEN" "$TENANT" "$AUDITOR_USER_ID" "codex-model-auditor" "admin" "audit:read,model:read,admin:*"
assert_psql "model version create audit persisted" \
  "SELECT EXISTS (SELECT 1 FROM audit_logs WHERE tenant_id = '$TENANT' AND object_type = 'model_version' AND object_id = '$REGISTERED_VERSION' AND action = 'MODEL_VERSION_CREATE')::text;" \
  "true"
assert_psql "model version activate audit persisted" \
  "SELECT EXISTS (SELECT 1 FROM audit_logs WHERE tenant_id = '$TENANT' AND object_type = 'model_version' AND object_id = '$REGISTERED_VERSION' AND action = 'MODEL_VERSION_ACTIVATE')::text;" \
  "true"
assert_psql "model version deprecate audit persisted" \
  "SELECT EXISTS (SELECT 1 FROM audit_logs WHERE tenant_id = '$TENANT' AND object_type = 'model_version' AND object_id = '$REGISTERED_VERSION' AND action = 'MODEL_VERSION_DEPRECATE')::text;" \
  "true"
assert_psql "model version failure audit persisted" \
  "SELECT EXISTS (SELECT 1 FROM audit_logs WHERE tenant_id = '$TENANT' AND object_type = 'model_version' AND object_id = '$DEPRECATE_INVALID_VERSION' AND action = 'MODEL_VERSION_DEPRECATE_failed')::text;" \
  "true"

PASSED="$(jq -s '[.[] | select(.ok == true)] | length' "$REPORT")"
FAILED="$(jq -s '[.[] | select(.ok != true)] | length' "$REPORT")"
TOTAL="$(jq -s 'length' "$REPORT")"
RESULT="pass"
if [[ "$FAILED" != "0" ]]; then
  RESULT="fail"
fi

jq -n \
  --arg run_id "$RUN_ID" \
  --arg result "$RESULT" \
  --arg tenant "$TENANT" \
  --arg other_tenant "$OTHER_TENANT" \
  --arg model_id "$MODEL_ID" \
  --arg registered_version "$REGISTERED_VERSION" \
  --arg previous_active_version "$PREVIOUS_ACTIVE_VERSION" \
  --arg invalid_version "$DEPRECATE_INVALID_VERSION" \
  --arg cross_version "$CROSS_ACTIVE_VERSION" \
  --arg viewer_version "$VIEWER_VERSION" \
  --arg report "$REPORT" \
  --argjson total "$TOTAL" \
  --argjson passed "$PASSED" \
  --argjson failed "$FAILED" \
  --slurpfile checks "$REPORT" \
  '{
    run_id: $run_id,
    result: $result,
    tenant: $tenant,
    other_tenant: $other_tenant,
    model_id: $model_id,
    registered_version: $registered_version,
    previous_active_version: $previous_active_version,
    invalid_version: $invalid_version,
    cross_version: $cross_version,
    viewer_version: $viewer_version,
    total: $total,
    passed: $passed,
    failed: $failed,
    blockers: $failed,
    warnings: 0,
    report: $report,
    checks: $checks
  }' >"$SUMMARY"

cp "$SUMMARY" "$LATEST_JSON"
cat >"$LATEST_MD" <<EOF
# Model Version State Machine Live Regression

- Run ID: \`$RUN_ID\`
- Result: \`$RESULT\`
- Passed: \`$PASSED/$TOTAL\`
- Blockers: \`$FAILED\`
- Tenant: \`$TENANT\`
- Model: \`$MODEL_ID\`
- NDJSON: \`$REPORT\`
- Summary: \`$SUMMARY\`
- API/DB/Audit responses: \`$LOG_DIR/$RUN_SLUG-*.json\`, \`$LOG_DIR/$RUN_SLUG-*.txt\`

This report validates the model registry state machine: model versions register into \`registered\`, registered versions can activate, activation deprecates any previous active version for the same model, active versions can deprecate, registered deprecate returns 409, cross-tenant and read-only requests return 403, and \`MODEL_VERSION_CREATE\` / \`MODEL_VERSION_ACTIVATE\` / \`MODEL_VERSION_DEPRECATE\` plus failure audit rows are persisted in \`audit_logs\`.
EOF

jq . "$SUMMARY"

if [[ "$RESULT" != "pass" ]]; then
  exit 1
fi
