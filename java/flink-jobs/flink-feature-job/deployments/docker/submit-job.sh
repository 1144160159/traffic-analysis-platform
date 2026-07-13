#!/bin/bash
# =============================================================================
# Flink Feature Job Submission Script
# =============================================================================

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
FLINK_HOME=${FLINK_HOME:-/opt/flink}
FEATURE_JOB_HOME=${FEATURE_JOB_HOME:-/opt/feature-job}

# ==================== 默认配置 ====================
JOBMANAGER_HOST=${JOBMANAGER_HOST:-localhost}
JOBMANAGER_PORT=${JOBMANAGER_PORT:-8081}
PARALLELISM=${PARALLELISM:-4}
SAVEPOINT_PATH=${SAVEPOINT_PATH:-}

# ==================== 颜色输出 ====================
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

log_info() {
    echo -e "${GREEN}[INFO]${NC} $1"
}

log_warn() {
    echo -e "${YELLOW}[WARN]${NC} $1"
}

log_error() {
    echo -e "${RED}[ERROR]${NC} $1"
}

# ==================== 查找 JAR ====================
find_job_jar() {
    local jar_file=$(ls ${FEATURE_JOB_HOME}/lib/flink-feature-job-*.jar 2>/dev/null | head -1)
    
    if [ -z "$jar_file" ]; then
        log_error "Feature job JAR not found in ${FEATURE_JOB_HOME}/lib/"
        exit 1
    fi
    
    echo "$jar_file"
}

# ==================== 检查集群状态 ====================
check_cluster() {
    log_info "Checking Flink cluster at ${JOBMANAGER_HOST}:${JOBMANAGER_PORT}..."
    
    local response=$(curl -s -o /dev/null -w "%{http_code}" \
        "http://${JOBMANAGER_HOST}:${JOBMANAGER_PORT}/overview" 2>/dev/null)
    
    if [ "$response" != "200" ]; then
        log_error "Flink cluster is not available (HTTP $response)"
        exit 1
    fi
    
    log_info "Flink cluster is healthy"
    
    # 显示集群信息
    curl -s "http://${JOBMANAGER_HOST}:${JOBMANAGER_PORT}/overview" | jq '.'
}

# ==================== 提交作业 ====================
submit_job() {
    local jar_file=$(find_job_jar)
    local config_file="${FEATURE_JOB_HOME}/conf/runtime.properties"
    
    log_info "Submitting job: $jar_file"
    log_info "Config: $config_file"
    log_info "Parallelism: $PARALLELISM"
    
    local flink_args="-m ${JOBMANAGER_HOST}:${JOBMANAGER_PORT}"
    flink_args="$flink_args -p ${PARALLELISM}"
    flink_args="$flink_args -c com.traffic.flink.feature.FeatureJob"
    
    # 如果指定了 savepoint
    if [ -n "$SAVEPOINT_PATH" ]; then
        log_info "Resuming from savepoint: $SAVEPOINT_PATH"
        flink_args="$flink_args -s ${SAVEPOINT_PATH}"
    fi
    
    ${FLINK_HOME}/bin/flink run $flink_args "$jar_file" --config "$config_file"
    
    if [ $? -eq 0 ]; then
        log_info "Job submitted successfully!"
    else
        log_error "Job submission failed!"
        exit 1
    fi
}

# ==================== 列出运行中的作业 ====================
list_jobs() {
    log_info "Listing running jobs..."
    
    curl -s "http://${JOBMANAGER_HOST}:${JOBMANAGER_PORT}/jobs/overview" | \
        jq -r '.jobs[] | "\(.jid) | \(.name) | \(.state) | \(.start-time | . / 1000 | strftime("%Y-%m-%d %H:%M:%S"))"'
}

