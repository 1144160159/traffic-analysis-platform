#!/bin/bash
# =============================================================================
# Flink CEP Job Docker Entrypoint
# =============================================================================

set -e

# 颜色输出
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m'

log_info() {
    echo -e "${GREEN}[INFO]${NC} $(date '+%Y-%m-%d %H:%M:%S') $1"
}

log_warn() {
    echo -e "${YELLOW}[WARN]${NC} $(date '+%Y-%m-%d %H:%M:%S') $1"
}

log_error() {
    echo -e "${RED}[ERROR]${NC} $(date '+%Y-%m-%d %H:%M:%S') $1"
}

# 等待服务可用
wait_for_service() {
    local host=$1
    local port=$2
    local service_name=$3
    local max_attempts=${4:-30}
    local attempt=1

    log_info "Waiting for $service_name ($host:$port)..."
    
    while ! nc -z "$host" "$port" 2>/dev/null; do
        if [ $attempt -ge $max_attempts ]; then
            log_error "$service_name is not available after $max_attempts attempts"
            return 1
        fi
        log_info "Attempt $attempt/$max_attempts: $service_name not ready, waiting..."
        sleep 2
        ((attempt++))
    done
    
    log_info "$service_name is available"
    return 0
}

# 环境变量默认值
export FLINK_MODE=${FLINK_MODE:-"standalone"}
export JOB_MANAGER_RPC_ADDRESS=${JOB_MANAGER_RPC_ADDRESS:-"jobmanager"}
export KAFKA_BROKERS=${KAFKA_BROKERS:-"kafka:9092"}
export CLICKHOUSE_URL=${CLICKHOUSE_URL:-"clickhouse:8123"}

# 更新 Flink 配置
update_flink_conf() {
    log_info "Updating Flink configuration..."
    
    # JobManager 地址
    sed -i "s/jobmanager.rpc.address:.*/jobmanager.rpc.address: ${JOB_MANAGER_RPC_ADDRESS}/" /opt/flink/conf/flink-conf.yaml
    
    # TaskManager 内存
    if [ -n "$TASK_MANAGER_MEMORY" ]; then
        sed -i "s/taskmanager.memory.process.size:.*/taskmanager.memory.process.size: ${TASK_MANAGER_MEMORY}/" /opt/flink/conf/flink-conf.yaml
    fi
    
    # JobManager 内存
    if [ -n "$JOB_MANAGER_MEMORY" ]; then
        sed -i "s/jobmanager.memory.process.size:.*/jobmanager.memory.process.size: ${JOB_MANAGER_MEMORY}/" /opt/flink/conf/flink-conf.yaml
    fi
    
    # TaskSlots
    if [ -n "$TASK_SLOTS" ]; then
        sed -i "s/taskmanager.numberOfTaskSlots:.*/taskmanager.numberOfTaskSlots: ${TASK_SLOTS}/" /opt/flink/conf/flink-conf.yaml
    fi
    
    # Parallelism
    if [ -n "$PARALLELISM" ]; then
        sed -i "s/parallelism.default:.*/parallelism.default: ${PARALLELISM}/" /opt/flink/conf/flink-conf.yaml
    fi
    
    # Checkpoint 目录 (S3)
    if [ -n "$CHECKPOINT_DIR" ]; then
        sed -i "s|state.checkpoints.dir:.*|state.checkpoints.dir: ${CHECKPOINT_DIR}|" /opt/flink/conf/flink-conf.yaml
    fi
    
    log_info "Flink configuration updated"
}

# 启动 JobManager
start_jobmanager() {
    log_info "Starting Flink JobManager..."
    
    update_flink_conf
    
    exec /opt/flink/bin/jobmanager.sh start-foreground
}

# 启动 TaskManager
start_taskmanager() {
    log_info "Starting Flink TaskManager..."
    
    update_flink_conf
    
    # 等待 JobManager
    wait_for_service "$JOB_MANAGER_RPC_ADDRESS" 6123 "JobManager" 60
    
    exec /opt/flink/bin/taskmanager.sh start-foreground
}

