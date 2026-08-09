# UI-ADD-002 路由与状态模型保护

## 本次实现切片

- `AuditLogPage` 的选中日志和详情 Tab 改为 URL 权威状态：`log_id` 与 `detail` 支持直接 URL、刷新、前进和后退回放。
- 审计详情 Tab 使用稳定 slug：`diff`、`operation-context`、`related-chain`、`operation-detail`、`review`。未知值回退为 `diff`；无 `audit:write` 权限时，直接访问 `review` 会被替换为 `diff`，不会绕过权限入口。
- `object_id`、`object_type` 仍作为跨页面只读上下文，并在浏览器历史变化时同步对象类型筛选。
- 专题三 Tab 切换和数据质量“进入重放对账”不再覆盖不属于自身的 query 参数；`mergeRouteSearchParams` 统一执行“只修改所拥有键、保留其他深链上下文、空值删除”。
- 资产页切换分类、对象和详情时只替换 `tab/assetId/detail/search`，保留战役、trace 等来源上下文；资产变更审计入口改为注册路由 `/audit-log` 和规范参数 `object_type/object_id`。
- 行为基线原先跳转不存在的 `/audit?objectType=...`，现统一使用 `/audit-log?object_type=baseline&object_id=...`；取证入口改写规范 `baseline_id`。
- 取证页现在解析规范 `asset_id/alert_id/campaign_id/baseline_id/evidence_id/evidence_type`，并兼容既有 `assetId/alert/campaign/baselineId/evidence/type`。来源标识进入真实 `GET/POST /v1/pcap/jobs`：新建任务把来源引用持久化在 PostgreSQL `tasks.params`，列表按租户和来源引用精确过滤；`create=1` 仅打开确认抽屉，不自动提交。
- 告警、战役、基线到取证的新增跳转使用规范参数；旧链接继续由解析器兼容。历史任务未保存来源引用时不会被前端猜测为关联任务，而会返回真实空结果。
- 加密流量、战役列表、战役详情和攻击链移除“服务端仅受理后由浏览器拼装 JSON 并称为报告”的路径。战役详情复用已有报告 worker，等待终态并校验大小/SHA256 后下载；列表和攻击链在没有下载终态时只显示已受理。模型浏览器 JSON 保留但明确标记 `current-browser-snapshot`、`audit_evidence=false` 和“不是服务端审计证据”。
- React Router 生产和测试环境均启用相同的 v7 future 行为，降低升级后相对 splat 路由和状态调度差异。

## 当前合同

| 页面 | URL 权威状态 | 默认值 | 权限/兼容规则 |
|---|---|---|---|
| 审计日志 | `object_id`、`object_type`、`log_id`、`detail` | `detail=diff`；首条真实记录只作为显示回退，不伪造 URL ID | `detail=review` 要求 `audit:write`；未知 Tab 回退 `diff` |
| 专题面板 | `topic`、兼容 `tab` | `tunnel` | 同步写入两键以保留旧链接；不删除其他 query |
| 数据质量 | `tab` | `overview` | 打开 DLQ 重放切换到 `replay-reconcile`，不删除其他 query |
| 资产台账 | `tab`、`assetId`、`detail`、`search` | `tab=endpoint` | 只替换自身键；保留来源 query；审计跳转使用规范键 |
| 行为基线 | `tab`、兼容 `assetId` | `tab=asset` | 审计与取证跨页链接使用已注册路由和规范参数 |
| 取证分析 | `asset_id/alert_id/campaign_id/baseline_id/evidence_id/evidence_type/tab/create`；兼容旧别名 | `focus=all`、不自动创建 | 来源引用进入租户作用域任务查询和新任务持久化；`create=1` 只打开确认态 |

## 验证

```bash
cd web/ui
npm test -- --run
npm run lint
npm run build

cd ../../go/control-plane
go test ./internal/forensics/api ./internal/forensics/converter ./internal/forensics/repository ./internal/forensics/task
```

当前结果：前端55个测试文件、285个用例全部通过，eslint 0 error/0 warning，TypeScript 和 production build 通过；取证 API、converter、repository、task 四个 Go 包通过。Vite 的既有 ECharts/Ant Design 原始 chunk 大于500 kB警告继续作为性能项，不在无 profile 的情况下改写 chunk 拓扑。

## 未关闭项

UI-ADD-002 整体仍为 `IN_PROGRESS`。还需逐页登记28页 query/path 参数所有权，并完成筛选、分页、选中对象、Drawer/Modal、Tab、返回来源、权限拒绝态的直接 URL、刷新、前进后退和旧链接回放。取证来源引用需要在新候选运行时执行“跨页进入→创建任务→PostgreSQL params→按同一来源查询→审计”的真实对账；旧任务无来源字段不是通过证据。浏览器证据必须绑定新候选 bundle；本次代码构建尚未部署，不能作为 G5 关闭证据。
