#!/usr/bin/env bash
set -euo pipefail

APISIX="${APISIX:-http://10.0.5.8:30180}"
TENANT="${TENANT:-default}"
OTHER_TENANT="${OTHER_TENANT:-audit-governance-other}"
RUN_ID="${RUN_ID:-$(date +%Y%m%d%H%M%S)-audit-governance}"
LOG_DIR="${LOG_DIR:-doc/02_acceptance/runs/$RUN_ID}"
REGRESSION_DIR="${REGRESSION_DIR:-doc/02_acceptance/02-regression}"
KUBECTL="${KUBECTL:-kubectl}"
KUBE_SERVER="${KUBE_SERVER:-https://127.0.0.1:6443}"
KUBE_TLS_SERVER_NAME="${KUBE_TLS_SERVER_NAME:-10.0.5.8}"
JWT_SECRET_NAMESPACE="${JWT_SECRET_NAMESPACE:-traffic-analysis}"
JWT_SECRET_NAME="${JWT_SECRET_NAME:-traffic-credentials}"
PG_NAMESPACE="${PG_NAMESPACE:-databases}"
PG_POD="${PG_POD:-postgres-primary-0}"
REPORT="$LOG_DIR/audit-governance-checks.ndjson"
SUMMARY="$LOG_DIR/audit-governance-summary.json"

mkdir -p "$LOG_DIR" "$REGRESSION_DIR"
: >"$REPORT"

kctl() {
  env -u HTTP_PROXY -u HTTPS_PROXY -u ALL_PROXY -u http_proxy -u https_proxy -u all_proxy \
    "$KUBECTL" --server="$KUBE_SERVER" --tls-server-name="$KUBE_TLS_SERVER_NAME" "$@"
}

log_check() {
  local phase="$1" name="$2" passed="$3" detail="${4:-}" artifact="${5:-}"
  jq -nc --arg ts "$(date -Iseconds)" --arg phase "$phase" --arg name "$name" \
    --argjson passed "$passed" --arg detail "$detail" --arg artifact "$artifact" \
    '{ts:$ts,phase:$phase,name:$name,passed:$passed,severity:(if $passed then "info" else "blocker" end),detail:$detail,artifact:$artifact}' >>"$REPORT"
}

trim_file() {
  [[ -s "$1" ]] && head -c 1200 "$1" | tr '\n' ' '
}

make_token() {
  local username="$1" tenant="$2" roles_json="$3" permissions_json="$4"
  JWT_SECRET="$JWT_SECRET" USERNAME="$username" TENANT_ID="$tenant" ROLES_JSON="$roles_json" PERMISSIONS_JSON="$permissions_json" RUN_ID="$RUN_ID" python3 - <<'PY'
import base64, hashlib, hmac, json, os, time, uuid
def enc(value):
    return base64.urlsafe_b64encode(value).rstrip(b'=').decode()
now = int(time.time())
header = {'alg': 'HS256', 'typ': 'JWT'}
claims = {
    'iss': 'traffic-auth-service', 'sub': str(uuid.uuid4()), 'jti': str(uuid.uuid4()),
    'user_id': str(uuid.uuid4()), 'tenant_id': os.environ['TENANT_ID'],
    'username': os.environ['USERNAME'], 'roles': json.loads(os.environ['ROLES_JSON']),
    'permissions': json.loads(os.environ['PERMISSIONS_JSON']), 'token_type': 'access',
    'session_id': 'codex-audit-governance-' + os.environ['RUN_ID'], 'iat': now, 'exp': now + 1800,
}
unsigned = '.'.join((enc(json.dumps(header,separators=(',',':')).encode()), enc(json.dumps(claims,separators=(',',':')).encode())))
signature = hmac.new(os.environ['JWT_SECRET'].encode(), unsigned.encode(), hashlib.sha256).digest()
print(unsigned + '.' + enc(signature))
PY
}

