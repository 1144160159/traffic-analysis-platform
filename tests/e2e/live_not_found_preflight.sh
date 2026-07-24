#!/usr/bin/env bash
set -euo pipefail

APISIX="${APISIX:-http://10.0.5.8:30180}"
RUN_ID="${RUN_ID:-not-found-$(date +%Y%m%d%H%M%S)}"
LOG_DIR="${LOG_DIR:-doc/02_acceptance/runs/${RUN_ID}}"
SUMMARY="${SUMMARY:-doc/02_acceptance/02-regression/not-found-preflight-latest.json}"
KUBECTL_SERVER="${KUBECTL_SERVER:-https://127.0.0.1:6443}"
KUBECTL_TLS_SERVER_NAME="${KUBECTL_TLS_SERVER_NAME:-10.0.5.8}"
mkdir -p "$LOG_DIR" "$(dirname "$SUMMARY")"

kctl() {
  env -u HTTP_PROXY -u HTTPS_PROXY -u ALL_PROXY -u http_proxy -u https_proxy -u all_proxy \
    kubectl --server="$KUBECTL_SERVER" --tls-server-name="$KUBECTL_TLS_SERVER_NAME" "$@"
}

JWT_SECRET="$(kctl -n traffic-analysis get secret traffic-credentials -o jsonpath='{.data.JWT_SECRET}' | base64 -d)"
PG_PASSWORD="$(kctl -n traffic-analysis get secret traffic-credentials -o jsonpath='{.data.PG_PASSWORD}' | base64 -d)"
EVENT_ID="nav-$(python3 -c 'import uuid; print(uuid.uuid4())')"
USER_ID="$(python3 -c 'import uuid; print(uuid.uuid4())')"
OTHER_USER_ID="$(python3 -c 'import uuid; print(uuid.uuid4())')"

make_token() {
  local user_id="$1" tenant_id="$2"
  JWT_SECRET="$JWT_SECRET" USER_ID="$user_id" TENANT_ID="$tenant_id" RUN_ID="$RUN_ID" python3 - <<'PY'
import base64, hashlib, hmac, json, os, time, uuid
def enc(value):
    return base64.urlsafe_b64encode(value).rstrip(b"=").decode()
now = int(time.time())
header = enc(json.dumps({"alg":"HS256","typ":"JWT"}, separators=(",",":")).encode())
claims = enc(json.dumps({
    "iss":"traffic-auth-service", "sub":os.environ["USER_ID"], "jti":str(uuid.uuid4()),
    "user_id":os.environ["USER_ID"], "tenant_id":os.environ["TENANT_ID"], "username":"codex-not-found-preflight",
    "roles":["viewer"], "permissions":["alert:read"], "token_type":"access",
    "session_id":"not-found-" + os.environ["RUN_ID"], "iat":now, "exp":now + 1800,
}, separators=(",",":")).encode())
body = f"{header}.{claims}"
signature = enc(hmac.new(os.environ["JWT_SECRET"].encode(), body.encode(), hashlib.sha256).digest())
print(f"{body}.{signature}")
PY
}

TOKEN="$(make_token "$USER_ID" default)"
OTHER_USER_TOKEN="$(make_token "$OTHER_USER_ID" default)"
OTHER_TENANT_TOKEN="$(make_token "$OTHER_USER_ID" tenant-b)"

request() {
  local output="$1" token="$2" body="$3" path="${4:-}"
  local headers="$output.headers"
  local auth=()
  [[ -n "$token" ]] && auth=(-H "Authorization: Bearer $token")
  curl --noproxy '*' -sS -m 30 -D "$headers" -o "$output" -w '%{http_code}' \
    -X POST "${auth[@]}" -H 'Content-Type: application/json' --data "$body" \
    "$APISIX/api/v1/auth/navigation-miss${path}"
}

