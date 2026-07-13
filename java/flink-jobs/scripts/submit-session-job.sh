#!/bin/bash
# =============================================================================
# Flink Session Job 提交脚本（生产版）
# 依据：agent.md 数据采集流程 + rules/java.md Flink 规范
#
# 数据流：
#   flow.events.v1 (Kafka) → Sessionize → session.events.v1 (Kafka)
#   → ClickHouse sessions (Distributed, + OpenSearch 可选)
#
# 用法：
#   ./submit-session-job.sh [--process|--window]
# =============================================================================
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
JOB_JAR="${SCRIPT_DIR}/../flink-session-job/target/flink-session-job-1.0.0-SNAPSHOT.jar"
MODE="${1:---process}"
. "${SCRIPT_DIR}/clickhouse-password.sh"

KAFKA_BROKERS="${KAFKA_BROKERS:-kafka-bootstrap.middleware.svc:9092}"
CLICKHOUSE_HOST="${CLICKHOUSE_HOST:-clickhouse-1.middleware.svc}"
CHECKPOINT_DIR="${CHECKPOINT_DIR:-s3://flink-checkpoints/checkpoints/session-job}"
SESSION_PARALLELISM="${SESSION_PARALLELISM:-12}"
resolve_clickhouse_password

if [ ! -f "${JOB_JAR}" ]; then
  echo "==> Building session job JAR..."
  cd "${SCRIPT_DIR}/.."
  mvn -pl flink-session-job -am package -DskipTests -q
fi

echo "==> Submitting Session Job..."
echo "    Mode: ${MODE#--}"
echo "    Session gap: 5s | Watermark: 5s | Active timeout: 1800s | Checkpoint: 30s"
echo "    Parallelism: ${SESSION_PARALLELISM}"

flink run -d \
  -p "${SESSION_PARALLELISM}" \
  -c com.traffic.flink.session.SessionJob \
  "${JOB_JAR}" \
  --session.mode "${MODE#--}" \
  --kafka.brokers "${KAFKA_BROKERS}" \
  --input.topic "flow.events.v1" \
  --output.topic "session.events.v1" \
  --input.dlq.topic "dlq.v1" \
  --clickhouse.url "jdbc:clickhouse://${CLICKHOUSE_HOST}:8123/traffic" \
  --clickhouse.table "sessions" \
  --flow.raw.sink.enabled true \
  --flow.raw.clickhouse.table "flows_raw" \
  --clickhouse.batch.size 5000 \
  --clickhouse.batch.interval.ms 2000 \
  --checkpoint.path "${CHECKPOINT_DIR}" \
  --checkpoint.interval.ms 30000 \
  --checkpoint.timeout.ms 600000 \
  --session.gap.ms 5000 \
  --active.timeout.ms 1800000 \
  --watermark.delay.ms 5000 \
  --state.ttl.ms 1800000 \
  --state.ttl.enabled true \
  --parallelism "${SESSION_PARALLELISM}" \
  --max.parallelism 128 \
  --clickhouse.password "$CLICKHOUSE_PASSWORD"
