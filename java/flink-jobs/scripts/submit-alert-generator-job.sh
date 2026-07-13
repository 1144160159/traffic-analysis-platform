#!/bin/bash
# Flink Alert Generator Job — Detection → Alert, 去重 + 证据生成
set -euo pipefail
SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
JAR="${SCRIPT_DIR}/../flink-alert-generator-job/target/flink-alert-generator-job-1.0.0-SNAPSHOT.jar"
KAFKA="${KAFKA_BROKERS:-kafka-bootstrap.middleware.svc:9092}"
CH="${CLICKHOUSE_HOST:-clickhouse-1.middleware.svc}"
. "${SCRIPT_DIR}/clickhouse-password.sh"
resolve_clickhouse_password

[ ! -f "$JAR" ] && { mvn -pl flink-alert-generator-job -am package -DskipTests -q; }

echo "==> Submitting Alert Generator Job..."
flink run -d \
  -p 4 \
  -c com.traffic.flink.alert.AlertGeneratorJob \
  "$JAR" \
  --kafka.brokers "$KAFKA" \
  --kafka.input.topic.behavior "detections.behavior.v1" \
  --kafka.input.topic.business "detections.business.v1" \
  --kafka.output.topic "alerts.v1" \
  --enable.business.detection false \
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
  --parallelism 4 \
  --clickhouse.password "$CLICKHOUSE_PASSWORD"
