#!/bin/bash
################################################################################
# Entrypoint Script for Flink Alert Generator Job
#
# 功能：
# 1. 等待依赖服务就绪（Kafka、ClickHouse、OpenSearch）
# 2. 动态生成 Flink 配置
# 3. 启动 Flink Job
#
# 环境变量说明见 Dockerfile
################################################################################

set -euo pipefail

# ==================== 颜色输出 ====================
readonly RED='\033[0;31m'
readonly GREEN='\033[0;32m'
readonly YELLOW='\033[1;33m'
readonly BLUE='\033[0;34m'
readonly NC='\033[0m'

log_info()  { echo -e "${GREEN}[INFO]${NC}  $(date +'%Y-%m-%d %H:%M:%S') $*"; }
log_warn()  { echo -e "${YELLOW}[WARN]${NC}  $(date +'%Y-%m-%d %H:%M:%S') $*"; }
log_error() { echo -e "${RED}[ERROR]${NC} $(date +'%Y-%m-%d %H:%M:%S') $*"; }
log_debug() { [[ "${DEBUG_PRINT:-false}" == "true" ]] && echo -e "${BLUE}[DEBUG]${NC} $(date +'%Y-%m-%d %H:%M:%S') $*" || true; }

# ==================== 服务健康检查 ====================
wait_for_service() {
    local host=$1
    local port=$2
    local service_name=$3
    local max_attempts=${4:-30}
    local attempt=0

    log_info "Waiting for ${service_name} at ${host}:${port}..."

    while [[ $attempt -lt $max_attempts ]]; do
        if nc -z -w5 "$host" "$port" 2>/dev/null; then
            log_info "✓ ${service_name} is ready!"
            return 0
        fi

        attempt=$((attempt + 1))
        log_warn "Attempt $attempt/$max_attempts: ${service_name} not ready, retrying in 5s..."
        sleep 5
    done

    log_error "✗ Timeout waiting for ${service_name} at ${host}:${port}"
    return 1
}

# ==================== 解析服务地址 ====================
parse_kafka_address() {
    local brokers="${KAFKA_BROKERS:-localhost:9092}"
    # 取第一个 broker
    local first_broker="${brokers%%,*}"
    KAFKA_HOST="${first_broker%:*}"
    KAFKA_PORT="${first_broker##*:}"
    log_debug "Kafka parsed: host=$KAFKA_HOST, port=$KAFKA_PORT"
}

parse_clickhouse_address() {
    local url="${CLICKHOUSE_URL:-localhost:8123}"
    CLICKHOUSE_HOST="${url%:*}"
    CLICKHOUSE_PORT="${url##*:}"
    log_debug "ClickHouse parsed: host=$CLICKHOUSE_HOST, port=$CLICKHOUSE_PORT"
}

parse_opensearch_address() {
    local url="${OPENSEARCH_URL:-http://localhost:9200}"
    # 移除协议前缀
    local addr="${url#*://}"
    # 移除路径后缀
    addr="${addr%%/*}"
    OPENSEARCH_HOST="${addr%:*}"
    OPENSEARCH_PORT="${addr##*:}"
    log_debug "OpenSearch parsed: host=$OPENSEARCH_HOST, port=$OPENSEARCH_PORT"
}

# ==================== 等待依赖服务 ====================
wait_for_dependencies() {
    if [[ "${SKIP_HEALTH_CHECK:-false}" == "true" ]]; then
        log_warn "Health check skipped (SKIP_HEALTH_CHECK=true)"
        return 0
    fi

    parse_kafka_address
    parse_clickhouse_address
    parse_opensearch_address

    wait_for_service "$KAFKA_HOST" "$KAFKA_PORT" "Kafka" 60 || exit 1
    wait_for_service "$CLICKHOUSE_HOST" "$CLICKHOUSE_PORT" "ClickHouse" 30 || exit 1
    wait_for_service "$OPENSEARCH_HOST" "$OPENSEARCH_PORT" "OpenSearch" 30 || exit 1

    log_info "All dependencies are ready!"
}

