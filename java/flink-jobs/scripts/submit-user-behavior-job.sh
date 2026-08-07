#!/bin/bash
set -euo pipefail
SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
JAR="${SCRIPT_DIR}/../flink-user-behavior-job/target/flink-user-behavior-job-1.0.0-SNAPSHOT.jar"
KAFKA="${KAFKA_BROKERS:-kafka-bootstrap.middleware.svc:9092}"
CH="${CLICKHOUSE_HOST:-clickhouse-1.middleware.svc}"
CHECKPOINT_PATH="${CHECKPOINT_PATH:-s3://flink-checkpoints/checkpoints/user-behavior-job}"
CH_ANOMALY_TABLE="${CLICKHOUSE_USER_ANOMALY_TABLE:-traffic.user_anomalies_v2}"
CH_BATCH_SIZE="${CLICKHOUSE_BATCH_SIZE:-500}"
CH_BATCH_INTERVAL_MS="${CLICKHOUSE_BATCH_INTERVAL_MS:-2000}"
CH_MAX_RETRIES="${CLICKHOUSE_MAX_RETRIES:-3}"
REPLAY_ID="${FLINK_REPLAY_ID:-}"
. "${SCRIPT_DIR}/clickhouse-password.sh"
resolve_clickhouse_password
[ ! -f "$JAR" ] && { cd "${SCRIPT_DIR}/.." && mvn -pl flink-user-behavior-job -am package -DskipTests -q; }
echo "==> Submitting User Behavior Job (Travel+Brute+Privilege)..."
flink run -d \
  -p 2 \
  -c com.traffic.flink.behavior.user.UserBehaviorJob \
  "$JAR" \
  --kafka.brokers "$KAFKA" \
  --kafka.input.topic "user.events.v1" \
  --kafka.output.topic "alerts.v1" \
  --kafka.dlq.topic "dlq.v1" \
  --clickhouse.url "jdbc:clickhouse://${CH}:8123/traffic" \
  --clickhouse.anomaly.table "$CH_ANOMALY_TABLE" \
  --clickhouse.batch.size "$CH_BATCH_SIZE" \
  --clickhouse.batch.interval.ms "$CH_BATCH_INTERVAL_MS" \
  --clickhouse.max.retries "$CH_MAX_RETRIES" \
  --replay.id "$REPLAY_ID" \
  --checkpoint.path "$CHECKPOINT_PATH" \
  --checkpoint.interval.ms 60000 \
  --checkpoint.timeout.ms 600000 \
  --parallelism 2 \
  --clickhouse.password "$CLICKHOUSE_PASSWORD"
