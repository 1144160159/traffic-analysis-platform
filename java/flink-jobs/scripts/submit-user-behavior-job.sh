#!/bin/bash
set -euo pipefail
SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
JAR="${SCRIPT_DIR}/../flink-user-behavior-job/target/flink-user-behavior-job-1.0.0-SNAPSHOT.jar"
KAFKA="${KAFKA_BROKERS:-kafka-bootstrap.middleware.svc:9092}"
CH="${CLICKHOUSE_HOST:-clickhouse-1.middleware.svc}"
CHECKPOINT_PATH="${CHECKPOINT_PATH:-s3://flink-checkpoints/checkpoints/user-behavior-job}"
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
  --clickhouse.url "jdbc:clickhouse://${CH}:8123/traffic" \
  --checkpoint.path "$CHECKPOINT_PATH" \
  --checkpoint.interval.ms 60000 \
  --checkpoint.timeout.ms 600000 \
  --parallelism 2 \
  --clickhouse.password "$CLICKHOUSE_PASSWORD"
