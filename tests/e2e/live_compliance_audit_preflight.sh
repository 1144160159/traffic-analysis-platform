#!/usr/bin/env bash
set -euo pipefail

APISIX="${APISIX:-http://10.0.5.8:30180}"
TENANT="${TENANT:-default}"
OTHER_TENANT="${OTHER_TENANT:-tenant-b}"
LOG_DIR="${LOG_DIR:-doc/02_acceptance/runs/$(date +%Y%m%d%H%M%S)-compliance-audit-preflight}"
RUN_ID="${RUN_ID:-$(date +%Y%m%d%H%M%S)-compliance-audit-preflight}"
REGRESSION_DIR="${REGRESSION_DIR:-doc/02_acceptance/02-regression}"
KUBECTL="${KUBECTL:-kubectl}"
ALLOW_BLOCKERS="${ALLOW_BLOCKERS:-false}"
JWT_SECRET_NAMESPACE="${JWT_SECRET_NAMESPACE:-traffic-analysis}"
JWT_SECRET_NAME="${JWT_SECRET_NAME:-traffic-credentials}"
JWT_SECRET_KEY="${JWT_SECRET_KEY:-JWT_SECRET}"
PG_NAMESPACE="${PG_NAMESPACE:-databases}"
PG_POD="${PG_POD:-postgres-primary-0}"
PG_SECRET_KEY="${PG_SECRET_KEY:-PG_PASSWORD}"

REPORT="$LOG_DIR/live-compliance-audit-preflight-$RUN_ID.ndjson"
SUMMARY="$LOG_DIR/live-compliance-audit-preflight-$RUN_ID-summary.json"
LOCAL_REPORT="$LOG_DIR/local-report.md"
RUN_SLUG="$(printf '%s' "$RUN_ID" | tr -c 'a-zA-Z0-9-' '-' | cut -c1-64 | sed 's/-*$//')"
REPORT_TYPE="${REPORT_TYPE:-weekly}"

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
    head -c 1000 "$file" \
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
    "session_id": "codex-compliance-audit-" + os.environ["RUN_ID"],
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
  local name="$1" method="$2" path="$3" expected_code="$4" token="$5" output="$6" body="${7:-}"
  local err_file code rc
  err_file="$output.err"
  set +e
  if [[ -n "$body" ]]; then
    code="$(curl --noproxy '*' -sS -m 30 -o "$output" -w '%{http_code}' \
      -X "$method" \
      -H "Authorization: Bearer $token" \
      -H "X-Tenant-ID: $TENANT" \
      -H "Content-Type: application/json" \
      --data "$body" \
      "$APISIX$path" 2>"$err_file")"
  else
    code="$(curl --noproxy '*' -sS -m 30 -o "$output" -w '%{http_code}' \
      -X "$method" \
      -H "Authorization: Bearer $token" \
      -H "X-Tenant-ID: $TENANT" \
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

