# T-CH-004 ClickHouse 追加事实与版本语义

## 当前结论

状态为 `IMPLEMENTING`。Go 告警域的 8 个 alerts/evidence 写入点已统一写逻辑 Distributed 表，不再写 `_local`；未被调用的同步 evidence DELETE mutation 已移除。该结果不代表全项目 writer、重放或删除语义已完成。

## 写入规则

- 默认只追加事实；状态更新写新版本行或状态事件。
- `_local` 直写必须有明确 shard 选择、拓扑稳定和故障恢复合同，否则写 Distributed 表。
- 同步请求不得等待 ClickHouse mutation。
- 普通删除使用 tombstone 与 TTL；合规擦除使用独立审批和分区级流程。
- 重放使用稳定 event_id，版本化投影携带 aggregate_version 和 ingested_at。
- event_time 与 ingestion_time 分离，迟到策略不能使用当前系统时间代替事件事实。

## 后续验证

1. 目录化 Go、Java、Flink、维护脚本的每个 writer、目标表和路由 owner。
2. 对 alerts/evidence 按 event_id 与业务键执行重复、乱序和重放对账。
3. 故障注入 Distributed 转发、分片不可用、副本延迟和客户端超时。
4. 补 aggregate_version、ingested_at、tombstone 和 late-data 合同。
5. 完成灰度、回滚和 T+0/T+1/T+3/T+7 观察。

## 回滚

若 Distributed 写入出现持续部分失败或路由资源越界，停止扩大候选并恢复上一镜像；不得通过重新启用无分片合同的 `_local` 直写掩盖故障。保留失败事件和业务键对账证据后再设计补偿。

## 未关闭项

- Java/Flink 和维护 writer 尚未完整签认。
- event_id、aggregate_version、ingested_at 与 tombstone 尚未覆盖所有表。
- 未执行真实重放、分片故障、性能、发布、回滚和观察门禁。
