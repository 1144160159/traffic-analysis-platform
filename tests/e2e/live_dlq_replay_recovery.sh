#!/usr/bin/env bash
set -euo pipefail

APISIX="${APISIX:-http://10.0.5.8:30180}"
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
KAFKA_NAMESPACE="${KAFKA_NAMESPACE:-middleware}"
KAFKA_POD="${KAFKA_POD:-kafka-0}"
KAFKA_BOOTSTRAP="${KAFKA_BOOTSTRAP:-kafka-bootstrap.middleware.svc:9092}"
KAFKA_TOPIC="${KAFKA_TOPIC:-dlq.v1}"
KAFKA_CONSUMER_BIN="${KAFKA_CONSUMER_BIN:-/opt/kafka/bin/kafka-console-consumer.sh}"
KAFKA_LOOKUP_TIMEOUT_MS="${KAFKA_LOOKUP_TIMEOUT_MS:-15000}"
KAFKA_LOOKUP_MAX_MESSAGES="${KAFKA_LOOKUP_MAX_MESSAGES:-5000}"
PORT_BASE="${PORT_BASE:-18080}"

REPORT="$LOG_DIR/live-dlq-replay-recovery-$RUN_ID.ndjson"
SUMMARY="$LOG_DIR/live-dlq-replay-recovery-$RUN_ID-summary.json"
FIRST_RESPONSE="$LOG_DIR/$RUN_ID-replay-first.json"
SECOND_RESPONSE="$LOG_DIR/$RUN_ID-replay-second.json"
CONSUMER_LOG="$LOG_DIR/$RUN_ID-kafka-consumer.log"
CONSUMER_ERR="$LOG_DIR/$RUN_ID-kafka-consumer.err"
FALLBACK_LINE="$LOG_DIR/$RUN_ID-fallback-line.txt"
TOKEN_FILE="$LOG_DIR/$RUN_ID-token.txt"

mkdir -p "$LOG_DIR"

FAILURES=0
TOKEN_ID=""
REDIS_KEYS=()
PF_PIDS=()
CONSUMER_PID=""
POD_A=""
POD_B=""
FALLBACK_FILE_A=""
FALLBACK_FILE_B=""

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
  for pid in "${PF_PIDS[@]}"; do
    kill "$pid" >/dev/null 2>&1 || true
  done
  if [[ -n "$CONSUMER_PID" ]]; then
    wait "$CONSUMER_PID" >/dev/null 2>&1 || true
  fi
  if [[ -n "$TOKEN_ID" ]]; then
    PGPASSWORD="${PGPASSWORD:-}" psql_exec "DELETE FROM api_tokens WHERE token_id = '$TOKEN_ID';" >/dev/null 2>&1 || true
  fi
  for key in "${REDIS_KEYS[@]}"; do
    redis_exec "DEL '$key'" >/dev/null 2>&1 || true
  done
  for item in "$POD_A:$FALLBACK_FILE_A" "$POD_B:$FALLBACK_FILE_B"; do
    local pod="${item%%:*}"
    local file="${item#*:}"
    if [[ -n "$pod" && -n "$file" ]]; then
      kctl -n "$INGEST_NAMESPACE" exec "$pod" -c "$INGEST_CONTAINER" -- sh -c "rm -f '$file'" >/dev/null 2>&1 || true
    fi
  done
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
  local command="$1"
  local pod
  pod="$(find_redis_master_pod)"
  kctl -n databases exec "$pod" -- sh -c "redis-cli $command"
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
  local pod="$1"
  local port="$2"
  local log_file="$LOG_DIR/$RUN_ID-port-forward-$pod-$port.log"
  kctl -n "$INGEST_NAMESPACE" port-forward "pod/$pod" "$port:8080" >"$log_file" 2>&1 &
  local pid=$!
  PF_PIDS+=("$pid")
  wait_http "http://127.0.0.1:$port/health"
}