psql_exec() {
  local sql="$1"
  kctl -n "$PG_NAMESPACE" exec "$PG_POD" -- env PGPASSWORD="$PG_PASSWORD" \
    psql -U postgres -d traffic_platform -v ON_ERROR_STOP=1 -Atc "$sql"
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

need_cmd git
need_cmd jq
need_cmd curl
need_cmd python3
need_cmd "$KUBECTL"

git rev-parse HEAD >"$LOG_DIR/commit-sha.txt"
git status --short >"$LOG_DIR/git-status.txt"

JWT_SECRET="$(kctl -n "$JWT_SECRET_NAMESPACE" get secret "$JWT_SECRET_NAME" -o "jsonpath={.data.$JWT_SECRET_KEY}" | base64 -d)"
PG_PASSWORD="$(kctl -n "$JWT_SECRET_NAMESPACE" get secret "$JWT_SECRET_NAME" -o "jsonpath={.data.$PG_SECRET_KEY}" | base64 -d)"

ADMIN_TOKEN="$(make_token codex-compliance-admin "$TENANT" '["admin"]' '["*","admin:*","audit:read","user:read"]')"
READ_TOKEN="$(make_token codex-compliance-reader "$TENANT" '["compliance-reader"]' '["compliance:read"]')"
WRITE_TOKEN="$(make_token codex-compliance-writer "$TENANT" '["compliance-writer"]' '["compliance:write"]')"
EXPORT_TOKEN="$(make_token codex-compliance-exporter "$TENANT" '["compliance-exporter"]' '["compliance:export"]')"
REMEDIATE_TOKEN="$(make_token codex-compliance-remediator "$TENANT" '["compliance-remediator"]' '["compliance:remediate"]')"
FINALIZE_TOKEN="$(make_token codex-compliance-finalizer "$TENANT" '["compliance-finalizer"]' '["compliance:finalize"]')"
VIEWER_TOKEN="$(make_token codex-compliance-viewer "$TENANT" '["viewer"]' '["user:read","audit:read"]')"
NO_READ_TOKEN="$(make_token codex-compliance-no-read "$TENANT" '["viewer"]' '["user:read"]')"
OTHER_TOKEN="$(make_token codex-compliance-other-admin "$OTHER_TENANT" '["admin"]' '["*","admin:*","audit:read","user:read"]')"

curl_json "compliance reports list is readable" "GET" "/api/v1/compliance/reports?limit=5" "200" "$ADMIN_TOKEN" "$LOG_DIR/compliance-reports-before.json"
assert_json "compliance reports response shape" "$LOG_DIR/compliance-reports-before.json" '.success == true and (.data.reports | type == "array") and (.data.total | type == "number")'
curl_json "dedicated compliance read scope lists reports" "GET" "/api/v1/compliance/reports?limit=5" "200" "$READ_TOKEN" "$LOG_DIR/compliance-reports-dedicated-read.json"

curl_json "compliance audit trail is readable" "GET" "/api/v1/compliance/audit-trail?limit=5" "200" "$ADMIN_TOKEN" "$LOG_DIR/compliance-audit-trail-before.json"
assert_json "compliance audit trail response shape" "$LOG_DIR/compliance-audit-trail-before.json" '.success == true and (.data.trails | type == "array") and (.data.total | type == "number")'

curl_json "user without compliance read cannot list reports" "GET" "/api/v1/compliance/reports?limit=1" "403" "$NO_READ_TOKEN" "$LOG_DIR/compliance-reports-no-read.json" || true
curl_json "user without audit read cannot list audit trail" "GET" "/api/v1/compliance/audit-trail?limit=1" "403" "$NO_READ_TOKEN" "$LOG_DIR/compliance-audit-no-read.json" || true

curl_json "audit log page API is readable" "GET" "/api/v1/audit/logs?limit=5" "200" "$ADMIN_TOKEN" "$LOG_DIR/audit-logs-before.json"
assert_json "audit log response shape" "$LOG_DIR/audit-logs-before.json" '.success == true and (.data.trails | type == "array") and (.data.total | type == "number")'

INVALIDATED_REPORT_ID="$(psql_exec "SELECT report_id::text FROM compliance_reports WHERE tenant_id='$TENANT' AND status='invalidated' ORDER BY generated_at DESC LIMIT 1;")"
if [[ -n "$INVALIDATED_REPORT_ID" ]]; then
  curl_json "complete report list excludes invalidated legacy rows" "GET" "/api/v1/compliance/reports?limit=100" "200" "$ADMIN_TOKEN" "$LOG_DIR/compliance-reports-without-invalidated.json"
  NORMAL_REPORT_COUNT="$(psql_exec "SELECT count(*) FROM compliance_reports WHERE tenant_id='$TENANT' AND status <> 'invalidated';")"
  assert_json "invalidated legacy report is excluded from list and total" "$LOG_DIR/compliance-reports-without-invalidated.json" \
    --arg report_id "$INVALIDATED_REPORT_ID" --argjson normal_count "$NORMAL_REPORT_COUNT" '.data.total == $normal_count and ([.data.reports[] | select(.report_id == $report_id or .status == "invalidated")] | length) == 0'
  curl_json "invalidated legacy report cannot be exported" "POST" "/api/v1/compliance/reports/$INVALIDATED_REPORT_ID/export" "404" "$ADMIN_TOKEN" "$LOG_DIR/compliance-invalidated-export.json" '{"format":"pdf"}' || true
  curl_json "invalidated legacy report cannot create remediation" "POST" "/api/v1/compliance/reports/$INVALIDATED_REPORT_ID/remediations" "404" "$ADMIN_TOKEN" "$LOG_DIR/compliance-invalidated-remediation.json" '{}' || true
  curl_json "invalidated legacy report cannot be finalized" "POST" "/api/v1/compliance/reports/$INVALIDATED_REPORT_ID/finalize" "404" "$ADMIN_TOKEN" "$LOG_DIR/compliance-invalidated-finalize.json" '{}' || true
else
  json_log "postgres" "legacy zero-evidence reports have been invalidated" "blocker" false "missing" "expected migrated invalidated report evidence" ""
fi

generate_body="$(jq -nc --arg report_type "$REPORT_TYPE" '{report_type:$report_type}')"
curl_json "dedicated compliance write scope generates report" "POST" "/api/v1/compliance/reports/generate" "200" "$WRITE_TOKEN" "$LOG_DIR/compliance-generate-admin.json" "$generate_body"
assert_json "generated compliance report has persisted fail-closed identity" "$LOG_DIR/compliance-generate-admin.json" \
  --arg report_type "$REPORT_TYPE" '.success == true and (.data.report_id | type == "string" and length > 0) and .data.report_type == $report_type and .data.status == "insufficient_evidence" and (.data.sections | length) >= 7 and ([.data.sections[] | select(.status == "pass")] | length) == 0'

REPORT_ID="$(jq -r '.data.report_id // ""' "$LOG_DIR/compliance-generate-admin.json")"
if [[ -n "$REPORT_ID" ]]; then
  curl_json "generated compliance report is queryable" "GET" "/api/v1/compliance/reports?report_type=$REPORT_TYPE&limit=50" "200" "$ADMIN_TOKEN" "$LOG_DIR/compliance-reports-after.json"
  assert_json "generated report appears in compliance report list" "$LOG_DIR/compliance-reports-after.json" \
    --arg report_id "$REPORT_ID" '.success == true and ([.data.reports[] | select(.report_id == $report_id)] | length) == 1'

  curl_json "compliance audit trail has generated report event" "GET" "/api/v1/compliance/audit-trail?action=COMPLIANCE_REPORT_GENERATED&object_id=$REPORT_ID&limit=10" "200" "$ADMIN_TOKEN" "$LOG_DIR/compliance-audit-trail-generated.json"
  assert_json "compliance audit trail contains generated event" "$LOG_DIR/compliance-audit-trail-generated.json" \
    --arg report_id "$REPORT_ID" '.success == true and (.data.total >= 1) and ([.data.trails[] | select(.resource_id == $report_id and .action == "COMPLIANCE_REPORT_GENERATED")] | length) >= 1'

  curl_json "audit log page API has generated report event" "GET" "/api/v1/audit/logs?action=COMPLIANCE_REPORT_GENERATED&object_id=$REPORT_ID&limit=10" "200" "$ADMIN_TOKEN" "$LOG_DIR/audit-logs-generated.json"
  assert_json "audit log page API contains generated event" "$LOG_DIR/audit-logs-generated.json" \
    --arg report_id "$REPORT_ID" '.success == true and (.data.total >= 1) and ([.data.trails[] | select(.resource_id == $report_id and .action == "COMPLIANCE_REPORT_GENERATED")] | length) >= 1'

  curl_json "dedicated compliance export scope exports evidence package" "POST" "/api/v1/compliance/reports/$REPORT_ID/evidence-package" "200" "$EXPORT_TOKEN" "$LOG_DIR/compliance-evidence-package.json"
  assert_json "evidence package has zip payload and checksum" "$LOG_DIR/compliance-evidence-package.json" \
    --arg report_id "$REPORT_ID" '.success == true and .data.report_id == $report_id and .data.artifact_type == "evidence_package" and .data.mime_type == "application/zip" and (.data.sha256 | startswith("sha256:")) and (.data.content_base64 | length > 32)'
  python3 - "$LOG_DIR/compliance-evidence-package.json" "$LOG_DIR/compliance-evidence-package.zip" <<'PY'
import base64, hashlib, json, pathlib, sys, zipfile
source, target = map(pathlib.Path, sys.argv[1:])
payload = json.loads(source.read_text())['data']
content = base64.b64decode(payload['content_base64'])
target.write_bytes(content)
assert payload['sha256'] == 'sha256:' + hashlib.sha256(content).hexdigest()
with zipfile.ZipFile(target) as archive:
    assert sorted(archive.namelist()) == ['manifest.json', 'report.json']
PY
  if [[ -s "$LOG_DIR/compliance-evidence-package.zip" ]]; then
    json_log "artifact" "evidence package opens and checksum matches" "info" true "ok" "$REPORT_ID" "compliance-evidence-package.zip"
  else
    json_log "artifact" "evidence package opens and checksum matches" "blocker" false "missing" "$REPORT_ID" "compliance-evidence-package.zip"
  fi
  curl_json "compliance audit trail has evidence export event" "GET" "/api/v1/compliance/audit-trail?action=COMPLIANCE_EVIDENCE_EXPORTED&object_id=$REPORT_ID&limit=10" "200" "$ADMIN_TOKEN" "$LOG_DIR/compliance-audit-trail-exported.json"
  assert_json "compliance audit trail contains export event" "$LOG_DIR/compliance-audit-trail-exported.json" \
    --arg report_id "$REPORT_ID" '.success == true and (.data.total >= 1) and ([.data.trails[] | select(.resource_id == $report_id and .action == "COMPLIANCE_EVIDENCE_EXPORTED")] | length) >= 1'

  for format in pdf docx; do
    curl_json "dedicated compliance export scope exports complete $format report" "POST" "/api/v1/compliance/reports/$REPORT_ID/export" "200" "$EXPORT_TOKEN" "$LOG_DIR/compliance-report-$format.json" "{\"format\":\"$format\"}"
    assert_json "$format report has payload and checksum" "$LOG_DIR/compliance-report-$format.json" \
      --arg report_id "$REPORT_ID" --arg format "$format" '.success == true and .data.report_id == $report_id and .data.artifact_type == ("report_" + $format) and (.data.sha256 | startswith("sha256:")) and (.data.content_base64 | length > 32)'
  done
  python3 - "$LOG_DIR/compliance-report-pdf.json" "$LOG_DIR/compliance-report-docx.json" "$LOG_DIR" <<'PY'
import base64, hashlib, io, json, pathlib, sys, zipfile
pdf_source, docx_source, output_dir = pathlib.Path(sys.argv[1]), pathlib.Path(sys.argv[2]), pathlib.Path(sys.argv[3])
for source in (pdf_source, docx_source):
    payload = json.loads(source.read_text())['data']
    content = base64.b64decode(payload['content_base64'])
    assert payload['sha256'] == 'sha256:' + hashlib.sha256(content).hexdigest()
    (output_dir / payload['filename']).write_bytes(content)
pdf = base64.b64decode(json.loads(pdf_source.read_text())['data']['content_base64'])
assert pdf.startswith(b'%PDF-1.4')
assert b'Sections:' in pdf and b'Audit trail:' in pdf and b'COMPLIANCE_REPORT_GENERATED' in pdf
docx = base64.b64decode(json.loads(docx_source.read_text())['data']['content_base64'])
with zipfile.ZipFile(io.BytesIO(docx)) as archive:
    document = archive.read('word/document.xml')
    assert b'alert_response' in document and b'COMPLIANCE_REPORT_GENERATED' in document
PY
  json_log "artifact" "PDF and DOCX contain sections and audit trail" "info" true "ok" "$REPORT_ID" "compliance-report-pdf.json,compliance-report-docx.json"

  curl_json "dedicated remediation scope creates persistent tasks" "POST" "/api/v1/compliance/reports/$REPORT_ID/remediations" "200" "$REMEDIATE_TOKEN" "$LOG_DIR/compliance-remediations-first.json" '{}'
  curl_json "dedicated remediation scope reuses persistent tasks" "POST" "/api/v1/compliance/reports/$REPORT_ID/remediations" "200" "$REMEDIATE_TOKEN" "$LOG_DIR/compliance-remediations-second.json" '{}'
  assert_json "remediation repeat is idempotent" "$LOG_DIR/compliance-remediations-second.json" \
    --slurpfile first "$LOG_DIR/compliance-remediations-first.json" '.success == true and .data.created == 0 and .data.reused == .data.total and ([.data.tasks[].task_id] | sort) == ([$first[0].data.tasks[].task_id] | sort)'

  curl_json "dedicated finalize scope finalizes canonical report snapshot" "POST" "/api/v1/compliance/reports/$REPORT_ID/finalize" "200" "$FINALIZE_TOKEN" "$LOG_DIR/compliance-finalize-first.json" '{}'
  curl_json "dedicated finalize scope rejects repeat finalization" "POST" "/api/v1/compliance/reports/$REPORT_ID/finalize" "409" "$FINALIZE_TOKEN" "$LOG_DIR/compliance-finalize-second.json" '{}' || true
  assert_json "finalization returns canonical hash" "$LOG_DIR/compliance-finalize-first.json" '.success == true and .data.status == "finalized" and (.data.report_sha256 | startswith("sha256:"))'
  python3 - "$LOG_DIR/compliance-evidence-package.zip" "$LOG_DIR/compliance-finalize-first.json" <<'PY'
import json, pathlib, sys, zipfile
package, finalization = pathlib.Path(sys.argv[1]), pathlib.Path(sys.argv[2])
with zipfile.ZipFile(package) as archive:
    manifest = json.loads(archive.read('manifest.json'))
finalized = json.loads(finalization.read_text())['data']
assert manifest['report_sha256'] == finalized['report_sha256']
PY
  json_log "artifact" "evidence manifest and finalization share canonical report hash" "info" true "ok" "$REPORT_ID" "compliance-finalize-first.json"

  set +e
  psql_exec "UPDATE compliance_finalizations SET finalized_by=finalized_by WHERE tenant_id='$TENANT' AND report_id::text='$REPORT_ID';" >"$LOG_DIR/pg-finalization-mutation.txt" 2>"$LOG_DIR/pg-finalization-mutation.err"
  immutable_rc=$?
  set -e
  if [[ "$immutable_rc" -ne 0 ]] && grep -q "compliance finalizations are immutable" "$LOG_DIR/pg-finalization-mutation.err"; then
    json_log "postgres" "finalization row rejects database mutation" "info" true "immutable" "$REPORT_ID" "pg-finalization-mutation.err"
  else
    json_log "postgres" "finalization row rejects database mutation" "blocker" false "mutable" "rc=$immutable_rc $(trim_file "$LOG_DIR/pg-finalization-mutation.err")" "pg-finalization-mutation.err"
  fi

  curl_json "viewer cannot export report" "POST" "/api/v1/compliance/reports/$REPORT_ID/export" "403" "$VIEWER_TOKEN" "$LOG_DIR/compliance-export-viewer.json" '{"format":"pdf"}' || true
  curl_json "viewer cannot create remediation" "POST" "/api/v1/compliance/reports/$REPORT_ID/remediations" "403" "$VIEWER_TOKEN" "$LOG_DIR/compliance-remediation-viewer.json" '{}' || true
  curl_json "viewer cannot finalize report" "POST" "/api/v1/compliance/reports/$REPORT_ID/finalize" "403" "$VIEWER_TOKEN" "$LOG_DIR/compliance-finalize-viewer.json" '{}' || true
  curl_json "other tenant cannot export report" "POST" "/api/v1/compliance/reports/$REPORT_ID/export" "404" "$OTHER_TOKEN" "$LOG_DIR/compliance-export-other-tenant.json" '{"format":"pdf"}' || true
  curl_json "other tenant cannot create remediation" "POST" "/api/v1/compliance/reports/$REPORT_ID/remediations" "404" "$OTHER_TOKEN" "$LOG_DIR/compliance-remediation-other-tenant.json" '{}' || true
  curl_json "other tenant cannot finalize report" "POST" "/api/v1/compliance/reports/$REPORT_ID/finalize" "404" "$OTHER_TOKEN" "$LOG_DIR/compliance-finalize-other-tenant.json" '{}' || true

  set +e
  psql_exec "SELECT count(*) FROM compliance_reports WHERE tenant_id = '$TENANT' AND report_id::text = '$REPORT_ID' AND report_type = '$REPORT_TYPE';" >"$LOG_DIR/pg-compliance-report-count.txt" 2>"$LOG_DIR/pg-compliance-report-count.err"
  report_count_rc=$?
  psql_exec "SELECT count(*) FROM audit_logs WHERE tenant_id = '$TENANT' AND action = 'COMPLIANCE_REPORT_GENERATED' AND object_type = 'compliance_report' AND object_id = '$REPORT_ID';" >"$LOG_DIR/pg-compliance-audit-count.txt" 2>"$LOG_DIR/pg-compliance-audit-count.err"
  audit_count_rc=$?
  set -e
  report_count="$(tr -d '[:space:]' <"$LOG_DIR/pg-compliance-report-count.txt")"
  audit_count="$(tr -d '[:space:]' <"$LOG_DIR/pg-compliance-audit-count.txt")"
  if [[ "$report_count_rc" -eq 0 && "$report_count" =~ ^[0-9]+$ && "$report_count" -ge 1 ]]; then
    json_log "postgres" "generated compliance report persisted in PostgreSQL" "info" true "ok" "$REPORT_ID" "pg-compliance-report-count.txt"
  else
    json_log "postgres" "generated compliance report persisted in PostgreSQL" "blocker" false "missing" "$(trim_file "$LOG_DIR/pg-compliance-report-count.err")" "pg-compliance-report-count.err"
  fi
  if [[ "$audit_count_rc" -eq 0 && "$audit_count" =~ ^[0-9]+$ && "$audit_count" -ge 1 ]]; then
    json_log "postgres" "generated compliance audit row persisted in PostgreSQL" "info" true "ok" "$REPORT_ID" "pg-compliance-audit-count.txt"
  else
    json_log "postgres" "generated compliance audit row persisted in PostgreSQL" "blocker" false "missing" "$(trim_file "$LOG_DIR/pg-compliance-audit-count.err")" "pg-compliance-audit-count.err"
  fi

  curl_json "other tenant cannot see generated audit row" "GET" "/api/v1/audit/logs?action=COMPLIANCE_REPORT_GENERATED&object_id=$REPORT_ID&limit=10" "200" "$OTHER_TOKEN" "$LOG_DIR/other-tenant-audit-logs-generated.json"
  assert_json "audit log query is tenant isolated" "$LOG_DIR/other-tenant-audit-logs-generated.json" '.success == true and .data.total == 0'
else
  json_log "assert" "generated compliance report id extracted" "blocker" false "missing" "admin generation did not return report_id" "compliance-generate-admin.json"
fi

curl_json "viewer cannot generate compliance report" "POST" "/api/v1/compliance/reports/generate" "403" "$VIEWER_TOKEN" "$LOG_DIR/compliance-generate-viewer.json" "$generate_body" || true
curl_json "unknown report type is rejected" "POST" "/api/v1/compliance/reports/generate" "400" "$ADMIN_TOKEN" "$LOG_DIR/compliance-invalid-report-type.json" '{"report_type":"unknown"}' || true

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
  --arg report_type "$REPORT_TYPE" \
  --arg report_id "${REPORT_ID:-}" \
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
    report_type:$report_type,
    report_id:$report_id,
    report:$report,
    local_report:$local_report,
    total:$total,
    passed:$passed,
    blockers:$blockers,
    warnings:$warnings,
    checks:$checks
  }' >"$SUMMARY"

