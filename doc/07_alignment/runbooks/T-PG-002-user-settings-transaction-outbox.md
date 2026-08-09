# T-PG-002 用户偏好事务、审计与 UserEvent Outbox 纵切

## 本次关闭的源码缺口

`PUT/PATCH /api/v1/auth/settings/{category}` 不再直接 upsert 后异步补审计，也不再在仓储缺失时返回伪成功。命令在 serializable PostgreSQL 事务中统一提交 `user_settings`、`user_settings_history`、兼容的 `USER_UPDATE` 审计、`user_settings_outbox` 和 `user_settings_requests`。

命令接受稳定 `action_id`、`Idempotency-Key`、原因和 `expected_revision`。同 key、同 payload 返回已提交结果；同 key、不同 payload 返回幂等冲突；陈旧 revision 返回版本冲突。请求中的命令元数据与实际 settings 文档分离，非法元数据返回 400，审计明细不保存偏好正文或敏感值。

## 发布语义

auth-service 启动独立 outbox worker，将确定性 event ID、tenant、user、aggregate version 和 partition key 发布到 `user.events.v1`。worker 使用 `FOR UPDATE SKIP LOCKED`、60 秒租约、最多 10 次指数退避和 dead 状态；只有 protobuf `UserEvent` 获得 Kafka ACK 后才将行标记为 published。ACK 后标记失败会以同一 event ID 重试，消费者必须按 event ID 和 aggregate version 幂等。

`user.events.v1` 已同步 canonical topic、ACL、auth-service 配置和 Kubernetes 环境变量，目录状态由 consumer-only 提升为 active。此次没有向生产 Kafka 创建 topic 或应用 ACL。

## Schema 与验证

expand-only migration 为 `202608031500_user_settings_transaction_v2.sql`。IAM 四张表已同步至 common、Docker merged 与 Kubernetes ConfigMap。隔离 PostgreSQL 16 对三入口各回放两次，22 张受管表共 297 列的列、约束和索引摘要一致，SHA-256 均为 `80610729fc7a60e5781ec020fd2edeffdc9ae26a45f0b0f77b61f56e08c88253`；临时数据库与容器在验证后删除。

```bash
go -C go/control-plane test ./cmd/auth-service ./internal/auth/config ./internal/auth/repository ./internal/auth/service ./internal/auth/api -count=1
npm --prefix web/ui test -- --run src/services/pageApiPlans.test.ts
python3 scripts/alignment/check_migrations.py
python3 scripts/alignment/verify_pg_schema_entrypoints_ephemeral.py --run-id iam-user-settings-20260803-01
python3 scripts/alignment/verify_pg_transaction_outbox.py
python3 scripts/alignment/inventory_pg_mutations.py --check
python3 scripts/alignment/check_event_catalog.py
python3 scripts/alignment/generate_kafka_acl_plan.py --check-generated
```

## 未关闭门禁

真实 release-candidate PostgreSQL/Kafka/Flink、端到端消费与跨存储对账、commit/publish/ACK 边界故障注入、锁竞争和 outbox lag 性能、浏览器实际保存、灰度、回滚与 T+0/T+1/T+3/T+7 观察仍开放。因此本纵切只能标记为 repository/component complete，不能写成 F-PREF-001、T-PG-002、G7 或项目整体完成。
