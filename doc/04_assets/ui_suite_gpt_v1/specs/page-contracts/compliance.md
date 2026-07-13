# 合规审计 前端实现契约

## 基本信息

- ID：`compliance`
- 路由：`/compliance`
- 领域：`audit-config`
- React 页面：`ComplianceAuditPage`
- 目标图：`doc/04_assets/ui_suite_gpt_v1/screens/pages/compliance.png`
- API：`/api/v1/compliance/reports`、`/api/v1/compliance/audit-trail`

## 必须实现的业务层

- 验收门禁
- 指标映射
- 证据包
- 运行报告
- 缺口治理
- 第三方评测

## 分层参数

- `topbar`：global-app-shell，bbox=`{"x":0,"y":0,"w":1920,"h":80}`
- `sidebar`：global-app-shell，bbox=`{"x":0,"y":80,"w":166,"h":917}`
- `content`：page-workspace，bbox=`{"x":198,"y":80,"w":1722,"h":917}`
- `bottombar`：global-app-shell，bbox=`{"x":0,"y":997,"w":1920,"h":83}`
- `right-rail`：closed-loop-rail，bbox=`{"x":1460,"y":104,"w":420,"h":860}`

## 组件映射

- AppShell
- WorkPanel
- MetricTile
- Table
- Tabs
- ECharts
- StatusTag

## 关联浮层

- `drawer-compliance-gate-detail`：合规门禁详情，Drawer
- `modal-compliance-evidence-package-export`：合规证据包导出，Modal
- `modal-compliance-report-export`：合规运行报告导出，Modal

## 验收清单

- [ ] 最终 PNG 必须为 1920x1080
- [ ] 中文为主，只保留必要英文技术词和单位
- [ ] 状态色必须遵守 success/info/warning/danger/critical token
- [ ] 危险动作必须具备影响范围、权限提示和审计留痕
- [ ] 公共 AppShell 必须与 screen.png 目标参数一致
- [ ] 页面主工作区不得复用相邻页面的业务组件组合
- [ ] 所有 API 调用必须经 services/api.ts 或现有服务封装
- [ ] React Query 必须覆盖 loading/error/empty 状态
