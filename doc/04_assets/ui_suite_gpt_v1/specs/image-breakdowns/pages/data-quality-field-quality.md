# data-quality-field-quality 拆解记录

- page-id: `data-quality-field-quality`
- route: `/data-quality?tab=field-quality`
- type: `menu-state`
- parent: `data-quality`
- target UI: `doc/04_assets/ui_suite_gpt_v1/screens/pages/data-quality-field-quality.png`
- evidence: `evidence/ui-image-breakdowns/pages/data-quality-field-quality/`
- production URL: `http://10.0.5.8:30180/data-quality?tab=field-quality`
- viewport: `1920 x 1080`
- image tag: `traffic/web-ui:ui-data-quality-tabs-stable-20260712-r292`
- status: `interaction-accepted-r292`（业务像素验收沿用 r271）

## 人眼读图

业务区顶部无菜单路径，左侧标题为“数据质量”，Tab 中“字段质量”选中。页面包含时间范围筛选、自动刷新、7 张字段质量 KPI、上排三块业务面板、下排三块业务面板和右侧处置/证据栏。进入该 tab 仍保持左侧“采集监测 / 数据质量”菜单选中。

## 业务模块

- KPI: 字段质量分 94/100、完整率 98.7%、格式合规 97.9%、一致性 96.4%、异常字段 23 项、影响记录 18.4K 条、待修复任务 7 个。
- 关键字段质量矩阵: 五元组、community_id、tenant、asset_id、protocol、timestamp、direction、bytes、packets、alert_id，多维热力状态。
- 字段异常趋势: 缺失值、格式不合法、映射不一致、时间漂移、未知协议，ECharts 多线趋势和阈值线。
- 五元组与 community_id 校验: 校验状态、匹配率、不匹配数、哈希碰撞告警、流程图、校验明细和 Top 5 不匹配样例。
- 下方: 异常样本表、字段血缘与映射、修复任务与规则建议。
- 右侧: 字段质量异常、快速定位、修复建议、证据与报告。

## 坐标测量

详见 `measurement.json`。主要业务区裁剪为 `(198, 80, 1920, 997)`；实现侧关键 boxes 记录在 `capture-meta-r124-final.json`。

## 动态图示分类

业务动态图示均为 API/typed fallback 数据驱动，不使用截图替代：

- 字段 KPI / Sparkline: `DataQualityVisuals.fieldKpis` + `fieldKpiTrends` -> React card + 6 个 `DataQualityKpiSparklineChart`（ECharts），30s refresh。
- 字段质量矩阵: `DataQualityVisuals.fieldQualityRows` -> React heatmap grid，30s refresh。
- 字段异常趋势: `DataQualityVisuals.fieldTrend` -> `DataQualityFieldTrendChart`（ECharts），30s refresh；数据不可用时走同类型 typed fallback。
- community_id 校验: `communityCheckRows` / `communityMismatchRows` -> React flow + tables，30s refresh。
- 字段血缘与映射: `fieldLineageRows` -> React pipeline，30s refresh。
- 修复任务: `fieldRepairRows` -> React table，30s refresh。
- 业务交互: 异常样本、修复任务、右侧定位/修复/证据按钮均进入字段详情 Drawer；创建、导出、同步类操作提供模拟提交与成功反馈，重放入口切换到重放对账 Tab。
- 表格承载: 异常样本与修复任务均为每页 5 条的受控分页；行区 `overflow-y: auto`，动态数据超出可视高度时保留稳定滚动条槽位。

## 验证摘要

