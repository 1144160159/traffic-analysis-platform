#!/usr/bin/env bash
set -euo pipefail

LOG_DIR="${LOG_DIR:-.artifacts/e2e}"
RUN_ID="${RUN_ID:-$(date +%Y%m%d%H%M%S)-$$}"
KUBECTL="${KUBECTL:-kubectl}"
KAFKA_NAMESPACE="${KAFKA_NAMESPACE:-middleware}"
KAFKA_POD="${KAFKA_POD:-kafka-0}"
KAFKA_BOOTSTRAP="${KAFKA_BOOTSTRAP:-kafka-bootstrap.middleware.svc:9092}"
KAFKA_PRODUCER_BIN="${KAFKA_PRODUCER_BIN:-/opt/kafka/bin/kafka-console-producer.sh}"
FLOW_TOPIC="${FLOW_TOPIC:-flow.events.v1}"
DLQ_TOPIC="${DLQ_TOPIC:-dlq.v1}"
TENANT="${TENANT:-campus-a}"
FLINK_NAMESPACE="${FLINK_NAMESPACE:-flink}"
FLINK_JM_POD="${FLINK_JM_POD:-flink-jobmanager-0}"
FLINK_JOB_NAME="${FLINK_JOB_NAME:-Session Aggregation Job V2}"
DLQ_TIMEOUT_SECONDS="${DLQ_TIMEOUT_SECONDS:-75}"
CHECKPOINT_TIMEOUT_SECONDS="${CHECKPOINT_TIMEOUT_SECONDS:-95}"

mkdir -p "$LOG_DIR"
LOG_DIR="$(cd "$LOG_DIR" && pwd)"

REPORT="$LOG_DIR/live-kafka-flink-bad-message-dlq-$RUN_ID.ndjson"
SUMMARY="$LOG_DIR/live-kafka-flink-bad-message-dlq-$RUN_ID-summary.json"
JOBS_BEFORE="$LOG_DIR/$RUN_ID-flink-jobs-before.json"
JOBS_AFTER="$LOG_DIR/$RUN_ID-flink-jobs-after.json"
CHECKPOINT_BEFORE="$LOG_DIR/$RUN_ID-flink-checkpoints-before.json"
CHECKPOINT_AFTER="$LOG_DIR/$RUN_ID-flink-checkpoints-after.json"
DLQ_MATCH="$LOG_DIR/$RUN_ID-deadletter-match.json"
GO_SRC="$LOG_DIR/$RUN_ID-consume-deadletter.go"
GO_BIN="$LOG_DIR/$RUN_ID-consume-deadletter"
REMOTE_GO_BIN="/tmp/codex-consume-deadletter-$RUN_ID"

FAILURES=0
SOURCE_KEY="$TENANT:codex-flink-bad-message:$RUN_ID"
BAD_PAYLOAD="codex-invalid-flowevent-protobuf-$RUN_ID"

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

cleanup() {
  set +e
  kctl -n "$KAFKA_NAMESPACE" exec "$KAFKA_POD" -- rm -f "$REMOTE_GO_BIN" >/dev/null 2>&1 || true
  rm -f "$GO_BIN"
}
trap cleanup EXIT

flink_get() {
  local path="$1"
  kctl -n "$FLINK_NAMESPACE" exec "$FLINK_JM_POD" -- curl -s "http://localhost:8081$path"
}

current_session_job() {
  jq -c --arg name "$FLINK_JOB_NAME" '[.jobs[] | select(.name == $name and .state == "RUNNING")] | sort_by(."start-time") | last // empty'
}

session_job_id() {
  local file="$1"
  current_session_job <"$file" | jq -r '.jid // empty'
}

completed_checkpoint_id() {
  local file="$1"
  jq -r '.latest.completed.id // 0' "$file"
}

write_consumer_source() {
  cat >"$GO_SRC" <<'GO'
package main

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/segmentio/kafka-go"
	"google.golang.org/protobuf/proto"

	pb "github.com/1144160159/traffic-analysis-platform/go/control-plane/pkg/proto/traffic/v1"
)

func env(name, fallback string) string {
	if value := os.Getenv(name); value != "" {
		return value
	}
	return fallback
}

