#!/usr/bin/env bash
set -euo pipefail

APISIX="${APISIX:-http://10.0.5.8:30180}"
TENANT="${TENANT:-default}"
OTHER_TENANT="${OTHER_TENANT:-tenant-b}"
LOG_DIR="${LOG_DIR:-doc/02_acceptance/runs/$(date +%Y%m%d%H%M%S)-notification-governance-preflight}"
RUN_ID="${RUN_ID:-$(date +%Y%m%d%H%M%S)-notification-governance-preflight}"
REGRESSION_DIR="${REGRESSION_DIR:-doc/02_acceptance/02-regression}"
KUBECTL="${KUBECTL:-kubectl}"
ALLOW_BLOCKERS="${ALLOW_BLOCKERS:-false}"
JWT_SECRET_NAMESPACE="${JWT_SECRET_NAMESPACE:-traffic-analysis}"
JWT_SECRET_NAME="${JWT_SECRET_NAME:-traffic-credentials}"
JWT_SECRET_KEY="${JWT_SECRET_KEY:-JWT_SECRET}"
PG_NAMESPACE="${PG_NAMESPACE:-databases}"
PG_POD="${PG_POD:-postgres-primary-0}"
PG_SECRET_KEY="${PG_SECRET_KEY:-PG_PASSWORD}"

REPORT="$LOG_DIR/live-notification-governance-preflight-$RUN_ID.ndjson"
SUMMARY="$LOG_DIR/live-notification-governance-preflight-$RUN_ID-summary.json"
LOCAL_REPORT="$LOG_DIR/local-report.md"
RUN_SLUG="$(printf '%s' "$RUN_ID" | tr -c 'a-zA-Z0-9-' '-' | cut -c1-64 | sed 's/-*$//')"
RULE_NAME="${RULE_NAME:-codex notification silence ${RUN_SLUG:-run}}"

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
    head -c 1200 "$file" \
      | tr '\n' ' ' \
      | sed -E 's/Bearer [A-Za-z0-9._-]+/Bearer <redacted>/g'
  fi
}