call_api() {
  local name="$1" method="$2" path="$3" expected="$4" token="$5" output="$6" body="${7:-}"
  local code rc
  set +e
  if [[ -n "$body" ]]; then
    code="$(curl --noproxy '*' -sS -m 30 -o "$output" -w '%{http_code}' -X "$method" \
      -H "Authorization: Bearer $token" -H 'Content-Type: application/json' --data "$body" "$APISIX$path" 2>"$output.err")"
  else
    code="$(curl --noproxy '*' -sS -m 30 -o "$output" -w '%{http_code}' -X "$method" \
      -H "Authorization: Bearer $token" "$APISIX$path" 2>"$output.err")"
  fi
  rc=$?
  set -e
  if [[ "$rc" -eq 0 && "$code" == "$expected" ]]; then
    log_check api "$name" true "http-$code" "$(basename "$output")"
    return 0
  fi
  log_check api "$name" false "expected=$expected actual=$code rc=$rc body=$(trim_file "$output") error=$(trim_file "$output.err")" "$(basename "$output")"
  return 1
}

assert_json() {
  local name="$1" file="$2" filter="$3"
  if jq -e "$filter" "$file" >/dev/null 2>&1; then
    log_check assert "$name" true "$filter" "$(basename "$file")"
  else
    log_check assert "$name" false "$filter body=$(trim_file "$file")" "$(basename "$file")"
  fi
}

psql_exec() {
  local sql="$1"
  kctl -n "$PG_NAMESPACE" exec "$PG_POD" -- env PGPASSWORD="$PG_PASSWORD" \
    psql -U postgres -d traffic_platform -v ON_ERROR_STOP=1 -Atc "$sql"
}

for command in jq curl python3 base64 "$KUBECTL"; do
  command -v "$command" >/dev/null || { echo "missing command: $command" >&2; exit 2; }
done

JWT_SECRET="$(kctl -n "$JWT_SECRET_NAMESPACE" get secret "$JWT_SECRET_NAME" -o jsonpath='{.data.JWT_SECRET}' | base64 -d)"
PG_PASSWORD="$(kctl -n "$JWT_SECRET_NAMESPACE" get secret "$JWT_SECRET_NAME" -o jsonpath='{.data.PG_PASSWORD}' | base64 -d)"
ADMIN_TOKEN="$(make_token audit-admin "$TENANT" '["admin"]' '["*","admin:*"]')"
READER_TOKEN="$(make_token audit-reader "$TENANT" '["viewer"]' '["audit:read"]')"
VIEWER_TOKEN="$(make_token audit-viewer "$TENANT" '["viewer"]' '["audit:read"]')"
NO_READ_TOKEN="$(make_token audit-no-read "$TENANT" '["viewer"]' '["alert:read"]')"
OTHER_TOKEN="$(make_token audit-other "$OTHER_TENANT" '["admin"]' '["*","admin:*"]')"

call_api 'admin lists tenant audit logs' GET '/api/v1/audit/logs?limit=10' 200 "$ADMIN_TOKEN" "$LOG_DIR/list-admin.json"
assert_json 'list exposes rows, summary and retention' "$LOG_DIR/list-admin.json" '.success == true and (.data.logs|type=="array") and (.data.trails|type=="array") and (.data.total|type=="number") and (.data.summary.integrity_rate|type=="number") and (.data.retention.retention_days|type=="number")'
call_api 'dedicated audit reader lists logs' GET '/api/v1/audit/logs?limit=3' 200 "$READER_TOKEN" "$LOG_DIR/list-reader.json"
call_api 'missing audit read fails closed' GET '/api/v1/audit/logs?limit=1' 403 "$NO_READ_TOKEN" "$LOG_DIR/list-no-read.json" || true

query_name="codex-audit-$RUN_ID"
save_body="$(jq -nc --arg name "$query_name" '{name:$name,filters:{risk:"high"}}')"
call_api 'admin persists saved query' POST '/api/v1/audit/saved-queries' 201 "$ADMIN_TOKEN" "$LOG_DIR/save-query.json" "$save_body"
assert_json 'saved query response matches web contract' "$LOG_DIR/save-query.json" '.success == true and (.data.query_id|type=="string" and length>0) and .data.query_id == .data.saved_query_id'
QUERY_ID="$(jq -r '.data.query_id // ""' "$LOG_DIR/save-query.json")"

call_api 'saved-query audit event is queryable' GET "/api/v1/audit/logs?action=AUDIT_SAVED_QUERY_CREATED&object_id=$QUERY_ID&limit=10" 200 "$ADMIN_TOKEN" "$LOG_DIR/query-event.json"
assert_json 'saved-query audit event is present' "$LOG_DIR/query-event.json" '.success == true and .data.total >= 1 and (.data.trails[0].request_id|type=="string") and (.data.trails[0].trace_id|type=="string")'
LOG_ID="$(jq -r '.data.trails[0].log_id // ""' "$LOG_DIR/query-event.json")"