# ==================== 取消作业 ====================
cancel_job() {
    local job_id=$1
    
    if [ -z "$job_id" ]; then
        log_error "Job ID is required"
        exit 1
    fi
    
    log_info "Cancelling job: $job_id"
    
    curl -X PATCH "http://${JOBMANAGER_HOST}:${JOBMANAGER_PORT}/jobs/${job_id}?mode=cancel"
    
    log_info "Cancel request sent"
}

# ==================== 创建 Savepoint ====================
create_savepoint() {
    local job_id=$1
    local target_dir=${2:-"/data/flink/savepoints"}
    
    if [ -z "$job_id" ]; then
        log_error "Job ID is required"
        exit 1
    fi
    
    log_info "Creating savepoint for job: $job_id"
    log_info "Target directory: $target_dir"
    
    local response=$(curl -s -X POST \
        -H "Content-Type: application/json" \
        -d "{\"target-directory\": \"${target_dir}\", \"cancel-job\": false}" \
        "http://${JOBMANAGER_HOST}:${JOBMANAGER_PORT}/jobs/${job_id}/savepoints")
    
    local request_id=$(echo $response | jq -r '.request-id')
    
    if [ "$request_id" == "null" ]; then
        log_error "Failed to trigger savepoint"
        echo $response | jq '.'
        exit 1
    fi
    
    log_info "Savepoint triggered, request ID: $request_id"
    
    # 等待 savepoint 完成
    local status="IN_PROGRESS"
    while [ "$status" == "IN_PROGRESS" ]; do
        sleep 2
        local status_response=$(curl -s \
            "http://${JOBMANAGER_HOST}:${JOBMANAGER_PORT}/jobs/${job_id}/savepoints/${request_id}")
        status=$(echo $status_response | jq -r '.status.id')
        log_info "Savepoint status: $status"
    done
    
    if [ "$status" == "COMPLETED" ]; then
        local location=$(curl -s \
            "http://${JOBMANAGER_HOST}:${JOBMANAGER_PORT}/jobs/${job_id}/savepoints/${request_id}" | \
            jq -r '.operation.location')
        log_info "Savepoint created at: $location"
    else
        log_error "Savepoint failed"
        exit 1
    fi
}

# ==================== 停止作业（带 Savepoint）====================
stop_with_savepoint() {
    local job_id=$1
    local target_dir=${2:-"/data/flink/savepoints"}
    
    if [ -z "$job_id" ]; then
        log_error "Job ID is required"
        exit 1
    fi
    
    log_info "Stopping job with savepoint: $job_id"
    
    local response=$(curl -s -X POST \
        -H "Content-Type: application/json" \
        -d "{\"targetDirectory\": \"${target_dir}\", \"drain\": false}" \
        "http://${JOBMANAGER_HOST}:${JOBMANAGER_PORT}/jobs/${job_id}/stop")
    
    echo $response | jq '.'
}

# ==================== 主入口 ====================
case "$1" in
    check)
        check_cluster
        ;;
    submit)
        check_cluster
        submit_job
        ;;
    list)
        list_jobs
        ;;
    cancel)
        cancel_job "$2"
        ;;
    savepoint)
        create_savepoint "$2" "$3"
        ;;
    stop)
        stop_with_savepoint "$2" "$3"
        ;;
    *)
        echo "Usage: $0 {check|submit|list|cancel|savepoint|stop}"
        echo ""
        echo "Commands:"
        echo "  check              Check Flink cluster status"
        echo "  submit             Submit feature job to cluster"
        echo "  list               List running jobs"
        echo "  cancel <job_id>    Cancel a running job"
        echo "  savepoint <job_id> [target_dir]  Create savepoint"
        echo "  stop <job_id> [target_dir]       Stop job with savepoint"
        echo ""
        echo "Environment Variables:"
        echo "  JOBMANAGER_HOST    JobManager hostname (default: localhost)"
        echo "  JOBMANAGER_PORT    JobManager REST port (default: 8081)"
        echo "  PARALLELISM        Job parallelism (default: 4)"
        echo "  SAVEPOINT_PATH     Savepoint path for resuming"
        ;;
esac
