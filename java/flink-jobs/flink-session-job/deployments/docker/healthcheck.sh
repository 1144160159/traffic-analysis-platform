#!/bin/bash
# ==============================================================================
# Flink Session Job 健康检查脚本
# ==============================================================================

set -e

# 检查 JobManager 健康状态
if [ "$FLINK_MODE" = "jobmanager" ]; then
    # 检查 JobManager REST API
    HTTP_CODE=$(curl -s -o /dev/null -w "%{http_code}" http://localhost:8081/overview || echo "000")
    
    if [ "$HTTP_CODE" = "200" ]; then
        echo "JobManager is healthy"
        exit 0
    else
        echo "JobManager is unhealthy (HTTP $HTTP_CODE)"
        exit 1
    fi
fi

# 检查 TaskManager 健康状态
if [ "$FLINK_MODE" = "taskmanager" ]; then
    # 检查 TaskManager 进程是否存在
    if pgrep -f "org.apache.flink.runtime.taskexecutor.TaskManagerRunner" > /dev/null; then
        echo "TaskManager is healthy"
        exit 0
    else
        echo "TaskManager process not found"
        exit 1
    fi
fi

# 默认健康
echo "Health check passed"
exit 0
