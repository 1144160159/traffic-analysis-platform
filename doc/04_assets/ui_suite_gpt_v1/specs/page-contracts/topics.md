# 专题面板 前端实现契约

## 基本信息

- ID：`topics`
- 路由：`/topics`
- 领域：`overview`
- React 页面：`TopicWorkbenchPage`
- 目标图：无单张页面主图，使用专题状态输入图。
- API：`/api/v1/topics/tunnel`、`/api/v1/topics/exfil`、`/api/v1/topics/apt`

## 必须实现的业务层

- 专题范围定义
- 专题专属 KPI
- 局部影响面
- 专题分析
- 关联事件与证据
- 专题报告与交付

## 分层参数

- `topbar`：global-app-shell，bbox=`{"x":0,"y":0,"w":1920,"h":80}`
- `sidebar`：global-app-shell，bbox=`{"x":0,"y":80,"w":166,"h":917}`
- `topic-content`：topic-workspace，bbox=`{"x":198,"y":80,"w":1722,"h":917}`
- `bottombar`：global-app-shell，bbox=`{"x":0,"y":997,"w":1920,"h":83}`

## 组件映射

- AppShell
- Segmented
- Tabs
- MetricTile
- Table
- Graph
- Modal
- Drawer

## 关联浮层

- `modal-topic-save-view`：专题保存视图，Modal
- `drawer-topic-scope-edit`：专题范围编辑，Drawer
- `modal-topic-report-export`：专题报告导出，Modal
- `modal-topic-evidence-package-export`：专题证据包导出，Modal
- `drawer-topic-subscription`：专题订阅配置，Drawer
- `dropdown-topic-share-favorite`：专题分享收藏菜单，Dropdown/Menu

## 验收清单

- [ ] 最终 PNG 必须为 1920x1080
- [ ] 中文为主，只保留必要英文技术词和单位
- [ ] 状态色必须遵守 success/info/warning/danger/critical token
- [ ] 危险动作必须具备影响范围、权限提示和审计留痕
- [ ] 公共 AppShell 必须与 screen.png 目标参数一致
- [ ] 页面主工作区不得复用相邻页面的业务组件组合
- [ ] 所有 API 调用必须经 services/api.ts 或现有服务封装
- [ ] React Query 必须覆盖 loading/error/empty 状态
