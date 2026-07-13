#!/usr/bin/env bash
set -euo pipefail

APISIX="${APISIX:-http://10.0.5.8:30180}"
TENANT="${TENANT:-default}"
OTHER_TENANT="${OTHER_TENANT:-campus-a}"
LOG_DIR="${LOG_DIR:-.artifacts/e2e}"
RUN_ID="${RUN_ID:-$(date +%Y%m%d%H%M%S)-$$}"
KUBECTL="${KUBECTL:-kubectl}"
JWT_SECRET_NAMESPACE="${JWT_SECRET_NAMESPACE:-traffic-analysis}"
JWT_SECRET_NAME="${JWT_SECRET_NAME:-traffic-credentials}"
JWT_SECRET_KEY="${JWT_SECRET_KEY:-JWT_SECRET}"
PG_SECRET_KEY="${PG_SECRET_KEY:-PG_PASSWORD}"
MINIO_ACCESS_KEY_SECRET="${MINIO_ACCESS_KEY_SECRET:-MINIO_ACCESS_KEY}"
MINIO_SECRET_KEY_SECRET="${MINIO_SECRET_KEY_SECRET:-MINIO_SECRET_KEY}"
MINIO_ALIAS="${MINIO_ALIAS:-traffic-minio}"
MINIO_ENDPOINT="${MINIO_ENDPOINT:-http://10.0.5.8:30000}"
PCAP_BUCKET="${PCAP_BUCKET:-pcap-archive}"

REPORT="$LOG_DIR/live-pcap-forensics-integrity-$RUN_ID.ndjson"
SUMMARY="$LOG_DIR/live-pcap-forensics-integrity-$RUN_ID-summary.json"
mkdir -p "$LOG_DIR"

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
  local phase="$1" name="$2" ok="$3" status="$4" detail="${5:-}"
  jq -nc \
    --arg ts "$(date -Iseconds)" \
    --arg phase "$phase" \
    --arg name "$name" \
    --argjson ok "$ok" \
    --arg status "$status" \
    --arg detail "$detail" \
    '{ts:$ts, phase:$phase, name:$name, ok:$ok, status:$status, detail:$detail}' >>"$REPORT"
}

trim_file() {
  local file="$1"
  if [[ -s "$file" ]]; then
    head -c 600 "$file" | tr '\n' ' '
  fi
}

need_cmd curl
need_cmd jq
need_cmd python3
need_cmd sha256sum
need_cmd mc
need_cmd "$KUBECTL"

RUN_SLUG="$(echo "$RUN_ID" | tr -c 'A-Za-z0-9-' '-' | sed 's/-$//')"
KEY="results/$TENANT/2026/06/29/pcap-integrity-$RUN_SLUG.pcap"
PCAP_FILE="$LOG_DIR/pcap-integrity-$RUN_SLUG.pcap"
DOWNLOAD_FILE="$LOG_DIR/pcap-integrity-$RUN_SLUG.download.pcap"
DOWNLOAD_HEADERS="$LOG_DIR/pcap-integrity-$RUN_SLUG.download.headers"
TASK_ID="$(python3 - <<'PY'
import uuid
print(uuid.uuid4())
PY
)"

JWT_SECRET="$(kctl -n "$JWT_SECRET_NAMESPACE" get secret "$JWT_SECRET_NAME" -o "jsonpath={.data.$JWT_SECRET_KEY}" | base64 -d)"
PGPASS="$(kctl -n "$JWT_SECRET_NAMESPACE" get secret "$JWT_SECRET_NAME" -o "jsonpath={.data.$PG_SECRET_KEY}" | base64 -d)"
MINIO_ACCESS="$(kctl -n "$JWT_SECRET_NAMESPACE" get secret "$JWT_SECRET_NAME" -o "jsonpath={.data.$MINIO_ACCESS_KEY_SECRET}" | base64 -d)"
MINIO_SECRET="$(kctl -n "$JWT_SECRET_NAMESPACE" get secret "$JWT_SECRET_NAME" -o "jsonpath={.data.$MINIO_SECRET_KEY_SECRET}" | base64 -d)"

