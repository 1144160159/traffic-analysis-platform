#!/usr/bin/env bash
set -euo pipefail

APISIX="${APISIX:-http://10.0.5.8:30180}"
TENANT="${TENANT:-default}"
LOG_DIR="${LOG_DIR:-doc/02_acceptance/runs/20260629-threat-intel-service}"
RUN_ID="${RUN_ID:-$(date +%Y%m%d%H%M%S)-threat-intel-service}"
REGRESSION_DIR="${REGRESSION_DIR:-doc/02_acceptance/02-regression}"
KUBECTL="${KUBECTL:-kubectl}"
JWT_SECRET_NAMESPACE="${JWT_SECRET_NAMESPACE:-traffic-analysis}"
JWT_SECRET_NAME="${JWT_SECRET_NAME:-traffic-credentials}"
JWT_SECRET_KEY="${JWT_SECRET_KEY:-JWT_SECRET}"
PG_NAMESPACE="${PG_NAMESPACE:-databases}"
PG_POD="${PG_POD:-postgres-primary-0}"
PG_SECRET_NAMESPACE="${PG_SECRET_NAMESPACE:-traffic-analysis}"
PG_SECRET_NAME="${PG_SECRET_NAME:-traffic-credentials}"
PG_SECRET_KEY="${PG_SECRET_KEY:-PG_PASSWORD}"
KAFKA_NAMESPACE="${KAFKA_NAMESPACE:-middleware}"
KAFKA_POD="${KAFKA_POD:-kafka-0}"
KAFKA_TOPIC="${KAFKA_TOPIC:-threat.intel.v1}"

REPORT="$LOG_DIR/live-threat-intel-service-$RUN_ID.ndjson"
SUMMARY="$LOG_DIR/live-threat-intel-service-$RUN_ID-summary.json"
LOCAL_REPORT="$LOG_DIR/local-report.md"

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
    head -c 1000 "$file" | tr '\n' ' '
  fi
}

check_file() {
  local name="$1" path="$2"
  if [[ -e "$path" ]]; then
    json_log "contract" "$name" "info" true "ok" "$path" "$path"
  else
    json_log "contract" "$name" "blocker" false "missing" "$path" "$path"
  fi
}

check_grep() {
  local name="$1" pattern="$2" path="$3"
  if grep -qE "$pattern" "$path"; then
    json_log "contract" "$name" "info" true "ok" "$path" "$path"
  else
    json_log "contract" "$name" "blocker" false "missing" "$pattern in $path" "$path"
  fi
}

curl_json() {
  local name="$1" method="$2" path="$3" output="$4" body_file="${5:-}"
  local err_file code rc
  err_file="$output.err"
  set +e
  if [[ "$method" == "POST" ]]; then
    code="$(curl --noproxy '*' -sS -m 20 -o "$output" -w '%{http_code}' \
      -X POST \
      -H 'Content-Type: application/json' \
      -H "Authorization: Bearer $ADMIN_TOKEN" \
      -H "X-Tenant-ID: $TENANT" \
      --data-binary "@$body_file" \
      "$APISIX$path" 2>"$err_file")"
    rc=$?
  else
    code="$(curl --noproxy '*' -sS -m 20 -o "$output" -w '%{http_code}' \
      -H "Authorization: Bearer $ADMIN_TOKEN" \
      -H "X-Tenant-ID: $TENANT" \
      "$APISIX$path" 2>"$err_file")"
    rc=$?
  fi
  set -e
  if [[ "$rc" -ne 0 ]]; then
    json_log "api" "$name" "blocker" false "curl-rc=$rc" "$(trim_file "$err_file")" "$(basename "$err_file")"
    return 1
  fi
  if [[ "$code" != 2* ]]; then
    json_log "api" "$name" "blocker" false "$code" "$(trim_file "$output")" "$(basename "$output")"
    return 1
  fi
  json_log "api" "$name" "info" true "$code" "$path" "$(basename "$output")"
  return 0
}

curl_status() {
  local name="$1" method="$2" path="$3" expected="$4" token="$5" output="$6" body_file="${7:-}"
  local err_file code rc
  err_file="$output.err"
  local args=(--noproxy '*' -sS -m 20 -o "$output" -w '%{http_code}')
  if [[ "$method" == "POST" ]]; then
    args+=(-X POST -H 'Content-Type: application/json' --data-binary "@$body_file")
  fi
  if [[ -n "$token" ]]; then
    args+=(-H "Authorization: Bearer $token" -H "X-Tenant-ID: $TENANT")
  fi
  args+=("$APISIX$path")
  set +e
  code="$(curl "${args[@]}" 2>"$err_file")"
  rc=$?
  set -e
  if [[ "$rc" -ne 0 ]]; then
    json_log "auth" "$name" "blocker" false "curl-rc=$rc" "$(trim_file "$err_file")" "$(basename "$err_file")"
    return 1
  fi
  if [[ "$code" == "$expected" ]]; then
    json_log "auth" "$name" "info" true "$code" "$path" "$(basename "$output")"
    return 0
  fi
  json_log "auth" "$name" "blocker" false "$code" "$(trim_file "$output")" "$(basename "$output")"
  return 1
}

