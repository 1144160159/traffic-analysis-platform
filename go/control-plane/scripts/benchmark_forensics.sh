#!/bin/bash
# scripts/benchmark_forensics.sh - 取证服务性能测试

set -e

API_URL="${API_URL:-http://localhost:8083}"
TENANT_ID="${TENANT_ID:-tenant-01}"

echo "=== Forensics Service Benchmark ==="
echo "API URL: $API_URL"
echo "Tenant ID: $TENANT_ID"
echo ""

# 同步裁剪测试
echo "1. Sync PCAP Cut Test (small time range)"
START_TIME=$(($(date +%s%3N) - 60000))  # 1 分钟前
END_TIME=$(date +%s%3N)

time curl -s -X POST "$API_URL/api/v1/pcap/cut" \
    -H "Content-Type: application/json" \
    -d "{
        \"tenant_id\": \"$TENANT_ID\",
        \"src_ip\": \"192.168.1.1\",
        \"start_time\": $START_TIME,
        \"end_time\": $END_TIME
    }" \
    -o /tmp/test_output.pcap

if [ -f /tmp/test_output.pcap ]; then
    SIZE=$(stat -f%z /tmp/test_output.pcap 2>/dev/null || stat -c%s /tmp/test_output.pcap)
    echo "Output size: $SIZE bytes"
    
    # 验证 PCAP 格式
    if command -v tcpdump &> /dev/null; then
        PACKET_COUNT=$(tcpdump -r /tmp/test_output.pcap 2>/dev/null | wc -l)
        echo "Packet count: $PACKET_COUNT"
    fi
fi

echo ""

# 异步任务测试
echo "2. Async Job Test"
START_TIME=$(($(date +%s%3N) - 3600000))  # 1 小时前
END_TIME=$(date +%s%3N)

JOB_RESPONSE=$(curl -s -X POST "$API_URL/api/v1/pcap/jobs" \
    -H "Content-Type: application/json" \
    -d "{
        \"tenant_id\": \"$TENANT_ID\",
        \"start_time\": $START_TIME,
        \"end_time\": $END_TIME
    }")

JOB_ID=$(echo "$JOB_RESPONSE" | jq -r '.job_id')
echo "Job created: $JOB_ID"

# 轮询任务状态
MAX_WAIT=60
ELAPSED=0
while [ $ELAPSED -lt $MAX_WAIT ]; do
    STATUS=$(curl -s "$API_URL/api/v1/pcap/jobs/$JOB_ID" | jq -r '.status')
    echo "Job status: $STATUS (${ELAPSED}s)"
    
    if [ "$STATUS" = "completed" ] || [ "$STATUS" = "failed" ]; then
        break
    fi
    
    sleep 2
    ELAPSED=$((ELAPSED + 2))
done

# 获取最终结果
FINAL_RESULT=$(curl -s "$API_URL/api/v1/pcap/jobs/$JOB_ID")
echo "Final result: $FINAL_RESULT"

echo ""
echo "3. Concurrent Request Test"
for i in {1..5}; do
    curl -s -X POST "$API_URL/api/v1/pcap/jobs" \
        -H "Content-Type: application/json" \
        -d "{
            \"tenant_id\": \"$TENANT_ID\",
            \"src_ip\": \"192.168.1.$i\",
            \"start_time\": $START_TIME,
            \"end_time\": $END_TIME
        }" &
done
wait

echo ""
echo "=== Benchmark Complete ==="