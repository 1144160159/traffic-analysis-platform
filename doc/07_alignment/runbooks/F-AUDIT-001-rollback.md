# F-AUDIT-001 审计批次入口回滚

适用范围：`audit_batch_fail_closed_v1` 内部批次入口、`traffic.v1.AuditLogBatch`
生产、PostgreSQL 审计物化、Kafka offset、DLQ durable barrier 以及事件身份冲突检测。

## 默认状态与触发条件

- Kubernetes 候选默认设置 `AUDIT_BATCH_FAIL_CLOSED_V1_ENABLED=false`；未经批准不得在生产开启。
- 任一租户覆盖、部分批次受理、未获得 Kafka `RequireAll` ACK 却返回 202、同一
  `event_id` 对应不同载荷、PostgreSQL 未持久化而 offset 前移、DLQ 未持久化而提交
  offset，均立即停止扩大。
- 批次积压、DLQ 增长、投影水位停滞或 P95/P99 超过批准预算时停止 canary，不以 HTTP
  2xx 代替最终物化与跨存储对账。

## 停止扩大

1. 将 `AUDIT_BATCH_FAIL_CLOSED_V1_ENABLED=false`，滚动恢复已批准的 alert-service
   候选；关闭后入口返回 503，不伪造受理。
2. 停止新增 tenant canary 和批次生产，但保留既有 `audit.logs` 消费、只读查询、指标、
   offset、DLQ 与审计证据，避免扩大未完成窗口。
3. 不删除 Kafka 消息、PostgreSQL `audit_logs`、ClickHouse 投影、DLQ 对象、consumer
   group offset 或迁移；本切片没有需要向下回滚的 schema。
4. 按 `audit_batch_job_id`、事件 ID 摘要、tenant、trace、partition/offset 和
   PostgreSQL event_id 导出冻结窗口。身份冲突必须进入永久失败/DLQ，不允许覆盖旧行。
5. 任何生产开关、offset 调整、DLQ replay 或数据修复都需要变更批准、精确范围和独立
   复核；禁止直接跳过或重置未对账 offset。

## 恢复与验证

修复后在候选环境依次验证：无权限、无 tenant、tenant 覆盖、重复 event_id、缺失稳定
`created_at`、任一事件无效、Kafka ACK 失败、PG 事务失败、同 ID 同载荷重放、同 ID
异载荷冲突、DLQ 写失败和恢复重放。只有同一 tenant、trace、event_id、批次摘要、
Kafka offset、PostgreSQL 行及 ClickHouse 水位可对账，且 T+0/T+1/T+3/T+7 观察通过，
才可重新开启 canary。

202 仅表示完整不可变批次已获 Kafka ACK，始终不是 PostgreSQL/ClickHouse 最终成功。
回滚关闭新入口但保留既有 `/audit-log` 页面和审计命令兼容能力。