# ==================== 生成 Flink 配置 ====================
generate_flink_config() {
    log_info "Generating Flink configuration..."

    local jm_host="${FLINK_JM_HOST:-localhost}"
    local jm_memory="${FLINK_JM_MEMORY:-2048m}"
    local tm_memory="${FLINK_TM_MEMORY:-4096m}"
    local task_slots="${FLINK_TASK_SLOTS:-4}"
    local parallelism="${FLINK_PARALLELISM:-4}"
    local checkpoint_interval="${FLINK_CHECKPOINT_INTERVAL:-60000}"
    local checkpoint_timeout="${FLINK_CHECKPOINT_TIMEOUT:-120000}"
    local checkpoint_dir="${FLINK_CHECKPOINT_DIR:-file:///opt/flink/checkpoints}"
    local savepoint_dir="${FLINK_SAVEPOINT_DIR:-file:///opt/flink/savepoints}"

    cat > /opt/flink/conf/flink-conf.yaml <<EOF
# ==================== Flink 基础配置 ====================
jobmanager.rpc.address: ${jm_host}
jobmanager.rpc.port: 6123
jobmanager.memory.process.size: ${jm_memory}

taskmanager.memory.process.size: ${tm_memory}
taskmanager.numberOfTaskSlots: ${task_slots}

parallelism.default: ${parallelism}

# ==================== Checkpoint 配置 ====================
execution.checkpointing.interval: ${checkpoint_interval}
execution.checkpointing.mode: EXACTLY_ONCE
execution.checkpointing.timeout: ${checkpoint_timeout}
execution.checkpointing.min-pause: 30000
execution.checkpointing.max-concurrent-checkpoints: 1
execution.checkpointing.externalized-checkpoint-retention: RETAIN_ON_CANCELLATION
execution.checkpointing.unaligned: true

# ==================== State Backend ====================
state.backend: rocksdb
state.backend.incremental: true
state.checkpoints.dir: ${checkpoint_dir}
state.savepoints.dir: ${savepoint_dir}

# ==================== RocksDB 优化（针对大内存节点）====================
state.backend.rocksdb.predefined-options: SPINNING_DISK_OPTIMIZED_HIGH_MEM
state.backend.rocksdb.checkpoint.transfer.thread.num: 4
state.backend.rocksdb.writebuffer.size: 128mb
state.backend.rocksdb.writebuffer.count: 4
state.backend.rocksdb.block.cache-size: 256mb

# ==================== 重启策略 ====================
restart-strategy: fixed-delay
restart-strategy.fixed-delay.attempts: 5
restart-strategy.fixed-delay.delay: 30s

# ==================== 网络配置 ====================
taskmanager.network.memory.fraction: 0.15
taskmanager.network.memory.min: 128mb
taskmanager.network.memory.max: 1gb

# ==================== Metrics ====================
metrics.reporter.prom.factory.class: org.apache.flink.metrics.prometheus.PrometheusReporterFactory
metrics.reporter.prom.port: 9249
metrics.reporter.prom.interval: 15 SECONDS

# ==================== Web UI ====================
web.submit.enable: true
web.upload.dir: /opt/flink/usrlib
web.timeout: 120000

# ==================== Classpath ====================
classloader.resolve-order: parent-first
classloader.check-leaked-classloader: false

# ==================== 日志配置 ====================
env.log.dir: /opt/flink/log
env.log.max: 10

# ==================== 高级配置 ====================
akka.ask.timeout: 60s
akka.tcp.timeout: 60s
heartbeat.interval: 10000
heartbeat.timeout: 50000
EOF

    log_info "Flink configuration generated: /opt/flink/conf/flink-conf.yaml"
}

