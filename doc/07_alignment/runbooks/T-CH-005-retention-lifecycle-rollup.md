# T-CH-005 ClickHouse TTL、rollup 与对象引用生命周期

## 当前结论

状态为 `IMPLEMENTING`。仓库已建立原始流、会话、告警、审计、PCAP、聚合、专题快照和报告对象的版本化保留矩阵。PCAP 对象候选由 14 天延长至 37 天，以覆盖 30 天索引和 7 天清理宽限；Docker sessions 初始化 TTL 已与公共 DDL 和 live 的 90 天对齐。上述配置尚未应用生产。

## 日聚合候选

`202608031600_sessions_daily_rollup_v1.sql` 新增 `sessions_daily_rollup_v1_local`、Distributed 表和本地物化视图。聚合按 tenant、day、protocol 和 `aggregate_version=1` 分组，保留 365 天。migration 只消费应用后进入本地 `sessions_local` 的新行；历史分区必须由有界 backfill 处理，禁止把建表成功当成历史聚合完成。

## 上线顺序

1. 在当前吞吐和对象增长率上计算 14→37 天的 MinIO 容量、成本和合规影响，并取得数据 owner 与安全批准。
2. 按节点运行 ClickHouse migration，确认表、列、MV、TTL、Replica 和 DDL 队列健康。
3. 对封闭日分区执行限速 backfill，按 tenant/day/protocol 对账 count、packet_count、byte_count、首末事件时间和 aggregate_version。
4. shadow 查询验证迟到事件和重复重放，未完成前不得切换报表读取。
5. 先内部 tenant 灰度 MinIO 37 天策略，确认实际 ILM；PCAP index 保持 30 天。
6. 在 T+0、T+1、T+3、T+7 观察容量、TTL merge、对象/索引悬挂、查询和错误预算。

## 停止条件与回滚

- 容量预测或实际增长越界时停止扩大，不导入 37 天策略。
- rollup 对账差异扩大、TTL merge 堆积或写入延迟越界时停止 backfill/read shadow；读路径保持详细表。
- MinIO 生命周期候选回滚只能恢复至经批准且不早于仍存索引引用的天数，禁止直接回到 14 天制造悬挂引用。
- 日聚合表和已确认行在观察期内保留；回滚读取而非破坏性删除。

## 未关闭项

- live MinIO 当前仍是 14 天，37 天候选未 apply。
- 日聚合 migration、历史 backfill、迟到/重放对账尚未在生产执行。
- 其他 ClickHouse TTL、MinIO 对象类和显式过期状态仍需完整签认。
- 性能、故障、灰度、回滚、观察、独立 G7 和外部 G8 未完成。
