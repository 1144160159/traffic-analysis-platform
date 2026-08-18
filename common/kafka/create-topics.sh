#!/bin/bash
# =============================================================================
# Kafka Topic 创建脚本 — 合并 old 设计 + 新增 topic
# 用法: bash create-topics.sh [bootstrap_server]
#
# 真源声明: K8s 运行时真源是 deployments/kubernetes/init-jobs/01-kafka-topics.yaml
# (init job 实际创建 topic)。本脚本是开发/本地对账辅助,必须与 init job 的
# topic 列表保持同步;任何新增 topic 需两处同时修改。
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
  "detections.behavior.v1:6:259200000:268435456:tenant_id+community_id:DetectionBehavior" \
  "detections.business.v1:6:259200000:268435456:tenant_id+community_id:DetectionBusiness" \
  "l2.trigger.v1:4:86400000:134217728:tenant_id+community_id:L2TriggerEvent" \
  "alerts.v1:8:259200000:268435456:tenant_id+community_id:Alert" \
  "alert.feedback.v1:3:259200000:268435456:tenant_id+alert_id:AlertFeedbackExtendedV1Json" \
  "model.feedback.v1:3:604800000:134217728:tenant_id+feedback_id:ModelFeedbackAdjudicatedV1Json" \
  "alert.response.requested.v1:3:259200000:268435456:tenant_id+job_id:AlertResponseRequestedV1Json" \
  "alert.assignment.events.v1:6:604800000:134217728:tenant_id+batch_id:AlertBatchAssignmentLifecycleV1Json" \
  "alert.saved-view.events.v1:3:604800000:134217728:tenant_id+view_id:AlertSavedViewSavedV1Json" \
  "whitelist.events.v2:3:604800000:134217728:tenant_id+entry_id:WhitelistLifecycleV2Json" \
  "notification.governance.events.v1:3:604800000:134217728:tenant_id+rule_id:NotificationRuleLifecycleV1Json" \
  "campaigns.v1:6:259200000:268435456:tenant_id+campaign_id:Campaign" \
  "campaign.events.v2:6:604800000:268435456:tenant_id+campaign_id:CampaignAggregateLifecycleV2Json" \
  "campaign.membership.events.v2:6:604800000:268435456:tenant_id+campaign_id:CampaignMembershipLifecycleV2Json" \
  "alert.evidence-links.v1:6:604800000:134217728:tenant_id+alert_id:AlertEvidenceLinkLifecycleV1Json" \
  "playbook.execution.events.v2:6:604800000:268435456:tenant_id+execution_id:PlaybookExecutionLifecycleV2Json" \
  "traffic.topic.action.v2:6:604800000:134217728:tenant_id+topic:TopicActionLifecycleV2Json" \
  "dashboard.task.events.v1:6:604800000:134217728:tenant_id+task_id:DashboardTaskLifecycleV1Json" \
  "fusion.commands.v1:6:604800000:134217728:tenant_id+job_id:FusionSourceSyncRequestedV1Json" \
  "baseline.lifecycle.v1:3:604800000:134217728:tenant_id+baseline_id:BehaviorBaselineLifecycleV1Json" \
  "baseline.activation-acks.v1:3:604800000:67108864:tenant_id+baseline_id:BehaviorBaselineActivationAckV1Json" \
  "graph.projections.v1:6:604800000:268435456:tenant_id+projection_id:GraphProjectionEvent" \
  "probe.control.v2:6:259200000:134217728:tenant_id+probe_id:ProbeOperationRequestedV2Json" \
  "probe.group-readiness.v1:3:86400000:67108864:consumer_group:ProbeGroupReadinessReceiptV1" \
  "probe.acks.v2:6:604800000:134217728:tenant_id+probe_id:ProbeOperationAgentAckV2Json" \
  "dlq.probe.acks.v2:3:604800000:134217728:tenant_id+probe_id:DLQMessageV1Json" \
  "probe.events.v2:6:604800000:134217728:tenant_id+probe_id:ProbeOperationLifecycleV2Json" \
  "pcap.index.v1:8:259200000:536870912:tenant_id+probe_id:PcapIndexMeta" \
  "rule.updates:1:86400000:134217728:rule_id:RuleCommandV1Json" \
  "rule-update-applied.v1:4:259200000:134217728:event_id:RuleUpdateAppliedAckV1Json" \
  "model-updates:1:86400000:134217728:model_id:ModelUpdateEventV1Json" \
  "model-update-applied.v1:4:259200000:134217728:event_id:ModelUpdateAppliedAckV1Json" \
  "model-shadow-observations.v1:6:604800000:268435456:observation_id:ChampionChallengerObservationV1Json" \
  "model-actions.v1:3:259200000:134217728:model_id:ModelActionRequestedV1Json" \
  "deployment.events.v1:6:259200000:268435456:deployment_id:DeploymentEventV1Json" \
  "audit.logs:3:259200000:268435456:tenant_id:AuditEventV1Json" \
  "asset.bindings.v1:4:86400000:134217728:utf8(tenant_id + ':' + canonical_mac_address):MacIpBinding" \
  "asset.events.v2:6:604800000:268435456:tenant_id+asset_id:AssetUpsertedV2Json" \
  "asset.discovery.events.v1:3:604800000:134217728:tenant_id+run_id:AssetDiscoveryLifecycleV1Json" \
  "asset.exports.v1:3:604800000:134217728:tenant_id+job_id:AssetExportLifecycleV1Json" \
  "device.logs.v1:8:259200000:268435456:tenant_id+device_ip:DeviceLog" \
  "user.events.v1:4:259200000:268435456:tenant_id+user_id:UserEvent" \
  "threat.intel.v1:3:604800000:134217728:tenant_id (legacy composite v1 keys accepted only for replay):ThreatIntelV1Json" \
  "dlq.v1:4:604800000:268435456:tenant_id:DLQMessageV1Json" \
  "dlq.cep-job:2:604800000:268435456:tenant_id:DLQMessageV1Json" \
  "dlq.feature-job:2:604800000:268435456:tenant_id:DLQMessageV1Json" \
  "dlq.rule-job:2:604800000:268435456:tenant_id:DLQMessageV1Json"; do

  name=${entry%%:*}; remainder=${entry#*:}
  parts=${remainder%%:*}; remainder=${remainder#*:}
  ret=${remainder%%:*}; remainder=${remainder#*:}
  ret_bytes=${remainder%%:*}; remainder=${remainder#*:}
  key=${remainder%:*}
  msg=${remainder##*:}

  echo "Creating: $name (partitions=$parts, retention=${ret}ms, retention.bytes=${ret_bytes}, key=$key, msg=$msg)"
  $KAFKA_BIN --bootstrap-server "$BOOTSTRAP" --create --if-not-exists \
    --topic "$name" --partitions "$parts" --replication-factor "$REPLICATION_FACTOR" \
    --config retention.ms="$ret" \
    --config retention.bytes="$ret_bytes" 2>&1 | grep -v "WARNING"
done

echo ""
echo "=== Topic List ==="
$KAFKA_BIN --bootstrap-server "$BOOTSTRAP" --list 2>&1 | sort
