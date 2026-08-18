# T1-M09-N002 加密流量统计接缝回滚

## 适用范围

本回滚仅适用于 `GET /api/v1/encrypted-traffic/stats` 的行为等价重构。该变更不包含数据库迁移、事件版本、路由变更、权限变更或生产开关。

## 触发条件

- HTTP 状态、成功/错误包络或字段值相对重构前发生漂移；
- 已认证租户或 `start_time`/`end_time` 边界未原样传入查询；
- `traffic.feature_fp` 不可用时，基础统计不再保持成功；
- ClickHouse 必需查询失败未映射为既有 `500 / INTERNAL`；
- 延迟或错误率观察显示新增接缝引入回归。

## 回滚步骤

1. 从 `SystemHandler` 构造中移除 `encryptedTrafficStatsService` 绑定。
2. 将 `GetEncryptedTrafficStats` 恢复为直接执行原有 `traffic.sessions`、`system.tables`、`traffic.feature_fp` 查询的实现。
3. 删除 `encrypted_traffic_stats_service.go`；保留 characterization test 作为回滚后的行为护栏。
4. 运行 `go test ./internal/alert/api -run 'Test(GetEncryptedTrafficStats|EncryptedTraffic)' -count=1`，随后运行整个 `internal/alert/api` 包测试。
5. 在隔离 K8s Job 中复跑同一测试二进制；确认 Job 和 Pod 已按 run-id 清理。

## 数据与兼容性

回滚不修改 PostgreSQL、ClickHouse、Kafka、MinIO 或 Flink 数据。路由、请求参数、响应字段及错误合同保持原版本，因此无需数据恢复或客户端迁移。

