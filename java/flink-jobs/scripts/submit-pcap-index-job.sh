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

    log_info "Prerequisites check passed"
}

submit_job() {
    local detached="$1"
    local local_mode="$2"

    log_info "Submitting Flink job..."
    log_info "  Job Name: ${JOB_NAME}"
    log_info "  JAR: ${JOB_JAR}"
    log_info "  Parallelism: ${PARALLELISM}"

    local cmd=""
    
    if [[ "${local_mode}" == "true" ]]; then
        cmd="java -cp ${JOB_JAR} ${MAIN_CLASS}"
    else
        if [[ -f "${FLINK_HOME}/bin/flink" ]]; then
            cmd="${FLINK_HOME}/bin/flink run"
        else
            cmd="flink run"
        fi

        if [[ "${detached}" == "true" ]]; then
            cmd="${cmd} -d"
        fi

        cmd="${cmd} -m ${FLINK_MASTER}"
        cmd="${cmd} -p ${PARALLELISM}"
        cmd="${cmd} -c ${MAIN_CLASS}"
        cmd="${cmd} ${JOB_JAR}"
    fi

    cmd="${cmd} \
        --kafka.brokers ${KAFKA_BROKERS} \
        --clickhouse.url ${CLICKHOUSE_URL} \
        --checkpoint.path ${CHECKPOINT_PATH} \
        --parallelism ${PARALLELISM}"

    log_info "Executing: ${cmd} --clickhouse.password <redacted>"
    eval "${cmd} --clickhouse.password \"\$CLICKHOUSE_PASSWORD\""
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
