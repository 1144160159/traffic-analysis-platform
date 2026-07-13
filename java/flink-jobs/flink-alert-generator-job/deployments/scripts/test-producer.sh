#!/bin/bash
################################################################################
# 测试数据生产者 - 向 Kafka 发送模拟检测数据
#
# 用法:
#   ./test-producer.sh [count] [interval_ms]
#
# 示例:
#   ./test-producer.sh 100 100   # 发送 100 条，间隔 100ms
################################################################################

set -e

KAFKA_BROKER="${KAFKA_BROKER:-localhost:9093}"
TOPIC_BEHAVIOR="${TOPIC_BEHAVIOR:-detections.behavior.v1}"
TOPIC_BUSINESS="${TOPIC_BUSINESS:-detections.business.v1}"

COUNT="${1:-10}"
INTERVAL_MS="${2:-1000}"

echo "============================================================"
echo "  测试数据生产者"
echo "============================================================"
echo "  Kafka Broker:  $KAFKA_BROKER"
echo "  Behavior Topic: $TOPIC_BEHAVIOR"
echo "  Business Topic: $TOPIC_BUSINESS"
echo "  Count:         $COUNT"
echo "  Interval:      ${INTERVAL_MS}ms"
echo "============================================================"

# 检查 Kafka 是否可用
if ! nc -z localhost 9093 2>/dev/null; then
    echo "❌ Kafka 不可用，请先启动服务"
    exit 1
fi

# 生成随机 IP
random_ip() {
    echo "$((RANDOM % 256)).$((RANDOM % 256)).$((RANDOM % 256)).$((RANDOM % 256))"
}

# 生成 UUID
uuid() {
    cat /proc/sys/kernel/random/uuid 2>/dev/null || uuidgen 2>/dev/null || echo "$(date +%s)-$(( RANDOM ))"
}

# 检测类型
DETECTION_TYPES=("malware" "c2" "exfiltration" "lateral_movement" "brute_force" "scan" "dos")
SEVERITIES=("low" "medium" "high" "critical")

echo ""
echo "🚀 开始发送测试数据..."

for ((i=1; i<=COUNT; i++)); do
    TIMESTAMP=$(date +%s%3N)
    EVENT_ID=$(uuid)
    COMMUNITY_ID="1:$(echo -n "$(random_ip):$(random_ip):$((RANDOM % 65535)):$((RANDOM % 65535)):6" | md5sum | cut -c1-16)"
    
    # 随机选择检测类型
    DETECTION_TYPE="${DETECTION_TYPES[$((RANDOM % ${#DETECTION_TYPES[@]}))]}"
    SCORE=$(echo "scale=2; ($RANDOM % 100) / 100" | bc)
    
    # 构造 JSON 消息 (简化版，实际应使用 Protobuf)
    JSON_MSG=$(cat <<EOF
{
  "header": {
    "event_id": "$EVENT_ID",
    "tenant_id": "default",
    "run_id": "test-run-001",
    "event_ts": $TIMESTAMP,
    "ingest_ts": $TIMESTAMP,
    "probe_id": "probe-001",
    "feature_set_id": "fs-v1"
  },
  "model_version": "v1.0.0",
  "community_id": "$COMMUNITY_ID",
  "object_type": "session",
  "object_id": "session-$(uuid)",
  "ts": $TIMESTAMP,
  "labels": ["$DETECTION_TYPE", "suspicious"],
  "scores": [$SCORE, $(echo "scale=2; $SCORE * 0.8" | bc)],
  "top_label": "$DETECTION_TYPE",
  "top_score": $SCORE
}
EOF
)
    
    # 发送到 Kafka (使用 kafkacat/kcat 或 kafka-console-producer)
    if command -v kcat &> /dev/null; then
        echo "$JSON_MSG" | kcat -P -b "$KAFKA_BROKER" -t "$TOPIC_BEHAVIOR" -k "default:$COMMUNITY_ID"
    elif command -v kafkacat &> /dev/null; then
        echo "$JSON_MSG" | kafkacat -P -b "$KAFKA_BROKER" -t "$TOPIC_BEHAVIOR" -k "default:$COMMUNITY_ID"
    else
        echo "$JSON_MSG" | docker exec -i alert-job-kafka kafka-console-producer \
            --bootstrap-server kafka:9092 \
            --topic "$TOPIC_BEHAVIOR" \
            --property "parse.key=true" \
            --property "key.separator=|" <<< "default:$COMMUNITY_ID|$JSON_MSG"
    fi
    
    echo "[$i/$COUNT] Sent: $DETECTION_TYPE (score: $SCORE) -> $COMMUNITY_ID"
    
    if [[ $i -lt $COUNT ]]; then
        sleep "$(echo "scale=3; $INTERVAL_MS / 1000" | bc)"
    fi
done

echo ""
echo "✅ 发送完成！共发送 $COUNT 条测试数据"
echo ""
echo "查看 Flink 作业状态: http://localhost:8081"
echo "查看 Kafka 消息: http://localhost:8080"