create_token() {
  local token_name="codex-dlq-replay-recovery-$RUN_ID"
  python3 - "$token_name" "$TENANT" "$TOKEN_FILE" <<'PY'
import hashlib
import secrets
import sys
import uuid

name, tenant, token_file = sys.argv[1:4]
token = "codex-dlq-" + secrets.token_urlsafe(32)
token_hash = hashlib.sha256(token.encode("utf-8")).hexdigest()
token_id = str(uuid.uuid4())
prefix = token[:18]
open(token_file, "w", encoding="utf-8").write(token)
print("\t".join([token_id, name, tenant, token_hash, prefix]))
PY
}

make_fallback_line() {
  local key="$1"
  local payload="$2"
  python3 - "$key" "$payload" "$TENANT" "$RUN_ID" "$KAFKA_TOPIC" <<'PY'
import base64
import json
import sys
import time

key, payload, tenant, run_id, topic = sys.argv[1:6]
msg = {
    "original_topic": topic,
    "event_type": "codex.dlq.replay.recovery",
    "tenant_id": tenant,
    "probe_id": "codex-dlq-replay",
    "event_id": run_id,
    "failed_at": time.strftime("%Y-%m-%dT%H:%M:%SZ", time.gmtime()),
    "error_message": "codex controlled fallback replay recovery drill",
    "retry_count": 0,
    "headers": {
        "tenant_id": tenant,
        "x-codex-run-id": run_id,
        "x-codex-dlq-recovery": "true"
    },
    "payload_base64": base64.b64encode(payload.encode("utf-8")).decode("ascii")
}
print(f"{topic}|{key}|{json.dumps(msg, separators=(',', ':'))}")
PY
}

seed_fallback_file() {
  local pod="$1"
  local file="$2"
  kctl -n "$INGEST_NAMESPACE" exec "$pod" -c "$INGEST_CONTAINER" -- sh -c "mkdir -p '$FALLBACK_DIR'"
  kctl -n "$INGEST_NAMESPACE" exec -i "$pod" -c "$INGEST_CONTAINER" -- sh -c "cat > '$file'" <"$FALLBACK_LINE"
  kctl -n "$INGEST_NAMESPACE" exec "$pod" -c "$INGEST_CONTAINER" -- sh -c "test -s '$file'"
}

post_replay() {
  local port="$1"
  local outfile="$2"
  local dry_run="$3"
  local idempotency_key="$4"
  local token
  token="$(cat "$TOKEN_FILE")"
  local body
  body="$(jq -nc \
    --arg tenant_id "$TENANT" \
    --arg requested_by "codex-dlq-replay-operator" \
    --arg approved_by "dlq-approver-2" \
    --arg approval_id "APPROVAL-$RUN_ID-DLQ-RECOVERY" \
    --arg reason "recover controlled fallback replay drill" \
    --arg repair_summary "validated malformed payload repair before replay drill" \
    --arg idempotency_key "$idempotency_key" \
    --argjson dry_run "$dry_run" \
    '{tenant_id:$tenant_id,requested_by:$requested_by,approved_by:$approved_by,approval_id:$approval_id,reason:$reason,repair_summary:$repair_summary,idempotency_key:$idempotency_key,dry_run:$dry_run}')"
  curl --noproxy '*' -sS -m 20 -o "$outfile" -w '%{http_code}' \
    -X POST "http://127.0.0.1:$port/api/v1/dlq/replay/fallback" \
    -H "Authorization: Bearer $token" \
    -H "Content-Type: application/json" \
    --data "$body"
}

