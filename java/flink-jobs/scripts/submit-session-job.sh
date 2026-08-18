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
FLINK_REST_URL="${FLINK_REST_URL:-http://flink-jobmanager.middleware.svc:8081}"
M02_CONSUMER_FIRST_MODE="${M02_CONSUMER_FIRST_MODE:-}"
M02_READINESS_TIMEOUT_SEC="${M02_READINESS_TIMEOUT_SEC:-180}"
M02_READINESS_RECEIPT_PATH="${M02_READINESS_RECEIPT_PATH:-}"
M02_ROLLOUT_ID="${M02_ROLLOUT_ID:-}"
M02_CANDIDATE_ID="${M02_CANDIDATE_ID:-}"
resolve_clickhouse_password

if [[ "${M02_CONSUMER_FIRST_MODE}" != "idle" ]]; then
  echo "ERROR: M02_CONSUMER_FIRST_MODE must be exactly idle" >&2
  exit 64
fi
if [[ -z "${M02_READINESS_RECEIPT_PATH}" || -z "${M02_ROLLOUT_ID}" || -z "${M02_CANDIDATE_ID}" ]]; then
  echo "ERROR: M02 readiness receipt path, rollout ID and candidate ID are required" >&2
  exit 64
fi
if [[ "${M02_READINESS_RECEIPT_PATH}" != /* ]]; then
  echo "ERROR: M02_READINESS_RECEIPT_PATH must be absolute" >&2
  exit 64
fi

if [ ! -f "${JOB_JAR}" ]; then
  echo "==> Building session job JAR..."
  cd "${SCRIPT_DIR}/.."
  mvn -pl flink-session-job -am package -DskipTests -q
fi

echo "==> Submitting Session Job..."
echo "    Mode: ${MODE#--}"
echo "    Session gap: 5s | Watermark: 5s | Active timeout: 1800s | Checkpoint: 30s"
echo "    Parallelism: ${SESSION_PARALLELISM}"

SUBMIT_OUTPUT="$(flink run -d \
  -p "${SESSION_PARALLELISM}" \
  -c com.traffic.flink.session.SessionJob \
  "${JOB_JAR}" \
  --session.mode "${MODE#--}" \
  --kafka.brokers "${KAFKA_BROKERS}" \
  --input.topic "flow.events.v1" \
  --output.topic "session.events.v1" \
  --input.dlq.topic "dlq.v1" \
  --late.data.topic "dlq.v1" \
  --ch.dlq.topic "dlq.v1" \
  --os.dlq.topic "dlq.v1" \
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
  --state.ttl.ms 3600000 \
  --state.ttl.enabled true \
  --parallelism "${SESSION_PARALLELISM}" \
  --max.parallelism 128 \
  --clickhouse.password "$CLICKHOUSE_PASSWORD")"
printf '%s\n' "${SUBMIT_OUTPUT}"

JOB_ID="$(printf '%s\n' "${SUBMIT_OUTPUT}" | sed -nE 's/.*JobID[[:space:]]+([0-9a-fA-F]{32}).*/\1/p' | tail -n 1)"
if [[ ! "${JOB_ID}" =~ ^[0-9a-fA-F]{32}$ ]]; then
  echo "ERROR: Flink submission did not return one valid JobID" >&2
  exit 1
fi

deadline=$((SECONDS + M02_READINESS_TIMEOUT_SEC))
state="UNKNOWN"
while (( SECONDS < deadline )); do
  state="$(curl --fail --silent --show-error "${FLINK_REST_URL%/}/jobs/${JOB_ID}" | python3 -c 'import json,sys; print(json.load(sys.stdin).get("state", "UNKNOWN"))')" || state="REST_ERROR"
  [[ "${state}" == "RUNNING" ]] && break
  case "${state}" in FAILED|CANCELED|FINISHED|SUSPENDED)
    echo "ERROR: session consumer entered terminal state ${state}" >&2
    exit 1
  esac
  sleep 2
done
if [[ "${state}" != "RUNNING" ]]; then
  flink cancel "${JOB_ID}" >/dev/null 2>&1 || true
  echo "ERROR: session consumer readiness timed out; job canceled" >&2
  exit 1
fi

checkpoint_id=""
while (( SECONDS < deadline )); do
  checkpoint_id="$(curl --fail --silent --show-error "${FLINK_REST_URL%/}/jobs/${JOB_ID}/checkpoints" | python3 -c 'import json,sys; data=json.load(sys.stdin); latest=data.get("latest",{}).get("completed"); print("" if not latest else latest.get("id", ""))')" || checkpoint_id=""
  [[ "${checkpoint_id}" =~ ^[0-9]+$ ]] && break
  sleep 2
done
if [[ ! "${checkpoint_id}" =~ ^[0-9]+$ ]]; then
  flink cancel "${JOB_ID}" >/dev/null 2>&1 || true
  echo "ERROR: session consumer produced no completed checkpoint; job canceled" >&2
  exit 1
fi
READY_OBSERVED_AT="$(date -u +%Y-%m-%dT%H:%M:%SZ)"

export JOB_ID CHECKPOINT_ID="${checkpoint_id}" READY_OBSERVED_AT JOB_JAR MODE KAFKA_BROKERS M02_READINESS_RECEIPT_PATH M02_ROLLOUT_ID M02_CANDIDATE_ID
python3 - <<'PY'
import hashlib, json, os, pathlib, tempfile
target = pathlib.Path(os.environ["M02_READINESS_RECEIPT_PATH"])
target.parent.mkdir(parents=True, exist_ok=True)
body = {
    "schema_version": 1,
    "artifact_kind": "M02_CONSUMER_READINESS_RECEIPT",
    "rollout_id": os.environ["M02_ROLLOUT_ID"],
    "candidate_id": os.environ["M02_CANDIDATE_ID"],
    "consumer": "flink-session-job",
    "job_id": os.environ["JOB_ID"],
    "completed_checkpoint_id": int(os.environ["CHECKPOINT_ID"]),
    "state": "RUNNING",
    "activation": "CONSUMER_FIRST_IDLE",
    "input_topics": ["flow.events.v1"],
    "output_topics": ["session.events.v1", "dlq.v1"],
    "kafka_brokers": os.environ["KAFKA_BROKERS"],
    "jar_sha256": hashlib.sha256(pathlib.Path(os.environ["JOB_JAR"]).read_bytes()).hexdigest(),
    "ready_observed_at": os.environ["READY_OBSERVED_AT"],
    "producer_enabled": False,
}
fd, temporary = tempfile.mkstemp(prefix=target.name + ".", dir=target.parent)
with os.fdopen(fd, "w", encoding="utf-8") as stream:
    json.dump(body, stream, sort_keys=True, indent=2)
    stream.write("\n")
os.replace(temporary, target)
PY
echo "==> Session consumer RUNNING receipt: ${M02_READINESS_RECEIPT_PATH}"