# ==================== 生成作业配置 ====================
generate_job_config() {
    log_info "Generating job configuration..."

    cat > /opt/flink/config/alert-generator-job.properties <<EOF
# ==================== Auto-generated Job Configuration ====================
# Generated at: $(date -Iseconds)

# ==================== Kafka 配置 ====================
kafka.brokers=${KAFKA_BROKERS:-localhost:9092}
kafka.input.topic.behavior=${KAFKA_INPUT_TOPIC_BEHAVIOR:-detections.behavior.v1}
kafka.input.topic.business=${KAFKA_INPUT_TOPIC_BUSINESS:-detections.business.v1}
kafka.output.topic=${KAFKA_OUTPUT_TOPIC:-alerts.v1}
kafka.group.id=${KAFKA_GROUP_ID:-flink-alert-generator-job}

# ==================== ClickHouse 配置 ====================
clickhouse.url=${CLICKHOUSE_URL:-localhost:8123}
clickhouse.database=${CLICKHOUSE_DB:-traffic}
clickhouse.alert.table=${CLICKHOUSE_ALERT_TABLE:-alerts_local}
clickhouse.evidence.table=${CLICKHOUSE_EVIDENCE_TABLE:-evidence_local}
clickhouse.user=${CLICKHOUSE_USER:-default}
clickhouse.password=${CLICKHOUSE_PASSWORD:-}

# ==================== OpenSearch 配置 ====================
opensearch.url=${OPENSEARCH_URL:-http://localhost:9200}
opensearch.index=${OPENSEARCH_INDEX:-alerts}
opensearch.user=${OPENSEARCH_USER:-admin}
opensearch.password=${OPENSEARCH_PASSWORD:?set OPENSEARCH_PASSWORD}

# ==================== Arkime 配置 ====================
arkime.url=${ARKIME_URL:-http://localhost:8005}
arkime.time.buffer.seconds=${ARKIME_TIME_BUFFER:-120}

# ==================== Checkpoint 配置 ====================
checkpoint.path=${FLINK_CHECKPOINT_DIR:-file:///opt/flink/checkpoints}
checkpoint.interval.ms=${FLINK_CHECKPOINT_INTERVAL:-60000}
checkpoint.timeout.ms=${FLINK_CHECKPOINT_TIMEOUT:-120000}

# ==================== 去重配置 ====================
dedup.window.minutes=${DEDUP_WINDOW_MINUTES:-10}

# ==================== Severity 阈值配置 ====================
severity.threshold.critical=${SEVERITY_CRITICAL:-0.9}
severity.threshold.high=${SEVERITY_HIGH:-0.7}
severity.threshold.medium=${SEVERITY_MEDIUM:-0.5}
severity.threshold.low=${SEVERITY_LOW:-0.3}

# ==================== 作业配置 ====================
parallelism=${FLINK_PARALLELISM:-4}
sink.parallelism=${SINK_PARALLELISM:-2}

# ==================== 业务检测开关 ====================
enable.business.detection=${ENABLE_BUSINESS_DETECTION:-true}

# ==================== 调试配置 ====================
debug.print=${DEBUG_PRINT:-false}
EOF

    log_info "Job configuration generated: /opt/flink/config/alert-generator-job.properties"
}

# ==================== 打印配置摘要 ====================
print_config_summary() {
    log_info "============================================================"
    log_info "         Alert Generator Job Configuration Summary"
    log_info "============================================================"
    log_info "Job Name:            ${FLINK_JOB_NAME}"
    log_info "Parallelism:         ${FLINK_PARALLELISM:-4} (sink: ${SINK_PARALLELISM:-2})"
    log_info "------------------------------------------------------------"
    log_info "Kafka Brokers:       ${KAFKA_BROKERS}"
    log_info "  Input (Behavior):  ${KAFKA_INPUT_TOPIC_BEHAVIOR}"
    log_info "  Input (Business):  ${KAFKA_INPUT_TOPIC_BUSINESS} (enabled: ${ENABLE_BUSINESS_DETECTION})"
    log_info "  Output:            ${KAFKA_OUTPUT_TOPIC}"
    log_info "------------------------------------------------------------"
    log_info "ClickHouse:          ${CLICKHOUSE_URL}/${CLICKHOUSE_DB}"
    log_info "  Alert Table:       ${CLICKHOUSE_ALERT_TABLE}"
    log_info "  Evidence Table:    ${CLICKHOUSE_EVIDENCE_TABLE}"
    log_info "------------------------------------------------------------"
    log_info "OpenSearch:          ${OPENSEARCH_URL}/${OPENSEARCH_INDEX}"
    log_info "------------------------------------------------------------"
    log_info "Arkime:              ${ARKIME_URL} (buffer: ${ARKIME_TIME_BUFFER}s)"
    log_info "------------------------------------------------------------"
    log_info "Dedup Window:        ${DEDUP_WINDOW_MINUTES} minutes"
    log_info "Severity Thresholds: C=${SEVERITY_CRITICAL} H=${SEVERITY_HIGH} M=${SEVERITY_MEDIUM} L=${SEVERITY_LOW}"
    log_info "------------------------------------------------------------"
    log_info "Checkpoint:          ${FLINK_CHECKPOINT_DIR}"
    log_info "  Interval:          ${FLINK_CHECKPOINT_INTERVAL}ms"
    log_info "  Timeout:           ${FLINK_CHECKPOINT_TIMEOUT}ms"
    log_info "============================================================"
}

# ==================== 主流程 ====================
main() {
    log_info "Starting Alert Generator Job entrypoint..."

    # 等待依赖服务
    wait_for_dependencies

    # 生成配置
    generate_flink_config
    generate_job_config

    # 打印配置摘要
    print_config_summary

    # 执行原始 Flink 入口点
    log_info "Launching Flink with command: $*"
    exec /docker-entrypoint.sh "$@"
}

main "$@"
