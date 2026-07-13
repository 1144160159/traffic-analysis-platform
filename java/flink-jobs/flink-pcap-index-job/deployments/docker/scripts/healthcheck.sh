#!/bin/bash
# =============================================================================
# Flink PCAP Index Job 健康检查脚本
# =============================================================================

set -e

# 检查 Flink REST API
check_flink_rest() {
    local response
    response=$(curl -sf http://localhost:8081/overview 2>/dev/null) || return 1
    
    # 检查 TaskManagers 数量
    local taskmanagers
    taskmanagers=$(echo "$response" | jq -r '.taskmanagers // 0')
    
    if [ "$taskmanagers" -lt 1 ]; then
        echo "No TaskManagers available"
        return 1
    fi
    
    return 0
}

# 检查 Prometheus Metrics 端点
check_metrics() {
    curl -sf http://localhost:9249/metrics > /dev/null 2>&1
}

# 检查作业运行状态（仅在 JobManager 模式下）
check_job_running() {
    local jobs
    jobs=$(curl -sf http://localhost:8081/jobs 2>/dev/null) || return 0
    
    # 检查是否有 RUNNING 状态的作业
    local running_count
    running_count=$(echo "$jobs" | jq '[.jobs[] | select(.status == "RUNNING")] | length')
    
    if [ "$running_count" -lt 1 ]; then
        echo "No running jobs found"
        # 返回 0，因为作业可能还在启动中
        return 0
    fi
    
    return 0
}

# 主检查逻辑
main() {
    # 检查进程是否存在
    if ! pgrep -f "org.apache.flink" > /dev/null; then
        echo "Flink process not found"
        exit 1
    fi
    
    # 检查 REST API（如果是 JobManager）
    if [ -n "$FLINK_JOB_MANAGER" ]; then
        if ! check_flink_rest; then
            echo "Flink REST API check failed"
            exit 1
        fi
        
        if ! check_job_running; then
            echo "Job status check failed"
            exit 1
        fi
    fi
    
    # 检查 Metrics 端点
    if ! check_metrics; then
        echo "Metrics endpoint check failed"
        exit 1
    fi
    
    echo "Health check passed"
    exit 0
}

main "$@"
