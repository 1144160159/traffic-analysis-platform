#!/usr/bin/env bash
set -euo pipefail

TENANT="${TENANT:-campus-a}"
LOG_DIR="${LOG_DIR:-.artifacts/e2e}"
RUN_ID="${RUN_ID:-$(date +%Y%m%d%H%M%S)-$$}"
KUBECTL="${KUBECTL:-kubectl}"
PG_NAMESPACE="${PG_NAMESPACE:-databases}"
PG_POD="${PG_POD:-postgres-primary-0}"
PG_SECRET_NAMESPACE="${PG_SECRET_NAMESPACE:-traffic-analysis}"
PG_SECRET_NAME="${PG_SECRET_NAME:-traffic-credentials}"
PG_SECRET_KEY="${PG_SECRET_KEY:-PG_PASSWORD}"
INGEST_NAMESPACE="${INGEST_NAMESPACE:-traffic-analysis}"
INGEST_SELECTOR="${INGEST_SELECTOR:-app=ingest-gateway}"
INGEST_CONTAINER="${INGEST_CONTAINER:-ingest-gateway}"
FALLBACK_DIR="${FALLBACK_DIR:-/var/log/ingest-gateway/dlq-fallback}"
PORT_BASE="${PORT_BASE:-18082}"

REPORT="$LOG_DIR/live-dlq-replay-failure-regression-$RUN_ID.ndjson"
SUMMARY="$LOG_DIR/live-dlq-replay-failure-regression-$RUN_ID-summary.json"
FIRST_RESPONSE="$LOG_DIR/$RUN_ID-replay-partial-first.json"
SECOND_RESPONSE="$LOG_DIR/$RUN_ID-replay-partial-second.json"
FALLBACK_LINE="$LOG_DIR/$RUN_ID-invalid-fallback-line.txt"
TOKEN_FILE="$LOG_DIR/$RUN_ID-token.txt"

mkdir -p "$LOG_DIR"

FAILURES=0
TOKEN_ID=""
TOKEN_NAME=""
PF_PID=""
POD=""
FALLBACK_FILE=""
REDIS_IDEMPOTENCY_KEY=""

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
  if [[ "$ok" != "true" ]]; then
    FAILURES=$((FAILURES + 1))
  fi
}

trim_file() {
  local file="$1"
  if [[ -s "$file" ]]; then
    head -c 500 "$file" | tr '\n' ' '
  fi
}

cleanup() {
  set +e
  if [[ -n "$PF_PID" ]]; then
    kill "$PF_PID" >/dev/null 2>&1 || true
    wait "$PF_PID" >/dev/null 2>&1 || true
  fi
  if [[ -n "$TOKEN_ID" ]]; then
    psql_exec "DELETE FROM api_tokens WHERE token_id = '$TOKEN_ID';" >/dev/null 2>&1 || true
  fi
  if [[ -n "$REDIS_IDEMPOTENCY_KEY" ]]; then
    redis_exec DEL "$REDIS_IDEMPOTENCY_KEY" >/dev/null 2>&1 || true
  fi
  if [[ -n "$POD" && -n "$FALLBACK_FILE" ]]; then
    kctl -n "$INGEST_NAMESPACE" exec "$POD" -c "$INGEST_CONTAINER" -- sh -c "rm -f '$FALLBACK_FILE'" >/dev/null 2>&1 || true
  fi
  rm -f "$TOKEN_FILE"
}
trap cleanup EXIT

psql_exec() {
  local sql="$1"
  kctl -n "$PG_NAMESPACE" exec "$PG_POD" -- env PGPASSWORD="$PGPASSWORD" \
    psql -U postgres -d traffic_platform -v ON_ERROR_STOP=1 -Atc "$sql"
}

