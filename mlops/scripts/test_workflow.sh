#!/bin/bash
set -e

# 测试脚本：验证 MLOps Workflow 部署和运行

NAMESPACE="traffic-analysis"
WORKFLOW_NAME=""

echo "=================================================="
echo "MLOps Workflow 测试脚本"
echo "=================================================="

# 1. 检查命名空间
echo ""
echo "1. 检查命名空间..."
if ! kubectl get namespace $NAMESPACE &> /dev/null; then
    echo "❌ 命名空间 $NAMESPACE 不存在，正在创建..."
    kubectl create namespace $NAMESPACE
else
    echo "✓ 命名空间 $NAMESPACE 已存在"
fi

# 2. 检查 Argo Workflows 安装
echo ""
echo "2. 检查 Argo Workflows..."
if ! kubectl get deployment -n argo workflow-controller &> /dev/null; then
    echo "❌ Argo Workflows 未安装"
    echo "请运行: kubectl apply -n argo -f https://github.com/argoproj/argo-workflows/releases/download/v3.5.0/install.yaml"
    exit 1
else
    echo "✓ Argo Workflows 已安装"
fi

# 3. 部署 Secrets
echo ""
echo "3. 部署 Secrets 和 RBAC..."
kubectl apply -f ../workflows/mlops-secrets.yaml
echo "✓ Secrets 部署完成"

# 4. 部署 ConfigMap
echo ""
echo "4. 部署 ConfigMap (脚本)..."
kubectl apply -f ../workflows/mlops-configmap.yaml
echo "✓ ConfigMap 部署完成"

# 5. 检查依赖服务
echo ""
echo "5. 检查依赖服务..."

# ClickHouse
if kubectl get service -n $NAMESPACE clickhouse &> /dev/null; then
    echo "✓ ClickHouse 服务已就绪"
else
    echo "⚠ ClickHouse 服务不存在 (测试可跳过)"
fi

# MinIO
if kubectl get service -n $NAMESPACE minio &> /dev/null; then
    echo "✓ MinIO 服务已就绪"
else
    echo "⚠ MinIO 服务不存在 (测试可跳过)"
fi

# Kafka
if kubectl get service -n $NAMESPACE kafka &> /dev/null; then
    echo "✓ Kafka 服务已就绪"
else
    echo "⚠ Kafka 服务不存在 (测试可跳过)"
fi

# 6. 提交测试 Workflow (干跑模式)
echo ""
echo "6. 提交测试 Workflow (干跑模式)..."
argo submit -n $NAMESPACE ../workflows/training-workflow.yaml \
    --parameter model-type=xgboost \
    --parameter lookback-days=1 \
    --parameter min-f1-score=0.5 \
    --dry-run \
    --output json | jq -r '.metadata.name'

echo "✓ Workflow 定义验证通过"

# 7. 实际提交 Workflow (可选)
echo ""
read -p "是否实际提交 Workflow 进行测试? (y/n) " -n 1 -r
echo
if [[ $REPLY =~ ^[Yy]$ ]]; then
    echo "提交 Workflow..."
    WORKFLOW_NAME=$(argo submit -n $NAMESPACE ../workflows/training-workflow.yaml \
        --parameter model-type=xgboost \
        --parameter lookback-days=1 \
        --parameter min-f1-score=0.5 \
        --output name)
    
    echo "✓ Workflow 已提交: $WORKFLOW_NAME"
    
    # 8. 实时查看日志
    echo ""
    read -p "是否查看实时日志? (y/n) " -n 1 -r
    echo
    if [[ $REPLY =~ ^[Yy]$ ]]; then
        argo logs -n $NAMESPACE $WORKFLOW_NAME --follow
    fi
    
    # 9. 查看状态
    echo ""
    echo "Workflow 状态:"
    argo get -n $NAMESPACE $WORKFLOW_NAME
    
    # 10. 查看 Artifacts
    echo ""
    echo "Workflow Artifacts:"
    argo get -n $NAMESPACE $WORKFLOW_NAME -o wide
else
    echo "跳过实际提交"
fi

# 11. 部署 CronWorkflow
echo ""
read -p "是否部署定时训练任务? (y/n) " -n 1 -r
echo
if [[ $REPLY =~ ^[Yy]$ ]]; then
    kubectl apply -f ../workflows/cron-training-workflow.yaml
    echo "✓ CronWorkflow 部署完成"
    
    echo ""
    echo "定时任务列表:"
    argo cron list -n $NAMESPACE
fi

echo ""
echo "=================================================="
echo "测试完成!"
echo "=================================================="
echo ""
echo "常用命令:"
echo "  - 查看所有 Workflows:"
echo "      argo list -n $NAMESPACE"
echo ""
echo "  - 查看日志:"
echo "      argo logs -n $NAMESPACE <workflow-name> --follow"
echo ""
echo "  - 删除 Workflow:"
echo "      argo delete -n $NAMESPACE <workflow-name>"
echo ""
echo "  - 手动触发定时任务:"
echo "      argo cron trigger -n $NAMESPACE weekly-model-training"
echo ""
echo "  - 查看 CronWorkflow:"
echo "      argo cron list -n $NAMESPACE"
echo ""
