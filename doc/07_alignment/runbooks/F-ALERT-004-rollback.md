# F-ALERT-004 批量指派回滚手册

## 适用范围

本手册仅覆盖默认关闭的 `ALERT_BATCH_ASSIGNMENT_V1_ENABLED`、`ALERT_BATCH_ASSIGNMENT_PIPELINE_V1_ENABLED` 与 `ALERT_BATCH_ASSIGNMENT_COMPENSATION_V1_ENABLED` 路径。该路径把服务端冻结选择、批次、逐项状态、history、outbox、inbox、投影回执、补偿请求及逐项回执、DLQ确认回执和审计写入 PostgreSQL，并通过 `alert.assignment.events.v1` 独立消费后确定性投影 ClickHouse；`202 accepted` 不代表任何告警已经完成指派或补偿。

## 停止扩大

1. 先将 `ALERT_BATCH_ASSIGNMENT_COMPENSATION_V1_ENABLED=false` 停止接收新的补偿，再将 `ALERT_BATCH_ASSIGNMENT_PIPELINE_V1_ENABLED=false` 停止新的 dispatcher/consumer，最后将 `ALERT_BATCH_ASSIGNMENT_V1_ENABLED=false`；按单实例串行方式回滚 alert-service，禁止在仍有生产实例消费时改组或删除 topic。
2. 前端同时保持 `VITE_ALERT_BATCH_ASSIGNMENT_V1_ENABLED=false`，继续使用兼容期内的旧逐条指派入口。
3. 禁止删除或改写 `alert_assignment_selections`、`alert_assignment_batches`、`alert_assignment_batch_items`、三类 assignment history、`alert_assignment_states`、补偿请求/items/history/投影回执、outbox、inbox、assignment 投影回执、DLQ回执、请求收据和 `audit_logs`。
4. 不得把 `pending` outbox、`accepted/projecting` item 人工改成 `published/applied`；不得手工提交尚未形成 inbox 或 DLQ确认回执的 Kafka offset。
5. 保留当前 `ALERT_BATCH_SELECTION_SIGNING_SECRET`，至少覆盖全部未过期选择和在途重试窗口；密钥轮换必须先证明旧选择的查询与幂等回放迁移，不能直接使合法重试失效。

## 在途事实裁决

- 对每个 batch 按 tenant、batch_id、selection_id、selection SHA、trace_id、event_id 和 revision 对账。
- `accepted/running` 均视为非最终态；只有独立 consumer receipt、逐项 revision、ClickHouse精确目标版本和审计齐全时才允许形成 `applied`。
- changed 事件必须先与 PostgreSQL canonical outbox 事件及全部 `projecting` items 完整一致，再允许任何 ClickHouse 写入；截断、增加、换序、重复 position 或 identity 漂移均隔离到 `dlq.v1`。
- 同一 event/source tuple 的 inbox 精确重放直接成功；event identity 或 source tuple 碰撞必须失败关闭，不能再次投影。
- 超时、租约丢失或传输结果不确定时不得重新调用旧逐条指派接口；必须先查询权威 receipt。
- 同一 selection token 只能消费一次；回滚不得清空 `consumed_by_batch_id` 以制造第二次派发。
- selection token 由租户、selection_id 与专用密钥确定性派生，PostgreSQL 只保存 SHA-256；排障和导出不得记录明文 bearer token。
- 补偿只能针对原批次已形成 `applied` 的逐项事实；仅当当前 PostgreSQL/ClickHouse 权威状态仍与原 `resulting_state_version/resulting_status/assignee` 完全一致时，才追加更高版本的逆向投影。发生后续 revision 时必须记录 `REVISION_CONFLICT`，禁止覆盖新事实。
- 同一 batch 只允许一个补偿 aggregate；exact replay 只能返回持久化回执，不能重复写逆向投影。关闭补偿开关不会取消已进入 outbox/inbox 的事实，必须按 event_id、request_id 和 source tuple 裁决在途状态。

## 恢复与前滚

1. 在独立环境连续回放 `202608091900`、`202608092130` 和 `202608092300` migration，确认无 destructive DDL，并校验 Docker/Kubernetes 兼容入口与版本化 migration 字节同步。
2. 只读核对 batch/items/history/outbox/request、compensation requests/items/history/receipts 和 audit 数量与 SHA；差异必须形成新的 occurrence。
3. 经领域、QA、SRE、安全批准后，先 shadow dispatcher，再 canary consumer，最后单独开启补偿入口；逐项验证 stale revision、unauthorized assignee、partial、Kafka重投、DLQ确认屏障、补偿 exact replay、后续 revision 冲突和回滚在途边界。
4. 完成 T+0、T+1、T+3、T+7 观察前不得关闭 F-ALERT-004。