find_redis_master_pod() {
  local master_ip pod
  master_ip="$(kctl -n databases exec redis-sentinel-0 -- redis-cli -p 26379 SENTINEL get-master-addr-by-name mymaster 2>/dev/null | head -n 1 | tr -d '\r' || true)"
  if [[ -n "$master_ip" ]]; then
    pod="$(kctl -n databases get pods -o jsonpath='{range .items[*]}{.metadata.name}{" "}{.status.podIP}{"\n"}{end}' | awk -v ip="$master_ip" '$2 == ip {print $1; exit}')"
    if [[ -n "$pod" ]]; then
      echo "$pod"
      return
    fi
  fi
  pod="$(kctl -n databases get pods -o name | grep -E 'redis.*(master|node|server|0)' | head -n 1 | sed 's#pod/##')"
  echo "${pod:-redis-master-0}"
}

redis_exec() {
  local pod
  pod="$(find_redis_master_pod)"
  kctl -n databases exec "$pod" -- redis-cli "$@"
}

wait_http() {
  local url="$1"
  for _ in $(seq 1 30); do
    if curl --noproxy '*' -fsS "$url" >/dev/null 2>&1; then
      return 0
    fi
    sleep 0.2
  done
  return 1
}

start_port_forward() {
  local log_file="$LOG_DIR/$RUN_ID-port-forward-$POD-$PORT_BASE.log"
  kctl -n "$INGEST_NAMESPACE" port-forward "pod/$POD" "$PORT_BASE:8080" >"$log_file" 2>&1 &
  PF_PID=$!
  wait_http "http://127.0.0.1:$PORT_BASE/health"
}

create_token() {
  local token_name="codex-dlq-replay-failure-$RUN_ID"
  python3 - "$token_name" "$TENANT" "$TOKEN_FILE" <<'PY'
import hashlib
import secrets
import sys
import uuid

name, tenant, token_file = sys.argv[1:4]
token = "codex-dlq-failure-" + secrets.token_urlsafe(32)
token_hash = hashlib.sha256(token.encode("utf-8")).hexdigest()
token_id = str(uuid.uuid4())
prefix = token[:24]
open(token_file, "w", encoding="utf-8").write(token)
print("\t".join([token_id, name, tenant, token_hash, prefix]))
PY
}

make_invalid_fallback_line() {
  local key="codex-dlq-failure-$RUN_ID"
  python3 - "$key" "$TENANT" "$RUN_ID" <<'PY'
import json
import sys
import time

key, tenant, run_id = sys.argv[1:4]
msg = {
    "original_topic": "flow.events.v1",
    "event_type": "codex.dlq.failure.regression",
    "tenant_id": tenant,
    "probe_id": "codex-dlq-failure",
    "event_id": run_id,
    "failed_at": time.strftime("%Y-%m-%dT%H:%M:%SZ", time.gmtime()),
    "error_message": "codex controlled invalid payload regression sample",
    "retry_count": 0,
    "headers": {
        "tenant_id": tenant,
        "x-codex-run-id": run_id,
        "x-codex-dlq-failure-regression": "true"
    },
    "payload_base64": "@@not-base64@@"
}
print(f"dlq.v1|{key}|{json.dumps(msg, separators=(',', ':'))}")
PY
}

seed_invalid_fallback_file() {
  kctl -n "$INGEST_NAMESPACE" exec "$POD" -c "$INGEST_CONTAINER" -- sh -c "mkdir -p '$FALLBACK_DIR'"
  local existing
  existing="$(kctl -n "$INGEST_NAMESPACE" exec "$POD" -c "$INGEST_CONTAINER" -- sh -c "find '$FALLBACK_DIR' -maxdepth 1 -type f -print 2>/dev/null | head -n 1" || true)"
  if [[ -n "$existing" ]]; then
    json_log "setup" "fallback-dir-empty-precondition" false "existing_files" "$existing"
    echo "fallback dir on $POD is not empty; refusing to replay unrelated live files" >&2
    exit 2
  fi
  make_invalid_fallback_line >"$FALLBACK_LINE"
  FALLBACK_FILE="$FALLBACK_DIR/dlq-fallback-codex-failure-$RUN_ID.log"
  kctl -n "$INGEST_NAMESPACE" exec -i "$POD" -c "$INGEST_CONTAINER" -- sh -c "cat > '$FALLBACK_FILE'" <"$FALLBACK_LINE"
  kctl -n "$INGEST_NAMESPACE" exec "$POD" -c "$INGEST_CONTAINER" -- sh -c "test -s '$FALLBACK_FILE'"
  json_log "setup" "invalid-fallback-file-seeded" true "seeded" "$POD:$FALLBACK_FILE"
}

