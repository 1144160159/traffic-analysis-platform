# F-ALERT-006 保存视图事务回滚手册

## 1. 适用边界

本手册只覆盖 `F-ALERT-006` 的保存视图 revision 门禁、PostgreSQL 原子事务和 `alert.saved-view.events.v1` outbox dispatcher。回滚不删除保存视图、history、审计、请求收据或 outbox，不回退到请求内直接发送 Kafka，也不代表 G6 或发布观察已经完成。

## 2. 触发条件

出现以下任一事实时停止扩大：

- 陈旧 `expected_revision` 仍改变了视图或产生 history、audit、outbox、request 行；
- 同一 tenant/idempotency key 的不同 payload 未返回冲突；
- 业务状态、history、audit、outbox 和 request receipt 数量或 trace/revision 不一致；
- `processing` 租约不回收、`dead` 持续新增、pending 年龄或发布重试超过批准预算；
- Kafka event identity、aggregate version、tenant、trace 或分区键与 PostgreSQL 不一致。

## 3. 停止扩大

1. 将候选配置恢复为 `ALERT_SAVED_VIEW_TRANSACTION_V2_ENABLED=false`，仅停止 dispatcher；已提交 outbox 保持 `pending`。
2. 停止继续扩大 canary tenant，不删除或改写已提交事实。
3. 保存候选源码哈希、镜像摘要、配置 effective hash、tenant、trace、view、event、outbox 和 Kafka offset 清单。
4. 若保存 API 的 revision 或事务边界本身异常，回滚到上一候选镜像；数据库继续保留加法字段和表。

## 4. 回滚后核验

```sql
SELECT status,count(*) AS rows,min(occurred_at) AS oldest,max(publish_attempts) AS max_attempts
FROM alert_saved_view_outbox
GROUP BY status ORDER BY status;

SELECT v.tenant_id,v.view_id,v.revision,
       count(DISTINCT h.revision) AS history_revisions,
       count(DISTINCT o.aggregate_version) AS outbox_revisions,
       count(DISTINCT r.resulting_revision) AS request_revisions
FROM alert_saved_views v
LEFT JOIN alert_saved_view_history h ON h.tenant_id=v.tenant_id AND h.view_id=v.view_id
LEFT JOIN alert_saved_view_outbox o ON o.tenant_id=v.tenant_id AND o.aggregate_id=v.view_id
LEFT JOIN alert_saved_view_requests r ON r.tenant_id=v.tenant_id AND r.view_id=v.view_id
GROUP BY v.tenant_id,v.view_id,v.revision
ORDER BY v.tenant_id,v.view_id;
```

确认 dispatcher 已停、API 不再扩大、已有视图仍可按 tenant 读取、所有 pending/dead 事件都有负责人和处置记录。只有经审查的兼容 worker 才能使用原 `event_id + aggregate_version` 恢复投递。

## 5. 禁止操作与恢复条件

- 禁止删除 `alert_saved_views`、history、outbox、requests 或 audit 行；禁止重建 event ID；禁止手工把 pending/dead 直接改为 published。
- 禁止将省略 revision 的兼容流量解释为严格客户端验收通过；必须分别记录 strict 与 legacy 请求量。
- 恢复扩大前，须用同一候选复证创建、更新、精确重试、payload 碰撞、陈旧 revision、审计失败全回滚、Kafka ACK 后 PG 标记失败恢复和下游幂等，并完成 QA、SRE、安全及领域负责人裁决。
