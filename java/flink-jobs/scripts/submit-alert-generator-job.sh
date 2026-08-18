#!/bin/bash
# Flink Alert Generator Job — Detection → Alert, 去重 + 证据生成
set -euo pipefail
SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
JAR="${SCRIPT_DIR}/../flink-alert-generator-job/target/flink-alert-generator-job-1.0.0-SNAPSHOT.jar"
KAFKA="${KAFKA_BROKERS:-kafka-bootstrap.middleware.svc:9092}"
CH="${CLICKHOUSE_HOST:-clickhouse-1.middleware.svc}"
FLINK_BIN="${FLINK_BIN:-flink}"
JOB_PARALLELISM="${PARALLELISM:-4}"
ENABLE_LEGACY_BEHAVIOR="${ENABLE_LEGACY_BEHAVIOR_DETECTION:-false}"
ENABLE_BUSINESS="${ENABLE_BUSINESS_DETECTION:-false}"
. "${SCRIPT_DIR}/clickhouse-password.sh"
resolve_clickhouse_password

[ ! -f "$JAR" ] && { mvn -pl flink-alert-generator-job -am package -DskipTests -q; }

echo "==> Submitting Alert Generator Job..."
"$FLINK_BIN" run -d \
  -p "$JOB_PARALLELISM" \
  -c com.traffic.flink.alert.AlertGeneratorJob \
  "$JAR" \
  --kafka.brokers "$KAFKA" \
  --kafka.input.topic "detections.v1" \
  --kafka.input.topic.behavior "detections.behavior.v1" \
  --kafka.input.topic.business "detections.business.v1" \
  --kafka.output.topic "alerts.v1" \
  --kafka.dlq.topic "dlq.v1" \
  --enable.legacy.behavior.detection "$ENABLE_LEGACY_BEHAVIOR" \
  --enable.business.detection "$ENABLE_BUSINESS" \
  --clickhouse.url "${CH}:8123" \
  --clickhouse.database "traffic" \
  --clickhouse.alert.table "alerts" \
  --clickhouse.evidence.table "evidence" \
  --clickhouse.batch.size 5000 \
  --clickhouse.batch.interval.ms 2000 \
  --checkpoint.path "${CHECKPOINT_DIR:-s3://flink-checkpoints/checkpoints}/alert-generator-job" \
  --checkpoint.interval.ms 30000 \
  --checkpoint.timeout.ms 600000 \
  --state.ttl.ms 1800000 \
  --parallelism "$JOB_PARALLELISM" \
  --clickhouse.password "$CLICKHOUSE_PASSWORD"