consume_replayed_message() {
  local key="$1"
  : >"$CONSUMER_LOG"
  : >"$CONSUMER_ERR"
  set +e
  kctl -n "$KAFKA_NAMESPACE" exec "$KAFKA_POD" -- \
    "$KAFKA_CONSUMER_BIN" \
      --bootstrap-server "$KAFKA_BOOTSTRAP" \
      --topic "$KAFKA_TOPIC" \
      --from-beginning \
      --property print.key=true \
      --property key.separator='|' \
      --max-messages "$KAFKA_LOOKUP_MAX_MESSAGES" \
      --timeout-ms "$KAFKA_LOOKUP_TIMEOUT_MS" >"$CONSUMER_LOG" 2>"$CONSUMER_ERR"
  local rc=$?
  set -e
  if rg -q "$key|.*$RUN_ID" "$CONSUMER_LOG"; then
    json_log "kafka" "replayed-message-consumed" true "$rc" "$(rg "$key" "$CONSUMER_LOG" | head -n 1)"
  else
    json_log "kafka" "replayed-message-consumed" false "$rc" "consumer_log=$(trim_file "$CONSUMER_LOG") consumer_err=$(trim_file "$CONSUMER_ERR")"
  fi
}

need_cmd curl
need_cmd jq
need_cmd python3
need_cmd rg
need_cmd "$KUBECTL"

PGPASSWORD="$(kctl -n "$PG_SECRET_NAMESPACE" get secret "$PG_SECRET_NAME" -o "jsonpath={.data.$PG_SECRET_KEY}" | base64 -d)"

mapfile -t PODS < <(kctl -n "$INGEST_NAMESPACE" get pods -l "$INGEST_SELECTOR" -o jsonpath='{range .items[?(@.status.phase=="Running")]}{.metadata.name}{"\n"}{end}' | sort)
if [[ "${#PODS[@]}" -lt 2 ]]; then
  echo "need at least 2 running ingest-gateway pods for cross-pod idempotency drill" >&2
  exit 2
fi
POD_A="${PODS[0]}"
POD_B="${PODS[1]}"

read -r TOKEN_ID TOKEN_NAME TOKEN_TENANT TOKEN_HASH TOKEN_PREFIX < <(create_token)
psql_exec "INSERT INTO api_tokens (token_id, tenant_id, name, description, token_type, token_hash, token_prefix, scopes, status, expires_at, created_at, updated_at, metadata, probe_id) VALUES ('$TOKEN_ID', '$TENANT', '$TOKEN_NAME', 'Codex DLQ live recovery drill token', 'api', '$TOKEN_HASH', '$TOKEN_PREFIX', '[\"dlq:replay\"]'::jsonb, 'active', now() + interval '20 minutes', now(), now(), '{\"codex_run_id\":\"$RUN_ID\"}'::jsonb, NULL);"
json_log "setup" "temporary-dlq-token" true "created" "token_id=$TOKEN_ID name=$TOKEN_NAME tenant=$TOKEN_TENANT"

UNIQUE_KEY="codex-dlq-replay-$RUN_ID"
PAYLOAD="{\"codex_run_id\":\"$RUN_ID\",\"kind\":\"dlq-replay-recovery\",\"ts\":\"$(date -Iseconds)\"}"
make_fallback_line "$UNIQUE_KEY" "$PAYLOAD" >"$FALLBACK_LINE"
FALLBACK_FILE_A="$FALLBACK_DIR/dlq-fallback-codex-$RUN_ID-a.log"
FALLBACK_FILE_B="$FALLBACK_DIR/dlq-fallback-codex-$RUN_ID-b.log"
seed_fallback_file "$POD_A" "$FALLBACK_FILE_A"
seed_fallback_file "$POD_B" "$FALLBACK_FILE_B"
json_log "setup" "fallback-files-seeded" true "seeded" "$POD_A:$FALLBACK_FILE_A and $POD_B:$FALLBACK_FILE_B"

start_port_forward "$POD_A" "$PORT_BASE"
start_port_forward "$POD_B" "$((PORT_BASE + 1))"
json_log "setup" "pod-port-forwards" true "ready" "$POD_A:$PORT_BASE $POD_B:$((PORT_BASE + 1))"

