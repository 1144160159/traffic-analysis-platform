# T-PG-002 Saved View 事务与 Outbox 发布恢复

## 1. 边界

本手册覆盖 `F-ALERT-006 / SaveAlertView` 的 PostgreSQL 业务状态、history、最小审计、outbox 和幂等请求同事务提交，以及提交后的 Kafka 发布恢复。它是 T-PG-002 的首个纵向切片，不代表所有可变 PG 命令已完成盘点，也不证明下游消费、生产发布或 G2-G8 通过。

## 2. 权威状态与事件

- `alert_saved_views` 是当前业务状态，`revision` 每次同名 upsert 单调增加。
- `alert_saved_view_history` 按 `(tenant_id, view_id, revision)` 唯一，保存不可变变更事实。
- `audit_logs` 的最小审计索引与上述两者、outbox、`alert_saved_view_requests` 在同一 serializable 事务中写入。
- `alert_saved_view_requests` 用 `(tenant_id, idempotency_key)` 防止重复副作用；相同键不同 payload 返回冲突。
- `alert_saved_view_outbox` 保存稳定 `event_id`、aggregate version、tenant、trace、payload 和发布状态。
- Kafka 主题为 `alert.saved-view.events.v1`，分区键为 `tenant_id:view_id`；当前目录状态是 `producer_only`，不得宣称下游投影已经闭环。

## 3. 发布状态机

worker 只领取 `pending/processing`、已到重试时间且租约为空或过期的记录，使用 `FOR UPDATE SKIP LOCKED` 避免多副本重复领取。领取后置为 `processing` 并持有 60 秒租约；Kafka required-acks 成功后才置为 `published`。

发送失败、载荷损坏或信封非法时，`publish_attempts` 增加并指数退避，最多 300 秒。第 10 次失败转为 `dead`，不会热循环。Kafka ACK 后、PG 标记前失败会形成至少一次重复投递；稳定 `event_id + aggregate_version` 是下游幂等键，未完成真实消费者证明前保持门禁开放。

## 4. 扩展、灰度与观测

1. 先执行 `202608031100_alert_saved_view_transaction_v2.sql`，核对四个 schema 入口一致。
2. 创建主题并应用生成的 literal ACL；确认 `traffic-alert-service` 仅获得该主题的 `Describe/Write`。
3. 发布候选镜像但先只开放内部 tenant，持续观测：

```sql
SELECT status,count(*) AS rows,min(occurred_at) AS oldest,max(publish_attempts) AS max_attempts
FROM alert_saved_view_outbox GROUP BY status ORDER BY status;

SELECT event_id,tenant_id,aggregate_id,aggregate_version,status,publish_attempts,
       next_retry_at,locked_by,locked_until,last_error
FROM alert_saved_view_outbox
WHERE status IN ('pending','processing','dead')
ORDER BY occurred_at LIMIT 100;
```

4. 对同一 tenant/view 比对 history revision、outbox aggregate version、Kafka event ID 和审计 event/detail；候选扩大前差异必须为 0。
5. 注入 broker 不可用、ACK 后阻断 PG update、worker kill 和租约过期，证明恢复后不丢事件、重复事件可被下游幂等处理。

## 5. 停止与回滚

出现跨租户、业务/history/audit/outbox 任一缺失、pending 持续增长、processing 租约不回收、dead 新增或 Kafka/schema 不兼容时立即停止扩大。回滚应用镜像不会删除新表或字段；旧路径不得恢复为请求内直接发 Kafka。保留 outbox，由兼容 worker 恢复发布。任何人工重放必须保留原 `event_id`，禁止复制成新业务事件。

仓库侧合同和故障边界测试通过后，本项仅可置为 `IMPLEMENTING`。生产 migration、候选 Kafka、真实消费者幂等、故障、性能、回滚和 T+0/T+1/T+3/T+7 观察证据齐全后，才能申请后续门禁。
