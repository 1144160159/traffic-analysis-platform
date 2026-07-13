#!/bin/bash
################################################################################
# FILE: docker/docker-entrypoint-job.sh
#
# Flink Behavior Job 容器入口点脚本
# 支持多种启动模式：jobmanager, taskmanager, standalone, submit
################################################################################

set -e

FLINK_HOME=${FLINK_HOME:-/opt/flink}
JOB_JAR="${FLINK_HOME}/usrlib/flink-behavior-job-1.0.0.jar"
JOB_CLASS="com.traffic.flink.behavior.BehaviorDetectionJob"

# 日志函数
log_info() {
    echo "[$(date '+%Y-%m-%d %H:%M:%S')] [INFO] $*"
}

log_error() {
    echo "[$(date '+%Y-%m-%d %H:%M:%S')] [ERROR] $*" >&2
}

# 等待服务可用
wait_for_service() {
    local host=$1
    local port=$2
    local max_attempts=${3:-30}
    local attempt=0
    
    log_info "Waiting for ${host}:${port}..."
    
    while ! nc -z "$host" "$port" 2>/dev/null; do
        attempt=$((attempt + 1))
        if [ $attempt -ge $max_attempts ]; then
            log_error "Timeout waiting for ${host}:${port}"
            return 1
        fi
        sleep 2
    done
    
    log_info "${host}:${port} is available"
}

# 配置 JVM 参数
configure_jvm() {
    # 默认 JVM 参数
    export JVM_ARGS="${JVM_ARGS:--Xms1g -Xmx2g}"
    
    # GC 配置
    export JVM_ARGS="${JVM_ARGS} -XX:+UseG1GC"
    export JVM_ARGS="${JVM_ARGS} -XX:MaxGCPauseMillis=200"
    export JVM_ARGS="${JVM_ARGS} -XX:+ParallelRefProcEnabled"
    
    # 调试配置（可选）
    if [ "${ENABLE_JMX:-false}" = "true" ]; then
        export JVM_ARGS="${JVM_ARGS} -Dcom.sun.management.jmxremote"
        export JVM_ARGS="${JVM_ARGS} -Dcom.sun.management.jmxremote.port=${JMX_PORT:-9999}"
        export JVM_ARGS="${JVM_ARGS} -Dcom.sun.management.jmxremote.authenticate=false"
        export JVM_ARGS="${JVM_ARGS} -Dcom.sun.management.jmxremote.ssl=false"
    fi
    
    # 远程调试（可选）
    if [ "${ENABLE_DEBUG:-false}" = "true" ]; then
        export JVM_ARGS="${JVM_ARGS} -agentlib:jdwp=transport=dt_socket,server=y,suspend=n,address=*:${DEBUG_PORT:-5005}"
    fi
    
    log_info "JVM Args: ${JVM_ARGS}"
}

# 构建作业参数
build_job_args() {
    local args=""
    
    # Kafka 配置
    [ -n "${KAFKA_BROKERS}" ] && args="${args} --kafka.brokers ${KAFKA_BROKERS}"
    [ -n "${KAFKA_INPUT_TOPIC}" ] && args="${args} --kafka.input.topic ${KAFKA_INPUT_TOPIC}"
    [ -n "${KAFKA_OUTPUT_TOPIC}" ] && args="${args} --kafka.output.topic ${KAFKA_OUTPUT_TOPIC}"
    [ -n "${KAFKA_GROUP_ID}" ] && args="${args} --kafka.group.id ${KAFKA_GROUP_ID}"
    
    # ClickHouse 配置
    [ -n "${CLICKHOUSE_URL}" ] && args="${args} --clickhouse.url ${CLICKHOUSE_URL}"
    [ -n "${CLICKHOUSE_DATABASE}" ] && args="${args} --clickhouse.database ${CLICKHOUSE_DATABASE}"
    [ -n "${CLICKHOUSE_TABLE}" ] && args="${args} --clickhouse.table ${CLICKHOUSE_TABLE}"
    [ -n "${CLICKHOUSE_USER}" ] && args="${args} --clickhouse.user ${CLICKHOUSE_USER}"
    [ -n "${CLICKHOUSE_PASSWORD}" ] && args="${args} --clickhouse.password ${CLICKHOUSE_PASSWORD}"
    
    # Checkpoint 配置
    [ -n "${CHECKPOINT_PATH}" ] && args="${args} --checkpoint.path ${CHECKPOINT_PATH}"
    [ -n "${CHECKPOINT_INTERVAL_MS}" ] && args="${args} --checkpoint.interval.ms ${CHECKPOINT_INTERVAL_MS}"
    
    # 性能配置
    [ -n "${PARALLELISM}" ] && args="${args} --parallelism ${PARALLELISM}"
    [ -n "${MAX_PARALLELISM}" ] && args="${args} --max.parallelism ${MAX_PARALLELISM}"
    
    # 模型配置
    [ -n "${MODEL_PATH}" ] && args="${args} --model.path ${MODEL_PATH}"
    [ -n "${MODEL_VERSION}" ] && args="${args} --model.version ${MODEL_VERSION}"
    [ -n "${MODEL_ENABLED}" ] && args="${args} --model.enabled ${MODEL_ENABLED}"
    
    # 推理配置
    [ -n "${INFERENCE_ASYNC_ENABLED}" ] && args="${args} --inference.async.enabled ${INFERENCE_ASYNC_ENABLED}"
    [ -n "${INFERENCE_THREADS}" ] && args="${args} --inference.threads ${INFERENCE_THREADS}"
    
    # 检测阈值
    [ -n "${DETECTION_MIN_CONFIDENCE}" ] && args="${args} --detection.min.confidence ${DETECTION_MIN_CONFIDENCE}"
    
    # 调试配置
    [ -n "${DEBUG_PRINT}" ] && args="${args} --debug.print ${DEBUG_PRINT}"
    
    echo "${args}"
}