# 提交作业
submit_job() {
    log_info "Submitting CEP Job..."
    
    # 等待 JobManager REST API
    wait_for_service "$JOB_MANAGER_RPC_ADDRESS" 8081 "JobManager REST API" 60
    
    # 等待 Kafka
    KAFKA_HOST=$(echo $KAFKA_BROKERS | cut -d: -f1 | cut -d, -f1)
    KAFKA_PORT=$(echo $KAFKA_BROKERS | cut -d: -f2 | cut -d, -f1)
    wait_for_service "$KAFKA_HOST" "${KAFKA_PORT:-9092}" "Kafka" 60
    
    # 构建作业参数
    JOB_ARGS=""
    JOB_ARGS="$JOB_ARGS --kafka.brokers $KAFKA_BROKERS"
    JOB_ARGS="$JOB_ARGS --clickhouse.url $CLICKHOUSE_URL"
    
    if [ -n "$KAFKA_INPUT_TOPIC" ]; then
        JOB_ARGS="$JOB_ARGS --kafka.input.topic $KAFKA_INPUT_TOPIC"
    fi
    
    if [ -n "$KAFKA_OUTPUT_TOPIC" ]; then
        JOB_ARGS="$JOB_ARGS --kafka.output.topic $KAFKA_OUTPUT_TOPIC"
    fi
    
    if [ -n "$CLICKHOUSE_DATABASE" ]; then
        JOB_ARGS="$JOB_ARGS --clickhouse.database $CLICKHOUSE_DATABASE"
    fi
    
    if [ -n "$CLICKHOUSE_USER" ]; then
        JOB_ARGS="$JOB_ARGS --clickhouse.user $CLICKHOUSE_USER"
    fi
    
    if [ -n "$CLICKHOUSE_PASSWORD" ]; then
        JOB_ARGS="$JOB_ARGS --clickhouse.password $CLICKHOUSE_PASSWORD"
    fi
    
    if [ -n "$PARALLELISM" ]; then
        JOB_ARGS="$JOB_ARGS --parallelism $PARALLELISM"
    fi
    
    if [ -n "$CHECKPOINT_PATH" ]; then
        JOB_ARGS="$JOB_ARGS --checkpoint.path $CHECKPOINT_PATH"
    fi
    
    log_info "Job arguments: $JOB_ARGS"
    
    # 提交作业
    /opt/flink/bin/flink run \
        -d \
        -c com.traffic.flink.cep.CepJob \
        ${JOB_JAR_PATH} \
        $JOB_ARGS
    
    log_info "CEP Job submitted successfully"
    
    # 保持容器运行
    tail -f /dev/null
}

# Standalone 模式 (单节点)
start_standalone() {
    log_info "Starting Flink in standalone mode..."
    
    update_flink_conf
    
    # 启动集群
    /opt/flink/bin/start-cluster.sh
    
    log_info "Flink cluster started, waiting for initialization..."
    sleep 10
    
    # 提交作业
    submit_job
}

# 主入口
main() {
    log_info "=== Flink CEP Job Container Starting ==="
    log_info "Mode: $FLINK_MODE"
    log_info "Kafka: $KAFKA_BROKERS"
    log_info "ClickHouse: $CLICKHOUSE_URL"
    
    case "$FLINK_MODE" in
        "jobmanager")
            start_jobmanager
            ;;
        "taskmanager")
            start_taskmanager
            ;;
        "submit")
            submit_job
            ;;
        "standalone")
            start_standalone
            ;;
        *)
            log_error "Unknown mode: $FLINK_MODE"
            log_info "Available modes: jobmanager, taskmanager, submit, standalone"
            exit 1
            ;;
    esac
}

# 处理信号
trap 'log_info "Received shutdown signal"; exit 0' SIGTERM SIGINT

main "$@"