if [[ -n "$LOG_ID" ]]; then
  call_api 'admin reads tenant-bound detail' GET "/api/v1/audit/logs/$LOG_ID" 200 "$ADMIN_TOKEN" "$LOG_DIR/detail-admin.json"
  assert_json 'detail preserves governance context' "$LOG_DIR/detail-admin.json" '.success == true and .data.log_id != "" and .data.tenant_id != "" and (.data.details|type=="object")'
  call_api 'other tenant cannot read detail' GET "/api/v1/audit/logs/$LOG_ID" 404 "$OTHER_TOKEN" "$LOG_DIR/detail-other.json" || true
  call_api 'other tenant cannot review detail' POST '/api/v1/audit/reviews' 404 "$OTHER_TOKEN" "$LOG_DIR/review-other.json" "$(jq -nc --arg id "$LOG_ID" '{log_id:$id,reason:"cross tenant review must fail"}')" || true
  call_api 'admin creates persisted review' POST '/api/v1/audit/reviews' 201 "$ADMIN_TOKEN" "$LOG_DIR/review-admin.json" "$(jq -nc --arg id "$LOG_ID" '{log_id:$id,reason:"high risk operation requires review"}')"
  assert_json 'review response matches web contract' "$LOG_DIR/review-admin.json" '.success == true and .data.log_id != "" and .data.status == "escalated" and (.data.review_id|type=="string")'
  call_api 'selected-row export filters by exact audit log id' POST '/api/v1/audit/exports' 200 "$ADMIN_TOKEN" "$LOG_DIR/export-selected.json" "$(jq -nc --arg id "$LOG_ID" '{format:"json",filters:{log_id:$id},mask_sensitive:true}')"
  assert_json 'selected-row export contains exactly one complete row' "$LOG_DIR/export-selected.json" '.success == true and .data.row_count == 1 and .data.total_matching == 1 and .data.truncated == false and .data.mask_sensitive == true'
else
  log_check assert 'extract saved-query audit log id' false 'missing log_id' 'query-event.json'
fi

call_api 'viewer cannot save query' POST '/api/v1/audit/saved-queries' 403 "$VIEWER_TOKEN" "$LOG_DIR/save-query-viewer.json" "$save_body" || true
call_api 'viewer cannot export evidence' POST '/api/v1/audit/exports' 403 "$VIEWER_TOKEN" "$LOG_DIR/export-viewer.json" '{"format":"pdf","filters":{}}' || true
call_api 'viewer cannot run integrity check' POST '/api/v1/audit/integrity-checks' 403 "$VIEWER_TOKEN" "$LOG_DIR/integrity-viewer.json" '{"filters":{}}' || true
call_api 'oversized synchronous export fails instead of truncating' POST '/api/v1/audit/exports' 413 "$ADMIN_TOKEN" "$LOG_DIR/export-oversized.json" '{"format":"json","filters":{}}' || true

for format in pdf csv json; do
  call_api "admin exports real $format artifact" POST '/api/v1/audit/exports' 200 "$ADMIN_TOKEN" "$LOG_DIR/export-$format.json" "{\"format\":\"$format\",\"filters\":{\"object_type\":\"audit_saved_query\"}}"
  FORMAT="$format" SOURCE="$LOG_DIR/export-$format.json" python3 - <<'PY'
import base64, hashlib, json, os
p=json.load(open(os.environ['SOURCE']))['data']; raw=base64.b64decode(p['content_base64'])
assert p['sha256']=='sha256:'+hashlib.sha256(raw).hexdigest()
assert p['size_bytes']==len(raw) and p['filename'].endswith('.'+os.environ['FORMAT'])
assert p['row_count']==p['total_matching'] and p['truncated'] is False and p['mask_sensitive'] is True
if os.environ['FORMAT']=='pdf': assert raw.startswith(b'%PDF-1.4') and raw.endswith(b'%%EOF\n')
elif os.environ['FORMAT']=='csv': assert raw.startswith(b'log_id,timestamp,')
else: json.loads(raw)
PY
  log_check artifact "$format checksum and content are valid" true 'sha256 and file signature verified' "export-$format.json"
done

