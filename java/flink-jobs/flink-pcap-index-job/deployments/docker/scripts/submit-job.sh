#!/bin/bash
# =============================================================================
# Flink PCAP Index Job 提交脚本
# =============================================================================
# 
# 用法:
#   ./submit-job.sh [环境]
#   
# 环境:
#   local   - 本地开发环境（默认）
#   dev     - 开发环境
#   staging - 预发布环境
#   prod    - 生产环境
#
# 示例:
#   ./submit-job.sh local
#   ./submit-job.sh prod
#
# =============================================================================

set -euo pipefail

# ==================== 配置 ====================
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="${SCRIPT_DIR}/../../../.."

# 作业信息
JOB_NAME="PCAP Index Job v2"
JOB_CLASS="com.traffic.flink.pcap.PcapIndexJob"
JOB_JAR="flink-pcap-index-job-1.0.0.jar"

# Flink 配置
FLINK_REST_URL="${FLINK_REST_URL:-http://localhost:8081}"

# 环境配置
ENV="${1:-local}"

# ==================== 环境配置映射 ====================
case "$ENV" in
    local)
        KAFKA_BROKERS="kafka:29092"
        KAFKA_INPUT_TOPIC="pcap.index.v1"
        KAFKA_DLQ_TOPIC="dlq.pcap-index-job"
        CLICKHOUSE_URL="clickhouse:8123"
        CLICKHOUSE_DATABASE="traffic"
        CLICKHOUSE_TABLE="pcap_index_local"
        PARALLELISM=2
        CHECKPOINT_PATH="s3://flink-checkpoints/checkpoints/pcap-index-job"
        ;;
    dev)
        KAFKA_BROKERS="${KAFKA_BROKERS:-10.0.5.8:9092,10.0.5.9:9092}"
        KAFKA_INPUT_TOPIC="pcap.index.v1"
        KAFKA_DLQ_TOPIC="dlq.pcap-index-job"
        CLICKHOUSE_URL="${CLICKHOUSE_URL:-10.0.5.8:8123}"
        CLICKHOUSE_DATABASE="traffic"
        CLICKHOUSE_TABLE="pcap_index_local"
        PARALLELISM=4
        CHECKPOINT_PATH="s3://flink-checkpoints/checkpoints/pcap-index-job"
        ;;
    staging)
        KAFKA_BROKERS="${KAFKA_BROKERS:-kafka-staging:9092}"
        KAFKA_INPUT_TOPIC="pcap.index.v1"
        KAFKA_DLQ_TOPIC="dlq.pcap-index-job"
        CLICKHOUSE_URL="${CLICKHOUSE_URL:-clickhouse-staging:8123}"
        CLICKHOUSE_DATABASE="traffic"
        CLICKHOUSE_TABLE="pcap_index_local"
        PARALLELISM=4
        CHECKPOINT_PATH="s3://flink-checkpoints/checkpoints/pcap-index-job"
        ;;
    prod)
        KAFKA_BROKERS="${KAFKA_BROKERS:-kafka-1:9092,kafka-2:9092,kafka-3:9092}"
        KAFKA_INPUT_TOPIC="pcap.index.v1"
        KAFKA_DLQ_TOPIC="dlq.pcap-index-job"
        CLICKHOUSE_URL="${CLICKHOUSE_URL:-clickhouse-cluster:8123}"
        CLICKHOUSE_DATABASE="traffic"
        CLICKHOUSE_TABLE="pcap_index_local"
        PARALLELISM=8
        CHECKPOINT_PATH="s3://flink-checkpoints/checkpoints/pcap-index-job"
        ;;
    *)
        echo "未知环境: $ENV"
        echo "支持的环境: local, dev, staging, prod"
        exit 1
        ;;
esac

# ==================== 颜色输出 ====================
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

log_info() {
    echo -e "${BLUE}[INFO]${NC} $1"
}

log_success() {
    echo -e "${GREEN}[SUCCESS]${NC} $1"
}

log_warn() {
    echo -e "${YELLOW}[WARN]${NC} $1"
}

log_error() {
    echo -e "${RED}[ERROR]${NC} $1"
}

# ==================== 检查 Flink 集群 ====================
check_flink_cluster() {
    log_info "检查 Flink 集群状态: ${FLINK_REST_URL}"
    
    if ! curl -sf "${FLINK_REST_URL}/overview" > /dev/null 2>&1; then
        log_error "无法连接到 Flink 集群: ${FLINK_REST_URL}"
        exit 1
    fi
    
    TASKMANAGERS=$(curl -sf "${FLINK_REST_URL}/overview" | jq -r '.taskmanagers')
    SLOTS_AVAILABLE=$(curl -sf "${FLINK_REST_URL}/overview" | jq -r '.["slots-available"]')
    
    log_info "TaskManagers: ${TASKMANAGERS}, 可用 Slots: ${SLOTS_AVAILABLE}"
    
    if [ "$SLOTS_AVAILABLE" -lt "$PARALLELISM" ]; then
        log_warn "可用 Slots ($SLOTS_AVAILABLE) 小于并行度 ($PARALLELISM)"
    fi
}

