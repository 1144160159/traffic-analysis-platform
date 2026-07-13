#!/bin/bash
set -euo pipefail
SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
JAR="${SCRIPT_DIR}/../flink-log-job/target/flink-log-job-1.0.0-SNAPSHOT.jar"
KAFKA="${KAFKA_BROKERS:-kafka-bootstrap.middleware.svc:9092}"
[ ! -f "$JAR" ] && { cd "${SCRIPT_DIR}/.." && mvn -pl flink-log-job -am package -DskipTests -q; }
echo "==> Submitting Log Job..."
flink run -d \
  -p 2 \
  -c com.traffic.flink.log.LogJob \
  "$JAR" \
  --kafka.brokers "$KAFKA" --checkpoint.interval.ms 60000 --parallelism 2