post_replay() {
  local outfile="$1"
  local token body
  token="$(cat "$TOKEN_FILE")"
  body="$(jq -nc \
    --arg tenant_id "$TENANT" \
    --arg requested_by "codex-dlq-failure-operator" \
    --arg approved_by "dlq-failure-approver" \
    --arg approval_id "APPROVAL-$RUN_ID-DLQ-FAILURE" \
    --arg reason "validate failed fallback sample handling" \
    --arg repair_summary "confirmed invalid payload must stay quarantined" \
    --arg idempotency_key "$TENANT:APPROVAL-$RUN_ID-DLQ-FAILURE:execute" \
    '{tenant_id:$tenant_id,requested_by:$requested_by,approved_by:$approved_by,approval_id:$approval_id,reason:$reason,repair_summary:$repair_summary,idempotency_key:$idempotency_key,dry_run:false}')"
  curl --noproxy '*' -sS -m 20 -o "$outfile" -w '%{http_code}' \
    -X POST "http://127.0.0.1:$PORT_BASE/api/v1/dlq/replay/fallback" \
    -H "Authorization: Bearer $token" \
    -H "Content-Type: application/json" \
    --data "$body"
}

need_cmd curl
need_cmd jq
need_cmd python3
need_cmd rg
need_cmd "$KUBECTL"

PGPASSWORD="$(kctl -n "$PG_SECRET_NAMESPACE" get secret "$PG_SECRET_NAME" -o "jsonpath={.data.$PG_SECRET_KEY}" | base64 -d)"

mapfile -t PODS < <(kctl -n "$INGEST_NAMESPACE" get pods -l "$INGEST_SELECTOR" -o jsonpath='{range .items[?(@.status.phase=="Running")]}{.metadata.name}{"\n"}{end}' | sort)
if [[ "${#PODS[@]}" -lt 1 ]]; then
  echo "need at least 1 running ingest-gateway pod for failure regression drill" >&2
  exit 2
fi
POD="${PODS[0]}"

read -r TOKEN_ID TOKEN_NAME TOKEN_TENANT TOKEN_HASH TOKEN_PREFIX < <(create_token)
psql_exec "INSERT INTO api_tokens (token_id, tenant_id, name, description, token_type, token_hash, token_prefix, scopes, status, expires_at, created_at, updated_at, metadata, probe_id) VALUES ('$TOKEN_ID', '$TENANT', '$TOKEN_NAME', 'Codex DLQ failure regression token', 'api', '$TOKEN_HASH', '$TOKEN_PREFIX', '[\"dlq:replay\"]'::jsonb, 'active', now() + interval '20 minutes', now(), now(), '{\"codex_run_id\":\"$RUN_ID\"}'::jsonb, NULL);"
json_log "setup" "temporary-dlq-token" true "created" "token_id=$TOKEN_ID name=$TOKEN_NAME tenant=$TOKEN_TENANT"

seed_invalid_fallback_file
start_port_forward
json_log "setup" "pod-port-forward" true "ready" "$POD:$PORT_BASE"

IDEMPOTENCY_KEY="$TENANT:APPROVAL-$RUN_ID-DLQ-FAILURE:execute"
REDIS_IDEMPOTENCY_KEY="$(python3 - "$IDEMPOTENCY_KEY" <<'PY'
import hashlib
import sys
print("dlq:replay:idempotency:" + hashlib.sha256(sys.argv[1].encode("utf-8")).hexdigest())
PY
)"

code_first="$(post_replay "$FIRST_RESPONSE")"
if [[ "$code_first" == "200" ]] && jq -e '.status == "partial" and .duplicate == false and .failed_files >= 1 and .replayed_files == 0 and .remaining_fallback_files >= 1 and ((.errors // []) | length > 0)' "$FIRST_RESPONSE" >/dev/null; then
  json_log "replay" "partial-failure-first-pod" true "$code_first" "$(jq -c '{status,duplicate,replayed_files,failed_files,remaining_fallback_files,replay_id,errors}' "$FIRST_RESPONSE")"
