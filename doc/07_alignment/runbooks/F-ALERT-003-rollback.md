# F-ALERT-003 告警响应任务回滚与停止扩大手册

适用范围：告警响应动作的受理、独立审批、事务 outbox、Kafka 消费、外部执行、权威回查、最终回执、取消和补偿边界。本手册不授权操作共享或生产环境；所有切换仍需候选哈希、配置哈希、变更窗口和负责人批准。

## 停止扩大条件

出现以下任一情况立即停止扩大：跨租户读取或执行；请求人与审批人相同；同一 `event_id/idempotency_key` 产生多个提供方副作用；同一补偿 `request_id/provider_idempotency_key` 调用逆操作超过一次；HTTP 2xx 缺少耐久 `provider_receipt_id`、明确 `effect_state` 或 effect ID 却被写成完成；传输超时、响应丢失或权威查询失败被写成确定失败或成功；Provider 回查的 tenant、job、event、request、trace、幂等键或 Provider 身份与原命令不一致；补偿回执未精确覆盖原 effect ID 集合；action、outbox、执行/补偿回执、权威查询历史和审计无法按同一 trace 对账；回执覆盖较新 revision；消费积压、失败率、P99 或资源超过批准预算。

## 回滚步骤

1. 先将目标候选的 `ALERT_RESPONSE_COMPENSATION_EXECUTOR_V1_ENABLED=false`，停止受理和执行新的外部逆操作；所有既有 `pending/executing/authority_pending` 行原样保留，UI 和 GET 不得把它们显示为补偿成功。
2. 将 `ALERT_RESPONSE_EXTERNAL_EXECUTOR_V1_ENABLED=false`，停止新的原始外部副作用；不得删除 API 路由或把请求改成本地伪成功。若 Provider 权威接口自身不可信，再将 `ALERT_RESPONSE_UNKNOWN_EFFECT_RECONCILIATION_V1_ENABLED=false`；该开关只停止周期查询，不得删除 recheck/history。
3. 将 `ALERT_RESPONSE_EXECUTION_V1_ENABLED=false`，停止 response-action outbox dispatcher 和消费组。新请求仍可耐久记录为待审批/待执行，但 UI 和 GET 必须显示真实 pending/blocked 状态。
4. 记录候选源码与镜像哈希、配置 effective hash、最后 `tenant_id/trace_id/job_id/event_id/request_id/revision/Kafka partition/offset` 和两类提供方幂等键。等待在途调用达到批准超时后再停止旧 Pod，不得并行启动旧消费者或补偿工作器。
5. 对传输结果不确定的原始动作，只能以完全相同的 `event_id/job_id/tenant_id/trace_id/idempotency_key` 查询 Provider；对不确定补偿只能以相同 `request_id/event_id/job_id/tenant_id/trace_id/provider_idempotency_key` 查询，禁止第二次调用逆操作。只有 `receipt_found` 且回执完整、Provider 身份和 effect ID 集合一致时才可恢复终态；`absent/unknown` 或查询失败均保持 partial/unknown 并保留有界历史。
6. 禁止删除或回退 `alert_response_actions`、`alert_response_outbox`、`alert_response_execution_receipts`、`alert_response_execution_authority_rechecks`、`alert_response_compensation_attempts`、`alert_response_compensation_receipts`、`alert_response_authority_check_history`、`alert_response_approvals`、`alert_response_control_requests` 和相关 `audit_logs`。四次迁移均为 expand-only；应用回滚不回滚证据 Schema。
7. 对每个已受理 job 按 action revision、outbox ACK、执行回执哈希、recheck/compensation attempt、补偿回执哈希和审计事件进行 PG/Kafka/Provider 对账。不得用直接 SQL 伪造 `completed`、`failed`、`compensated` 或 `compensation_failed`。
8. 已确认 `external_effect=true` 且 effect ID 非空的动作只能执行合同目录中登记的逆操作；适配器开关关闭时继续写入 `compensation_blocked_external_executor`，开启时 HTTP 202 只代表 PG 队列已提交，不代表逆操作成功。
9. 恢复扩大前重跑合同/OpenAPI、双重迁移回放、真实 PG + HTTP Provider、周期权威复核、补偿首次超时后仅回查测试，并在批准候选上完成 G2—G6 和 Windows Chrome 验收。

## 回滚成功判定

- 外部执行开关关闭后没有新的 Provider 调用或副作用，待处理 outbox 未丢失。
- 所有 `completed` 回执均具备唯一 Provider 回执、`effect_state=confirmed`、非空 effect ID、有效 SHA-256 和同 trace 审计。
- 所有连接不确定动作保持 `partial/effect_state=unknown` 并进入人工对账清单，没有被重试成第二个副作用。
- 补偿开关关闭后没有新的逆操作调用；传输不确定补偿保留 `authority_pending` 或 `compensation_partial`，不存在第二次逆操作调用。
- action、审批、outbox、Kafka offset、执行/补偿回执、权威查询历史和审计可按 tenant/job/event/request/trace/revision 完整关联。
- 回滚记录保留执行人、时间、候选哈希、配置哈希、最后水位、开放阻塞和恢复条件。
