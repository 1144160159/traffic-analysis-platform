# 通知配置 前端实现契约

## 基本信息

- ID：`notifications`
- 路由：`/notifications`
- 领域：`audit-config`
- React 页面：`NotificationConfigPage`
- 目标图：`doc/04_assets/ui_suite_gpt_v1/screens/pages/notifications.png`
- API：`/api/v1/notifications/settings`

## 必须实现的业务层

- 通知渠道
- 订阅规则
- 升级策略
- 模板管理
- 发送历史
- 抑制静默

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

- `drawer-notification-center`：通知中心抽屉，Drawer
- `modal-notification-channel-edit`：通知渠道编辑，Modal
- `modal-notification-template-preview-test`：通知模板预览测试，Modal
- `drawer-notification-silence-rule`：通知静默规则，Drawer

## 验收清单

- [ ] 最终 PNG 必须为 1920x1080
- [ ] 中文为主，只保留必要英文技术词和单位
- [ ] 状态色必须遵守 success/info/warning/danger/critical token
- [ ] 危险动作必须具备影响范围、权限提示和审计留痕
- [ ] 公共 AppShell 必须与 screen.png 目标参数一致
- [ ] 页面主工作区不得复用相邻页面的业务组件组合
- [ ] 所有 API 调用必须经 services/api.ts 或现有服务封装
- [ ] React Query 必须覆盖 loading/error/empty 状态