START_MS="$(( $(date +%s) * 1000 - 600000 ))"
END_MS="$(( $(date +%s) * 1000 + 120000 ))"
integrity_body="$(jq -nc --argjson start_ms "$START_MS" --argjson end_ms "$END_MS" '{filters:{start:$start_ms,end:$end_ms}}')"
call_api 'first exact-window integrity check establishes baselines' POST '/api/v1/audit/integrity-checks' 201 "$ADMIN_TOKEN" "$LOG_DIR/integrity-first.json" "$integrity_body"
assert_json 'first check is honest about baseline state' "$LOG_DIR/integrity-first.json" '.success == true and .data.valid == true and (.data.status == "baseline_created" or .data.status == "passed") and .data.records_checked >= 1 and (.data.root_sha256|startswith("sha256:"))'
call_api 'second exact-window integrity check verifies baselines' POST '/api/v1/audit/integrity-checks' 201 "$ADMIN_TOKEN" "$LOG_DIR/integrity-second.json" "$integrity_body"
assert_json 'second check compares persisted per-row baselines' "$LOG_DIR/integrity-second.json" '.success == true and .data.valid == true and .data.status == "passed" and .data.matched >= 1 and .data.mismatched == 0'

if [[ -n "$LOG_ID" ]]; then
  psql_exec "UPDATE audit_logs SET detail=jsonb_set(detail,'{codex_tamper_probe}','true'::jsonb) WHERE tenant_id='$TENANT' AND event_id='$LOG_ID';" >"$LOG_DIR/tamper-update.txt"
  call_api 'tamper probe is detected' POST '/api/v1/audit/integrity-checks' 201 "$ADMIN_TOKEN" "$LOG_DIR/integrity-tampered.json" "$integrity_body"
  assert_json 'tampered row fails integrity comparison' "$LOG_DIR/integrity-tampered.json" '.success == true and .data.valid == false and .data.status == "failed" and .data.mismatched >= 1'
  psql_exec "UPDATE audit_logs SET detail=detail-'codex_tamper_probe' WHERE tenant_id='$TENANT' AND event_id='$LOG_ID';" >"$LOG_DIR/tamper-restore.txt"
  call_api 'restored row passes integrity comparison' POST '/api/v1/audit/integrity-checks' 201 "$ADMIN_TOKEN" "$LOG_DIR/integrity-restored.json" "$integrity_body"
  assert_json 'restored row matches original immutable baseline' "$LOG_DIR/integrity-restored.json" '.success == true and .data.valid == true and .data.status == "passed" and .data.mismatched == 0'
fi

DELETE_PROBE_ID="audit-delete-probe-$RUN_ID"
DELETE_PROBE_OBJECT="delete-probe-$RUN_ID"
DELETE_PROBE_TS="$(date -u +%Y-%m-%dT%H:%M:%S.000Z)"
psql_exec "INSERT INTO audit_logs(event_id,tenant_id,user_id,action,object_type,object_id,detail,ip_addr,user_agent,request_id,trace_id,success,error_message,risk_level,result,created_at) VALUES ('$DELETE_PROBE_ID','$TENANT',NULL,'AUDIT_DELETE_PROBE','audit_delete_probe','$DELETE_PROBE_OBJECT','{\"result\":\"success\",\"probe\":\"delete-detection\"}'::jsonb,'10.0.0.8','codex-preflight','req-$DELETE_PROBE_OBJECT','trace-$DELETE_PROBE_OBJECT',true,'','high','success','$DELETE_PROBE_TS');" >"$LOG_DIR/delete-probe-insert.txt"
DELETE_START_MS="$(( $(date -d "$DELETE_PROBE_TS" +%s) * 1000 - 1000 ))"
DELETE_END_MS="$(( DELETE_START_MS + 3000 ))"
delete_integrity_body="$(jq -nc --arg object_id "$DELETE_PROBE_OBJECT" --argjson start_ms "$DELETE_START_MS" --argjson end_ms "$DELETE_END_MS" '{filters:{object_type:"audit_delete_probe",object_id:$object_id,start:$start_ms,end:$end_ms}}')"
call_api 'deletion probe establishes fixed-window manifest' POST '/api/v1/audit/integrity-checks' 201 "$ADMIN_TOKEN" "$LOG_DIR/integrity-delete-baseline.json" "$delete_integrity_body"
assert_json 'deletion probe baseline is created' "$LOG_DIR/integrity-delete-baseline.json" '.success == true and .data.valid == true and .data.status == "baseline_created" and .data.records_checked == 1'
psql_exec "DELETE FROM audit_logs WHERE tenant_id='$TENANT' AND event_id='$DELETE_PROBE_ID';" >"$LOG_DIR/delete-probe-delete.txt"
call_api 'deleted audit row is detected by fixed-window manifest' POST '/api/v1/audit/integrity-checks' 201 "$ADMIN_TOKEN" "$LOG_DIR/integrity-delete-detected.json" "$delete_integrity_body"
assert_json 'deleted row fails integrity comparison' "$LOG_DIR/integrity-delete-detected.json" '.success == true and .data.valid == false and .data.status == "failed" and .data.missing == 1'
psql_exec "INSERT INTO audit_logs(event_id,tenant_id,user_id,action,object_type,object_id,detail,ip_addr,user_agent,request_id,trace_id,success,error_message,risk_level,result,created_at) VALUES ('$DELETE_PROBE_ID','$TENANT',NULL,'AUDIT_DELETE_PROBE','audit_delete_probe','$DELETE_PROBE_OBJECT','{\"result\":\"success\",\"probe\":\"delete-detection\"}'::jsonb,'10.0.0.8','codex-preflight','req-$DELETE_PROBE_OBJECT','trace-$DELETE_PROBE_OBJECT',true,'','high','success','$DELETE_PROBE_TS');" >"$LOG_DIR/delete-probe-restore.txt"
call_api 'restored deleted row passes fixed-window manifest' POST '/api/v1/audit/integrity-checks' 201 "$ADMIN_TOKEN" "$LOG_DIR/integrity-delete-restored.json" "$delete_integrity_body"
assert_json 'restored deleted row matches manifest' "$LOG_DIR/integrity-delete-restored.json" '.success == true and .data.valid == true and .data.status == "passed" and .data.missing == 0 and .data.mismatched == 0'