psql_exec() {
  local sql="$1"
  kctl -n "$PG_NAMESPACE" exec "$PG_POD" -- env PGPASSWORD="$PG_PASSWORD" \
    psql -U postgres -d traffic_platform -v ON_ERROR_STOP=1 -Atc "$sql"
}

kafka_dump_topic() {
  local output="$1"
  local err_file="$output.err"
  set +e
  kctl -n "$KAFKA_NAMESPACE" exec "$KAFKA_POD" -- /opt/kafka/bin/kafka-console-consumer.sh \
    --bootstrap-server localhost:9092 \
    --topic "$KAFKA_TOPIC" \
    --from-beginning \
    --timeout-ms 15000 \
    >"$output" 2>"$err_file"
  local rc=$?
  set -e
  if [[ "$rc" -eq 0 || -s "$output" ]]; then
    return 0
  fi
  return "$rc"
}

assert_json() {
  local name="$1" file="$2"
  shift 2
  if jq -e "$@" "$file" >/dev/null 2>&1; then
    json_log "assert" "$name" "info" true "ok" "" "$(basename "$file")"
  else
    json_log "assert" "$name" "blocker" false "failed" "$(trim_file "$file")" "$(basename "$file")"
  fi
}

make_jwt() {
  local username="$1" roles_json="$2" permissions_json="$3"
  local tenant_id="${4:-$TENANT}"
  local expires_in="${5:-1800}"
  JWT_SECRET="$JWT_SECRET" TENANT="$tenant_id" RUN_ID="$RUN_ID" USERNAME="$username" ROLES_JSON="$roles_json" PERMISSIONS_JSON="$permissions_json" EXPIRES_IN="$expires_in" python3 - <<'PY'
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
    "email": f'{os.environ["USERNAME"]}@local',
    "roles": json.loads(os.environ["ROLES_JSON"]),
    "permissions": json.loads(os.environ["PERMISSIONS_JSON"]),
    "token_type": "access",
    "session_id": "codex-threat-intel-" + os.environ["RUN_ID"] + "-" + os.environ["USERNAME"],
    "iat": now,
    "exp": now + int(os.environ["EXPIRES_IN"]),
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

need_cmd git
need_cmd jq
need_cmd python3
need_cmd curl
need_cmd grep
need_cmd "$KUBECTL"

git rev-parse HEAD >"$LOG_DIR/commit-sha.txt"
git branch --show-current >"$LOG_DIR/git-branch.txt"
git status --short >"$LOG_DIR/git-status.txt"
git diff --stat >"$LOG_DIR/git-diff-stat.txt" || true

JWT_SECRET="$(kctl -n "$JWT_SECRET_NAMESPACE" get secret "$JWT_SECRET_NAME" -o "jsonpath={.data.$JWT_SECRET_KEY}" | base64 -d)"
PG_PASSWORD="$(kctl -n "$PG_SECRET_NAMESPACE" get secret "$PG_SECRET_NAME" -o "jsonpath={.data.$PG_SECRET_KEY}" | base64 -d)"
OTHER_TENANT="${OTHER_TENANT:-codex-threat-intel-other}"
ADMIN_TOKEN="$(make_jwt "codex-threat-intel-admin" '["admin"]' '["*","admin:*","alert:read","alert:write"]')"
READER_TOKEN="$(make_jwt "codex-threat-intel-reader" '["viewer"]' '["alert:read"]')"
OTHER_ADMIN_TOKEN="$(make_jwt "codex-threat-intel-other-admin" '["admin"]' '["*","admin:*","alert:read","alert:write"]' "$OTHER_TENANT")"
EXPIRED_ADMIN_TOKEN="$(make_jwt "codex-threat-intel-expired-admin" '["admin"]' '["*","admin:*","alert:read","alert:write"]' "$TENANT" "-60")"
json_log "auth" "short-lived JWT tokens generated" "info" true "ok" "admin and read-only smoke tokens" ""

check_file "threat intel service entrypoint exists" "go/control-plane/cmd/threat-intel-service/main.go"
check_file "threat intel reusable module exists" "go/control-plane/internal/alert/threatintel/threat_intel.go"
check_grep "threat intel service publishes Kafka events" 'publishThreatIntelEvent' "go/control-plane/cmd/threat-intel-service/main.go"
check_grep "threat intel service writes audit_logs" 'THREAT_INTEL_FEED_IMPORTED' "go/control-plane/cmd/threat-intel-service/main.go"
check_grep "threat intel feed scheduler exists" 'startFeedScheduler' "go/control-plane/cmd/threat-intel-service/main.go"
check_grep "threat intel feed source schema exists" 'threat_intel_feeds' "go/control-plane/internal/alert/threatintel/threat_intel.go"
check_grep "Kafka topic contract includes threat.intel.v1" 'threat\.intel\.v1:3:' "common/kafka/create-topics.sh"
check_grep "K8s manifest deploys threat-intel-service" 'name: threat-intel-service' "deployments/kubernetes/applications/go-services.yaml"
check_grep "APISIX routes threat intel API" '/api/v1/threat-intel\*' "deployments/kubernetes/configmaps/apisix-routes.yaml"

DEPLOY_JSON="$LOG_DIR/k8s-threat-intel-deployment.json"
if kctl -n traffic-analysis get deploy threat-intel-service -o json >"$DEPLOY_JSON" 2>"$DEPLOY_JSON.err"; then
  READY="$(jq -r '.status.readyReplicas // 0' "$DEPLOY_JSON")"
  DESIRED="$(jq -r '.spec.replicas // 1' "$DEPLOY_JSON")"
  IMAGE="$(jq -r '.spec.template.spec.containers[] | select(.name=="threat-intel-service") | .image' "$DEPLOY_JSON")"
  if [[ "$READY" -ge 1 && "$READY" -eq "$DESIRED" ]]; then
    json_log "k8s" "threat-intel-service deployment ready" "info" true "ready=$READY/$DESIRED" "$IMAGE" "$(basename "$DEPLOY_JSON")"
  else
    json_log "k8s" "threat-intel-service deployment ready" "blocker" false "ready=$READY/$DESIRED" "$IMAGE" "$(basename "$DEPLOY_JSON")"
  fi
else
  json_log "k8s" "threat-intel-service deployment readable" "blocker" false "kubectl-failed" "$(trim_file "$DEPLOY_JSON.err")" "$(basename "$DEPLOY_JSON.err")"
fi

PODS_TXT="$LOG_DIR/k8s-threat-intel-pods.txt"
kctl -n traffic-analysis get pods -l app=threat-intel-service -o wide >"$PODS_TXT" 2>"$PODS_TXT.err" || true

LOGS_TXT="$LOG_DIR/k8s-threat-intel-logs-tail.txt"
kctl -n traffic-analysis logs deploy/threat-intel-service --tail=100 >"$LOGS_TXT" 2>"$LOGS_TXT.err" || true
if [[ -s "$LOGS_TXT" ]] && ! grep -qiE 'panic|fatal|failed' "$LOGS_TXT"; then
  json_log "k8s" "threat-intel-service recent logs clean" "info" true "ok" "" "$(basename "$LOGS_TXT")"
else
  json_log "k8s" "threat-intel-service recent logs clean" "warning" false "inspect" "$(trim_file "$LOGS_TXT")" "$(basename "$LOGS_TXT")"
fi

TOPIC_TXT="$LOG_DIR/kafka-threat-intel-topic.txt"
if kctl -n "$KAFKA_NAMESPACE" exec "$KAFKA_POD" -- /opt/kafka/bin/kafka-topics.sh \
  --bootstrap-server localhost:9092 \
  --describe --topic "$KAFKA_TOPIC" >"$TOPIC_TXT" 2>"$TOPIC_TXT.err"; then
  if grep -q 'Topic: threat.intel.v1' "$TOPIC_TXT" && grep -q 'PartitionCount: 3' "$TOPIC_TXT"; then
    json_log "kafka" "threat.intel.v1 topic healthy" "info" true "partition_count=3" "" "$(basename "$TOPIC_TXT")"
  else
    json_log "kafka" "threat.intel.v1 topic healthy" "blocker" false "unexpected-describe" "$(trim_file "$TOPIC_TXT")" "$(basename "$TOPIC_TXT")"
  fi
else
  json_log "kafka" "threat.intel.v1 topic healthy" "blocker" false "describe-failed" "$(trim_file "$TOPIC_TXT.err")" "$(basename "$TOPIC_TXT.err")"
fi

NOAUTH_JSON="$LOG_DIR/api-lookup-noauth.json"
curl_status "unauthenticated lookup is rejected" "GET" "/api/v1/threat-intel/lookup?type=ip&value=185.130.5.253" "401" "" "$NOAUTH_JSON" || true

EXPIRED_JSON="$LOG_DIR/api-lookup-expired-token.json"
curl_status "expired token is rejected" "GET" "/api/v1/threat-intel/lookup?type=ip&value=185.130.5.253" "401" "$EXPIRED_ADMIN_TOKEN" "$EXPIRED_JSON" || true

READER_LOOKUP_JSON="$LOG_DIR/api-lookup-reader-c2.json"
if curl_status "read-only token can lookup threat intel" "GET" "/api/v1/threat-intel/lookup?type=ip&value=185.130.5.253" "200" "$READER_TOKEN" "$READER_LOOKUP_JSON"; then
  assert_json "read-only lookup returns c2 reputation" "$READER_LOOKUP_JSON" '.success == true and .data.found == true and .data.reputation == "c2"'
fi

BUILTIN_JSON="$LOG_DIR/api-lookup-builtin-c2.json"
if curl_json "lookup builtin C2 IP through APISIX" "GET" "/api/v1/threat-intel/lookup?type=ip&value=185.130.5.253" "$BUILTIN_JSON"; then
  assert_json "builtin lookup returns c2 reputation" "$BUILTIN_JSON" '.success == true and .data.found == true and .data.reputation == "c2" and .data.entry.source == "builtin"'
fi

RUN_ID_SAFE="$(tr '[:upper:]_' '[:lower:]-' <<<"$RUN_ID" | sed -E 's/[^a-z0-9-]+/-/g; s/^-+|-+$//g')"
SMOKE_VALUE="${SMOKE_VALUE:-codex-threat-intel-${RUN_ID_SAFE}.example}"
ENTRY_BODY="$LOG_DIR/api-upsert-entry-body.json"
jq -nc \
  --arg value "$SMOKE_VALUE" \
  '{type:"domain", value:$value, reputation:"malicious", category:"test", source:"codex-live-smoke", description:"Codex live smoke indicator"}' >"$ENTRY_BODY"

READER_WRITE_JSON="$LOG_DIR/api-upsert-reader-denied.json"
curl_status "read-only token cannot upsert threat intel" "POST" "/api/v1/threat-intel/entries" "403" "$READER_TOKEN" "$READER_WRITE_JSON" "$ENTRY_BODY" || true

UPSERT_JSON="$LOG_DIR/api-upsert-entry.json"
if curl_json "upsert smoke threat intel entry" "POST" "/api/v1/threat-intel/entries" "$UPSERT_JSON" "$ENTRY_BODY"; then
  assert_json "upsert echoes smoke entry and publishing evidence" "$UPSERT_JSON" --arg value "$SMOKE_VALUE" '.success == true and .data.entry.value == $value and .data.entry.reputation == "malicious" and .data.audit_written == true and .data.kafka_published == true and .data.kafka_topic == "threat.intel.v1" and (.data.event_id | startswith("ti-"))'
fi
UPSERT_EVENT_ID="$(jq -r '.data.event_id // ""' "$UPSERT_JSON" 2>/dev/null || true)"

LOOKUP_JSON="$LOG_DIR/api-lookup-smoke-entry.json"
if curl_json "lookup persisted smoke entry" "GET" "/api/v1/threat-intel/lookup?type=domain&value=$SMOKE_VALUE" "$LOOKUP_JSON"; then
  assert_json "persisted smoke entry is found" "$LOOKUP_JSON" --arg value "$SMOKE_VALUE" '.success == true and .data.found == true and .data.entry.value == $value and .data.reputation == "malicious"'
fi

LIST_JSON="$LOG_DIR/api-list-smoke-source.json"
if curl_json "list codex-live-smoke entries" "GET" "/api/v1/threat-intel/entries?source=codex-live-smoke&limit=20" "$LIST_JSON"; then
  assert_json "list returns smoke entry" "$LIST_JSON" --arg value "$SMOKE_VALUE" '.success == true and any(.data[]?; .value == $value)'
fi

OTHER_VALUE="codex-threat-intel-other-${RUN_ID_SAFE}.example"
OTHER_ENTRY_BODY="$LOG_DIR/api-upsert-other-tenant-body.json"
jq -nc \
  --arg value "$OTHER_VALUE" \
  '{type:"domain", value:$value, reputation:"malicious", category:"cross-tenant", source:"codex-live-cross-tenant", description:"Codex cross tenant isolation indicator"}' >"$OTHER_ENTRY_BODY"

OTHER_UPSERT_JSON="$LOG_DIR/api-upsert-other-tenant.json"
if curl_status "other tenant can upsert isolated threat intel entry" "POST" "/api/v1/threat-intel/entries" "201" "$OTHER_ADMIN_TOKEN" "$OTHER_UPSERT_JSON" "$OTHER_ENTRY_BODY"; then
  assert_json "other tenant upsert records tenant id" "$OTHER_UPSERT_JSON" --arg value "$OTHER_VALUE" --arg tenant "$OTHER_TENANT" '.success == true and .data.entry.value == $value and .data.entry.tenant_id == $tenant and .data.audit_written == true and .data.kafka_published == true'
fi

CROSS_LOOKUP_JSON="$LOG_DIR/api-lookup-cross-tenant-default.json"
if curl_json "default tenant cannot lookup other tenant threat intel entry" "GET" "/api/v1/threat-intel/lookup?type=domain&value=$OTHER_VALUE" "$CROSS_LOOKUP_JSON"; then
  assert_json "cross tenant lookup is not found" "$CROSS_LOOKUP_JSON" '.success == true and .data.found == false and .data.reputation == "unknown"'
fi

CROSS_LIST_JSON="$LOG_DIR/api-list-cross-tenant-default.json"
if curl_json "default tenant list excludes other tenant source" "GET" "/api/v1/threat-intel/entries?source=codex-live-cross-tenant&limit=20" "$CROSS_LIST_JSON"; then
  assert_json "cross tenant source list is empty" "$CROSS_LIST_JSON" --arg value "$OTHER_VALUE" '.success == true and ([.data[]? | select(.value == $value)] | length) == 0'
fi

IMPORT_VALUE="codex-threat-intel-import-${RUN_ID_SAFE}.example"
IMPORT_BODY="$LOG_DIR/api-import-body.json"
jq -nc \
  --arg value "$IMPORT_VALUE" \
  '{source:"codex-live-import", entries:[{type:"domain", value:$value, reputation:"suspicious", category:"test", description:"Codex live import indicator"}]}' >"$IMPORT_BODY"

IMPORT_JSON="$LOG_DIR/api-import.json"
if curl_json "import threat intel feed entry" "POST" "/api/v1/threat-intel/import" "$IMPORT_JSON" "$IMPORT_BODY"; then
  assert_json "import reports one entry and publishing evidence" "$IMPORT_JSON" '.success == true and .data.imported == 1 and .data.source == "codex-live-import" and .data.audit_written == true and .data.kafka_published == true and .data.kafka_topic == "threat.intel.v1" and (.data.event_id | startswith("ti-"))'
fi
IMPORT_EVENT_ID="$(jq -r '.data.event_id // ""' "$IMPORT_JSON" 2>/dev/null || true)"

SCHEDULED_SOURCE="codex-live-scheduled"
SCHEDULED_VALUE="codex-threat-intel-scheduled-${RUN_ID_SAFE}.example"
SCHEDULED_STARTED_AT="$(date -u +%Y-%m-%dT%H:%M:%SZ)"
FEED_BODY="$LOG_DIR/api-feed-upsert-body.json"
jq -nc \
  --arg source "$SCHEDULED_SOURCE" \
  --arg value "$SCHEDULED_VALUE" \
  '{name:$source, enabled:true, interval_seconds:30, entries:[{type:"domain", value:$value, reputation:"malicious", category:"scheduled", description:"Codex scheduled feed indicator"}]}' >"$FEED_BODY"

FEED_UPSERT_JSON="$LOG_DIR/api-feed-upsert.json"
if curl_json "upsert scheduled threat intel feed" "POST" "/api/v1/threat-intel/feeds" "$FEED_UPSERT_JSON" "$FEED_BODY"; then
  assert_json "scheduled feed is enabled" "$FEED_UPSERT_JSON" --arg source "$SCHEDULED_SOURCE" '.success == true and .data.name == $source and .data.enabled == true and .data.interval_seconds == 30'
fi

SCHEDULED_LOOKUP_JSON="$LOG_DIR/api-scheduled-feed-lookup.json"
SCHEDULED_ENTRY_FOUND=false
for _ in $(seq 1 30); do
  if curl --noproxy '*' -sS -m 10 -o "$SCHEDULED_LOOKUP_JSON" \
    -H "Authorization: Bearer $ADMIN_TOKEN" \
    -H "X-Tenant-ID: $TENANT" \
    "$APISIX/api/v1/threat-intel/lookup?type=domain&value=$SCHEDULED_VALUE" 2>"$SCHEDULED_LOOKUP_JSON.err"; then
    if jq -e --arg value "$SCHEDULED_VALUE" '.success == true and .data.found == true and .data.entry.value == $value and .data.entry.source == "codex-live-scheduled"' "$SCHEDULED_LOOKUP_JSON" >/dev/null 2>&1; then
      SCHEDULED_ENTRY_FOUND=true
      break
    fi
  fi
  sleep 2
done
if [[ "$SCHEDULED_ENTRY_FOUND" == true ]]; then
  json_log "api" "scheduled feed imports entry" "info" true "ok" "$SCHEDULED_VALUE" "$(basename "$SCHEDULED_LOOKUP_JSON")"
else
  json_log "api" "scheduled feed imports entry" "blocker" false "not-found-after-poll" "$(trim_file "$SCHEDULED_LOOKUP_JSON")" "$(basename "$SCHEDULED_LOOKUP_JSON")"
fi

FEED_LIST_JSON="$LOG_DIR/api-feed-list-scheduled.json"
if curl_json "list scheduled threat intel feed" "GET" "/api/v1/threat-intel/feeds?name=$SCHEDULED_SOURCE" "$FEED_LIST_JSON"; then
  assert_json "scheduled feed status records success run" "$FEED_LIST_JSON" --arg source "$SCHEDULED_SOURCE" '.success == true and (.data | length) == 1 and .data[0].name == $source and .data[0].last_status == "success" and .data[0].run_count >= 1'
fi

AUDIT_UPSERT_TXT="$LOG_DIR/pg-audit-upsert-count.txt"
if [[ -n "$UPSERT_EVENT_ID" ]]; then
  if psql_exec "SELECT count(*) FROM audit_logs WHERE tenant_id = '$TENANT' AND action = 'THREAT_INTEL_ENTRY_UPSERTED' AND object_type = 'threat_intel' AND detail->>'event_id' = '$UPSERT_EVENT_ID';" >"$AUDIT_UPSERT_TXT" 2>"$AUDIT_UPSERT_TXT.err"; then
    if [[ "$(tr -d '[:space:]' <"$AUDIT_UPSERT_TXT")" != "0" ]]; then
      json_log "audit" "upsert writes audit_logs row" "info" true "ok" "$UPSERT_EVENT_ID" "$(basename "$AUDIT_UPSERT_TXT")"
    else
      json_log "audit" "upsert writes audit_logs row" "blocker" false "missing" "$UPSERT_EVENT_ID" "$(basename "$AUDIT_UPSERT_TXT")"
    fi
  else
    json_log "audit" "upsert writes audit_logs row" "blocker" false "psql-failed" "$(trim_file "$AUDIT_UPSERT_TXT.err")" "$(basename "$AUDIT_UPSERT_TXT.err")"
  fi
else
  json_log "audit" "upsert writes audit_logs row" "blocker" false "missing-event-id" "" "$(basename "$UPSERT_JSON")"
fi

AUDIT_IMPORT_TXT="$LOG_DIR/pg-audit-import-count.txt"
if [[ -n "$IMPORT_EVENT_ID" ]]; then
  if psql_exec "SELECT count(*) FROM audit_logs WHERE tenant_id = '$TENANT' AND action = 'THREAT_INTEL_FEED_IMPORTED' AND object_type = 'threat_intel' AND detail->>'event_id' = '$IMPORT_EVENT_ID';" >"$AUDIT_IMPORT_TXT" 2>"$AUDIT_IMPORT_TXT.err"; then
    if [[ "$(tr -d '[:space:]' <"$AUDIT_IMPORT_TXT")" != "0" ]]; then
      json_log "audit" "import writes audit_logs row" "info" true "ok" "$IMPORT_EVENT_ID" "$(basename "$AUDIT_IMPORT_TXT")"
    else
      json_log "audit" "import writes audit_logs row" "blocker" false "missing" "$IMPORT_EVENT_ID" "$(basename "$AUDIT_IMPORT_TXT")"
    fi
  else
    json_log "audit" "import writes audit_logs row" "blocker" false "psql-failed" "$(trim_file "$AUDIT_IMPORT_TXT.err")" "$(basename "$AUDIT_IMPORT_TXT.err")"
  fi
else
  json_log "audit" "import writes audit_logs row" "blocker" false "missing-event-id" "" "$(basename "$IMPORT_JSON")"
fi

AUDIT_SCHEDULED_TXT="$LOG_DIR/pg-audit-scheduled-event-id.txt"
SCHEDULED_EVENT_ID=""
if psql_exec "SELECT detail->>'event_id' FROM audit_logs WHERE tenant_id = '$TENANT' AND action = 'THREAT_INTEL_FEED_SCHEDULED_IMPORT' AND object_type = 'threat_intel' AND object_id = '$SCHEDULED_SOURCE' AND created_at >= '$SCHEDULED_STARTED_AT'::timestamptz ORDER BY created_at DESC LIMIT 1;" >"$AUDIT_SCHEDULED_TXT" 2>"$AUDIT_SCHEDULED_TXT.err"; then
  SCHEDULED_EVENT_ID="$(tr -d '[:space:]' <"$AUDIT_SCHEDULED_TXT")"
  if [[ "$SCHEDULED_EVENT_ID" == ti-* ]]; then
    json_log "audit" "scheduled feed writes audit_logs row" "info" true "ok" "$SCHEDULED_EVENT_ID" "$(basename "$AUDIT_SCHEDULED_TXT")"
  else
    json_log "audit" "scheduled feed writes audit_logs row" "blocker" false "missing" "$SCHEDULED_SOURCE" "$(basename "$AUDIT_SCHEDULED_TXT")"
  fi
else
  json_log "audit" "scheduled feed writes audit_logs row" "blocker" false "psql-failed" "$(trim_file "$AUDIT_SCHEDULED_TXT.err")" "$(basename "$AUDIT_SCHEDULED_TXT.err")"
fi

KAFKA_EVENTS_TXT="$LOG_DIR/kafka-threat-intel-events.txt"
if kafka_dump_topic "$KAFKA_EVENTS_TXT"; then
  if [[ -n "$UPSERT_EVENT_ID" ]] && grep -q "$UPSERT_EVENT_ID" "$KAFKA_EVENTS_TXT" && grep -q 'threat_intel.entry_upserted' "$KAFKA_EVENTS_TXT"; then
    json_log "kafka" "upsert event consumed from threat.intel.v1" "info" true "ok" "$UPSERT_EVENT_ID" "$(basename "$KAFKA_EVENTS_TXT")"
  else
    json_log "kafka" "upsert event consumed from threat.intel.v1" "blocker" false "missing" "$UPSERT_EVENT_ID" "$(basename "$KAFKA_EVENTS_TXT")"
  fi
  if [[ -n "$IMPORT_EVENT_ID" ]] && grep -q "$IMPORT_EVENT_ID" "$KAFKA_EVENTS_TXT" && grep -q 'threat_intel.feed_imported' "$KAFKA_EVENTS_TXT"; then
    json_log "kafka" "import event consumed from threat.intel.v1" "info" true "ok" "$IMPORT_EVENT_ID" "$(basename "$KAFKA_EVENTS_TXT")"
  else
    json_log "kafka" "import event consumed from threat.intel.v1" "blocker" false "missing" "$IMPORT_EVENT_ID" "$(basename "$KAFKA_EVENTS_TXT")"
  fi
  if [[ -n "$SCHEDULED_EVENT_ID" ]] && grep -q "$SCHEDULED_EVENT_ID" "$KAFKA_EVENTS_TXT" && grep -q 'threat_intel.feed_scheduled_imported' "$KAFKA_EVENTS_TXT"; then
    json_log "kafka" "scheduled feed event consumed from threat.intel.v1" "info" true "ok" "$SCHEDULED_EVENT_ID" "$(basename "$KAFKA_EVENTS_TXT")"
  else
    json_log "kafka" "scheduled feed event consumed from threat.intel.v1" "blocker" false "missing" "$SCHEDULED_EVENT_ID" "$(basename "$KAFKA_EVENTS_TXT")"
  fi
else
  json_log "kafka" "threat.intel.v1 events consumed" "blocker" false "consumer-failed" "$(trim_file "$KAFKA_EVENTS_TXT.err")" "$(basename "$KAFKA_EVENTS_TXT.err")"
fi

ENRICH_JSON="$LOG_DIR/api-enrich-builtin-c2.json"
if curl_json "enrich alert tuple with threat intel" "GET" "/api/v1/threat-intel/enrich?src_ip=185.130.5.253&dst_ip=10.0.0.1" "$ENRICH_JSON"; then
  assert_json "enrich adds C2 tag and risk" "$ENRICH_JSON" '.success == true and .data.ips["185.130.5.253"] == "c2" and (.data.tags | index("threat_intel:src_c2")) and .data.risk_score > 0'
fi

jq -s \
  --arg run_id "$RUN_ID" \
  --arg apisix "$APISIX" \
  --arg report "$REPORT" \
  --arg smoke_value "$SMOKE_VALUE" \
  --arg import_value "$IMPORT_VALUE" \
  --arg scheduled_value "$SCHEDULED_VALUE" \
  --arg other_value "$OTHER_VALUE" \
  '{
    run_id: $run_id,
    generated_at: now | todateiso8601,
    apisix: $apisix,
    result: (if ([.[] | select(.severity == "blocker" and .passed == false)] | length) == 0 then "pass" else "blocked" end),
    side_effects: {
      postgres_rows_upserted: [$smoke_value, $import_value, $scheduled_value, $other_value],
      source_tags: ["codex-live-smoke", "codex-live-import", "codex-live-scheduled", "codex-live-cross-tenant"]
    },
    totals: {
      total: length,
      passed: ([.[] | select(.passed == true)] | length),
      failed: ([.[] | select(.passed == false)] | length),
      blockers: ([.[] | select(.severity == "blocker" and .passed == false)] | length),
      warnings: ([.[] | select(.severity == "warning" and .passed == false)] | length)
    },
    artifacts: {
      ndjson: $report,
      k8s_deployment: "k8s-threat-intel-deployment.json",
      kafka_topic: "kafka-threat-intel-topic.txt",
      kafka_events: "kafka-threat-intel-events.txt",
      audit_upsert_count: "pg-audit-upsert-count.txt",
      audit_import_count: "pg-audit-import-count.txt",
      audit_scheduled_event_id: "pg-audit-scheduled-event-id.txt",
      scheduled_feed: "api-feed-list-scheduled.json",
      cross_tenant_lookup: "api-lookup-cross-tenant-default.json",
      expired_token_lookup: "api-lookup-expired-token.json",
      builtin_lookup: "api-lookup-builtin-c2.json",
      smoke_lookup: "api-lookup-smoke-entry.json",
      scheduled_lookup: "api-scheduled-feed-lookup.json",
      enrichment: "api-enrich-builtin-c2.json"
    },
    checks: .
  }' "$REPORT" >"$SUMMARY"