IDEMPOTENCY_KEY="$TENANT:APPROVAL-$RUN_ID-DLQ-RECOVERY:execute"
REDIS_IDEMPOTENCY_KEY="$(python3 - "$IDEMPOTENCY_KEY" <<'PY'
import hashlib
import sys
print("dlq:replay:idempotency:" + hashlib.sha256(sys.argv[1].encode("utf-8")).hexdigest())
PY
)"
REDIS_KEYS+=("$REDIS_IDEMPOTENCY_KEY")
code_first="$(post_replay "$PORT_BASE" "$FIRST_RESPONSE" false "$IDEMPOTENCY_KEY")"
if [[ "$code_first" == "200" ]] && jq -e '.status == "completed" and .duplicate == false and .replayed_files >= 1' "$FIRST_RESPONSE" >/dev/null; then
  json_log "replay" "execute-first-pod" true "$code_first" "$(jq -c '{status,duplicate,replayed_files,failed_files,remaining_fallback_files,replay_id}' "$FIRST_RESPONSE")"
else
  json_log "replay" "execute-first-pod" false "$code_first" "$(trim_file "$FIRST_RESPONSE")"
fi

consume_replayed_message "$UNIQUE_KEY"

code_second="$(post_replay "$((PORT_BASE + 1))" "$SECOND_RESPONSE" false "$IDEMPOTENCY_KEY")"
if [[ "$code_second" == "200" ]] && jq -en --slurpfile second "$SECOND_RESPONSE" --slurpfile first "$FIRST_RESPONSE" '$second[0].duplicate == true and $second[0].replay_id == $first[0].replay_id' >/dev/null; then
  json_log "replay" "duplicate-second-pod" true "$code_second" "$(jq -c '{status,duplicate,replayed_files,failed_files,remaining_fallback_files,replay_id}' "$SECOND_RESPONSE")"
else
  json_log "replay" "duplicate-second-pod" false "$code_second" "$(trim_file "$SECOND_RESPONSE")"
fi

if ! kctl -n "$INGEST_NAMESPACE" exec "$POD_A" -c "$INGEST_CONTAINER" -- sh -c "test -e '$FALLBACK_FILE_A'" >/dev/null 2>&1; then
  json_log "cleanup" "first-pod-file-removed-by-replay" true "removed" "$FALLBACK_FILE_A"
else
  json_log "cleanup" "first-pod-file-removed-by-replay" false "still_exists" "$FALLBACK_FILE_A"
fi

if kctl -n "$INGEST_NAMESPACE" exec "$POD_B" -c "$INGEST_CONTAINER" -- sh -c "test -e '$FALLBACK_FILE_B'" >/dev/null 2>&1; then
  json_log "idempotency" "second-pod-file-preserved-by-duplicate" true "preserved" "$FALLBACK_FILE_B"
else
  json_log "idempotency" "second-pod-file-preserved-by-duplicate" false "missing" "$FALLBACK_FILE_B"
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

redis_exec "DEL '$REDIS_IDEMPOTENCY_KEY'" >/dev/null
redis_exists="$(redis_exec "EXISTS '$REDIS_IDEMPOTENCY_KEY'" | tr -d '\r')"
if [[ "$redis_exists" == "0" ]]; then
  json_log "cleanup" "redis-idempotency-key-absent" true "0" "$REDIS_IDEMPOTENCY_KEY"
else
  json_log "cleanup" "redis-idempotency-key-absent" false "$redis_exists" "$REDIS_IDEMPOTENCY_KEY"
fi
REDIS_KEYS=()

jq -s --arg run_id "$RUN_ID" --arg report "$REPORT" --arg first "$FIRST_RESPONSE" --arg second "$SECOND_RESPONSE" --arg consumer "$CONSUMER_LOG" '
  {
    run_id: $run_id,
    report: $report,
    first_response: $first,
    second_response: $second,
    consumer_log: $consumer,
    total_checks: length,
    failed_checks: map(select(.ok != true)) | length,
    result: (if (map(select(.ok != true)) | length) == 0 then "pass" else "fail" end),
    checks: .
  }
' "$REPORT" >"$SUMMARY"

cat "$SUMMARY"
exit "$FAILURES"
