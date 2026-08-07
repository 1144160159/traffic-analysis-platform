# F-TOPIC-002 专题动作执行器回滚

适用范围：`topic_executor_v2` 的动作目录、幂等受理、状态机、内部执行器、history、outbox 和 receipt。

## 触发条件

- 重复幂等键产生多个 job 或多个外部效果；
- accepted/running 被 UI 当作最终成功；
- snapshot revision 冲突未拒绝、出现跨租户目标或审计缺失；
- lease 无法回收、连续失败达到 5 次或 worker 资源越界。

## 停止扩大

1. 停止扩大候选 tenant，并将 `TOPIC_EXECUTOR_V2_ENABLED=false`。
2. Web UI 暂时只保留读取和旧兼容操作；高风险动作继续 fail-closed。
3. `accepted` 任务原样保留；`running` 任务等待两分钟 lease 到期，禁止人工改写为成功。
4. 保留 `topic_actions`、history、outbox、receipt 和 audit；不得删除失败回执。

## 恢复与验证

恢复前执行重复提交、revision 冲突、worker 重启、lease 过期、审计失败回滚和跨租户负向用例。
每个 completed 状态必须能查到 executor receipt、history、outbox 与相同 trace。回滚不删除
migration，也不自动重放未批准的高风险动作。