RESULT="$(jq -r '.result' "$SUMMARY")"
TOTAL="$(jq -r '.totals.total' "$SUMMARY")"
PASSED="$(jq -r '.totals.passed' "$SUMMARY")"
BLOCKERS="$(jq -r '.totals.blockers' "$SUMMARY")"
WARNINGS="$(jq -r '.totals.warnings' "$SUMMARY")"

{
  echo "# Threat Intel Service Live Preflight Report"
  echo
  echo "- Run ID: \`$RUN_ID\`"
  echo "- Result: \`$RESULT\`"
  echo "- APISIX: \`$APISIX\`"
  echo "- Checks: $PASSED/$TOTAL passed, blockers=$BLOCKERS, warnings=$WARNINGS"
  echo "- Side effect: writes/updates PostgreSQL test indicators with sources \`codex-live-smoke\`, \`codex-live-import\`, \`codex-live-scheduled\` and \`codex-live-cross-tenant\`"
  echo
  echo "## Blockers"
  jq -r '.checks[] | select(.severity == "blocker" and .passed == false) | "- " + .name + ": " + .status + " " + .detail' "$SUMMARY"
  if [[ "$BLOCKERS" == "0" ]]; then
    echo "- None"
  fi
  echo
  echo "## Warnings"
  jq -r '.checks[] | select(.severity == "warning" and .passed == false) | "- " + .name + ": " + .status + " " + .detail' "$SUMMARY"
  if [[ "$WARNINGS" == "0" ]]; then
    echo "- None"
  fi
  echo
  echo "## Evidence"
  echo "- NDJSON: \`$REPORT\`"
  echo "- Summary: \`$SUMMARY\`"
  echo "- K8s deployment: \`$LOG_DIR/k8s-threat-intel-deployment.json\`"
  echo "- Kafka topic: \`$LOG_DIR/kafka-threat-intel-topic.txt\`"
  echo "- Kafka events: \`$LOG_DIR/kafka-threat-intel-events.txt\`"
  echo "- Audit upsert count: \`$LOG_DIR/pg-audit-upsert-count.txt\`"
  echo "- Audit import count: \`$LOG_DIR/pg-audit-import-count.txt\`"
  echo "- Audit scheduled event: \`$LOG_DIR/pg-audit-scheduled-event-id.txt\`"
  echo "- Scheduled feed status: \`$LOG_DIR/api-feed-list-scheduled.json\`"
  echo "- Cross tenant lookup: \`$LOG_DIR/api-lookup-cross-tenant-default.json\`"
  echo "- Expired token lookup: \`$LOG_DIR/api-lookup-expired-token.json\`"
  echo "- Builtin lookup: \`$LOG_DIR/api-lookup-builtin-c2.json\`"
  echo "- Smoke lookup: \`$LOG_DIR/api-lookup-smoke-entry.json\`"
  echo "- Scheduled lookup: \`$LOG_DIR/api-scheduled-feed-lookup.json\`"
  echo "- Enrichment: \`$LOG_DIR/api-enrich-builtin-c2.json\`"
  echo
  echo "## Scope"
  echo
  echo "This preflight verifies the repo contract, Kubernetes workload, Kafka topic catalog entry, APISIX route, JWT/RBAC read-write gates, tenant-scoped PostgreSQL-backed upsert/lookup/list/import, cross-tenant and expired-token negative cases, scheduled feed import, synchronous audit_logs writes, threat.intel.v1 publish/consume evidence, and alert enrichment API for the Threat Intel service."
} >"$LOCAL_REPORT"

cp "$SUMMARY" "$REGRESSION_DIR/threat-intel-service-latest.json"
cp "$LOCAL_REPORT" "$REGRESSION_DIR/threat-intel-service-latest.md"

if [[ "$RESULT" != "pass" ]]; then
  exit 1
fi