if [[ -n "$QUERY_ID" ]]; then
  SAVED_COUNT="$(psql_exec "SELECT count(*) FROM audit_saved_queries WHERE tenant_id='$TENANT' AND saved_query_id::text='$QUERY_ID';")"
  EXPORT_COUNT="$(psql_exec "SELECT count(*) FROM audit_exports WHERE tenant_id='$TENANT' AND created_at > now()-interval '10 minutes';")"
  REVIEW_COUNT="$(psql_exec "SELECT count(*) FROM audit_reviews WHERE tenant_id='$TENANT' AND audit_log_id='$LOG_ID';")"
  if [[ "$SAVED_COUNT" -ge 1 && "$EXPORT_COUNT" -ge 3 && "$REVIEW_COUNT" -ge 1 ]]; then
    log_check postgres 'saved query, exports and review persisted' true "saved=$SAVED_COUNT exports=$EXPORT_COUNT reviews=$REVIEW_COUNT"
  else
    log_check postgres 'saved query, exports and review persisted' false "saved=$SAVED_COUNT exports=$EXPORT_COUNT reviews=$REVIEW_COUNT"
  fi
fi

TOTAL="$(jq -s 'length' "$REPORT")"
PASSED="$(jq -s '[.[]|select(.passed)]|length' "$REPORT")"
BLOCKERS="$(jq -s '[.[]|select(.passed|not)]|length' "$REPORT")"
RESULT=pass
[[ "$BLOCKERS" -eq 0 ]] || RESULT=blocked
jq -n --arg run_id "$RUN_ID" --arg result "$RESULT" --arg apisix "$APISIX" --arg tenant "$TENANT" \
  --arg other_tenant "$OTHER_TENANT" --arg query_id "$QUERY_ID" --arg log_id "$LOG_ID" \
  --argjson total "$TOTAL" --argjson passed "$PASSED" --argjson blockers "$BLOCKERS" --slurpfile checks "$REPORT" \
  '{run_id:$run_id,result:$result,apisix:$apisix,tenant:$tenant,other_tenant:$other_tenant,query_id:$query_id,log_id:$log_id,total:$total,passed:$passed,blockers:$blockers,checks:$checks}' >"$SUMMARY"
cp "$SUMMARY" "$REGRESSION_DIR/audit-governance-preflight-latest.json"
echo "audit governance preflight result: $RESULT ($PASSED/$TOTAL, blockers=$BLOCKERS)"
echo "summary: $SUMMARY"
[[ "$RESULT" == pass ]]
