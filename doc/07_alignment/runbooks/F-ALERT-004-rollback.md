# F-ALERT-004 批量指派回滚手册

## 适用范围

本手册仅覆盖默认关闭的 `ALERT_BATCH_ASSIGNMENT_V1_ENABLED` 路径。该路径把服务端冻结选择、批次、逐项状态、history、outbox、请求收据和审计写入 PostgreSQL；`202 accepted` 不代表任何告警已经完成指派。

## 停止扩大

1. 将 `ALERT_BATCH_ASSIGNMENT_V1_ENABLED=false`，按单实例串行方式回滚 alert-service。
2. 前端同时保持 `VITE_ALERT_BATCH_ASSIGNMENT_V1_ENABLED=false`，继续使用兼容期内的旧逐条指派入口。
3. 禁止删除或改写 `alert_assignment_selections`、`alert_assignment_batches`、`alert_assignment_batch_items`、两类 history、outbox、请求收据和 `audit_logs`。
4. 停止尚未批准的 dispatcher/consumer；不得把 `pending` outbox 或 `accepted` item 人工改成 `published/applied`。
5. 保留当前 `ALERT_BATCH_SELECTION_SIGNING_SECRET`，至少覆盖全部未过期选择和在途重试窗口；密钥轮换必须先证明旧选择的查询与幂等回放迁移，不能直接使合法重试失效。

## 在途事实裁决

- 对每个 batch 按 tenant、batch_id、selection_id、selection SHA、trace_id、event_id 和 revision 对账。
- `accepted/running` 均视为非最终态；只有独立 consumer receipt、逐项 revision 和审计齐全时才允许形成 `applied`。
- 超时、租约丢失或传输结果不确定时不得重新调用旧逐条指派接口；必须先查询权威 receipt。
- 同一 selection token 只能消费一次；回滚不得清空 `consumed_by_batch_id` 以制造第二次派发。
- selection token 由租户、selection_id 与专用密钥确定性派生，PostgreSQL 只保存 SHA-256；排障和导出不得记录明文 bearer token。

## 恢复与前滚

1. 在独立环境连续回放 migration，确认无 destructive DDL。
2. 只读核对 batch/items/history/outbox/request/audit 数量与 SHA；差异必须形成新的 occurrence。
3. 经领域、QA、SRE、安全批准后，先 shadow dispatcher，再 canary consumer；逐项验证 stale revision、unauthorized assignee、partial 和补偿边界。
4. 完成 T+0、T+1、T+3、T+7 观察前不得关闭 F-ALERT-004。
