# F-ALERT-003 告警响应任务回滚与停止扩大手册

适用范围：告警响应动作的受理、独立审批、事务 outbox、Kafka 消费、外部执行、权威回查、最终回执、取消和补偿边界。本手册不授权操作共享或生产环境；所有切换仍需候选哈希、配置哈希、变更窗口和负责人批准。

## 停止扩大条件

出现以下任一情况立即停止扩大：跨租户读取或执行；请求人与审批人相同；同一 `event_id/idempotency_key` 产生多个提供方副作用；HTTP 2xx 缺少耐久 `provider_receipt_id`、明确 `effect_state` 或 effect ID 却被写成完成；传输超时、响应丢失或权威查询失败被写成确定失败或成功；Provider 回查的 tenant、job、event、trace、幂等键或 Provider 身份与原命令不一致；action、outbox、执行回执和审计无法按同一 trace 对账；回执覆盖较新 revision；消费积压、失败率、P99 或资源超过批准预算。

## 回滚步骤

1. 先将目标候选的 `ALERT_RESPONSE_EXTERNAL_EXECUTOR_V1_ENABLED=false`，停止新的外部副作用；不得删除 API 路由或把请求改成本地伪成功。
2. 将 `ALERT_RESPONSE_EXECUTION_V1_ENABLED=false`，停止 response-action outbox dispatcher 和消费组。新请求仍可耐久记录为待审批/待执行，但 UI 和 GET 必须显示真实 pending/blocked 状态。
3. 记录候选源码与镜像哈希、配置 effective hash、最后 `tenant_id/trace_id/job_id/event_id/revision/Kafka partition/offset` 和提供方幂等键。等待在途调用达到批准超时后再停止旧 Pod，不得并行启动旧消费者。
4. 对传输结果不确定的动作，只能以完全相同的 `event_id/job_id/tenant_id/trace_id/idempotency_key` 查询 Provider。只有 `receipt_found` 且回执完整、Provider 身份一致时才可恢复完成态；`absent/unknown` 或查询失败均保持 `partial/effect_state=unknown`，不得盲目重试。
5. 禁止删除或回退 `alert_response_actions`、`alert_response_outbox`、`alert_response_execution_receipts`、`alert_response_approvals`、`alert_response_control_requests` 和相关 `audit_logs`。三次迁移均为 expand-only；应用回滚不回滚证据 Schema。
6. 对每个已受理 job 按 action revision、outbox ACK、执行回执哈希和审计事件进行 PG/Kafka/Provider 对账。不得用直接 SQL 伪造 `completed`、`failed` 或补偿成功。
7. 已确认 `external_effect=true` 的动作只能执行合同目录中登记的逆操作；当前补偿适配器未上线时保持 `compensation_blocked_external_executor`，不得把补偿受理解释为补偿成功。
8. 恢复扩大前重跑合同/OpenAPI、双重迁移回放、真实 PG + HTTP Provider、重放与丢失响应回查测试，并在批准候选上完成 G2—G6 和 Windows Chrome 验收。

## 回滚成功判定

- 外部执行开关关闭后没有新的 Provider 调用或副作用，待处理 outbox 未丢失。
- 所有 `completed` 回执均具备唯一 Provider 回执、`effect_state=confirmed`、非空 effect ID、有效 SHA-256 和同 trace 审计。
- 所有连接不确定动作保持 `partial/effect_state=unknown` 并进入人工对账清单，没有被重试成第二个副作用。
- action、审批、outbox、Kafka offset、回执和审计可按 tenant/job/event/trace/revision 完整关联。
- 回滚记录保留执行人、时间、候选哈希、配置哈希、最后水位、开放阻塞和恢复条件。