func main() {
	brokers := strings.Split(env("KAFKA_BROKERS", "kafka-bootstrap.middleware.svc:9092"), ",")
	topic := env("DLQ_TOPIC", "dlq.v1")
	sourceKey := os.Getenv("SOURCE_KEY")
	badPayload := os.Getenv("BAD_PAYLOAD")
	timeout, _ := time.ParseDuration(env("DLQ_TIMEOUT_SECONDS", "75") + "s")
	deadline := time.Now().Add(timeout)

	reader := kafka.NewReader(kafka.ReaderConfig{
		Brokers:     brokers,
		Topic:       topic,
		GroupID:     "codex-flink-bad-message-dlq-" + env("RUN_ID", fmt.Sprint(time.Now().UnixNano())),
		MinBytes:    1,
		MaxBytes:    10 * 1024 * 1024,
		MaxWait:     500 * time.Millisecond,
		StartOffset: kafka.FirstOffset,
	})
	defer reader.Close()

	scanned := 0
	for time.Now().Before(deadline) {
		ctx, cancel := context.WithTimeout(context.Background(), time.Until(deadline))
		msg, err := reader.ReadMessage(ctx)
		cancel()
		if err != nil {
			break
		}
		scanned++
		dlq, ok := decodeDeadLetter(msg.Value)
		if !ok {
			continue
		}
		if dlq.SourceKey != sourceKey || dlq.SourceTopic != "flow.events.v1" {
			continue
		}
		raw, rawErr := base64.StdEncoding.DecodeString(dlq.RawPayload)
		rawMatches := rawErr == nil && string(raw) == badPayload
		out := map[string]interface{}{
			"event_id":        dlq.EventId,
			"tenant_id":       dlq.TenantId,
			"source_topic":    dlq.SourceTopic,
			"source_key":      dlq.SourceKey,
			"error_msg":       dlq.ErrorMsg,
			"retry_count":     dlq.RetryCount,
			"created_at":      dlq.CreatedAt,
			"raw_payload":     dlq.RawPayload,
			"raw_matches":     rawMatches,
			"kafka_partition": msg.Partition,
			"kafka_offset":    msg.Offset,
			"scanned":         scanned,
		}
		enc := json.NewEncoder(os.Stdout)
		enc.SetEscapeHTML(false)
		_ = enc.Encode(out)
		if !rawMatches {
			os.Exit(3)
		}
		return
	}
	fmt.Fprintf(os.Stderr, "deadletter not found for source_key=%s scanned=%d\n", sourceKey, scanned)
	os.Exit(2)
}

func decodeDeadLetter(value []byte) (*pb.DeadLetter, bool) {
	var batch pb.DeadLetterBatch
	if err := proto.Unmarshal(value, &batch); err == nil && len(batch.Events) > 0 {
		return batch.Events[0], true
	}
	var dlq pb.DeadLetter
	if err := proto.Unmarshal(value, &dlq); err == nil && dlq.EventId != "" {
		return &dlq, true
	}
	var raw map[string]interface{}
	if err := json.Unmarshal(value, &raw); err == nil {
		eventID, _ := raw["event_id"].(string)
		if eventID == "" {
			return nil, false
		}
		return &pb.DeadLetter{
			EventId:     eventID,
			TenantId:    stringValue(raw, "tenant_id"),
			SourceTopic: stringValue(raw, "source_topic"),
			SourceKey:   stringValue(raw, "source_key"),
			ErrorMsg:    stringValue(raw, "error_msg"),
			RawPayload:  stringValue(raw, "raw_payload"),
			RetryCount:  uint32(numberValue(raw, "retry_count")),
			CreatedAt:   int64(numberValue(raw, "created_at")),
		}, true
	}
	return nil, false
}

func stringValue(raw map[string]interface{}, key string) string {
	value, _ := raw[key].(string)
	return value
}

func numberValue(raw map[string]interface{}, key string) float64 {
	value, _ := raw[key].(float64)
	return value
}
GO
}

build_consumer_binary() {
  write_consumer_source
  (
    cd go/control-plane
    CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o "$GO_BIN" "$GO_SRC"
  )
  kctl -n "$KAFKA_NAMESPACE" exec -i "$KAFKA_POD" -- sh -c "cat > '$REMOTE_GO_BIN' && chmod +x '$REMOTE_GO_BIN'" <"$GO_BIN"
}

inject_bad_message() {
  printf '%s|%s\n' "$SOURCE_KEY" "$BAD_PAYLOAD" | \
    kctl -n "$KAFKA_NAMESPACE" exec -i "$KAFKA_POD" -- \
      "$KAFKA_PRODUCER_BIN" \
        --bootstrap-server "$KAFKA_BOOTSTRAP" \
        --topic "$FLOW_TOPIC" \
        --property parse.key=true \
        --property key.separator='|'
}

wait_checkpoint_advance() {
  local job_id="$1"
  local before_id="$2"
  local deadline=$((SECONDS + CHECKPOINT_TIMEOUT_SECONDS))
  while (( SECONDS < deadline )); do
    flink_get "/jobs/$job_id/checkpoints" >"$CHECKPOINT_AFTER" || true
    local after_id
    after_id="$(completed_checkpoint_id "$CHECKPOINT_AFTER")"
    if [[ "$after_id" =~ ^[0-9]+$ ]] && (( after_id > before_id )); then
      json_log "flink" "checkpoint-advanced" true "$after_id" "before=$before_id after=$after_id"
      return 0
    fi
    sleep 3
  done
  json_log "flink" "checkpoint-advanced" false "timeout" "before=$before_id"
  return 1
}

