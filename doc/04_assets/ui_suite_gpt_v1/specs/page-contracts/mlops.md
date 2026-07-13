# MLOps 编排 前端实现契约

## 基本信息

- ID：`mlops`
- 路由：`/mlops`
- 领域：`detection-ops`
- React 页面：`MlopsOrchestrationPage`
- 目标图：`doc/04_assets/ui_suite_gpt_v1/screens/pages/mlops.png`
- API：`/api/v1/mlops/status`、`/api/v1/mlops/conditions`

## 必须实现的业务层

- 任务 DAG
- 标注回流
- 训练评估
- 模型注册
- 在线效果

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

- `drawer-mlops-task-detail`：MLOps 任务详情，Drawer

## 验收清单

- [ ] 最终 PNG 必须为 1920x1080
- [ ] 中文为主，只保留必要英文技术词和单位
- [ ] 状态色必须遵守 success/info/warning/danger/critical token
- [ ] 危险动作必须具备影响范围、权限提示和审计留痕
- [ ] 公共 AppShell 必须与 screen.png 目标参数一致
- [ ] 页面主工作区不得复用相邻页面的业务组件组合
- [ ] 所有 API 调用必须经 services/api.ts 或现有服务封装
- [ ] React Query 必须覆盖 loading/error/empty 状态