- runtime: pass，console/pageerror/requestfailed/HTTP 4xx/5xx 均为 0，overflow 0，关键文本缺失 0，禁用资源 0。
- 8 Tab 统一性: pass，`capture-meta-r124.json` 显示数据质量 8 个 Tab 的 active state、title/tabs/filter/KPI shell、runtime 和 overflow/clipping 均通过。
- 8 Tab 固定几何回归: pass，r147 生产镜像 `traffic/web-ui:ui-data-quality-tabs-stable-20260709-r147` 在 Windows Chrome `1920 x 1080` 与 `1366 x 768` 下复验，8 个 Tab 切换时 `x/y/width/height` 最大差值为 `0`；Tab 条使用 8 个等宽固定槽位、禁用横向滚动重排，文本仅在槽内省略且 `title` 可看全文。证据见 `evidence/ui-image-breakdowns/pages/data-quality-tabs-stable/tab-geometry-r147-tabs-stability.json`。
- 8 Tab 静态门禁: pass，`src/routes/dataQualityTabs.test.ts` 锁定 8 个 tab、固定等宽 CSS、无 `min-width + overflow-x` 回退；`src/routes/noBitmapUi.test.ts` pass。
- r235 Windows Chrome 交互: pass；`GET /api/v1/data-quality` 的首次、切换近 7 天、30 秒自动刷新和手动刷新均为 `200`；关闭自动刷新后 31.5 秒内不发新请求。异常样本详情、异常样本第二页、修复任务详情、右侧创建动作、表格滚动容器均已实测。字段趋势 1 张和 KPI 趋势 6 张均检测到 ECharts canvas。证据：`interaction-r235.json`。
- full diff diagnostic: `pass`, ratio `0.10892650462962963`, tolerance `90`，不作为业务门槛。
- business diff: `pass`, `content-root=(198,80,1722,917)`, ratio `0.12001590805750711`, max `<0.125`, tolerance `90`。
- noBitmapUi: pass。
- build: pass。

## 差异说明

剩余 diff 热点集中在文字抗锯齿、ECharts canvas 曲线像素和表格密度/状态块透明度差异。业务标题、模块层级、字段文本、右侧动作和 runtime 门禁通过，差异在当前业务 ROI 阈值内接受。

## r271 最新生产验收

- 字段质量 7 张趋势/分布图均为 API/typed fallback 驱动的动态 ECharts，业务动作统一映射 `POST /v1/data-quality/actions` 与审计事件。
- 从总览真实点击字段质量后，前三个 Tab 的 `x/y/width/height` 完全不变；逐路由 8 Tab 几何差也为 `0`。Tab 切换保留现有查询参数。
- 业务 ROI `0.11759296904388268 < 0.125`；证据：`metrics-business-r271.json`、`diff-business-r271.png`、`../data-quality/interaction-r271-field-quality.png`。

## r292 Tab 几何回修验收

- 根因：Topic/Flink 的右轨宽度为 `274px`，其余相关页面为 `198px`，Tab 轨道未稳定补偿这项 `75.988px` 的主列差值。
- 修复：统一使用 8 等分网格，并通过 `--dq-tab-track-adjustment` 显式补偿右轨差值；未修改公共顶部、左侧、底部及字段质量业务模块。
- Windows Chrome CDP `Chrome/150.0.7871.49` 全 8 Tab 真实路由验收为 `pass`：逐路由最大 `x/y/width/height` 差值 `0`；从总览真实点击字段质量后，前三个 Tab 最大差值 `0`。
- 8 页 ECharts canvas 数量分别为 `22/7/11/7/9/7/5/6`；动作抽屉、`POST /v1/data-quality/actions` 和 `DATA_QUALITY_ACTION_REQUESTED` 均可见；HTTP、console、pageerror、requestfailed 数量均为 `0`。
- 生产镜像：`traffic/web-ui:ui-data-quality-tabs-stable-20260712-r292`，Deployment `1/1 Ready`；证据：`evidence/ui-image-breakdowns/pages/data-quality/interaction-r292-all-tabs.json`、`interaction-r292-field-quality.png`。

## 基本信息

- id：`data-quality-field-quality`；分类：`pages`；状态：`字段质量` 激活。
- 路由：`/data-quality?tab=field-quality`；目标图：1920 x 1080。
- 业务 ROI 为 `198,80,1722,917`，公共 AppShell 不纳入本页像素计算。
- 页面覆盖字段完整性、格式、一致性、哈希校验、血缘和修复闭环。

## 目标图观察

