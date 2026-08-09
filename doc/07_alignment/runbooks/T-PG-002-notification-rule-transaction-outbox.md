# T-PG-002 通知规则事务、历史与 Outbox 纵切

## 本次关闭的源码缺口

通知规则、通知模板、升级策略、静默规则和全局设置命令现在通过 serializable PostgreSQL 事务统一提交：业务表、带 actor/reason/trace 的 `audit_logs`、`notification_governance_history`、`notification_governance_outbox` 和 `notification_governance_requests`。命令使用 tenant + `Idempotency-Key` 的事务级 advisory lock；相同 key 和 payload 重放已提交响应，不同 payload、陈旧 revision 或 template version 返回冲突。

Web UI 为规则、模板、升级策略、静默规则和设置的创建、启停与保存提交 `action_id`、原因、预期 revision/version 与 `Idempotency-Key`。响应包含 revision/version、event ID、outbox 状态和重放标记；没有 PostgreSQL 仓库时设置命令返回 503，不再伪装保存成功。

## 发布语义

outbox worker 使用 `FOR UPDATE SKIP LOCKED`、60 秒租约、最多 10 次指数退避和 dead 状态。只有 Kafka `SendJSON` 返回 ACK 后才标记 published；ACK 成功但标记失败时允许携带同一 event ID 和 aggregate version 的 at-least-once 重复。

`notification.governance.events.v1` 已进入 JSON Schema、canonical topic、ACL、topic init、alert-service 配置和 Kubernetes 部署。当前目录明确标记为 `producer_only`，不虚构尚未实现的 consumer。

## Schema 与验证

版本化 migration 为 `202608031300_notification_rule_transaction_v2.sql`，同步到 common、Docker merged 和 Kubernetes ConfigMap。隔离 PostgreSQL 16 对三入口各回放两次，18 张 playbook/saved-view/notification 相关表共 253 列的列、约束和索引摘要一致，SHA-256 均为 `2cb4d289e72b06275345e014d64be18c473efe280785ee9b17fe8031e7f327ac`。

```bash
python3 scripts/alignment/verify_pg_transaction_outbox.py
python3 scripts/alignment/verify_pg_schema_entrypoints_ephemeral.py --run-id notification-rule-v1
go -C go/control-plane test ./internal/alert/api ./internal/alert/config ./cmd/alert-service -count=1
cd web/ui && npx vitest run src/services/notificationGovernanceApi.test.ts
make alignment-validate
```

## 未关闭门禁

真实 release-candidate PostgreSQL/Kafka、独立 consumer、故障注入、PG-Kafka-audit 对账、锁竞争、outbox lag/dead 告警、浏览器、灰度、回滚和 T+0/T+1/T+3/T+7 观察仍开放。该纵切不得写成 T-PG-002、F-NOTIFICATION-001 或项目整体完成。
