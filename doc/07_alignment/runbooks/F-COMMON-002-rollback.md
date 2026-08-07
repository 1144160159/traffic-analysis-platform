# F-COMMON-002 统一页面快照回滚

## 范围

本手册只回滚新增的 Page Snapshot profile 及其生成类型，不删除仪表盘、资产、专题的旧路由、读接口、字段、权限或审计事件。该 profile 是构建和发布护栏，不是生产运行时注册表依赖。

## 停止条件

- 同一页面的 `snapshot_id/as_of` 出现扩大性不一致。
- 合法 0 值、空集合或 partial 被展示成其他业务事实。
- 认证租户与快照源租户不一致。
- P99、源端资源或缓存陈旧度超出批准预算。

## 回滚步骤

1. 停止扩大 `common_page_snapshot_v1` 灰度，保留当前证据 run、HAR、trace 和源水位。
2. 将前端读路径切回已保留的领域接口；不使用 fixture 补齐缺失 section。
3. 回滚到上一个经 hash 绑定的 OpenAPI、TypeScript 生成物和候选 bundle。
4. 若已产生快照 manifest，仅停止新建，保留旧 manifest 供审计和对账，不删数据。
5. 对内部租户复验旧读路径、租户隔离、空值语义和 trace，再决定是否恢复灰度。

## 回滚验收

旧路由和接口可用，无跨租户，无伪数据补齐，`removed_routes/actions/api_operations/response_fields/scopes/audit_events` 全部为空。