make_token() {
  local username="$1" tenant="$2" roles_json="$3" perms_json="$4" ttl="${5:-1800}"
  JWT_SECRET="$JWT_SECRET" TENANT="$tenant" USERNAME="$username" ROLES_JSON="$roles_json" PERMS_JSON="$perms_json" RUN_ID="$RUN_ID" TTL="$ttl" python3 - <<'PY'
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
    "email": os.environ["USERNAME"] + "@local",
    "roles": json.loads(os.environ["ROLES_JSON"]),
    "permissions": json.loads(os.environ["PERMS_JSON"]),
    "token_type": "access",
    "session_id": "codex-notification-governance-" + os.environ["RUN_ID"],
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

psql_exec() {
  local sql="$1"
  kctl -n "$PG_NAMESPACE" exec "$PG_POD" -- env PGPASSWORD="$PG_PASSWORD" \
    psql -U postgres -d traffic_platform -v ON_ERROR_STOP=1 -Atc "$sql"
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

for cmd in git jq curl python3 "$KUBECTL"; do
  need_cmd "$cmd"
done

git rev-parse HEAD >"$LOG_DIR/commit-sha.txt"
git status --short >"$LOG_DIR/git-status.txt"

JWT_SECRET="$(kctl -n "$JWT_SECRET_NAMESPACE" get secret "$JWT_SECRET_NAME" -o "jsonpath={.data.$JWT_SECRET_KEY}" | base64 -d)"
PG_PASSWORD="$(kctl -n "$JWT_SECRET_NAMESPACE" get secret "$JWT_SECRET_NAME" -o "jsonpath={.data.$PG_SECRET_KEY}" | base64 -d)"

ADMIN_TOKEN="$(make_token codex-notify-admin "$TENANT" '["admin"]' '["*","admin:*","audit:read","user:read"]')"
VIEWER_TOKEN="$(make_token codex-notify-viewer "$TENANT" '["viewer"]' '["user:read","audit:read"]')"
OTHER_TOKEN="$(make_token codex-notify-other-admin "$OTHER_TENANT" '["admin"]' '["*","admin:*","audit:read","user:read"]')"

if grep -q "notification-silence-rule-create" web/ui/src/services/pageApiPlans.ts && grep -q "NOTIFICATION_TEST_SENT" web/ui/src/services/pageApiPlans.ts; then
  json_log "contract" "frontend notification action contract present" "info" true "ok" "notification settings/test/silence actions declared" "web/ui/src/services/pageApiPlans.ts"
else
  json_log "contract" "frontend notification action contract present" "blocker" false "missing" "notification action contracts missing" "web/ui/src/services/pageApiPlans.ts"
fi

curl_json "notification settings readable" "GET" "/api/v1/notifications/settings" "200" "$ADMIN_TOKEN" "$LOG_DIR/notification-settings-before.json"
assert_json "notification settings response shape" "$LOG_DIR/notification-settings-before.json" '.success == true and (.data.channels | type == "object")'

curl_json "notification silence rules readable" "GET" "/api/v1/notifications/silence-rules?limit=5" "200" "$ADMIN_TOKEN" "$LOG_DIR/notification-silence-rules-before.json"
assert_json "notification silence rules response shape" "$LOG_DIR/notification-silence-rules-before.json" '.success == true and (.data.rules | type == "array") and (.data.total | type == "number")'

settings_body="$(jq -nc '{enabled:true,min_severity:"high",rate_limit_per_min:7,channels:{email:true,webhook:false,wechat:true},secret_ref:"traffic-analysis/notification-secret"}')"
curl_json "admin can update notification settings" "PUT" "/api/v1/notifications/settings" "200" "$ADMIN_TOKEN" "$LOG_DIR/notification-settings-update.json" "$settings_body"
assert_json "notification settings update merged channels and secret ref" "$LOG_DIR/notification-settings-update.json" '.success == true and .data.channels.email == true and .data.channels.webhook == false and .data.secret_ref == "traffic-analysis/notification-secret"'

inline_secret_body="$(jq -nc '{webhook_token:"plain-text-token"}')"
curl_json "inline notification secret is rejected" "PUT" "/api/v1/notifications/settings" "400" "$ADMIN_TOKEN" "$LOG_DIR/notification-settings-inline-secret.json" "$inline_secret_body" || true
curl_json "viewer cannot update notification settings" "PUT" "/api/v1/notifications/settings" "403" "$VIEWER_TOKEN" "$LOG_DIR/notification-settings-viewer.json" '{"enabled":false}' || true

curl_json "admin can send notification test" "POST" "/api/v1/notifications/test" "200" "$ADMIN_TOKEN" "$LOG_DIR/notification-test-admin.json"
assert_json "notification test response shape" "$LOG_DIR/notification-test-admin.json" '.success == true and .message == "test notification sent"'
curl_json "viewer cannot send notification test" "POST" "/api/v1/notifications/test" "403" "$VIEWER_TOKEN" "$LOG_DIR/notification-test-viewer.json" || true

STARTS_AT="$(date -u -d '+1 hour' '+%Y-%m-%dT%H:%M:%SZ')"
ENDS_AT="$(date -u -d '+5 hours' '+%Y-%m-%dT%H:%M:%SZ')"
silence_body="$(jq -nc \
  --arg name "$RULE_NAME" \
  --arg starts_at "$STARTS_AT" \
  --arg ends_at "$ENDS_AT" \
  '{name:$name,scope:"main-campus",starts_at:$starts_at,ends_at:$ends_at,affected_targets:["core-switch","probe-agent"],policy:"night-escalation",reason:"codex live notification governance preflight",enabled:true}')"
curl_json "admin can create notification silence rule" "POST" "/api/v1/notifications/silence-rules" "201" "$ADMIN_TOKEN" "$LOG_DIR/notification-silence-create.json" "$silence_body"
assert_json "created silence rule has identity and enabled state" "$LOG_DIR/notification-silence-create.json" \
  --arg name "$RULE_NAME" '.success == true and (.data.rule_id | type == "string" and length > 0) and .data.name == $name and .data.enabled == true'

RULE_ID="$(jq -r '.data.rule_id // ""' "$LOG_DIR/notification-silence-create.json")"
if [[ -n "$RULE_ID" ]]; then
  curl_json "created silence rule is queryable" "GET" "/api/v1/notifications/silence-rules?limit=20" "200" "$ADMIN_TOKEN" "$LOG_DIR/notification-silence-rules-after.json"
  assert_json "silence rule appears in list" "$LOG_DIR/notification-silence-rules-after.json" \
    --arg rule_id "$RULE_ID" '.success == true and ([.data.rules[] | select(.rule_id == $rule_id)] | length) == 1'

  curl_json "admin can disable silence rule" "PATCH" "/api/v1/notifications/silence-rules/$RULE_ID" "200" "$ADMIN_TOKEN" "$LOG_DIR/notification-silence-disable.json" '{"enabled":false}'
  assert_json "silence rule disabled response" "$LOG_DIR/notification-silence-disable.json" '.success == true and .data.enabled == false'

  curl_json "other tenant cannot update silence rule" "PATCH" "/api/v1/notifications/silence-rules/$RULE_ID" "404" "$OTHER_TOKEN" "$LOG_DIR/notification-silence-other-tenant.json" '{"enabled":true}' "$OTHER_TENANT" || true
  curl_json "viewer cannot update silence rule" "PATCH" "/api/v1/notifications/silence-rules/$RULE_ID" "403" "$VIEWER_TOKEN" "$LOG_DIR/notification-silence-viewer.json" '{"enabled":true}' || true

  curl_json "audit log page API has silence create event" "GET" "/api/v1/audit/logs?action=NOTIFICATION_SILENCE_RULE_CREATED&object_id=$RULE_ID&limit=10" "200" "$ADMIN_TOKEN" "$LOG_DIR/notification-audit-silence-created.json"
  assert_json "audit log contains silence create event" "$LOG_DIR/notification-audit-silence-created.json" \
    --arg rule_id "$RULE_ID" '.success == true and (.data.total >= 1) and ([.data.trails[] | select(.resource_id == $rule_id and .action == "NOTIFICATION_SILENCE_RULE_CREATED")] | length) >= 1'

  assert_psql_count "notification silence rule persisted in PostgreSQL" \
    "SELECT count(*) FROM notification_silence_rules WHERE tenant_id = '$TENANT' AND rule_id = '$RULE_ID' AND enabled = false;" \
    "pg-notification-silence-rule-count.txt"
  assert_psql_count "notification settings update audit persisted" \
    "SELECT count(*) FROM audit_logs WHERE tenant_id = '$TENANT' AND action = 'NOTIFICATION_SETTINGS_UPDATED' AND object_type = 'notification_settings';" \
    "pg-notification-settings-audit-count.txt"
  assert_psql_count "notification test audit persisted" \
    "SELECT count(*) FROM audit_logs WHERE tenant_id = '$TENANT' AND action = 'NOTIFICATION_TEST_SENT' AND object_type = 'notification_test';" \
    "pg-notification-test-audit-count.txt"
  assert_psql_count "notification silence audit persisted" \
    "SELECT count(*) FROM audit_logs WHERE tenant_id = '$TENANT' AND action IN ('NOTIFICATION_SILENCE_RULE_CREATED','NOTIFICATION_SILENCE_RULE_UPDATED') AND object_type = 'notification_silence_rule' AND object_id = '$RULE_ID';" \
    "pg-notification-silence-audit-count.txt"
else
  json_log "assert" "created notification silence rule id extracted" "blocker" false "missing" "create response did not return rule_id" "notification-silence-create.json"
fi

TOTAL="$(jq -s 'length' "$REPORT")"
PASSED="$(jq -s '[.[] | select(.passed == true)] | length' "$REPORT")"
BLOCKERS="$(jq -s '[.[] | select(.passed == false and .severity == "blocker")] | length' "$REPORT")"
WARNINGS="$(jq -s '[.[] | select(.severity == "warn")] | length' "$REPORT")"
RESULT="pass"
if [[ "$BLOCKERS" -gt 0 ]]; then
  RESULT="blocked"
elif [[ "$WARNINGS" -gt 0 ]]; then
  RESULT="warn"
fi

jq -n \
  --arg run_id "$RUN_ID" \
  --arg result "$RESULT" \
  --arg apisix "$APISIX" \
  --arg tenant "$TENANT" \
  --arg other_tenant "$OTHER_TENANT" \
  --arg rule_id "${RULE_ID:-}" \
  --arg rule_name "$RULE_NAME" \
  --arg report "$REPORT" \
  --arg local_report "$LOCAL_REPORT" \
  --argjson total "$TOTAL" \
  --argjson passed "$PASSED" \
  --argjson blockers "$BLOCKERS" \
  --argjson warnings "$WARNINGS" \
  --slurpfile checks "$REPORT" \
  '{
    run_id:$run_id,
    result:$result,
    apisix:$apisix,
    tenant:$tenant,
    other_tenant:$other_tenant,
    rule_id:$rule_id,
    rule_name:$rule_name,
    report:$report,
    local_report:$local_report,
    total:$total,
    passed:$passed,
    blockers:$blockers,
    warnings:$warnings,
    checks:$checks
  }' >"$SUMMARY"

cat >"$LOCAL_REPORT" <<MD
# Notification Governance Live Preflight

- Run: \`$RUN_ID\`
- Result: \`$RESULT\`
- Checks: \`$PASSED/$TOTAL\` passed, \`$BLOCKERS\` blockers, \`$WARNINGS\` warnings
- Silence rule: \`${RULE_ID:-missing}\`

This gate closes the notification part of the audit-config business loop:
settings update, inline-secret rejection, notification test send, silence rule
create/disable, viewer write denial, cross-tenant isolation, PostgreSQL
persistence, and audit-log queryability.
MD

cp "$SUMMARY" "$REGRESSION_DIR/notification-governance-preflight-latest.json"
cp "$LOCAL_REPORT" "$REGRESSION_DIR/notification-governance-preflight-latest.md"
cp "$LOG_DIR/notification-settings-update.json" "$REGRESSION_DIR/notification-settings-latest.json" 2>/dev/null || true
cp "$LOG_DIR/notification-silence-rules-after.json" "$REGRESSION_DIR/notification-silence-rules-latest.json" 2>/dev/null || true
cp "$LOG_DIR/notification-audit-silence-created.json" "$REGRESSION_DIR/notification-audit-latest.json" 2>/dev/null || true

echo "notification governance preflight result: $RESULT"
echo "summary: $SUMMARY"
echo "local report: $LOCAL_REPORT"

if [[ "$RESULT" == "blocked" && "$ALLOW_BLOCKERS" != "true" ]]; then
  exit 1
fi