- 顶部为统一八 Tab，字段质量使用蓝色描边激活态。
- 工具栏含时间范围、固定日期窗、自动刷新和刷新按钮。
- 七 KPI 为质量分、完整率、格式合规、一致性、异常字段、影响记录、待修复任务。
- 左上为 10 行字段乘 6 个维度的质量矩阵。
- 中上为五条异常趋势线和红色 2,000 阈值线。
- 右上为五元组到 community_id 的校验流程、汇总和 Top 5 样本。
- 下排为异常样本、API SVG 字段血缘、修复任务表。
- 右栏分异常、定位、修复、证据四段，超高时栏内滚动。

## 区域与坐标

| 区域 | bbox | 内容 |
|---|---:|---|
| 画布 | `0,0,1920,1080` | 完整截图 |
| 公共顶部 | `0,0,1920,63` | 全局站点与指标 |
| 左侧导航 | `8,64,174,925` | 采集监测/数据质量 |
| 业务根 | `198,80,1722,917` | ROI 区域 |
| 标题与 Tab | `200,76,1504,73` | 标题及八 Tab |
| 工具栏 | `200,157,1504,31` | 时间与刷新 |
| KPI 条 | `200,198,1504,114` | 七指标 |
| 质量矩阵 | `200,320,505,368` | 字段热力表 |
| 异常趋势 | `713,320,430,368` | 动态 ECharts |
| community 校验 | `1151,320,553,402` | 哈希流程与表 |
| 异常样本 | `200,695,519,282` | 分页表 |
| 血缘映射 | `727,695,416,282` | API SVG 拓扑 |
| 修复任务 | `1151,728,553,249` | 分页任务表 |
| 右侧栏 | `1712,155,198,822` | 闭环动作 |

## 文本清单

- 八 Tab：`质量总览 / Topic 健康 / Flink 质量 / 字段质量`。
- 八 Tab 后半：`存储质量 / 重放对账 / 质量报告 / 质量设置`。
- 工具栏：`时间范围 / 近 24 小时 / 自动刷新 / 刷新`。
- KPI：`字段质量分 94/100 / 完整率 98.7% / 格式合规 97.9%`。
- KPI：`一致性 96.4% / 异常字段 23 项 / 影响记录 18.4K 条 / 待修复任务 7 个`。
- 字段：`五元组 / community_id / tenant / asset_id / protocol`。
- 字段：`timestamp / direction / bytes / packets / alert_id`。
- 图例：`缺失值 / 格式不合法 / 映射不一致 / 时间漂移 / 未知协议`。
- 标题：`关键字段质量矩阵 / 字段异常趋势（近 24 小时）`。
- 标题：`五元组与 community_id 校验 / 异常样本表（按影响时间排序）`。
- 标题：`字段血缘与映射 / 修复任务与规则建议`。
- 右栏：`字段质量异常（近 24 小时） / 快速定位 / 修复建议 / 证据与报告`。

## 组件清单

- `DataQualityTabs`：统一八 Tab 与 query 同步。
- `FieldQualityKpiStrip`：`fieldKpis` 指标卡和 sparkline。
- `FieldQualityMatrix`：`fieldQualityRows` React heatmap。
- `DataQualityFieldTrendChart`：ECharts line series 与 markLine。
- `CommunityIdValidation`：五元组、SHA-1、汇总和样本表。
- `FieldAnomalyTable`：异常样本分页表。
- `FieldLineageMap`：API 驱动 SVG 节点和边，不改用 ECharts。
- `FieldRepairTable`：修复任务分页表。
- `FieldQualityActionRail`：定位、修复和证据动作。

## 图标清单

- 质量分使用盾牌星标图标，颜色绑定健康语义。
- 刷新使用 `ReloadOutlined` 并显示 loading。
- Sink 健康使用绿色 `CheckCircleFilled`。
- 映射异常使用琥珀 `WarningFilled` 和数量。
- 快速定位使用文件检索图标且按钮可点击。
- 修复任务使用工具图标并保留键盘焦点态。
- 证据导出使用文档图标，失败时显示错误反馈。

