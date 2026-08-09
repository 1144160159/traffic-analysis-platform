# F-DASHBOARD-001 统一快照回滚与停止扩大手册

适用范围：`GET /api/v1/dashboard/snapshot`、生产 Web UI Dashboard 单快照读取以及旧 Dashboard GET 的认证租户加固。本手册不授权修改共享环境；实际发布必须绑定获批候选镜像、tenant 和变更窗口。

## 停止扩大条件

出现任一情况立即停止：跨租户指标或队列可见；同一响应包含不同时间窗；`snapshot_id` 与水位不稳定；真实零被替换为其他指标；数据源失败显示为零或成功；P99/资源超预算；OpenSearch/ClickHouse 对账差持续扩大；生产 bundle 仍请求旧三接口或生成假队列。

## 回滚步骤

1. 将候选 tenant 的 `DASHBOARD_SNAPSHOT_V1_ENABLED` 设为 `false`，统一接口明确返回 503；保留路由和旧兼容 API，不删除字段。
2. 将 Web UI 回退到上一份带 hash 的候选 bundle；禁止回退到前端 mock、默认 tenant 或本地派生队列。
3. 保存最后一个 `snapshot_id`、`trace_id`、查询时间窗、`missing_sections` 和全部 `source_watermarks`。
4. 按认证 `tenant_id` 在 ClickHouse、PostgreSQL、OpenSearch 和 Redis 复核该时间窗；若发现跨租户，立即撤销候选访问并进入安全事件流程。
5. 对 partial 激增定位具体 missing section；不得通过填零、复用其他 KPI 或隐藏 warning 恢复绿色页面。
6. 修复后重新执行合同、Go/前端测试、真实四源对账、性能、Windows Chrome mock-off、灰度与回滚验证。

## 回滚成功判定

- 新统一请求稳定返回 `FEATURE_DISABLED`，没有不受控的 shadow 读取。
- 旧兼容 API 仍要求认证身份和 `alert:read`，query `tenant_id` 不能覆盖认证租户。
- 已保存的候选证据和水位可重放，没有删除任务、审计或投影数据。
- 回滚记录包含候选 hash、tenant、最后 trace/snapshot、原因、负责人和复核人。
