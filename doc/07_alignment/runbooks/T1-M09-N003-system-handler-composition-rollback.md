# T1-M09-N003 SystemHandler 组合根回滚

## 适用范围

本回滚仅适用于 `alert-service` 对 encrypted-traffic stats 服务的显式装配。变更不迁移其他 route/worker，不改变启动配置、数据库、消息主题或 feature flag。

## 触发条件

- `alert-service` 无法编译或启动；
- `GET /api/v1/encrypted-traffic/stats` 未使用 composition root 注入的服务；
- 既有 route 注册、worker 启动顺序或配置读取发生漂移；
- 出现包循环依赖。

## 回滚步骤

1. 将 `main.go` 的 handler 创建恢复为 `api.NewSystemHandler(chClient, db, logger)`。
2. 删除 `newAlertSystemHandler` helper 与 composition characterization test。
3. `SetEncryptedTrafficStatsService` 和导出的窄服务合同可保留，供 API 包内部默认绑定使用；如需一并回退，按 N002 回滚手册恢复。
4. 运行 `go test ./cmd/alert-service ./internal/alert/api -count=1` 和 `go vet ./cmd/alert-service ./internal/alert/api`。
5. 在隔离 K8s Job 中复跑 composition 测试候选，确认 run-scoped Job/Pod 清理完成。

## 数据与运行时

该回滚不触碰共享 PostgreSQL、ClickHouse、Kafka、MinIO 或 Flink，不需数据恢复。生产部署未包含在本任务执行范围内。