## Token 与样式

- `canvas-bg #020e18`：页面底色。
- `panel-bg #031723`：业务面板。
- `border #0c4261`：1px 面板与表格线。
- `active #168cff`：Tab、链接和动作。
- `text-primary #dbe6ef`；`text-secondary #8299aa`。
- `success #54c84d`：通过与 >=98%。
- `warning #f5a20b`：95%-98% 和待修复。
- `danger #f0443e`：<95%、异常和阈值。
- 正文 12-13px，面板标题 15-16px，页面标题约 20px。
- 面板圆角 4px；间距按 8px 基线；图表和表格固定高度。

## 状态与交互

- 点击 Tab 更新 query，全部八项几何保持不变。
- 时间范围变化后刷新全部字段模块并保留筛选状态。
- 自动刷新开启时 30 秒 refetch，关闭后停止轮询。
- 手动刷新触发真实查询并显示 loading。
- 点击矩阵单元格按字段和维度过滤异常样本。
- 趋势 hover 显示时间、五序列值和阈值，数据变化必须重绘 canvas。
- 点击不匹配样本打开详情 Drawer，宿主页面保持可见。
- 异常样本表和修复任务表必须分页，翻页后高度稳定。
- 创建修复任务进入可审计流程，不能是假按钮。
- 右栏超高时内部滚动，业务区不得横向溢出。

## 实现映射

| 模块 | 数据字段 | 实现 |
|---|---|---|
| KPI | `fieldKpis` | 指标卡 + sparkline |
| 质量矩阵 | `fieldQualityRows` | React heatmap |
| 异常趋势 | `fieldTrend`, `fieldTrendSummary` | ECharts line |
| community 校验 | `communityCheckRows`, `communityMismatchRows` | SVG + 表格 |
| 异常样本 | `fieldAnomalyRows` | 分页表 |
| 字段血缘 | `fieldLineageRows` | API SVG 拓扑 |
| 修复任务 | `fieldRepairRows` | 分页任务表 |
| 右栏 | `fieldRail*Rows` | 动作列表与 Drawer |

## 验收证据

- target：`evidence/ui-image-breakdowns/pages/data-quality-field-quality/target.png`。
- implementation：`evidence/ui-image-breakdowns/pages/data-quality-field-quality/implementation.png`。
- diff / metrics：同目录 `diff.png`、`metrics.json`。
- overlay：同目录 `regions-overlay.png`。
- interaction：同目录 `interaction-r235.json`。
- r292 八 Tab 最大几何 delta 为 0。
- r271 业务 ratio `0.11759296904388268 < 0.125`。
- runtime 为 0 console/page/request/HTTP 错误。

## 差异清单

- Windows 字体抗锯齿和目标图文字栅格存在差异。
- ECharts 折线采样、线宽和 canvas 抗锯齿存在像素差异。
- 公共顶部时间与指标为实时值，目标图为静态值。
- 业务 ROI 已通过 0.125，但不宣称严格逐像素一致。
- 字段血缘保留 API SVG 拓扑，不用 ECharts 替代。
- typed fallback 必须标注为仿真，不能伪称实时 API。

## 结论

该页已形成可执行的目标结构、动态 ECharts、API SVG 拓扑、分页表格、真实交互、数据映射和证据账本。当前保持 `business-pixel-accepted-r271`，严格像素差异留待后续精调。

### 实施门禁补充

- 图表数据变化必须产生 ECharts option 与 canvas 变化。
- 两张业务表都必须出现分页器并可切换页码。
- 所有右栏命令均需真实点击反馈和审计记录。
- 加载、空数据、错误、无权限四种状态不得改变 Tab 几何。
- 页面纵向看不全时只增加业务容器滚动条。
- 所有截断文本提供 title 或 Tooltip。
- 快照仿真数据和真实 API 数据必须可辨识。
- 目标 PNG 只用于拆解与 diff，运行时禁止加载。
- 业务 ROI 门和交互门必须同时通过才可升级结论。
- 严格像素未通过时必须继续保留差异说明。
