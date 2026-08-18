# T1-M09-N005 encrypted adapter/plan 回滚

## 适用范围

本回滚仅覆盖 encrypted-traffic 页面 plan、snapshot adapter 与共享 envelope 工具的单页迁移，不覆盖 N004 typed client 或后端 API。

## 触发条件

- encrypted 页面的 metrics、rows、timeline、evidence 或 visuals 相对 characterization test 漂移；
- stats 与五个 secondary endpoint 的顺序、17 个 action 合同或权限元数据漂移；
- envelope 的 error、partial、missing_sections 或 source_watermarks 丢失；
- 其他页面 adapter 因共享 envelope 工具迁移发生回归；
- 生产 bundle 缺失 encrypted adapter 的数据空态或证据标签。

## 回滚步骤

1. 将 encrypted adapter 实现恢复到 `pageSnapshotAdapters.ts`，恢复 dispatcher 的旧函数调用。
2. 将 encrypted plan literal 恢复到 `pageApiPlans.ts`，移除独立 plan 引用。
3. 将共享 envelope helper 恢复到聚合 adapter 文件；删除独立 adapter/plan/envelope 文件。
4. 运行 encrypted/adapter/plan/envelope Vitest、目标 ESLint 与 `npm run build`。
5. 在隔离 K8s Job 中验证回滚 bundle，确认 run-scoped Job/Pod 已清理。

## 数据与兼容性

该重构不改变 API、页面路由、服务端事实或部署开关。回滚不触碰共享 PostgreSQL、ClickHouse、Kafka、MinIO 或 Flink。

