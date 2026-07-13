#!/usr/bin/env bash
# java/flink-jobs/scripts/submit-feature-job.sh
# Flink Feature Job 提交脚本

set -euo pipefail

# =============================================================================
# 配置
# =============================================================================

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_DIR="$(cd "${SCRIPT_DIR}/.." && pwd)"
. "${SCRIPT_DIR}/clickhouse-password.sh"

# Flink 配置
FLINK_HOME="${FLINK_HOME:-/opt/flink}"
FLINK_MASTER="${FLINK_MASTER:-localhost:8081}"

# 作业配置
JOB_NAME="feature-extraction-job"
JOB_JAR="${PROJECT_DIR}/flink-feature-job/target/flink-feature-job-1.0.0-SNAPSHOT.jar"
MAIN_CLASS="com.traffic.flink.feature.FeatureJob"

# 默认参数
PARALLELISM="${PARALLELISM:-4}"
KAFKA_BROKERS="${KAFKA_BROKERS:-kafka-bootstrap.middleware.svc:9092}"
CLICKHOUSE_URL="${CLICKHOUSE_URL:-localhost:8123}"
CHECKPOINT_PATH="${CHECKPOINT_PATH:-s3://flink-checkpoints/checkpoints/feature-job}"
POSTGRES_URL="${POSTGRES_URL:-jdbc:postgresql://postgres-primary.databases.svc:5432/traffic_platform}"
POSTGRES_USER="${POSTGRES_USER:-postgres}"
resolve_clickhouse_password
if [[ -z "${POSTGRES_PASSWORD:-}" ]]; then
    echo "ERROR: POSTGRES_PASSWORD is required" >&2
    exit 1
fi

# 颜色
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m'

# =============================================================================
# 函数
# =============================================================================

log_info() {
    echo -e "${GREEN}[INFO]${NC} $1"
}

log_warn() {
    echo -e "${YELLOW}[WARN]${NC} $1"
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
    echo "  --parallelism N   Set parallelism (default: 4)"
    echo "  --help, -h        Show this help message"
    echo ""
    echo "Environment Variables:"
    echo "  FLINK_HOME        Flink installation directory"
    echo "  FLINK_MASTER      Flink JobManager address"
    echo "  KAFKA_BROKERS     Kafka bootstrap servers"
    echo "  CLICKHOUSE_URL    ClickHouse server URL"
    echo "  CHECKPOINT_PATH   Checkpoint storage path"
    echo ""
}

build_project() {
    log_info "Building project..."
    cd "${PROJECT_DIR}"
    mvn clean package -DskipTests -pl flink-feature-job -am
    log_info "Build completed"
}

check_prerequisites() {
    log_info "Checking prerequisites..."

    # 检查 JAR 文件
    if [[ ! -f "${JOB_JAR}" ]]; then
        log_error "Job JAR not found: ${JOB_JAR}"
        log_info "Please build the project first: $0 --build"
        exit 1
    fi

    # 检查 Flink
    if [[ ! -d "${FLINK_HOME}" ]]; then
        log_warn "FLINK_HOME not set or not found, using PATH"
    fi

    log_info "Prerequisites check passed"
}

submit_job() {
    local detached="$1"
    local local_mode="$2"

    log_info "Submitting Flink job..."
    log_info "  Job Name: ${JOB_NAME}"
    log_info "  JAR: ${JOB_JAR}"
    log_info "  Main Class: ${MAIN_CLASS}"
    log_info "  Parallelism: ${PARALLELISM}"
    log_info "  Kafka: ${KAFKA_BROKERS}"
    log_info "  ClickHouse: ${CLICKHOUSE_URL}"

    # 构建命令
    local cmd=""
    
    if [[ "${local_mode}" == "true" ]]; then
        # 本地模式
        cmd="java -cp ${JOB_JAR} ${MAIN_CLASS}"
    else
        # 集群模式
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

    # 添加作业参数
    cmd="${cmd} \
        --kafka.brokers ${KAFKA_BROKERS} \
        --clickhouse.url ${CLICKHOUSE_URL} \
        --postgres.url ${POSTGRES_URL} \
        --postgres.user ${POSTGRES_USER} \
        --checkpoint.path ${CHECKPOINT_PATH} \
        --parallelism ${PARALLELISM}"

    log_info "Executing: ${cmd} --postgres.password <redacted> --clickhouse.password <redacted>"
    eval "${cmd} --postgres.password \"\$POSTGRES_PASSWORD\" --clickhouse.password \"\$CLICKHOUSE_PASSWORD\""

    if [[ $? -eq 0 ]]; then
        log_info "Job submitted successfully"
    else
        log_error "Job submission failed"
        exit 1
    fi
}

# =============================================================================
# 主程序
# =============================================================================

main() {
    local do_build=false
    local detached=false
    local local_mode=false

    # 解析参数
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

    # 执行步骤
    if [[ "${do_build}" == "true" ]]; then
        build_project
    fi

    check_prerequisites
    submit_job "${detached}" "${local_mode}"
}

main "$@"
