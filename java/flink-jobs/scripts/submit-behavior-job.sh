#!/bin/bash
# Flink Behavior Detection Job — ML-based anomaly detection
set -euo pipefail
SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
JAR="${SCRIPT_DIR}/../flink-behavior-job/target/flink-behavior-job-1.0.0-SNAPSHOT.jar"
KAFKA="${KAFKA_BROKERS:-kafka-bootstrap.middleware.svc:9092}"
CH_URL="${CLICKHOUSE_URL:-${CLICKHOUSE_HOST:-clickhouse-1.middleware.svc}:8123}"
MODEL_UPDATE_TOPIC="${KAFKA_MODEL_UPDATE_TOPIC:-${MODEL_UPDATE_TOPIC:-model-updates}}"
MODEL_APPLIED_TOPIC="${KAFKA_MODEL_APPLIED_TOPIC:-model-update-applied.v1}"
DETECTION_MODE="${DETECTION_MODE:-off}"
KNOWN_PROFILE_ID="${DETECTION_KNOWN_PROFILE_ID:-m04-known-behavior-v1}"
KNOWN_PROFILE_SHA256="${DETECTION_KNOWN_PROFILE_SHA256:-3308cd498548716c68b79c8f665f5a1cb6d7b1d95769234853bbf1e9f7a03cdb}"
JOB_PARALLELISM="${PARALLELISM:-4}"
FLINK_BIN="${FLINK_BIN:-flink}"
. "${SCRIPT_DIR}/clickhouse-password.sh"
resolve_clickhouse_password

[ ! -f "$JAR" ] && { mvn -pl flink-behavior-job -am package -DskipTests -q; }

echo "==> Submitting Behavior Detection Job..."
"$FLINK_BIN" run -d \
  -p "$JOB_PARALLELISM" \
  -c com.traffic.flink.behavior.BehaviorDetectionJob \
  "$JAR" \
  --kafka.brokers "$KAFKA" \
  --kafka.input.topic "feature.stat.v1" \
  --kafka.output.topic "detections.v1" \
  --kafka.dlq.topic "dlq.v1" \
  --kafka.model.update.topic "$MODEL_UPDATE_TOPIC" \
  --kafka.model.applied.topic "$MODEL_APPLIED_TOPIC" \
  --clickhouse.url "$CH_URL" \
  --clickhouse.database "traffic" \
  --clickhouse.table "detections_behavior" \
  --clickhouse.batch.size 5000 \
  --clickhouse.batch.interval.ms 2000 \
  --checkpoint.path "${CHECKPOINT_DIR:-s3://flink-checkpoints/checkpoints}/behavior-job" \
  --checkpoint.interval.ms 30000 \
  --checkpoint.timeout.ms 600000 \
  --state.ttl.ms 1800000 \
  --detection.mode "$DETECTION_MODE" \
  --detection.known.profile.id "$KNOWN_PROFILE_ID" \
  --detection.known.profile.sha256 "$KNOWN_PROFILE_SHA256" \
  --model.hot.update.enabled false \
  --inference.async.enabled false \
  --parallelism "$JOB_PARALLELISM" \
  --clickhouse.password "$CLICKHOUSE_PASSWORD"
