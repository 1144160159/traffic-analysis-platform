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
  "flow.projection-replay.v1:6:604800000:268435456:tenant_id+community_id:FlowEvent" \
  "session.events.v1:8:86400000:268435456:tenant_id+community_id:SessionEvent" \
  "feature.stat.v1:12:86400000:268435456:tenant_id+community_id:FeatureStat" \
  "detections.v1:6:86400000:268435456:tenant_id+community_id:DetectionBehavior" \
  "alerts.v1:8:259200000:268435456:tenant_id+community_id:Alert" \
  "alert.feedback.v1:3:259200000:268435456:tenant_id+alert_id:AlertFeedbackExtendedV1Json" \
  "alert.response.requested.v1:3:259200000:268435456:tenant_id+job_id:AlertResponseRequestedV1Json" \
  "alert.saved-view.events.v1:3:604800000:134217728:tenant_id+view_id:AlertSavedViewSavedV1Json" \
  "whitelist.events.v2:3:604800000:134217728:tenant_id+entry_id:WhitelistLifecycleV2Json" \
  "notification.governance.events.v1:3:604800000:134217728:tenant_id+rule_id:NotificationRuleLifecycleV1Json" \
  "campaign.events.v2:6:604800000:268435456:tenant_id+campaign_id:CampaignAggregateLifecycleV2Json" \
  "campaign.membership.events.v2:6:604800000:268435456:tenant_id+campaign_id:CampaignMembershipLifecycleV2Json" \
  "playbook.execution.events.v2:6:604800000:268435456:tenant_id+execution_id:PlaybookExecutionLifecycleV2Json" \
  "traffic.topic.action.v2:6:604800000:134217728:tenant_id+topic:TopicActionLifecycleV2Json" \
  "dashboard.task.events.v1:6:604800000:134217728:tenant_id+task_id:DashboardTaskLifecycleV1Json" \
  "probe.control.v2:6:259200000:134217728:tenant_id+probe_id:ProbeOperationRequestedV2Json" \
  "probe.acks.v2:6:604800000:134217728:tenant_id+probe_id:ProbeOperationAgentAckV2Json" \
  "dlq.probe.acks.v2:3:604800000:134217728:tenant_id+probe_id:DLQMessageV1Json" \
  "probe.events.v2:6:604800000:134217728:tenant_id+probe_id:ProbeOperationLifecycleV2Json" \
  "pcap.index.v1:8:259200000:536870912:tenant_id+probe_id:PcapIndexMeta" \
  "rule.updates:1:86400000:134217728:rule_id:RuleCommandV1Json" \
  "model-updates:1:86400000:134217728:model_id:ModelUpdateEventV1Json" \
  "model-update-applied.v1:4:259200000:134217728:event_id:ModelUpdateAppliedAckV1Json" \
  "model-actions.v1:3:259200000:134217728:model_id:ModelActionRequestedV1Json" \
  "deployment.events.v1:6:259200000:268435456:deployment_id:DeploymentEventV1Json" \
  "audit.logs:3:259200000:268435456:tenant_id:AuditEventV1Json" \
  "asset.bindings.v1:4:86400000:134217728:tenant_id+mac:MacIpBinding" \
  "asset.events.v2:6:604800000:268435456:tenant_id+asset_id:AssetUpsertedV2Json" \
  "asset.discovery.events.v1:3:604800000:134217728:tenant_id+run_id:AssetDiscoveryLifecycleV1Json" \
  "asset.exports.v1:3:604800000:134217728:tenant_id+job_id:AssetExportLifecycleV1Json" \
  "device.logs.v1:8:259200000:268435456:tenant_id+device_ip:DeviceLog" \
  "user.events.v1:4:259200000:268435456:tenant_id+user_id:UserEvent" \
  "threat.intel.v1:3:604800000:134217728:tenant_id (legacy composite v1 keys accepted only for replay):ThreatIntelV1Json" \
  "dlq.v1:4:604800000:268435456:tenant_id:DLQMessageV1Json"; do

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
