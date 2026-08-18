#!/usr/bin/env bash
# java/flink-jobs/scripts/submit-pcap-index-job.sh

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_DIR="$(cd "${SCRIPT_DIR}/.." && pwd)"
. "${SCRIPT_DIR}/clickhouse-password.sh"

# Flink 配置
FLINK_HOME="${FLINK_HOME:-/opt/flink}"
FLINK_MASTER="${FLINK_MASTER:-localhost:8081}"

# 作业配置
JOB_NAME="pcap-index-job"
JOB_JAR="${PROJECT_DIR}/flink-pcap-index-job/target/flink-pcap-index-job-1.0.0-SNAPSHOT.jar"
MAIN_CLASS="com.traffic.flink.pcap.PcapIndexJob"

# 默认参数
PARALLELISM="${PARALLELISM:-2}"
KAFKA_BROKERS="${KAFKA_BROKERS:-kafka-bootstrap.middleware.svc:9092}"
CLICKHOUSE_URL="${CLICKHOUSE_URL:-clickhouse-1.middleware.svc:8123,clickhouse-2.middleware.svc:8123}"
CHECKPOINT_PATH="${CHECKPOINT_PATH:-s3://flink-checkpoints/checkpoints/pcap-index-job}"
FLINK_REST_URL="${FLINK_REST_URL:-http://${FLINK_MASTER}}"
M02_CONSUMER_FIRST_MODE="${M02_CONSUMER_FIRST_MODE:-}"
M02_READINESS_TIMEOUT_SEC="${M02_READINESS_TIMEOUT_SEC:-180}"
M02_READINESS_RECEIPT_PATH="${M02_READINESS_RECEIPT_PATH:-}"
M02_ROLLOUT_ID="${M02_ROLLOUT_ID:-}"
M02_CANDIDATE_ID="${M02_CANDIDATE_ID:-}"
resolve_clickhouse_password

# 颜色
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m'

log_info() {
    echo -e "${GREEN}[INFO]${NC} $1"
}

log_error() {
    echo -e "${RED}[ERROR]${NC} $1"
}

show_usage() {
    echo "Usage: $0 [OPTIONS]"
    echo ""
    echo "Options:"
    echo "  --build           Build the project before submitting"
    echo "  --local           Run in local mode (for testing)"
    echo "  --detached, -d    Run in detached mode"
    echo "  --parallelism N   Set parallelism (default: 2)"
    echo "  --help, -h        Show this help message"
    echo ""
}

build_project() {
    log_info "Building project..."
    cd "${PROJECT_DIR}"
    mvn clean package -DskipTests -pl flink-pcap-index-job -am
    log_info "Build completed"
}

