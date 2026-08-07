# F-ASSET-006 资产治理工单回滚

## 触发条件

- 发现跨租户读取或写入、无审计状态变化、重复命令产生不同结果，或资产与工单 revision 无法对账。
- 工单完成后生命周期、资产历史、工单历史或治理 outbox 任一事实缺失。
- PostgreSQL 锁等待、错误率或 P99 超过已批准预算并持续扩大。

## 回滚步骤

1. 停止扩大灰度，将 `ASSET_GOVERNANCE_V1_ENABLED=false`；前端同步关闭 `ASSET_GOVERNANCE_V1_ENABLED`。
2. 禁止创建和执行新工单；已受理工单保持原状态，不在数据库中伪造失败或成功。
3. 按 `tenant_id/work_order_id/trace_id` 对账 `asset_governance_work_orders`、history、control requests、audit 与 outbox。
4. 对已经完成且必须撤销的工单，仅在 resulting asset revision 仍为当前 revision 时调用显式 `asset-governance-compensate`；revision 已变化则转人工裁决。
5. 保留 migration、新增列、工单、历史、审计和 outbox，禁止删除证据或回退 revision。

## 恢复条件

- 根因修复通过 G0/G1，并在隔离 PostgreSQL 中重放创建、独立审批、执行、带证据完成、补偿、跨租户和重复提交用例。
- 全部差异已对账，候选 manifest、Schema hash、测试与回滚证据带不可变 hash。
- 先内部 tenant，再非关键 tenant；T+0、T+1、T+3、T+7 持续观察。
