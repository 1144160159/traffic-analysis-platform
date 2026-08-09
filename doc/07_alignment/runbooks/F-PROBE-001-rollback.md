# F-PROBE-001 探针操作 ACK 状态机回滚

适用范围：`probe_operation_ack_v2` 的稳定命令版本、幂等受理、操作状态查询、
ACK receipt、history、outbox、审计和 Web UI 终态轮询。

## 触发条件

- 同一幂等键产生多个 operation 或同一探针出现重复 command revision；
- 非探针令牌或 probe identity 不匹配的令牌可以提交 ACK；
- 重复、过期或乱序 ACK 覆盖较新的 reported state；
- accepted 被 UI 当作最终成功，或 operation、outbox、audit 未保持事务一致；
- 控制通道积压不可恢复、ACK 延迟持续越界或跨租户数据出现。

## 停止扩大

1. 停止扩大候选 tenant，并将 `PROBE_OPERATION_ACK_V2_ENABLED=false`。
2. 停止新的批量升级、配置下发、证书轮换和重启；只保留探针与历史操作只读查询。
3. 不删除 `probe_operations`、history、ACK receipts、outbox 或 audit；未确认命令不得人工改写为成功。
4. 已发布但未 ACK 的命令按 operation ID、command revision 和过期时间盘点；禁止无幂等键重发。
5. 若 Agent 已执行但 ACK 未落库，先保存 Agent 日志、配置 hash 和版本证据，再由批准人决定补偿或重放。

## 恢复与验证

恢复前执行重复提交、跨租户、令牌 probe identity 不匹配、revision 冲突、乱序 ACK、
过期 ACK、PG 事务回滚、Kafka 重复/乱序、Agent 离线恢复和 UI 重复提交用例。
每个 completed 状态必须能以同一 operation ID 和 trace 对齐 PG、Kafka 命令、Agent ACK、
reported state、history、outbox 与 audit。回滚只关闭新状态路径，不回退 migration，
也不删除晚到 ACK。