payload="$(jq -nc --arg id "$EVENT_ID" '{event_id:$id,source:"web-ui"}')"
CODE_ONE="$(request "$LOG_DIR/first.json" "$TOKEN" "$payload")"
CODE_TWO="$(request "$LOG_DIR/retry.json" "$TOKEN" "$payload")"
CODE_OTHER_USER="$(request "$LOG_DIR/other-user.json" "$OTHER_USER_TOKEN" "$payload")"
CODE_OTHER_TENANT="$(request "$LOG_DIR/other-tenant.json" "$OTHER_TENANT_TOKEN" "$payload")"
CODE_SUPPORT="$(request "$LOG_DIR/support.json" "$TOKEN" "$(jq -nc --arg id "$EVENT_ID" '{event_id:$id}')" /support)"
CODE_SUPPORT_RETRY="$(request "$LOG_DIR/support-retry.json" "$TOKEN" "$(jq -nc --arg id "$EVENT_ID" '{event_id:$id}')" /support)"
CODE_SUPPORT_OTHER="$(request "$LOG_DIR/support-other-user.json" "$OTHER_USER_TOKEN" "$(jq -nc --arg id "$EVENT_ID" '{event_id:$id}')" /support)"
CODE_UNAUTH="$(request "$LOG_DIR/unauthorized.json" '' "$payload")"
CODE_INVALID="$(request "$LOG_DIR/invalid.json" "$TOKEN" '{"event_id":"/internal/admin","source":"web-ui"}')"

TRACE_ONE="$(jq -r '.trace_id // empty' "$LOG_DIR/first.json")"
TRACE_TWO="$(jq -r '.trace_id // empty' "$LOG_DIR/retry.json")"
AUDIT_COUNT="$(kctl -n databases exec postgres-primary-0 -- env PGPASSWORD="$PG_PASSWORD" \
  psql -q -U postgres -d traffic_platform -Atc "SELECT count(*) FROM audit_logs WHERE tenant_id='default' AND event_id='${EVENT_ID}';" | tr -d '[:space:]')"
AUDIT_SAFE="$(kctl -n databases exec postgres-primary-0 -- env PGPASSWORD="$PG_PASSWORD" \
  psql -q -U postgres -d traffic_platform -Atc "SELECT count(*) FROM audit_logs WHERE event_id='${EVENT_ID}' AND action='navigation_not_found' AND object_type='frontend_route' AND trace_id <> '' AND detail->>'route_kind'='unknown' AND detail->>'source'='web-ui' AND detail::text NOT LIKE '%/internal/%';" | tr -d '[:space:]')"
AUDIT_TRACE="$(kctl -n databases exec postgres-primary-0 -- env PGPASSWORD="$PG_PASSWORD" \
  psql -q -U postgres -d traffic_platform -Atc "SELECT trace_id FROM audit_logs WHERE tenant_id='default' AND event_id='${EVENT_ID}';" | tr -d '[:space:]')"
SUPPORT_ID="support-${EVENT_ID#nav-}"
SUPPORT_TRACE="$(jq -r '.trace_id // empty' "$LOG_DIR/support.json")"
SUPPORT_RETRY_TRACE="$(jq -r '.trace_id // empty' "$LOG_DIR/support-retry.json")"
SUPPORT_COUNT="$(kctl -n databases exec postgres-primary-0 -- env PGPASSWORD="$PG_PASSWORD" \
  psql -q -U postgres -d traffic_platform -Atc "SELECT count(*) FROM audit_logs WHERE tenant_id='default' AND user_id::text='${USER_ID}' AND event_id='${SUPPORT_ID}' AND action='navigation_support_requested' AND detail->>'navigation_event_id'='${EVENT_ID}' AND detail->>'status'='queued';" | tr -d '[:space:]')"

