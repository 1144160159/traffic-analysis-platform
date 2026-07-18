# 数据质量 前端实现契约

## 基本信息

- ID：`data-quality`
- 路由：`/data-quality`
- 领域：`collection-monitoring`
- React 页面：`DataQualityPage`
- 目标图：`doc/04_assets/ui_suite_gpt_v1/screens/pages/data-quality.png`
- API：`/api/v1/data-quality`

## 必须实现的业务层

- 质量总分
- Topic 健康
- Flink 处理质量
- 字段质量
- 存储质量
- 重放与对账

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

- `drawer-dlq-sample`：DLQ 样例详情，Drawer
- `modal-data-replay-task`：数据重放任务，Modal
- `drawer-field-quality-sample`：字段质量样例，Drawer

## 验收清单

- [x] 最终 PNG 必须为 1920x1080
- [x] 中文为主，只保留必要英文技术词和单位
- [x] 状态色必须遵守 success/info/warning/danger/critical token
- [x] 危险动作必须具备影响范围、权限提示和审计留痕
- [x] 公共 AppShell 必须与 screen.png 目标参数一致
- [x] 页面主工作区不得复用相邻页面的业务组件组合
- [x] 所有 API 调用必须经 services/api.ts 或现有服务封装
- [x] React Query 必须覆盖 loading/error/empty 状态

## 2026-07-16 生产验收补充

- 八个独立视图：`overview`、`topic-health`、`flink-quality`、`field-quality`、`storage-quality`、`replay-reconcile`、`report`、`settings`。
- 业务壳统一使用单一纵向滚动条，右栏桌面宽度 320px；所有视图均验证可滚动至业务区域底部，不以 `overflow: hidden` 裁剪模块。
- Topic 分区倾斜热力图使用 ECharts `heatmap` 系列，canvas 宽高随父容器铺满。
- `/api/v1/data-quality` 结合 ClickHouse 实时检查与 PostgreSQL 激活式 `data_quality_ui_fixtures`，未激活时明确展示缺失态，不允许前端静态数据静默替代生产 API。
- 数据质量写操作使用 `/api/v1/data-quality/actions`，由 `data-quality:write` 权限保护，并原子写入 `data_quality_actions` 与 `DATA_QUALITY_ACTION_REQUESTED` 审计事件。
- Windows Chrome 1920x1080 验收：`evidence/ui-image-breakdowns/pages/data-quality/interaction-r276-scroll-echarts-actions-pass-all-tabs.json`，结果 `pass`；包含八个视图顶部与滚动到底截图、真实动作 POST、权限 200/403/403、数据库动作/审计各 8 条断言。

## 本轮布局与表格补充

- 八个 Tab 使用固定八等分槽位并跨越主栏与右栏，禁止横向滑动；Tab 栏在业务壳纵向滚动时保持固定。
- 主栏和 320px 右栏必须从 Tab 栏下方同一网格行开始，不允许右栏模块占用 Tab 栏所在高度。
- 八个视图内的数据表不展示分页控件，也不保留“共多少条 / 查看更多”脚注行；表体在固定面板内通过纵向滚动查看全部当前数据，分页栏不得重新出现或覆盖数据行。
- 激活式 PostgreSQL 可视化数据表仍通过 `GET /api/v1/data-quality/tables/{dataset}?page=1&page_size=100` 获取受控数据集；服务端只接受数据集白名单，按 JSON 数组 ordinality 稳定排序，并以 JWT 租户上下文隔离 fixture。响应继续返回 `items`、`total`、`page`、`page_size` 和 `fixture_version`，前端在单次请求结果内滚动展示。
- Windows Chrome 交互验收必须断言旧分页节点数量为 0、表格脚注数量为 0、表体滚动规则符合各视图契约；不得以 DOM 切片伪造分页。
- Topic 分区倾斜热力图使用 18 列 ECharts heatmap，横轴只展示约 7 个均匀时间刻度；均衡、轻度倾斜、严重倾斜使用三级语义色，图例与坐标轴不得重叠。
- `tests/e2e/ui_data_quality_all_tabs_interactions.mjs` 必须校验右栏起点、Tab sticky/无横向滚动、旧分页/脚注节点缺失、表体滚动模式，以及热力图 canvas 铺满率。

## 2026-07-16 日报 API 与布局补充

- 质量报告视图必须通过 `GET /api/v1/data-quality/reports/daily` 动态生成日报，生成时间、统计窗口、评分、检查项、异常归因、存储指标、验收包和导出记录不得由生产前端静态常量提供。
- 日报下载统一使用 `GET /api/v1/data-quality/reports/daily/download?format={pdf|json|csv}`；响应必须包含正确的 `Content-Type`、`Content-Disposition` 和非空文件内容，并继续受 `data-quality:read` 权限与 JWT 租户上下文保护。
- “导出记录”和“验收报告与审批”不得左右并排压缩；两个模块各自横向铺满报告右侧工作区，并按上下顺序排列。
- 日报预览内的关键指标表、存储写入表和异常归因表按内容自然展开，不设内部滚动条；“导出记录”单独使用固定高度纵向滚动容器，禁止横向滚动，并必须保证操作列所有下载按钮完整位于右边界内。
- 业务区域顶部必须显示“数据质量”菜单标题与当前视图提示，下方再显示固定八等分 Tab；标题不得被验收样式隐藏。
- “质量总分”使用 ECharts Gauge Canvas 渲染，环图与“良好 / 真实 API”文字分列布局，不允许文字与图形交叠。
- Windows Chrome 八页签最终验收：`evidence/ui-image-breakdowns/pages/data-quality/interaction-r306-probe-dq-containment-all-tabs.json`，结果 `pass`；标题可见、总分图引擎为 `echarts-canvas`、文字重叠量为 0，导出表仅 1 个内部滚动容器且操作列完整包含。

## 2026-07-17 多分辨率补充

- `1600x900` 与 `1366x768` 均保持菜单标题和八等分 Tab 完整可见；Tab 不横向滚动，业务内容在其下方使用单一纵向滚动主体。
- 窄屏下 Topic/Flink/字段/存储/重放/设置和质量报告的内容网格改为单列顺序展开，所有面板、ECharts Canvas 与下载按钮必须处于业务区右边界内。
- 质量报告中的普通日报表自然展开且无内部滚动；仅“导出记录”保留固定 154px 的纵向内部滚动，下载操作列必须完整可见。
- Windows Chrome 双分辨率报告：`evidence/ui-responsive/probes-data-quality/windows-chrome-responsive-latest.json`，结果 `pass`；八页签顶部截图以及总览/报告滚动到底截图位于同目录。
- 桌面交互回归 `r307-responsive-final` 结果 `pass`：八页签几何稳定、ECharts 质量总分图文交叠为 0、PDF/JSON/CSV 下载均返回正确附件，控制台/页面/请求错误均为 0。
