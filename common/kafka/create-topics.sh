#!/bin/bash
# =============================================================================
# Kafka Topic 创建脚本 — 合并 old 设计 + 新增 topic
# 用法: bash create-topics.sh [bootstrap_server]
# =============================================================================
BOOTSTRAP=${1:-localhost:9092}
KAFKA_BIN=${KAFKA_BIN:-/opt/kafka/bin/kafka-topics.sh}
REPLICATION_FACTOR=${KAFKA_REPLICATION_FACTOR:-3}

echo "Bootstrap: $BOOTSTRAP"

# 格式: topic:partitions:retention_ms:retention_bytes:key:message_type
for entry in \
  "flow.events.v1:16:86400000:268435456:tenant_id+community_id:FlowEvent" \
  "session.events.v1:8:86400000:268435456:tenant_id+community_id:SessionEvent" \
  "feature.stat.v1:12:86400000:268435456:tenant_id+community_id:FeatureStatV1" \
  "detections.v1:6:86400000:268435456:tenant_id+community_id:DetectionBatch" \
  "alerts.v1:8:259200000:268435456:tenant_id+alert_id:Alert" \
  "alert.feedback.v1:3:259200000:268435456:tenant_id+alert_id:AlertFeedback" \
  "alert.response.requested.v1:3:259200000:268435456:tenant_id+job_id:AlertResponseRequested" \
  "pcap.index.v1:8:259200000:536870912:tenant_id+probe_id:PcapIndexMeta" \
  "rule.updates:1:86400000:134217728:rule_id:RuleCommand" \
  "model-updates:1:86400000:134217728:model_id:ModelUpdateEvent" \
  "model-update-applied.v1:4:259200000:134217728:event_id:ModelUpdateAppliedAckV1Json" \
  "model-actions.v1:3:259200000:134217728:model_id:ModelActionRequestedV1Json" \
  "deployment.events.v1:6:259200000:268435456:deployment_id:DeploymentEventV1Json" \
  "audit.logs:3:259200000:268435456:tenant_id:AuditLog" \
  "asset.bindings.v1:4:86400000:134217728:tenant_id+mac:MacIpBinding" \
  "device.logs.v1:8:259200000:268435456:tenant_id+device_ip:DeviceLog" \
  "user.events.v1:4:259200000:268435456:tenant_id+user_id:UserEvent" \
  "threat.intel.v1:3:604800000:134217728:tenant_id+indicator:ThreatIntel" \
  "dlq.v1:4:604800000:268435456:tenant_id:DeadLetter"; do

  name=$(echo "$entry" | cut -d: -f1)
  parts=$(echo "$entry" | cut -d: -f2)
  ret=$(echo "$entry" | cut -d: -f3)
  ret_bytes=$(echo "$entry" | cut -d: -f4)
  key=$(echo "$entry" | cut -d: -f5)
  msg=$(echo "$entry" | cut -d: -f6)

  echo "Creating: $name (partitions=$parts, retention=${ret}ms, retention.bytes=${ret_bytes}, key=$key, msg=$msg)"
  $KAFKA_BIN --bootstrap-server "$BOOTSTRAP" --create --if-not-exists \
    --topic "$name" --partitions "$parts" --replication-factor "$REPLICATION_FACTOR" \
    --config retention.ms="$ret" \
    --config retention.bytes="$ret_bytes" 2>&1 | grep -v "WARNING"
done

echo ""
echo "=== Topic List ==="
$KAFKA_BIN --bootstrap-server "$BOOTSTRAP" --list 2>&1 | sort