else
  json_log "replay" "partial-failure-first-pod" false "$code_first" "$(trim_file "$FIRST_RESPONSE")"
fi

if kctl -n "$INGEST_NAMESPACE" exec "$POD" -c "$INGEST_CONTAINER" -- sh -c "test -s '$FALLBACK_FILE'" >/dev/null 2>&1; then
  json_log "regression" "invalid-file-preserved-after-partial" true "preserved" "$FALLBACK_FILE"
else
  json_log "regression" "invalid-file-preserved-after-partial" false "missing" "$FALLBACK_FILE"
fi

code_second="$(post_replay "$SECOND_RESPONSE")"
if [[ "$code_second" == "200" ]] && jq -en --slurpfile second "$SECOND_RESPONSE" --slurpfile first "$FIRST_RESPONSE" '$second[0].duplicate == true and $second[0].status == "partial" and $second[0].replay_id == $first[0].replay_id' >/dev/null; then
  json_log "idempotency" "duplicate-partial-result" true "$code_second" "$(jq -c '{status,duplicate,replayed_files,failed_files,remaining_fallback_files,replay_id}' "$SECOND_RESPONSE")"
else
  json_log "idempotency" "duplicate-partial-result" false "$code_second" "$(trim_file "$SECOND_RESPONSE")"
fi

psql_exec "DELETE FROM api_tokens WHERE token_id = '$TOKEN_ID';" >/dev/null
json_log "cleanup" "temporary-dlq-token-deleted" true "deleted" "token_id=$TOKEN_ID"
TOKEN_ID=""

remaining_tokens="$(psql_exec "SELECT count(*) FROM api_tokens WHERE name = '$TOKEN_NAME';")"
if [[ "$remaining_tokens" == "0" ]]; then
  json_log "cleanup" "temporary-token-absent" true "0" "$TOKEN_NAME"
else
  json_log "cleanup" "temporary-token-absent" false "$remaining_tokens" "$TOKEN_NAME"
fi

redis_exec DEL "$REDIS_IDEMPOTENCY_KEY" >/dev/null
redis_exists="$(redis_exec EXISTS "$REDIS_IDEMPOTENCY_KEY" | tr -d '\r')"
if [[ "$redis_exists" == "0" ]]; then
  json_log "cleanup" "redis-idempotency-key-absent" true "0" "$REDIS_IDEMPOTENCY_KEY"
else
  json_log "cleanup" "redis-idempotency-key-absent" false "$redis_exists" "$REDIS_IDEMPOTENCY_KEY"
fi
REDIS_IDEMPOTENCY_KEY=""

kctl -n "$INGEST_NAMESPACE" exec "$POD" -c "$INGEST_CONTAINER" -- sh -c "rm -f '$FALLBACK_FILE'"
if ! kctl -n "$INGEST_NAMESPACE" exec "$POD" -c "$INGEST_CONTAINER" -- sh -c "test -e '$FALLBACK_FILE'" >/dev/null 2>&1; then
  json_log "cleanup" "invalid-fallback-file-removed" true "removed" "$FALLBACK_FILE"
else
  json_log "cleanup" "invalid-fallback-file-removed" false "still_exists" "$FALLBACK_FILE"
fi
FALLBACK_FILE=""

jq -s --arg run_id "$RUN_ID" --arg report "$REPORT" --arg first "$FIRST_RESPONSE" --arg second "$SECOND_RESPONSE" '
  {
    run_id: $run_id,
    report: $report,
    first_response: $first,
    second_response: $second,
    total_checks: length,
    failed_checks: map(select(.ok != true)) | length,
    result: (if (map(select(.ok != true)) | length) == 0 then "pass" else "fail" end),
    checks: .
  }
' "$REPORT" >"$SUMMARY"

cat "$SUMMARY"
exit "$FAILURES"
