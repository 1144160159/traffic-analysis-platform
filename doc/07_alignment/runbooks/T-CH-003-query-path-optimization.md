# T-CH-003 ClickHouse 查询路径优化

## 当前结论

状态为 `IMPLEMENTING`，不是性能验收通过。首个仓库切片只将告警列表从 `toJSONString(groupArray(map(...)))` 改为有界结构化行读取，最大页仍为 1000。现有 tenant、时间过滤、`alerts_latest FINAL`、精确 total、offset、排序和攻击阶段语义全部保留。

## 为什么不同时删除 FINAL、COUNT 和 OFFSET

- `FINAL` 影响“最新版本”语义，必须先按业务键证明替代结果相同。
- total 是现有 API 契约的一部分，改为估算或异步统计需要版本化响应合同。
- offset 仍被现有路由和 UI 使用；游标必须增量加入并保留兼容路径。
- 当前 live query_log 样本没有告警列表形状，不能从静态代码宣称 P95 改善。

## 验证顺序

1. 固定候选、数据规模、tenant、24 小时时间窗、并发和冷热缓存。
2. 保存 normalized query、query_id、read_rows、read_bytes、memory_usage、duration 和异常。
3. 对同一条件比较旧 JSON 聚合与结构化行的 alert_id 集合、顺序和字段。
4. 再引入 `(last_seen,alert_id)` 稳定游标；重复、并发新写和翻页边界必须无缺失/重复。
5. 将 exact count 从首屏解耦前，先批准 `meta.total` 的精确、估算或延迟语义。
6. 只有 latest 表或版本过滤与 `FINAL` 逐键对账通过后，才允许灰度移除 `FINAL`。

## 停止与回滚

若结果集合、顺序、total、租户过滤发生差异，或 read_rows/read_bytes/memory/P99 超出预算，立即恢复上一候选。旧 offset API、`FINAL` 和 exact count 在各自兼容迁移完成前不得删除。

## 未关闭项

- 全部 ClickHouse 查询模式尚未完成目录化。
- cursor、count 解耦和 `FINAL` 替代尚未实现。
- 没有当前候选的 live query_log、profile、冷热缓存或故障证据。
- 没有生产灰度、回滚、T+0/T+1/T+3/T+7 与独立签认。
