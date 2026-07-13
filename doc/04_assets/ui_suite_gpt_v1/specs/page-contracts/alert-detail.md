# 告警详情 前端实现契约

## 基本信息

- ID：`alert-detail`
- 路由：`/alerts/:alertId`
- 领域：`threat-analysis`
- React 页面：`AlertDetailPage`
- 目标图：`doc/04_assets/ui_suite_gpt_v1/screens/pages/alert-detail.png`
- API：`/api/v1/alerts/{id}`、`/api/v1/alerts/{id}/evidence`、`/api/v1/alerts/{id}/feedback`

## 必须实现的业务层

- 告警摘要
- 状态机
- 证据链
- 处置反馈
- 上下文跳转

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

- `modal-alert-status`：更新告警状态，Modal
- `modal-alert-feedback`：提交告警反馈，Modal
- `modal-evidence-detail`：证据详情，Modal
- `modal-playbook-trigger`：从告警触发剧本，Modal
- `modal-whitelist-draft-from-alert`：从告警生成白名单草案，Modal

## 验收清单

- [ ] 最终 PNG 必须为 1920x1080
- [ ] 中文为主，只保留必要英文技术词和单位
- [ ] 状态色必须遵守 success/info/warning/danger/critical token
- [ ] 危险动作必须具备影响范围、权限提示和审计留痕
- [ ] 公共 AppShell 必须与 screen.png 目标参数一致
- [ ] 页面主工作区不得复用相邻页面的业务组件组合
- [ ] 所有 API 调用必须经 services/api.ts 或现有服务封装
- [ ] React Query 必须覆盖 loading/error/empty 状态
