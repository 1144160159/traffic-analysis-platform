# T1-M09-N004 encrypted typed client 回滚

## 适用范围

本回滚仅覆盖 encrypted-traffic 时间窗与两组 action client 从 `services/api.ts` 向 `services/encryptedTrafficApi.ts` 的迁移，以及共享 Axios 实例向 `services/httpClient.ts` 的等价移动。

## 触发条件

- 页面原有 `@/services/api` import 无法编译；
- action endpoint、`action`、`target`、`data_mode` 或响应解包发生漂移；
- Authorization/401 token 清理拦截器不再工作；
- encrypted snapshot 时间窗计算发生漂移；
- 生产 bundle 包含 mock worker 或缺失 encrypted action endpoint。

## 回滚步骤

1. 将 Axios 实例和拦截器恢复到 `services/api.ts`。
2. 将 encrypted time-range 类型、参数构造与 action 方法恢复到 `services/api.ts`。
3. 删除 `services/encryptedTrafficApi.ts`、`services/httpClient.ts` 及专用测试；恢复视觉合同测试对旧 locator 的断言。
4. 运行专用 Vitest、ESLint 与 `npm run build`。
5. 在隔离 K8s Job 中验证回滚后的静态 bundle，并清理 run-scoped Job/Pod。

## 数据与兼容性

该重构不改变 API、页面 import、服务端数据或部署开关。回滚不触碰共享 PostgreSQL、ClickHouse、Kafka、MinIO 或 Flink。