check_prerequisites() {
    log_info "Checking prerequisites..."

    if [[ ! -f "${JOB_JAR}" ]]; then
        log_error "Job JAR not found: ${JOB_JAR}"
        log_info "Please build the project first: $0 --build"
        exit 1
    fi

    if [[ "${CLICKHOUSE_URL}" == jdbc:* ]]; then
        log_error "CLICKHOUSE_URL must be host:port[,host:port], not a JDBC URL"
        log_info "Example: CLICKHOUSE_URL=clickhouse-1.middleware.svc:8123,clickhouse-2.middleware.svc:8123"
        exit 1
    fi

	if [[ "${M02_CONSUMER_FIRST_MODE}" != "idle" ]]; then
		log_error "M02_CONSUMER_FIRST_MODE must be exactly idle"
		exit 64
	fi
	if [[ -z "${M02_READINESS_RECEIPT_PATH}" || "${M02_READINESS_RECEIPT_PATH}" != /* || -z "${M02_ROLLOUT_ID}" || -z "${M02_CANDIDATE_ID}" ]]; then
		log_error "absolute readiness receipt path, rollout ID and candidate ID are required"
		exit 64
	fi

    log_info "Prerequisites check passed"
}

submit_job() {
    local detached="$1"
    local local_mode="$2"

    log_info "Submitting Flink job..."
    log_info "  Job Name: ${JOB_NAME}"
    log_info "  JAR: ${JOB_JAR}"
    log_info "  Parallelism: ${PARALLELISM}"

    local -a cmd
    
    if [[ "${local_mode}" == "true" ]]; then
		log_error "local mode cannot produce a consumer-first cluster readiness receipt"
		exit 64
    else
        if [[ -f "${FLINK_HOME}/bin/flink" ]]; then
			cmd=("${FLINK_HOME}/bin/flink" run)
        else
			cmd=(flink run)
        fi

        if [[ "${detached}" == "true" ]]; then
			cmd+=(-d)
		else
			log_error "consumer-first submission requires detached mode"
			exit 64
        fi

		cmd+=(-m "${FLINK_MASTER}" -p "${PARALLELISM}" -c "${MAIN_CLASS}" "${JOB_JAR}")
    fi

	cmd+=(
		--kafka.brokers "${KAFKA_BROKERS}"
		--kafka.input.topic "pcap.index.v1"
		--kafka.group.id "flink-pcap-index-job"
		--kafka.starting.offsets "committed-or-earliest"
		--pcap.carrier.enabled "true"
		--kafka.canonical.dlq.topic "dlq.v1"
		--pcap.kafka.dlq.acl.attested "true"
		--pcap.kafka.idempotent.acl.attested "true"
		--enable.auto.commit "false"
		--commit.offsets.on.checkpoint "true"
		--clickhouse.url "${CLICKHOUSE_URL}"
		--clickhouse.database "traffic"
		--clickhouse.table "pcap_index_v2"
		--clickhouse.local.table "pcap_index_v2_local"
		--checkpoint.path "${CHECKPOINT_PATH}"
		--checkpoint.interval.ms "30000"
		--checkpoint.timeout.ms "600000"
		--checkpoint.min.pause.ms "15000"
		--checkpoint.tolerable.failures "3"
		--restart.attempts "10"
		--restart.delay.seconds "30"
		--parallelism "${PARALLELISM}"
		--clickhouse.password "${CLICKHOUSE_PASSWORD}"
	)

	log_info "Executing Flink submission with redacted ClickHouse credential"
	local submit_output job_id state deadline ready_observed_at checkpoint_id
	submit_output="$("${cmd[@]}")"
	printf '%s\n' "${submit_output}"
	job_id="$(printf '%s\n' "${submit_output}" | sed -nE 's/.*JobID[[:space:]]+([0-9a-fA-F]{32}).*/\1/p' | tail -n 1)"
	if [[ ! "${job_id}" =~ ^[0-9a-fA-F]{32}$ ]]; then
		log_error "Flink submission did not return one valid JobID"
		exit 1
	fi

	deadline=$((SECONDS + M02_READINESS_TIMEOUT_SEC))
	state="UNKNOWN"
	while (( SECONDS < deadline )); do
		state="$(curl --fail --silent --show-error "${FLINK_REST_URL%/}/jobs/${job_id}" | python3 -c 'import json,sys; print(json.load(sys.stdin).get("state", "UNKNOWN"))')" || state="REST_ERROR"
		[[ "${state}" == "RUNNING" ]] && break
		case "${state}" in FAILED|CANCELED|FINISHED|SUSPENDED)
			log_error "PCAP consumer entered terminal state ${state}"
			exit 1
		esac
		sleep 2
	done
	if [[ "${state}" != "RUNNING" ]]; then
		"${cmd[0]}" cancel "${job_id}" >/dev/null 2>&1 || true
		log_error "PCAP consumer readiness timed out; job canceled"
		exit 1
	fi
	checkpoint_id=""
	while (( SECONDS < deadline )); do
		checkpoint_id="$(curl --fail --silent --show-error "${FLINK_REST_URL%/}/jobs/${job_id}/checkpoints" | python3 -c 'import json,sys; data=json.load(sys.stdin); latest=data.get("latest",{}).get("completed"); print("" if not latest else latest.get("id", ""))')" || checkpoint_id=""
		[[ "${checkpoint_id}" =~ ^[0-9]+$ ]] && break
		sleep 2
	done
	if [[ ! "${checkpoint_id}" =~ ^[0-9]+$ ]]; then
		"${cmd[0]}" cancel "${job_id}" >/dev/null 2>&1 || true
		log_error "PCAP consumer produced no completed checkpoint; job canceled"
		exit 1
	fi
	ready_observed_at="$(date -u +%Y-%m-%dT%H:%M:%SZ)"

	export JOB_ID="${job_id}" CHECKPOINT_ID="${checkpoint_id}" READY_OBSERVED_AT="${ready_observed_at}" JOB_JAR KAFKA_BROKERS M02_READINESS_RECEIPT_PATH M02_ROLLOUT_ID M02_CANDIDATE_ID
	python3 - <<'PY'
import hashlib, json, os, pathlib, tempfile
target = pathlib.Path(os.environ["M02_READINESS_RECEIPT_PATH"])
target.parent.mkdir(parents=True, exist_ok=True)
body = {
    "schema_version": 1,
    "artifact_kind": "M02_CONSUMER_READINESS_RECEIPT",
    "rollout_id": os.environ["M02_ROLLOUT_ID"],
    "candidate_id": os.environ["M02_CANDIDATE_ID"],
    "consumer": "flink-pcap-index-job",
    "job_id": os.environ["JOB_ID"],
    "completed_checkpoint_id": int(os.environ["CHECKPOINT_ID"]),
    "state": "RUNNING",
    "activation": "CONSUMER_FIRST_IDLE",
    "input_topics": ["pcap.index.v1"],
    "output_topics": ["dlq.v1"],
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
	log_info "PCAP consumer RUNNING receipt: ${M02_READINESS_RECEIPT_PATH}"
}

main() {
    local do_build=false
    local detached=false
    local local_mode=false

    while [[ $# -gt 0 ]]; do
        case $1 in
            --build)
                do_build=true
                shift
                ;;
            --local)
                local_mode=true
                shift
                ;;
            -d|--detached)
                detached=true
                shift
                ;;
            --parallelism)
                PARALLELISM="$2"
                shift 2
                ;;
            -h|--help)
                show_usage
                exit 0
                ;;
            *)
                log_error "Unknown option: $1"
                show_usage
                exit 1
                ;;
        esac
    done

    if [[ "${do_build}" == "true" ]]; then
        build_project
    fi

    check_prerequisites
    submit_job "${detached}" "${local_mode}"
}

main "$@"
