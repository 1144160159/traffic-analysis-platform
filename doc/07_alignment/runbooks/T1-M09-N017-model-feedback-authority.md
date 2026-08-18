# T1-M09-N017 TP/FP 反馈权威与 outbox

本任务保留原有 `alert.feedback.v1` 兼容链路，新增默认关闭的 `model.feedback.v1` 版本化仲裁路径。`POST /v1/alerts/{id}/feedback` 沿用原 API 版本，只增加 `adjudication_state` 和 `expected_label_revision`；没有新增数据库 migration、API version 或 event version。

## 代码所有权

- `FeedbackHandler.submitModelFeedbackRevision`：只使用服务端告警 `event_id` 作为 prediction identity，并绑定告警当前 `model_version`、`rule_version`；缺任一字段即 fail-closed。
- `FeedbackHandler.commitModelFeedbackRevision`：对 tenant+prediction 的 SHA-256 材料取得 PostgreSQL advisory transaction lock；同一事务内完成 revision head 校验、`alert_feedback`、`audit_logs` 和 `alert_feedback_outbox`。
- `modelFeedbackAggregateIdentity`：`feedback_id` 对 tenant+prediction 稳定，`event_id` 对单次命令稳定；二者不再混用。
- `FeedbackHandler.startTypedOutboxWorker`：旧 `alert.feedback.v1` 与新 `model.feedback.v1` 分别领取，禁止一个 worker 抢走另一事件类型。新事件的 Kafka header 从版本化 payload 派生，不能使用 legacy outbox 表中固定为 1 的兼容列冒充 revision。
- `VerifyModelFeedbackProducerReadiness`：要求非零 candidate、精确 contract hash、READY receipt 与同 topic/partition/offset 的 ACCEPTED projection receipt 联表命中。
- `main`：authority 与 producer 独立默认关闭；producer 打开而 authority/事务关闭、schema 缺失、candidate 全零或 receipt 不匹配都会终止启动。
- `AlertDetailPage` / `alertDetailApi`：读取并展示当前仲裁 revision/state/event，后续提交携带 `expected_label_revision`。

## 冲突与撤销

首次命令必须 `expected_label_revision=0` 且不能直接 `RETRACTED`。后续命令必须等于当前 head；每个 revision 写入 `previous_event_id`。重用同一 `Idempotency-Key` 和同一命令只返回原事件，不新增 audit/outbox；内容不同返回 409。`RETRACTED` 必须说明原因、label 与 head 相同，并成为终态。tenant、prediction、alert、model 或 rule identity 变化均不能覆盖原 aggregate。

## K8s 证据

2026-08-16 的 run `4c0a77ed-d4c6-49a1-9b28-8e6a93150446` 使用当前候选测试镜像 `traffic/model-feedback-test:m09-n017-20260816-r5` 和同时包含 N016–N022 前端功能的 Web 镜像 `traffic/web-ui:m09-n022-alert-detail-css-20260816-r2`。在现有 `postgres-primary.databases.svc` 上，run-scoped tenant 验证了三段 revision 链、精确命令幂等重放、stale/terminal/immutable-version 冲突、跨 tenant aggregate、原子 feedback/audit/outbox，以及 producer 关闭时 outbox 不被误标 published；清理 oracle 证明 run 数据已移除。

该运行同时确认共享 PostgreSQL 尚无 M08-N014 的 `model_feedback_revision_inbox`、`model_feedback_revision_receipt` 和 `model_feedback_consumer_readiness_receipt`。测试没有擅自创建这些目标表；readiness gate 因 schema 缺失而 fail-closed。证据位于 `doc/02_acceptance/topic1/tasks/t1-m09-n017/k8s-model-feedback-latest.json`。

## 启用与回滚

当前保持：

```text
MODEL_FEEDBACK_REVISION_AUTHORITY_V1_ENABLED=false
MODEL_FEEDBACK_REVISION_PRODUCER_V1_ENABLED=false
MODEL_FEEDBACK_CONSUMER_CANDIDATE_SHA256=0000000000000000000000000000000000000000000000000000000000000000
```

先应用并验证 M08 consumer migration，再以非零不可变 candidate 运行真实 Kafka canary，取得数据库中可与 ACCEPTED receipt 联接的 READY receipt。完成审批后才能先开 authority canary，再开 producer；不得反序。

回滚时先关闭 producer，再关闭 authority。不得删除 `alert_feedback`、outbox、audit 或已生成的 revision 事实，不得把 pending outbox 批量标记 published。旧 `alert.feedback.v1` worker 保持兼容运行，除非另有独立变更审批。
