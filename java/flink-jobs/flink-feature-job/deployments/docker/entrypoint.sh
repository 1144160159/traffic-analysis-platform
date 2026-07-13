#!/bin/bash
# =============================================================================
# Flink Feature Job Entrypoint
# =============================================================================

set -e

FLINK_HOME=${FLINK_HOME:-/opt/flink}
FEATURE_JOB_HOME=${FEATURE_JOB_HOME:-/opt/feature-job}

# ==================== 环境变量处理 ====================
# Kafka 配置
export KAFKA_BROKERS=${KAFKA_BROKERS:-localhost:9092}
export KAFKA_INPUT_TOPIC=${KAFKA_INPUT_TOPIC:-session.events.v1}
export KAFKA_OUTPUT_TOPIC=${KAFKA_OUTPUT_TOPIC:-feature.stat.v1}
export KAFKA_DLQ_TOPIC=${KAFKA_DLQ_TOPIC:-dlq.feature-job}
export KAFKA_L2_TRIGGER_TOPIC=${KAFKA_L2_TRIGGER_TOPIC:-l2.trigger.v1}
export KAFKA_GROUP_ID=${KAFKA_GROUP_ID:-flink-feature-job}

# PostgreSQL 配置
export POSTGRES_URL=${POSTGRES_URL:-jdbc:postgresql://postgres:5432/traffic}
export POSTGRES_USER=${POSTGRES_USER:-postgres}
export POSTGRES_PASSWORD=${POSTGRES_PASSWORD:-}

# ClickHouse 配置
export CLICKHOUSE_URL=${CLICKHOUSE_URL:-clickhouse:8123}
export CLICKHOUSE_DATABASE=${CLICKHOUSE_DATABASE:-traffic}
export CLICKHOUSE_TABLE=${CLICKHOUSE_TABLE:-feature_stat_local}
export CLICKHOUSE_USER=${CLICKHOUSE_USER:-default}
export CLICKHOUSE_PASSWORD=${CLICKHOUSE_PASSWORD:-}

# Checkpoint 配置
export CHECKPOINT_PATH=${CHECKPOINT_PATH:-file:///data/flink/checkpoints}
export CHECKPOINT_INTERVAL=${CHECKPOINT_INTERVAL:-60000}

# 并行度配置
export PARALLELISM=${PARALLELISM:-4}

# ==================== 等待依赖服务 ====================
wait_for_service() {
    local host=$1
    local port=$2
    local service=$3
    local max_attempts=${4:-30}
    local attempt=1

    echo "Waiting for $service at $host:$port..."
    
    while [ $attempt -le $max_attempts ]; do
        if nc -z "$host" "$port" 2>/dev/null; then
            echo "$service is available!"
            return 0
        fi
        
        echo "Attempt $attempt/$max_attempts: $service not ready, waiting..."
        sleep 2
        attempt=$((attempt + 1))
    done

    echo "ERROR: $service at $host:$port is not available after $max_attempts attempts"
    return 1
}

wait_for_dependencies() {
    # 解析 Kafka broker
    KAFKA_HOST=$(echo $KAFKA_BROKERS | cut -d: -f1)
    KAFKA_PORT=$(echo $KAFKA_BROKERS | cut -d: -f2)
    wait_for_service "$KAFKA_HOST" "$KAFKA_PORT" "Kafka"

    # 解析 PostgreSQL
    PG_HOST=$(echo $POSTGRES_URL | sed -n 's/.*\/\/\([^:\/]*\).*/\1/p')
    PG_PORT=$(echo $POSTGRES_URL | sed -n 's/.*:\([0-9]*\)\/.*/\1/p')
    PG_PORT=${PG_PORT:-5432}
    wait_for_service "$PG_HOST" "$PG_PORT" "PostgreSQL"

    # 解析 ClickHouse
    CH_HOST=$(echo $CLICKHOUSE_URL | cut -d: -f1)
    CH_PORT=$(echo $CLICKHOUSE_URL | cut -d: -f2)
    wait_for_service "$CH_HOST" "$CH_PORT" "ClickHouse"

    echo "All dependencies are ready!"
}

# ==================== 生成运行时配置 ====================
generate_runtime_config() {
    cat > ${FEATURE_JOB_HOME}/conf/runtime.properties << EOF
# Auto-generated runtime configuration

# Kafka
kafka.brokers=${KAFKA_BROKERS}
kafka.input.topic=${KAFKA_INPUT_TOPIC}
kafka.output.topic=${KAFKA_OUTPUT_TOPIC}
kafka.dlq.topic=${KAFKA_DLQ_TOPIC}
kafka.l2.trigger.topic=${KAFKA_L2_TRIGGER_TOPIC}
kafka.group.id=${KAFKA_GROUP_ID}

# PostgreSQL
postgres.url=${POSTGRES_URL}
postgres.user=${POSTGRES_USER}
postgres.password=${POSTGRES_PASSWORD}

# ClickHouse
clickhouse.url=${CLICKHOUSE_URL}
clickhouse.database=${CLICKHOUSE_DATABASE}
clickhouse.table=${CLICKHOUSE_TABLE}
clickhouse.user=${CLICKHOUSE_USER}
clickhouse.password=${CLICKHOUSE_PASSWORD}

# Checkpoint
checkpoint.path=${CHECKPOINT_PATH}
checkpoint.interval.ms=${CHECKPOINT_INTERVAL}

# Parallelism
parallelism=${PARALLELISM}
EOF

    echo "Runtime configuration generated at ${FEATURE_JOB_HOME}/conf/runtime.properties"
}

# ==================== 查找 JAR 文件 ====================
find_job_jar() {
    local jar_file=$(ls ${FEATURE_JOB_HOME}/lib/flink-feature-job-*.jar 2>/dev/null | head -1)
    
    if [ -z "$jar_file" ]; then
        echo "ERROR: Feature job JAR not found in ${FEATURE_JOB_HOME}/lib/"
        exit 1
    fi
    
    echo "$jar_file"
}

# ==================== 主入口 ====================
case "$1" in
    jobmanager)
        echo "Starting Flink JobManager..."
        exec ${FLINK_HOME}/bin/jobmanager.sh start-foreground
        ;;
        
    taskmanager)
        echo "Starting Flink TaskManager..."
        exec ${FLINK_HOME}/bin/taskmanager.sh start-foreground
        ;;
        
    standalone-job)
        echo "Starting standalone Flink job..."
        wait_for_dependencies
        generate_runtime_config
        
        JOB_JAR=$(find_job_jar)
        echo "Submitting job: $JOB_JAR"
        
        exec ${FLINK_HOME}/bin/standalone-job.sh start-foreground \
            --job-classname com.traffic.flink.feature.FeatureJob \
            -Djobmanager.rpc.address=localhost \
            -Dparallelism.default=${PARALLELISM} \
            "$JOB_JAR" \
            --config ${FEATURE_JOB_HOME}/conf/runtime.properties
        ;;
        
    submit)
        echo "Submitting job to Flink cluster..."
        wait_for_dependencies
        generate_runtime_config
        
        JOB_JAR=$(find_job_jar)
        JOBMANAGER_HOST=${JOBMANAGER_HOST:-jobmanager}
        JOBMANAGER_PORT=${JOBMANAGER_PORT:-8081}
        
        echo "Submitting $JOB_JAR to $JOBMANAGER_HOST:$JOBMANAGER_PORT"
        
        exec ${FLINK_HOME}/bin/flink run \
            -m ${JOBMANAGER_HOST}:${JOBMANAGER_PORT} \
            -p ${PARALLELISM} \
            -c com.traffic.flink.feature.FeatureJob \
            "$JOB_JAR" \
            --config ${FEATURE_JOB_HOME}/conf/runtime.properties
        ;;
        
    help|*)
        echo "Usage: $0 {jobmanager|taskmanager|standalone-job|submit|help}"
        echo ""
        echo "Commands:"
        echo "  jobmanager      Start Flink JobManager"
        echo "  taskmanager     Start Flink TaskManager"
        echo "  standalone-job  Run job in standalone mode"
        echo "  submit          Submit job to existing Flink cluster"
        echo ""
        echo "Environment Variables:"
        echo "  KAFKA_BROKERS        Kafka bootstrap servers (default: localhost:9092)"
        echo "  POSTGRES_URL         PostgreSQL JDBC URL"
        echo "  CLICKHOUSE_URL       ClickHouse host:port"
        echo "  PARALLELISM          Job parallelism (default: 4)"
        echo "  CHECKPOINT_PATH      Checkpoint storage path"
        ;;
esac
