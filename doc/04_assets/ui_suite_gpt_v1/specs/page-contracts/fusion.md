# 数据融合 前端实现契约

## 基本信息

- ID：`fusion`
- 路由：`/fusion`
- 领域：`asset-graph`
- React 页面：`FusionWorkbenchPage`
- 目标图：`doc/04_assets/ui_suite_gpt_v1/screens/pages/fusion.png`
- 读取 API：`/api/v1/fusion/workbench`、`/api/v1/threat-intel/entries`、`/api/v1/fusion/stats`、`/api/v1/fusion/entities`、`/api/v1/fusion/value-report`
- 写入 API：`POST /api/v1/fusion/conflicts/{id}/resolve`、`PATCH /api/v1/fusion/rules/{id}`、`POST /api/v1/fusion/evidence-packages`

## 必须实现的业务层

- 融合总览
- 冲突队列
- 来源可信度
- 回写审计
- 冲突处理
- 融合规则版本管理
- 显式验收数据标识与可清理 fixture

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

- `drawer-fusion-conflict`：数据融合冲突处理；按用户“不跳出业务区域”约束落为业务区右侧详情栏，不使用全屏 Drawer
- `modal-fusion-rule-edit`：融合规则编辑，Modal

## 真实数据与状态约束

- 页面使用 React Query 调用 `services/fusionApi.ts`，禁止请求路径 DDL、隐式播种和生产 mock。
- 规则、冲突、修复任务与审计来自 PostgreSQL；六个规则趋势读取规则记录的 `detail.recent_hits` 并由 ECharts 渲染。
- 26 条规则、18 条冲突和 50 条审计只允许通过显式、默认 suspend、可清理的验收 fixture 注入；页面必须显示“验收数据”。
- 规则写入使用 `expected_version`；冲突处理使用 `expected_state_version`；旧版本必须返回 409。
- `rule:write` 缺失时前端禁用写按钮，服务端规则更新、冲突处理和证据导出均必须返回 403。

## 验收清单

- [x] 最终 PNG 必须为 1920x1080（r625 Windows Chrome 生产路由截图）
- [x] 中文为主，只保留必要英文技术词和单位
- [x] 状态色必须遵守 success/info/warning/danger/critical token
- [x] 危险动作必须具备影响范围、权限提示和审计留痕
- [x] 公共 AppShell 必须与 screen.png 目标参数一致
- [x] 页面主工作区不得复用相邻页面的业务组件组合
- [x] 所有 API 调用必须经 services/api.ts 或现有服务封装
- [x] React Query 必须覆盖 loading/error/empty 状态

验收证据：`fusion-interactions-r625.json`、`fusion-r625-visual-metrics.json`、`20260722-fusion-r625-contract`，以及 `fusion-review-adjudication-latest.json`。
