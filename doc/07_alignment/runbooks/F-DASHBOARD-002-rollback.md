# F-DASHBOARD-002 回滚与停止扩大手册

适用范围：仪表盘闭环、证据、反馈、审计、SLA和合规任务的V2受理、事件分发、执行和最终效果链路。该手册不授权修改共享环境；执行部署动作前仍需候选环境和发布窗口批准。

## 停止扩大条件

出现以下任一情况立即停止扩大：跨租户任务可见、同一幂等键产生多个任务或提供方副作用、任务/历史/审计/outbox/执行或补偿回执数量不一致、持续PG或Kafka错误、事件积压或重试超过预算、永久错误在`dlq.v1`未获broker ACK或`dashboard_task_dlq_receipts`与审计未原子落库前提交源offset、执行器或补偿器HTTP 2xx被错误写成成功、未知副作用被写成失败、权威查询返回的`request_event_id/idempotency_key/provider`与原命令或回执不一致、没有`receipt_found`和有效耐久回执却恢复成功、候选bundle仍显示仿真成功、任务受理、执行或补偿P99及资源超过批准预算。

## 回滚步骤

1. 将候选tenant的`DASHBOARD_TASK_V2_ENABLED`设为`false`，先停止新命令；保留GET/POST路由，关闭状态明确返回503，不得回退成本地伪成功。
2. 先将`DASHBOARD_TASK_COMPENSATION_V1_ENABLED`设为`false`，停止新的补偿受理；再将`DASHBOARD_TASK_PIPELINE_V1_ENABLED`设为`false`并滚动停止该候选的consumer、dispatcher、executor和compensator worker。若权威查询端点本身发生身份或完整性故障，先将`DASHBOARD_TASK_PROVIDER_AUTHORITY_LOOKUP_V1_ENABLED=false`；否则保留查询能力直到在途HTTP调用完成对账，再随pipeline关闭。先阻断新租约，再等待当前HTTP调用达到批准超时，不得并行启动旧消费者。
3. 记录最后一个`trace_id`、`task_id`、执行/补偿请求`event_id`、结果`event_id`、Kafka partition/offset和两类提供方幂等键。对执行或补偿状态仍为`processing`的行标记为待人工对账；只能用完全相同的`request_event_id/idempotency_key/tenant_id/task_id/trace_id`查询Provider。`receipt_found`仍必须校验Provider身份和耐久回执；`pending/absent/unknown`或查询失败都不能推断无副作用、失败或成功，也不得直接重试产生第二次副作用。
4. 禁止删除`dashboard_tasks`、`dashboard_task_history`、`dashboard_task_outbox`、`dashboard_task_requests`、`dashboard_task_execution_attempts`、`dashboard_task_execution_receipts`、`dashboard_task_compensation_requests`、`dashboard_task_compensation_attempts`、`dashboard_task_compensation_receipts`、`dashboard_task_event_inbox`、`dashboard_task_dlq_receipts`或相关`audit_logs`；四次迁移均为expand-only，回滚应用不回滚Schema。
5. 对每个已受理任务执行PostgreSQL reconciliation，按`tenant_id/task_id/revision/event_id`核对任务、历史、审计、outbox、inbox、执行/补偿租约和提供方回执；不允许用直接SQL伪造completed状态，也不允许伪造compensated状态。执行调用不确定时保持`partial/effect_state=unknown`；补偿调用不确定时保持`compensation_partial/effect_state=unknown`，等待稳定幂等键查询或人工裁决。
6. 未发布的outbox保持`pending`并记录外部阻塞；已获Kafka ACK但尚未标记published的行允许按相同event ID重发，由inbox去重。不得删除或重写事件来追求数量一致。
7. 如需恢复为“仅受理”兼容路径，只启用`DASHBOARD_TASK_V2_ENABLED`，保持pipeline flag关闭；UI必须继续显示accepted/running而不是成功。
8. 恢复执行前重新运行OpenAPI/Feature Contract、隔离PG原子与事件重放测试、生产bundle构建和候选浏览器验收，并确认旧worker全部退出。

## 回滚成功判定

- 新建请求稳定返回`FEATURE_DISABLED`，没有新任务行。
- 已有任务仍可由受控查询或数据库对账恢复，审计和历史未丢失。
- 不存在跨租户结果、重复任务或提供方副作用、孤儿历史、孤儿outbox、孤儿回执，且任务状态未被伪造为完成。
- `dashboard_task_execution_receipts`中的completed均有`effect_state=confirmed`和非空稳定effect ID；连接不确定的任务仍为partial并进入人工对账清单。
- `dashboard_task_compensation_receipts`中的compensated均有`effect_state=confirmed`和非空稳定effect ID；连接不确定的补偿仍为compensation_partial并进入人工对账清单。
- 通过权威查询恢复的执行和补偿在任务结果、`dashboard_task_history.snapshot`与对应`audit_logs.detail`中均有`authority_lookup.state=receipt_found`、`recovered_receipt=true`和Provider查询时间；三者与耐久回执在同一PostgreSQL事务中提交。
- 每个已提交的永久错误源`topic/partition/offset`都有且只有一个`dashboard_task_dlq_receipts`和一个`DASHBOARD_TASK_EVENT_QUARANTINED`审计；记录明确`dlq_acknowledged=true/source_offset_commit_pending=true`，不存在DLQ ACK失败却提前落库或提交offset的事实。
- 发布记录保留关闭时间、操作者、候选hash、最后trace和reconciliation结果。