CHECKS="$(jq -n \
  --argjson first "$([[ "$CODE_ONE" == 201 ]] && echo true || echo false)" \
  --argjson retry "$([[ "$CODE_TWO" == 201 ]] && echo true || echo false)" \
  --argjson response "$(jq -e '.persisted == true and .audit_action == "navigation_not_found" and (.trace_id | test("^[0-9a-f-]{36}$")) and (.occurred_at | length > 0) and (.tenant_name | length > 0) and (.site_name | length > 0) and (.access_source | length > 0) and (.statuses | length == 4) and ([.statuses[].state] | all(. == "healthy"))' "$LOG_DIR/first.json" >/dev/null && echo true || echo false)" \
  --argjson idempotent "$([[ "$AUDIT_COUNT" == 1 ]] && echo true || echo false)" \
  --argjson safe "$([[ "$AUDIT_SAFE" == 1 ]] && echo true || echo false)" \
  --argjson traces "$([[ -n "$TRACE_ONE" && "$TRACE_ONE" == "$TRACE_TWO" && "$TRACE_ONE" == "$AUDIT_TRACE" ]] && echo true || echo false)" \
  --argjson other_user "$([[ "$CODE_OTHER_USER" == 404 && "$(jq -r '.code // empty' "$LOG_DIR/other-user.json")" == BIZ_3011 ]] && echo true || echo false)" \
  --argjson other_tenant "$([[ "$CODE_OTHER_TENANT" == 404 && "$(jq -r '.code // empty' "$LOG_DIR/other-tenant.json")" == BIZ_3011 ]] && echo true || echo false)" \
  --argjson support "$([[ "$CODE_SUPPORT" == 201 && "$CODE_SUPPORT_RETRY" == 201 && "$SUPPORT_COUNT" == 1 && -n "$SUPPORT_TRACE" && "$SUPPORT_TRACE" == "$SUPPORT_RETRY_TRACE" ]] && echo true || echo false)" \
  --argjson support_other "$([[ "$CODE_SUPPORT_OTHER" == 404 && "$(jq -r '.code // empty' "$LOG_DIR/support-other-user.json")" == BIZ_3011 ]] && echo true || echo false)" \
  --argjson unauthorized "$([[ "$CODE_UNAUTH" == 401 ]] && echo true || echo false)" \
  --argjson invalid "$([[ "$CODE_INVALID" == 400 ]] && echo true || echo false)" \
  '[
    {name:"first authenticated write returns 201",passed:$first},
    {name:"idempotent retry returns 201",passed:$retry},
    {name:"response is database-backed and safe",passed:$response},
    {name:"event is persisted exactly once",passed:$idempotent},
    {name:"audit detail excludes raw internal paths",passed:$safe},
    {name:"idempotent retry returns the persisted audit trace",passed:$traces},
    {name:"cross-user event id reuse is rejected",passed:$other_user},
    {name:"cross-tenant event id reuse is rejected",passed:$other_tenant},
    {name:"support request is observable and idempotent",passed:$support},
    {name:"support request ownership is enforced",passed:$support_other},
    {name:"unauthenticated write is rejected",passed:$unauthorized},
    {name:"raw path event id is rejected",passed:$invalid}
  ]')"
PASSED="$(jq '[.[] | select(.passed)] | length' <<<"$CHECKS")"
TOTAL="$(jq 'length' <<<"$CHECKS")"
RESULT="$([[ "$PASSED" == "$TOTAL" ]] && echo pass || echo fail)"

jq -n --arg result "$RESULT" --arg run_id "$RUN_ID" --arg event_id "$EVENT_ID" \
  --arg trace_id "$TRACE_ONE" --arg support_request_id "$SUPPORT_ID" --arg support_trace_id "$SUPPORT_TRACE" --argjson audit_count "$AUDIT_COUNT" --argjson support_count "$SUPPORT_COUNT" --argjson checks "$CHECKS" \
  --arg auth_image "$(kctl -n traffic-analysis get deployment auth-service -o jsonpath='{.spec.template.spec.containers[0].image}')" \
  '{schema_version:1,result:$result,run_id:$run_id,event_id:$event_id,trace_id:$trace_id,audit_count:$audit_count,support_request_id:$support_request_id,support_trace_id:$support_trace_id,support_count:$support_count,auth_image:$auth_image,checks:$checks}' \
  >"$SUMMARY"

echo "not-found preflight result: $RESULT ($PASSED/$TOTAL)"
echo "summary: $SUMMARY"
[[ "$RESULT" == pass ]]