# ==================== 检查现有作业 ====================
check_existing_jobs() {
    log_info "检查是否存在同名作业..."
    
    EXISTING_JOBS=$(curl -sf "${FLINK_REST_URL}/jobs" | jq -r '.jobs[] | select(.status == "RUNNING") | .id')
    
    if [ -n "$EXISTING_JOBS" ]; then
        log_warn "发现正在运行的作业: $EXISTING_JOBS"
        read -p "是否取消现有作业并重新提交? (y/N): " -n 1 -r
        echo
        if [[ $REPLY =~ ^[Yy]$ ]]; then
            for job_id in $EXISTING_JOBS; do
                log_info "取消作业: $job_id"
                curl -sf -X PATCH "${FLINK_REST_URL}/jobs/${job_id}?mode=cancel" || true
            done
            sleep 5
        else
            log_info "保留现有作业，退出"
            exit 0
        fi
    fi
}

# ==================== 提交作业 ====================
submit_job() {
    log_info "提交作业: ${JOB_NAME}"
    log_info "环境: ${ENV}"
    log_info "Kafka: ${KAFKA_BROKERS}"
    log_info "ClickHouse: ${CLICKHOUSE_URL}"
    log_info "并行度: ${PARALLELISM}"
    
    # 构建参数
    ARGS=(
        "--kafka.brokers" "${KAFKA_BROKERS}"
        "--kafka.input.topic" "${KAFKA_INPUT_TOPIC}"
        "--kafka.dlq.topic" "${KAFKA_DLQ_TOPIC}"
        "--clickhouse.url" "${CLICKHOUSE_URL}"
        "--clickhouse.database" "${CLICKHOUSE_DATABASE}"
        "--clickhouse.table" "${CLICKHOUSE_TABLE}"
        "--checkpoint.path" "${CHECKPOINT_PATH}"
        "--parallelism" "${PARALLELISM}"
    )
    
    # 添加可选参数
    if [ -n "${CLICKHOUSE_USER:-}" ]; then
        ARGS+=("--clickhouse.user" "${CLICKHOUSE_USER}")
    fi
    
    if [ -n "${CLICKHOUSE_PASSWORD:-}" ]; then
        ARGS+=("--clickhouse.password" "${CLICKHOUSE_PASSWORD}")
    fi
    
    # 本地开发环境添加调试输出
    if [ "$ENV" == "local" ]; then
        ARGS+=("--debug.print" "true")
    fi
    
    # 使用 docker-compose 提交（本地环境）
    if [ "$ENV" == "local" ]; then
        log_info "使用 docker-compose 提交作业..."
        
        docker-compose -f "${SCRIPT_DIR}/../docker-compose.yml" exec -T flink-jobmanager \
            flink run \
            --detached \
            --class "${JOB_CLASS}" \
            /opt/flink/usrlib/${JOB_JAR} \
            "${ARGS[@]}"
    else
        # 远程提交
        log_info "使用 Flink CLI 远程提交作业..."
        
        flink run \
            --detached \
            --target remote \
            --jobmanager "${FLINK_REST_URL}" \
            --class "${JOB_CLASS}" \
            "${PROJECT_ROOT}/flink-jobs/flink-pcap-index-job/target/${JOB_JAR}" \
            "${ARGS[@]}"
    fi
    
    if [ $? -eq 0 ]; then
        log_success "作业提交成功！"
    else
        log_error "作业提交失败"
        exit 1
    fi
}

# ==================== 验证作业状态 ====================
verify_job() {
    log_info "验证作业状态..."
    
    sleep 5
    
    RUNNING_JOBS=$(curl -sf "${FLINK_REST_URL}/jobs" | jq -r '[.jobs[] | select(.status == "RUNNING")] | length')
    
    if [ "$RUNNING_JOBS" -gt 0 ]; then
        log_success "作业正在运行"
        
        # 显示作业信息
        curl -sf "${FLINK_REST_URL}/jobs" | jq '.jobs[] | select(.status == "RUNNING")'
    else
        log_warn "未检测到运行中的作业，请检查 Flink Web UI"
    fi
}

# ==================== 主流程 ====================
main() {
    echo "==========================================="
    echo "  Flink PCAP Index Job 提交工具"
    echo "==========================================="
    echo ""
    
    check_flink_cluster
    check_existing_jobs
    submit_job
    verify_job
    
    echo ""
    log_success "完成！"
    log_info "Flink Web UI: ${FLINK_REST_URL}"
    log_info "Grafana Dashboard: http://localhost:3000/d/pcap-index-overview"
}

main "$@"
