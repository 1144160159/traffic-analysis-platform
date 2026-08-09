# T-CH-002 ClickHouse 确定性分片 V2

## 当前结论

状态为 `IMPLEMENTING`，不是关闭。当前 live 清单中的 18 张 Distributed 表有 5 张使用确定性复合 hash，13 张仍使用 `rand()`。仓库只为低容量的 `alert_feedback` 建立首条可回滚 V2 候选；未执行生产 migration、回填、读切换或 V1 清理。

## 首条纵向候选

- V1：`traffic.alert_feedback`，保留写入与读取。
- V2 local：`traffic.alert_feedback_v2_local`。
- V2 Distributed：`traffic.alert_feedback_v2`。
- V2 分片键：`cityHash64(tenant_id,alert_id)`。
- 权威写入源：PostgreSQL `model_feedback_inbox`。
- 双写开关：`MODEL_FEEDBACK_CLICKHOUSE_PROJECTION_V2_ENABLED=false`。
- V2 表名：固定为 `traffic.alert_feedback_v2`，拒绝任意动态表名和 `_local` 绕行写入。

worker 先精确对账/写入 V1，再精确对账/写入 V2。两边均成功后才确认 PostgreSQL inbox。若 V1 成功而 V2 失败，inbox 保持 failed 并重试；下次运行会识别 V1 精确副本，再继续 V2，不会重复制造业务记录，也不会伪报成功。

## Expand 前置检查

1. 候选必须通过 G0，且候选 hash 与专项证据一致。
2. 对每个 ClickHouse shard/replica 采集 `system.tables`、`system.columns`、`system.replicas` 和 DDL 队列。
3. 所有副本必须非 readonly、session 未过期、队列和延迟在批准预算内。
4. 只能使用 `scripts/clickhouse/run-migrations.sh` 逐节点执行 migration；禁止手工 DDL 和 `ON CLUSTER`。
5. 首次只执行 expand migration，不开启 V2 写入。

## 迁移顺序

1. 执行 `202608031330_alert_feedback_v2.sql`，逐节点复核表定义和副本路径。
2. 对一个内部 tenant 开启 V2 双写，观察 V1/V2 的 `existing/inserted/error/conflict` 独立指标。
3. 按月份分区回填；每批固定行数、并发、CPU、IO 和副本队列停止阈值。
4. 按 `(tenant_id,feedback_id)` 对账业务键集合、总数、内容 hash 和抽样字段；差异不得自动忽略。
5. 增加 shadow read，比对按 tenant/alert 的反馈列表、统计与 MLOps 聚合，并记录正确性和 P50/P95/P99。
6. 先内部 tenant、后非关键 tenant 切读；V1 仍持续写入并保留回滚窗口。
7. 完成 T+0、T+1、T+3、T+7 观察后，才可评审停止 V1 写入；清理另立生命周期变更。

## 立即停止条件

- 任一副本 readonly、session 过期、队列/延迟持续扩大。
- V1/V2 出现业务键缺失、冲突或内容 hash 不一致。
- V2 错误率、P99、CPU、IO 或磁盘超过批准预算。
- inbox dead-letter 增长，或 V2 失败后仍被标记 applied。
- 出现跨 tenant 结果或 `_local` 直接写入。

## 回滚

1. 设置 `MODEL_FEEDBACK_CLICKHOUSE_PROJECTION_V2_ENABLED=false` 并滚动恢复上一个候选镜像。
2. 保持 V1 写入和读取；停止 backfill/shadow/cutover 扩大。
3. 保留 V2 表、migration ledger、失败批次和对账证据，不执行 DROP/TRUNCATE。
4. 对在途 inbox 重新确认 lease、attempt、V1 精确记录和最终状态。
5. 只有根因、补偿和重新验收批准后才能再次开启 V2。

## 未关闭项

- production migration 和 V2 双写尚未执行。
- 尚无分区回填、业务键/hash/抽样对账和 shadow read。
- 尚无性能、故障、灰度、回滚与 T+7 观察证据。
- 其余 12 张 `rand()` 表仍需逐域画像和 V2 迁移，不能由本候选代替。
