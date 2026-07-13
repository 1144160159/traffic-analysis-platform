#!/bin/bash
# ============================================================================
# Flink Rule Job Docker Entrypoint
# ============================================================================

set -e

FLINK_HOME="${FLINK_HOME:-/opt/flink}"
JOB_JAR="${JOB_JAR:-/opt/flink/usrlib/flink-rule-job.jar}"
JOB_CLASS="${JOB_CLASS:-com.traffic.flink.rule.RuleJob}"

# ==================== 辅助函数 ====================

log_info() {
    echo "[$(date '+%Y-%m-%d %H:%M:%S')] [INFO] $1"
}

log_error() {
    echo "[$(date '+%Y-%m-%d %H:%M:%S')] [ERROR] $1" >&2
}

wait_for_service() {
    local host=$1
    local port=$2
    local max_attempts=${3:-30}
    local attempt=1

    log_info "Waiting for $host:$port..."
    while ! nc -z "$host" "$port" 2>/dev/null; do
        if [ $attempt -ge $max_attempts ]; then
            log_error "Service $host:$port not available after $max_attempts attempts"
            return 1
        fi
        log_info "Attempt $attempt/$max_attempts: $host:$port not ready, waiting..."
        sleep 2
        ((attempt++))
    done
    log_info "$host:$port is available"
}

# ==================== 配置处理 ====================

apply_env_config() {
    log_info "Applying environment configuration..."

    # Kafka 配置
    if [ -n "$KAFKA_BROKERS" ]; then
        log_info "Setting kafka.brokers=$KAFKA_BROKERS"
    fi

    # ClickHouse 配置
    if [ -n "$CLICKHOUSE_URL" ]; then
        log_info "Setting clickhouse.url=$CLICKHOUSE_URL"
    fi

    # 并行度
    if [ -n "$PARALLELISM" ]; then
        log_info "Setting parallelism=$PARALLELISM"
    fi
}

# ==================== 主逻辑 ====================

case "$1" in
    jobmanager)
        log_info "Starting JobManager..."
        apply_env_config

        # 等待依赖服务
        if [ -n "$KAFKA_BROKERS" ]; then
            KAFKA_HOST=$(echo "$KAFKA_BROKERS" | cut -d: -f1)
            KAFKA_PORT=$(echo "$KAFKA_BROKERS" | cut -d: -f2)
            wait_for_service "$KAFKA_HOST" "$KAFKA_PORT" 60
        fi

        exec "$FLINK_HOME/bin/jobmanager.sh" start-foreground
        ;;

    taskmanager)
        log_info "Starting TaskManager..."
        apply_env_config

        # 等待 JobManager
        JM_HOST="${JOB_MANAGER_RPC_ADDRESS:-jobmanager}"
        wait_for_service "$JM_HOST" 6123 60

        exec "$FLINK_HOME/bin/taskmanager.sh" start-foreground
        ;;

    standalone-job)
        log_info "Starting Standalone Job..."
        apply_env_config
        shift

        # 等待依赖服务
        if [ -n "$KAFKA_BROKERS" ]; then
            KAFKA_HOST=$(echo "$KAFKA_BROKERS" | cut -d: -f1)
            KAFKA_PORT=$(echo "$KAFKA_BROKERS" | cut -d: -f2)
            wait_for_service "$KAFKA_HOST" "$KAFKA_PORT" 60
        fi

        # 构建参数
        JOB_ARGS=""
        [ -n "$KAFKA_BROKERS" ] && JOB_ARGS="$JOB_ARGS --kafka.brokers $KAFKA_BROKERS"
        [ -n "$CLICKHOUSE_URL" ] && JOB_ARGS="$JOB_ARGS --clickhouse.url $CLICKHOUSE_URL"
        [ -n "$PARALLELISM" ] && JOB_ARGS="$JOB_ARGS --parallelism $PARALLELISM"

        exec "$FLINK_HOME/bin/standalone-job.sh" start-foreground \
            --job-classname "$JOB_CLASS" \
            $JOB_ARGS "$@"
        ;;

    submit)
        log_info "Submitting job to cluster..."
        shift

        # 等待 JobManager
        JM_HOST="${JOB_MANAGER_RPC_ADDRESS:-jobmanager}"
        wait_for_service "$JM_HOST" 8081 60

        # 构建参数
        JOB_ARGS=""
        [ -n "$KAFKA_BROKERS" ] && JOB_ARGS="$JOB_ARGS --kafka.brokers $KAFKA_BROKERS"
        [ -n "$CLICKHOUSE_URL" ] && JOB_ARGS="$JOB_ARGS --clickhouse.url $CLICKHOUSE_URL"
        [ -n "$PARALLELISM" ] && JOB_ARGS="$JOB_ARGS --parallelism $PARALLELISM"

        exec "$FLINK_HOME/bin/flink" run \
            -m "$JM_HOST:8081" \
            -c "$JOB_CLASS" \
            "$JOB_JAR" \
            $JOB_ARGS "$@"
        ;;

    sql-client)
        log_info "Starting SQL Client..."
        exec "$FLINK_HOME/bin/sql-client.sh" "$@"
        ;;

    help)
        echo "Flink Rule Job Container"
        echo ""
        echo "Usage: docker run <image> <command> [options]"
        echo ""
        echo "Commands:"
        echo "  jobmanager     Start as JobManager"
        echo "  taskmanager    Start as TaskManager"
        echo "  standalone-job Start as standalone job (Application Mode)"
        echo "  submit         Submit job to existing cluster"
        echo "  sql-client     Start SQL client"
        echo "  help           Show this help message"
        echo ""
        echo "Environment Variables:"
        echo "  KAFKA_BROKERS      Kafka bootstrap servers"
        echo "  CLICKHOUSE_URL     ClickHouse JDBC URL"
        echo "  PARALLELISM        Job parallelism"
        echo "  JOB_MANAGER_RPC_ADDRESS  JobManager address"
        echo ""
        ;;

    *)
        exec "$@"
        ;;
esac