cat >"$LOCAL_REPORT" <<MD
# Compliance Audit Live Preflight

- Run: \`$RUN_ID\`
- Result: \`$RESULT\`
- Checks: \`$PASSED/$TOTAL\` passed, \`$BLOCKERS\` blockers, \`$WARNINGS\` warnings
- Report type: \`$REPORT_TYPE\`
- Report id: \`${REPORT_ID:-missing}\`

This gate closes the compliance/audit business loop for the audit-config menu:
admin report generation, report query, audit trail query, audit-log page API
query, PostgreSQL persistence, tenant isolation, and viewer write denial.
MD

cp "$SUMMARY" "$REGRESSION_DIR/compliance-audit-preflight-latest.json"
cp "$LOCAL_REPORT" "$REGRESSION_DIR/compliance-audit-preflight-latest.md"
cp "$LOG_DIR/compliance-reports-after.json" "$REGRESSION_DIR/compliance-reports-latest.json" 2>/dev/null || true
cp "$LOG_DIR/compliance-audit-trail-generated.json" "$REGRESSION_DIR/compliance-audit-trail-latest.json" 2>/dev/null || true
cp "$LOG_DIR/audit-logs-generated.json" "$REGRESSION_DIR/audit-log-generated-latest.json" 2>/dev/null || true

echo "compliance audit preflight result: $RESULT"
echo "summary: $SUMMARY"
echo "local report: $LOCAL_REPORT"

if [[ "$RESULT" == "blocked" && "$ALLOW_BLOCKERS" != "true" ]]; then
  exit 1
fi
