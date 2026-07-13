# 登录 前端实现契约

## 基本信息

- ID：`login`
- 路由：`/login`
- 领域：`auth`
- React 页面：`LoginPage`
- 目标图：`doc/04_assets/ui_suite_gpt_v1/screens/pages/login.png`
- API：按页面服务计划补齐

## 必须实现的业务层

- 页面主工作区
- 右侧闭环栏
- 审计与证据入口

## 分层参数

- `login-background`：auth-background，bbox=`{"x":0,"y":0,"w":1920,"h":1080}`
- `login-panel`：auth-form，bbox=`{"x":1160,"y":210,"w":520,"h":620}`

## 组件映射

- AppShell
- WorkPanel
- MetricTile
- Table
- Tabs
- ECharts
- StatusTag

## 关联浮层

- `modal-login-error-captcha`：登录异常与验证码状态，Modal

## 验收清单

- [ ] 最终 PNG 必须为 1920x1080
- [ ] 中文为主，只保留必要英文技术词和单位
- [ ] 状态色必须遵守 success/info/warning/danger/critical token
- [ ] 危险动作必须具备影响范围、权限提示和审计留痕
- [ ] 登录页不展示常规 AppShell
- [ ] 页面主工作区不得复用相邻页面的业务组件组合
- [ ] 所有 API 调用必须经 services/api.ts 或现有服务封装
- [ ] React Query 必须覆盖 loading/error/empty 状态
