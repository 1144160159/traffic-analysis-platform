# 系统设置 前端实现契约

## 基本信息

- ID：`settings`
- 路由：`/settings`
- 领域：`audit-config`
- React 页面：`SettingsGovernancePage`
- 目标图：`doc/04_assets/ui_suite_gpt_v1/screens/pages/settings.png`
- API：`/api/v1/tokens/scopes`、`/api/v1/tokens`、`/api/v1/tokens/scopes/probe`

## 必须实现的业务层

- 租户站点
- 权限矩阵
- API 令牌
- 留存策略
- 集成配置
- 安全策略
- 系统参数

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

- `dropdown-user-menu`：用户下拉菜单，Dropdown/Menu
- `modal-settings-token`：创建 API 令牌，Modal
- `popconfirm-settings-token-revoke`：API 令牌吊销确认，Popconfirm
- `drawer-settings-rbac-edit`：RBAC 权限编辑，Drawer

## 验收清单

- [ ] 最终 PNG 必须为 1920x1080
- [ ] 中文为主，只保留必要英文技术词和单位
- [ ] 状态色必须遵守 success/info/warning/danger/critical token
- [ ] 危险动作必须具备影响范围、权限提示和审计留痕
- [ ] 公共 AppShell 必须与 screen.png 目标参数一致
- [ ] 页面主工作区不得复用相邻页面的业务组件组合
- [ ] 所有 API 调用必须经 services/api.ts 或现有服务封装
- [ ] React Query 必须覆盖 loading/error/empty 状态