make_token() {
  local username="$1"
  local roles_json="$2"
  local perms_json="$3"
  JWT_SECRET="$JWT_SECRET" TENANT="$TENANT" USERNAME="$username" ROLES_JSON="$roles_json" PERMS_JSON="$perms_json" python3 - <<'PY'
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
    "session_id": "codex-pcap-integrity-" + str(uuid.uuid4()),
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

AUDIT_TOKEN="$(make_token codex-pcap-auditor '["admin"]' '["*","admin:*","audit:read","pcap:read","pcap:download"]')"

python3 - "$PCAP_FILE" <<'PY'
from pathlib import Path
import struct
import sys

data = struct.pack("<IHHIIII", 0xA1B2C3D4, 2, 4, 0, 0, 65535, 1)
Path(sys.argv[1]).write_bytes(data)
PY
SHA="$(sha256sum "$PCAP_FILE" | awk '{print $1}')"

mc alias set "$MINIO_ALIAS" "$MINIO_ENDPOINT" "$MINIO_ACCESS" "$MINIO_SECRET" >/dev/null
mc cp "$PCAP_FILE" "$MINIO_ALIAS/$PCAP_BUCKET/$KEY" >/dev/null
json_log "fixture" "upload deterministic pcap object" true "ok" "$KEY"

kctl -n databases exec postgres-primary-0 -- env PGPASSWORD="$PGPASS" psql -U postgres -d traffic_platform -v ON_ERROR_STOP=1 -Atc \
  "ALTER TABLE tasks ADD COLUMN IF NOT EXISTS result_sha256 TEXT NOT NULL DEFAULT '';
   INSERT INTO tasks (task_id, tenant_id, task_type, params, status, progress, result_file_key, result_sha256, result_packets, result_bytes, files_scanned, created_by, completed_at)
   VALUES ('$TASK_ID', '$TENANT', 'pcap_cut', '{\"source\":\"live_pcap_forensics_integrity\"}'::jsonb, 'completed', 100, '$KEY', '$SHA', 0, 24, 1, 'codex-live-e2e', now())
   ON CONFLICT (task_id) DO UPDATE SET status='completed', progress=100, result_file_key=EXCLUDED.result_file_key, result_sha256=EXCLUDED.result_sha256, result_packets=EXCLUDED.result_packets, result_bytes=EXCLUDED.result_bytes, files_scanned=EXCLUDED.files_scanned, updated_at=now(), completed_at=now();" >/dev/null
json_log "fixture" "upsert completed pcap task" true "ok" "$TASK_ID"

curl_json_check() {
  local name="$1" method="$2" path="$3" expected="$4" data="${5:-}" filter="${6:-}" token="${7:-}"
  local body_file err_file code rc ok detail
  body_file="$LOG_DIR/$RUN_SLUG-${name// /-}.json"
  err_file="$(mktemp)"
  local args=(--noproxy '*' -sS -m 20 -o "$body_file" -w '%{http_code}' -X "$method" -H "X-Tenant-ID: $TENANT")
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
  elif [[ -n "$filter" ]] && ! KEY="$KEY" SHA="$SHA" jq -e "$filter" "$body_file" >/dev/null 2>"$err_file"; then
    ok=false
    detail="jq filter failed filter=$filter body=$(trim_file "$body_file") err=$(trim_file "$err_file")"
  else
    ok=true
    detail="$method $path"
  fi
  json_log "api" "$name" "$ok" "$code" "$detail"
  rm -f "$err_file"
  [[ "$ok" == true ]]
}

curl_json_check "jobs expose sha256" "GET" "/api/v1/pcap/jobs?status=completed&limit=10" "200" "" \
  '.success == true and any(.data[]; .result_file_key == env.KEY and .sha256 == env.SHA)'

curl_json_check "presign caps expiry and exposes sha256" "POST" "/api/v1/pcap/presign" "200" \
  "{\"key\":\"$KEY\",\"expiry_seconds\":999999}" \
  '.success == true and .data.key == env.KEY and .data.sha256 == env.SHA and ((.data.expires_at - now) <= 86410)'

curl_json_check "verify matches registered sha256" "POST" "/api/v1/pcap/verify" "200" \
  "{\"key\":\"$KEY\",\"expected_sha256\":\"$SHA\"}" \
  '.success == true and .data.verified == true and .data.sha256 == env.SHA and .data.registered_sha256 == env.SHA and .data.size_bytes == 24'

curl_json_check "presign rejects path traversal" "POST" "/api/v1/pcap/presign" "400" \
  '{"key":"../etc/passwd"}' ''

body_file="$LOG_DIR/$RUN_SLUG-cross-tenant-presign.json"
code="$(curl --noproxy '*' -sS -m 20 -o "$body_file" -w '%{http_code}' -H "X-Tenant-ID: $OTHER_TENANT" -H "Content-Type: application/json" --data "{\"key\":\"$KEY\"}" "$APISIX/api/v1/pcap/presign")"
if [[ "$code" == "403" ]]; then
  json_log "api" "presign rejects cross tenant" true "$code" "$OTHER_TENANT -> $KEY"
else
  json_log "api" "presign rejects cross tenant" false "$code" "body=$(trim_file "$body_file")"
  exit 1
fi

curl --noproxy '*' -sS -m 20 -D "$DOWNLOAD_HEADERS" -H "X-Tenant-ID: $TENANT" -H "X-User-ID: 00000000-0000-4000-8000-000000000001" "$APISIX/api/v1/pcap/download/$KEY" -o "$DOWNLOAD_FILE"
DOWN_SHA="$(sha256sum "$DOWNLOAD_FILE" | awk '{print $1}')"
HEADER_SHA="$(awk 'BEGIN{IGNORECASE=1} /^X-Content-SHA256:/ {gsub("\r", "", $2); print $2}' "$DOWNLOAD_HEADERS")"
if [[ "$DOWN_SHA" == "$SHA" && "$HEADER_SHA" == "$SHA" ]]; then
  json_log "api" "download body sha matches header and expected" true "200" "$HEADER_SHA"
else
  json_log "api" "download body sha matches header and expected" false "200" "expected=$SHA header=$HEADER_SHA body=$DOWN_SHA"
  exit 1
fi

audit_file="$LOG_DIR/$RUN_SLUG-audit.json"
curl --noproxy '*' -sS -m 20 -H "Authorization: Bearer $AUDIT_TOKEN" -H "X-Tenant-ID: $TENANT" "$APISIX/api/v1/audit/logs?object_type=pcap&limit=20" >"$audit_file"
if jq -e --arg key "$KEY" '.success == true and ([.data.trails[] | select(.resource_id == $key) | .details.mode] | index("presign") and index("integrity_verify") and index("download"))' "$audit_file" >/dev/null; then
  json_log "audit" "pcap access audit queryable" true "200" "$audit_file"
else
  json_log "audit" "pcap access audit queryable" false "200" "body=$(trim_file "$audit_file")"
  exit 1
fi

jq -s \
  --arg run_id "$RUN_ID" \
  --arg tenant "$TENANT" \
  --arg other_tenant "$OTHER_TENANT" \
  --arg key "$KEY" \
  --arg task_id "$TASK_ID" \
  --arg sha256 "$SHA" \
  --arg report "$REPORT" \
  '{
    run_id: $run_id,
    tenant: $tenant,
    other_tenant: $other_tenant,
    key: $key,
    task_id: $task_id,
    sha256: $sha256,
    checks_total: length,
    checks_passed: map(select(.ok == true)) | length,
    checks_failed: map(select(.ok != true)) | length,
    report: $report,
    checks: .
  }' "$REPORT" >"$SUMMARY"

cat "$SUMMARY"