consume_deadletter() {
  set +e
  kctl -n "$KAFKA_NAMESPACE" exec "$KAFKA_POD" -- env \
    KAFKA_BROKERS="$KAFKA_BOOTSTRAP" \
    DLQ_TOPIC="$DLQ_TOPIC" \
    SOURCE_KEY="$SOURCE_KEY" \
    BAD_PAYLOAD="$BAD_PAYLOAD" \
    RUN_ID="$RUN_ID" \
    DLQ_TIMEOUT_SECONDS="$DLQ_TIMEOUT_SECONDS" \
    "$REMOTE_GO_BIN" >"$DLQ_MATCH" 2>"$LOG_DIR/$RUN_ID-deadletter-consumer.err"
  local rc=$?
  set -e
  if [[ "$rc" -eq 0 ]] && jq -e --arg source_key "$SOURCE_KEY" '.raw_matches == true and .source_key == $source_key and (.error_msg | contains("invalid FlowEvent protobuf"))' "$DLQ_MATCH" >/dev/null; then
    json_log "kafka" "deadletter-consumed" true "matched" "$(jq -c '{event_id,source_key,error_msg,kafka_partition,kafka_offset,scanned}' "$DLQ_MATCH")"
  else
    json_log "kafka" "deadletter-consumed" false "$rc" "match=$(head -c 500 "$DLQ_MATCH" 2>/dev/null || true) err=$(head -c 500 "$LOG_DIR/$RUN_ID-deadletter-consumer.err" 2>/dev/null || true)"
  fi
}

need_cmd jq
need_cmd go
need_cmd "$KUBECTL"

: >"$REPORT"

flink_get "/jobs/overview" >"$JOBS_BEFORE"
SESSION_JOB_ID="$(session_job_id "$JOBS_BEFORE")"
if [[ -z "$SESSION_JOB_ID" ]]; then
  json_log "flink" "session-job-running-before" false "missing" "$(head -c 500 "$JOBS_BEFORE")"
  jq -s \
    --arg run_id "$RUN_ID" \
    --arg source_key "$SOURCE_KEY" \
    '{run_id:$run_id, source_key:$source_key, total_checks:length, failed_checks:map(select(.ok == false)) | length, checks:.}' "$REPORT" >"$SUMMARY"
  echo "$SUMMARY"
  exit 1
else
  json_log "flink" "session-job-running-before" true "$SESSION_JOB_ID" "$(current_session_job <"$JOBS_BEFORE")"
fi

flink_get "/jobs/$SESSION_JOB_ID/checkpoints" >"$CHECKPOINT_BEFORE"
CHECKPOINT_BEFORE_ID="$(completed_checkpoint_id "$CHECKPOINT_BEFORE")"
json_log "flink" "checkpoint-before" true "$CHECKPOINT_BEFORE_ID" "job_id=$SESSION_JOB_ID"

build_consumer_binary
json_log "setup" "deadletter-consumer-binary" true "installed" "$KAFKA_POD:$REMOTE_GO_BIN"

inject_bad_message
json_log "kafka" "bad-flowevent-produced" true "$FLOW_TOPIC" "source_key=$SOURCE_KEY payload=$BAD_PAYLOAD"

wait_checkpoint_advance "$SESSION_JOB_ID" "$CHECKPOINT_BEFORE_ID" || true
consume_deadletter

flink_get "/jobs/overview" >"$JOBS_AFTER"
if jq -e --arg jid "$SESSION_JOB_ID" '.jobs[] | select(.jid == $jid and .state == "RUNNING" and .tasks.failed == 0)' "$JOBS_AFTER" >/dev/null; then
  json_log "flink" "session-job-running-after" true "$SESSION_JOB_ID" "$(jq -c --arg jid "$SESSION_JOB_ID" '.jobs[] | select(.jid == $jid)' "$JOBS_AFTER")"
else
  json_log "flink" "session-job-running-after" false "$SESSION_JOB_ID" "$(jq -c --arg jid "$SESSION_JOB_ID" '.jobs[] | select(.jid == $jid)' "$JOBS_AFTER")"
fi

jq -s \
  --arg run_id "$RUN_ID" \
  --arg source_key "$SOURCE_KEY" \
  --arg bad_payload "$BAD_PAYLOAD" \
  --arg report "$REPORT" \
  --arg dlq_match "$DLQ_MATCH" \
  '{
    run_id: $run_id,
    source_key: $source_key,
    bad_payload: $bad_payload,
    report: $report,
    dlq_match: $dlq_match,
    total_checks: length,
    failed_checks: map(select(.ok == false)) | length,
    checks: .
  }' "$REPORT" >"$SUMMARY"

if (( FAILURES > 0 )); then
  echo "live kafka/flink bad-message DLQ failed: $FAILURES checks failed"
  echo "$SUMMARY"
  exit 1
fi

echo "$SUMMARY"