# 启动 JobManager
start_jobmanager() {
    log_info "Starting Flink JobManager..."
    configure_jvm
    
    # 设置 JobManager 内存
    export FLINK_JM_HEAP=${FLINK_JM_HEAP:-1024m}
    
    exec "${FLINK_HOME}/bin/jobmanager.sh" start-foreground
}

# 启动 TaskManager
start_taskmanager() {
    log_info "Starting Flink TaskManager..."
    configure_jvm
    
    # 等待 JobManager
    if [ -n "${JOB_MANAGER_RPC_ADDRESS}" ]; then
        wait_for_service "${JOB_MANAGER_RPC_ADDRESS}" "${JOB_MANAGER_RPC_PORT:-6123}"
    fi
    
    # 设置 TaskManager 内存
    export FLINK_TM_HEAP=${FLINK_TM_HEAP:-2048m}
    
    exec "${FLINK_HOME}/bin/taskmanager.sh" start-foreground
}

# 提交作业
submit_job() {
    log_info "Submitting Flink Job..."
    
    # 等待 JobManager REST API
    local jm_host="${JOB_MANAGER_RPC_ADDRESS:-localhost}"
    local jm_port="${FLINK_REST_PORT:-8081}"
    wait_for_service "${jm_host}" "${jm_port}" 60
    
    # 等待 Kafka
    if [ -n "${KAFKA_BROKERS}" ]; then
        local kafka_host=$(echo "${KAFKA_BROKERS}" | cut -d',' -f1 | cut -d':' -f1)
        local kafka_port=$(echo "${KAFKA_BROKERS}" | cut -d',' -f1 | cut -d':' -f2)
        wait_for_service "${kafka_host}" "${kafka_port:-9092}" 30
    fi
    
    # 构建作业参数
    local job_args=$(build_job_args)
    
    log_info "Job JAR: ${JOB_JAR}"
    log_info "Job Class: ${JOB_CLASS}"
    log_info "Job Args: ${job_args}"
    
    # 提交作业
    exec "${FLINK_HOME}/bin/flink" run \
        -d \
        -c "${JOB_CLASS}" \
        ${JOB_JAR} \
        ${job_args}
}

# Standalone 模式（同时启动 JM 和 TM）
start_standalone() {
    log_info "Starting Flink Standalone Cluster..."
    configure_jvm
    
    # 启动 JobManager（后台）
    "${FLINK_HOME}/bin/jobmanager.sh" start
    
    # 等待 JobManager 启动
    sleep 10
    
    # 启动 TaskManager（前台）
    exec "${FLINK_HOME}/bin/taskmanager.sh" start-foreground
}

# Application 模式（JM 内运行作业）
start_application() {
    log_info "Starting Flink Application Mode..."
    configure_jvm
    
    local job_args=$(build_job_args)
    
    exec "${FLINK_HOME}/bin/standalone-job.sh" start-foreground \
        -Djobmanager.memory.process.size=${JM_MEMORY:-1600m} \
        -Dtaskmanager.memory.process.size=${TM_MEMORY:-4096m} \
        -Dtaskmanager.numberOfTaskSlots=${TASK_SLOTS:-4} \
        -Dparallelism.default=${PARALLELISM:-4} \
        --job-classname "${JOB_CLASS}" \
        ${job_args}
}

# 主入口
main() {
    local command="${1:-jobmanager}"
    
    log_info "=========================================="
    log_info "Flink Behavior Detection Job"
    log_info "Command: ${command}"
    log_info "=========================================="
    
    case "${command}" in
        jobmanager)
            start_jobmanager
            ;;
        taskmanager)
            start_taskmanager
            ;;
        standalone)
            start_standalone
            ;;
        submit)
            submit_job
            ;;
        application)
            start_application
            ;;
        help)
            echo "Usage: $0 {jobmanager|taskmanager|standalone|submit|application}"
            echo ""
            echo "Commands:"
            echo "  jobmanager   - Start Flink JobManager"
            echo "  taskmanager  - Start Flink TaskManager"
            echo "  standalone   - Start standalone cluster (JM + TM)"
            echo "  submit       - Submit job to running cluster"
            echo "  application  - Start in Application mode"
            ;;
        *)
            log_error "Unknown command: ${command}"
            echo "Use '$0 help' for usage information"
            exit 1
            ;;
    esac
}

main "$@"
