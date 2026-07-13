#!/bin/bash
# ============================================================================
# Flink Rule Job Deployment Verification Script
# ============================================================================

set -e

NAMESPACE=${NAMESPACE:-traffic-analysis}
APP_NAME="flink-rule-job"

echo "========================================="
echo "Flink Rule Job Deployment Verification"
echo "========================================="

# 1. Check if namespace exists
echo ""
echo "[1/8] Checking namespace..."
if kubectl get namespace $NAMESPACE &> /dev/null; then
    echo "✓ Namespace '$NAMESPACE' exists"
else
    echo "✗ Namespace '$NAMESPACE' not found"
    exit 1
fi

# 2. Check ConfigMap
echo ""
echo "[2/8] Checking ConfigMap..."
if kubectl get configmap flink-rule-job-config -n $NAMESPACE &> /dev/null; then
    echo "✓ ConfigMap 'flink-rule-job-config' exists"
else
    echo "✗ ConfigMap not found"
    exit 1
fi

# 3. Check Services
echo ""
echo "[3/8] Checking Services..."
SERVICES=("flink-rule-job-jobmanager" "flink-rule-job-jobmanager-rest")
for svc in "${SERVICES[@]}"; do
    if kubectl get service $svc -n $NAMESPACE &> /dev/null; then
        echo "✓ Service '$svc' exists"
    else
        echo "✗ Service '$svc' not found"
        exit 1
    fi
done

# 4. Check JobManager Deployment
echo ""
echo "[4/8] Checking JobManager..."
kubectl rollout status deployment/flink-rule-job-jobmanager -n $NAMESPACE --timeout=300s
JM_READY=$(kubectl get deployment flink-rule-job-jobmanager -n $NAMESPACE -o jsonpath='{.status.readyReplicas}')
if [ "$JM_READY" == "1" ]; then
    echo "✓ JobManager is ready (1/1)"
else
    echo "✗ JobManager is not ready ($JM_READY/1)"
    exit 1
fi

# 5. Check TaskManager Deployment
echo ""
echo "[5/8] Checking TaskManager..."
kubectl rollout status deployment/flink-rule-job-taskmanager -n $NAMESPACE --timeout=300s
TM_READY=$(kubectl get deployment flink-rule-job-taskmanager -n $NAMESPACE -o jsonpath='{.status.readyReplicas}')
TM_REPLICAS=$(kubectl get deployment flink-rule-job-taskmanager -n $NAMESPACE -o jsonpath='{.spec.replicas}')
if [ "$TM_READY" == "$TM_REPLICAS" ]; then
    echo "✓ TaskManager is ready ($TM_READY/$TM_REPLICAS)"
else
    echo "⚠ TaskManager is partially ready ($TM_READY/$TM_REPLICAS)"
fi

# 6. Check Flink UI
echo ""
echo "[6/8] Checking Flink UI..."
JM_POD=$(kubectl get pods -n $NAMESPACE -l app=flink-rule-job,component=jobmanager -o jsonpath='{.items[0].metadata.name}')
if kubectl exec -n $NAMESPACE $JM_POD -- curl -s -f http://localhost:8081/overview &> /dev/null; then
    echo "✓ Flink UI is accessible"
    
    # Get job status
    JOB_STATUS=$(kubectl exec -n $NAMESPACE $JM_POD -- curl -s http://localhost:8081/jobs | grep -o '"state":"[^"]*"' | head -1 | cut -d'"' -f4 || echo "UNKNOWN")
    echo "  Job Status: $JOB_STATUS"
else
    echo "✗ Flink UI is not accessible"
fi

# 7. Check Prometheus Metrics
echo ""
echo "[7/8] Checking Prometheus metrics..."
if kubectl exec -n $NAMESPACE $JM_POD -- curl -s -f http://localhost:9250/metrics &> /dev/null; then
    echo "✓ Prometheus metrics endpoint is accessible"
    
    # Sample metrics
    FEATURES_PROCESSED=$(kubectl exec -n $NAMESPACE $JM_POD -- curl -s http://localhost:9250/metrics | grep 'features_processed_total' | tail -1 || echo "")
    if [ -n "$FEATURES_PROCESSED" ]; then
        echo "  $FEATURES_PROCESSED"
    fi
else
    echo "⚠ Prometheus metrics endpoint is not accessible"
fi

# 8. Check Logs
echo ""
echo "[8/8] Checking recent logs..."
echo "JobManager logs (last 10 lines):"
kubectl logs -n $NAMESPACE $JM_POD --tail=10 | sed 's/^/  /'

echo ""
echo "========================================="
echo "Verification Summary"
echo "========================================="
echo "✓ Namespace: $NAMESPACE"
echo "✓ JobManager: Running"
echo "✓ TaskManager: $TM_READY/$TM_REPLICAS replicas"
echo "✓ Flink Job Status: $JOB_STATUS"
echo ""
echo "Access Flink UI:"
echo "  kubectl port-forward -n $NAMESPACE svc/flink-rule-job-jobmanager-rest 8081:8081"
echo "  http://localhost:8081"
echo ""
echo "View logs:"
echo "  kubectl logs -n $NAMESPACE -l app=flink-rule-job,component=jobmanager -f"
echo ""
echo "Prometheus metrics:"
echo "  kubectl port-forward -n $NAMESPACE $JM_POD 9250:9250"
echo "  http://localhost:9250/metrics"
echo "========================================="
