# data-quality-flink-quality.png 逐图精拆记录

## 基本信息

- page-id: `data-quality-flink-quality`
- route: `/data-quality?tab=flink-quality`
- type: `menu-state`
- parent: `data-quality`
- target UI: `doc/04_assets/ui_suite_gpt_v1/screens/pages/data-quality-flink-quality.png`
- evidence: `evidence/ui-image-breakdowns/pages/data-quality-flink-quality/`
- status: `business-pixel-accepted`

## 布局拆解

业务区顶部不显示菜单路径；左侧显示页面标题 `数据质量`，Tab 中 `Flink 质量` 选中。Tab 下方是时间窗/日期范围/自动刷新/刷新按钮。业务主体为 7 个 KPI、上方三栏、下方三栏、右侧 Flink 处置栏。

关键区域见 `measurement.json` 与 `regions-overlay.png`。业务 crop 为 `x=198,y=80,w=1722,h=917`。

## 业务模块

- KPI: Flink 质量分、运行作业、Checkpoint 成功率、Watermark 延迟 P95、Backpressure、迟到数据率、异常事件。
- 上方三栏: Flink 作业健康明细、Checkpoint 与 Watermark 趋势、Backpressure 热力图。
- 下方三栏: 迟到数据与窗口闭合、异常与失败原因 Top 10、Sink 写入质量。
- 右侧栏: Flink 质量异常、快速定位、修复建议、快收证据与报告。

## 图示分类

业务动态图示全部由 `PageSnapshot.visuals.dataQuality` API/typed fallback 驱动，React/SVG/CSS 实现；未使用目标截图替代业务图示。

## 验收

- runtime: pass
- full diff ratio: `0.109515335648`
- business diff ratio: `0.121231810542`
- production image: `traffic/web-ui:ui-data-quality-flink-quality-visual-20260708-r117`
- final URL: `http://10.0.5.8:30180/data-quality?tab=flink-quality&__codex_ui_breakdown_production=1&__capture=r117-final`

## r271 最新生产验收

- Flink 上下业务区固定为 `318px/309px`，右侧栏恢复 274px，作业表保留 5 条/页受控分页；Checkpoint/Watermark、Backpressure、Sink 等 11 张图均为动态 ECharts。
- Windows Chrome 业务动作 endpoint/audit Drawer 通过，运行时错误为 `0`。
- 逐路由与真实点击字段质量的 Tab 几何差均为 `0`；业务 ROI：`0.12448878266629683 < 0.125`；证据：`metrics-business-r271.json`、`diff-business-r271.png`、`../data-quality/interaction-r271-flink-quality.png`。

## 目标图观察

- 当前状态为八 Tab 中 `Flink 质量` 激活。
- 顶部工具栏包含最近 24 小时、日期窗、自动刷新和刷新。
- 七 KPI 同行排列，异常事件为红色，Backpressure 为琥珀色。
- 上排依次是作业表、Checkpoint/Watermark 趋势、Backpressure 热力图。
- 下排依次是迟到窗口、失败原因 Top 10、四个 Sink 写入卡。
- 右栏按异常、快速定位、修复建议、证据报告分段。
- 作业表有 8 个作业，`behavior-job` 是重点告警行。
- 趋势包含 checkpoint duration、age、watermark P95 和 SLA。
- 热力图按作业和 subtask 分桶，状态色由绿到黄到红。
- 表格看不全时局部滚动，公共区域位置不变。

## 区域与坐标

| 区域 | bbox | 内容 |
|---|---:|---|
| 画布 | `0,0,1920,1080` | 完整截图 |
| 顶部公共区 | `0,0,1920,64` | 站点和状态 |
| 左侧导航 | `8,70,172,918` | 数据质量选中 |
| 业务根 | `196,70,1430,890` | 主业务列 |
| 标题 Tab | `197,76,1429,77` | 标题和八 Tab |
| 筛选栏 | `197,161,1429,30` | 日期与刷新 |
| KPI 条 | `197,201,1429,116` | 七指标 |
| 作业表 | `197,325,565,320` | 8 行表 |
| 趋势图 | `769,325,454,320` | ECharts line |
| 热力图 | `1230,325,396,320` | ECharts heatmap |
| 迟到窗口 | `197,652,565,307` | 堆叠条与表 |
| 失败原因 | `769,652,454,307` | Top 10 表 |
| Sink 质量 | `1230,652,396,307` | 四张卡 |
| 右侧栏 | `1635,113,274,846` | 闭环动作 |

## 文本清单

- Tab：`质量总览 / Topic 健康 / Flink 质量 / 字段质量`。
- Tab：`存储质量 / 重放对账 / 质量报告 / 质量设置`。
- 筛选：`最近 24 小时 / 自动刷新 / 刷新`。
- KPI：`Flink 质量分 91/100`。
- KPI：`运行作业 9`。
- KPI：`Checkpoint 成功率 99.2%`。
- KPI：`Watermark 延迟 P95 1.6s`。
- KPI：`Backpressure 0.38`。
- KPI：`迟到数据率 0.67%`。
- KPI：`异常事件 312`。
- 面板：`Flink 作业健康明细`。
- 面板：`Checkpoint 与 Watermark 趋势`。
- 面板：`Backpressure 热力图（按作业 / Subtask）`。
- 面板：`迟到数据与窗口闭合（按来源 Topic）`。
- 面板：`异常与失败原因（Top 10）`。
- 面板：`Sink 写入质量（近 24h）`。
- 右栏：`Flink 质量异常 / 快速定位 / 修复建议 / 快收证据与报告`。
- 作业：`session-job / feature-job / rule-job / pcap-index-job`。
- 作业：`behavior-job / alert-generator-job / log-job / user-behavior-job`。
- 图注：`Checkpoint 耗时陡升 / Watermark 延迟升高`。

## 组件清单

- `AppShell`：公共顶部、侧栏和底栏，本页不得修改。
- `DataQualityTabs`：八等分固定 Tab 槽位。
- `FlinkQualityFilterBar`：时间、日期、自动刷新和刷新。
- `DataQualityMetricTile[]`：七指标卡和 sparkline。
- `FlinkJobHealthTable`：作业健康与操作列。
- `FlinkCheckpointWatermarkTrend`：动态 ECharts 多轴折线。
- `FlinkBackpressureHeatmap`：动态 ECharts heatmap。
- `LateWindowClosure`：Topic 堆叠条和窗口表。
- `FlinkFailureTable`：异常与失败原因 Top 10。
- `SinkQualityCards`：四个真实存储目标的质量卡。
- `FlinkQualitySideRail`：异常、定位、修复和证据。

## 图标清单

- 质量分使用盾牌，91 分显示健康绿轮廓。
- 刷新使用 `ReloadOutlined` 并提供 loading。
- 作业详情使用 `SearchOutlined`。
- 作业指标使用 `LineChartOutlined`。
- 告警使用 `WarningFilled` 并配严重文字。
- 报告使用 `ExportOutlined`，点击必须有反馈。
- Sink 使用绿色状态圆点并保留 `正常` 文案。

## Token 与样式

- `canvas-bg #020e18`：页面深蓝底。
- `panel-bg #031723`：业务面板。
- `border #0c4261`：表格和面板线。
- `active #168cff`：激活 Tab 和链接。
- `series-blue #2196f3`：checkpoint duration。
- `series-cyan #28c6df`：checkpoint age。
- `success #54c84d`：watermark 和健康状态。
- `warning #f5a20b`：backpressure 警告。
- `danger #f0443e`：严重告警与热点。
- `text-primary #dbe6ef`；次级文字 `#8299aa`。
- 表格正文 11-12px，面板标题约 15px。
- 面板圆角 4px，间距 8px，图表高度固定。

## 状态与交互

- 点击 Tab 更新 `tab=flink-quality` 并保留其他 query。
- 八 Tab 切换时几何最大 delta 必须为 0。
- 手动刷新触发真实 snapshot 请求并重绘图表。
- 自动刷新开启时 30 秒请求，关闭后停止轮询。
- 作业表必须分页，分页区高度稳定。
- 点击搜索图标打开作业详情 Drawer。
- 点击指标图标进入该作业指标视图。
- 趋势 hover 显示时间、全部序列和 SLA。
- 数据刷新后检查 ECharts option 或 canvas 像素变化。
- 热力图 hover 显示作业、subtask 和压力值。
- 点击迟到 Topic 后过滤窗口闭合表。
- 点击失败行打开异常上下文和修复建议。
- 右栏动作必须产生 Drawer、API 请求或下载反馈。
- 表格和右栏使用局部滚动，不改变 AppShell 几何。

## 面板逐项复核

- `session-job` 运行中，并行度 24，使用健康色。
- `feature-job` 与 `rule-job` 保持健康绿行。
- `pcap-index-job` checkpoint 较高但仍为运行态。
- `behavior-job` backpressure 0.78、迟到率 1.42%、异常 156。
- `behavior-job` 采用琥珀强调，不能与严重红混淆。
- `alert-generator-job`、`log-job`、`user-behavior-job` 为正常态。
- 趋势左轴承载 duration/age，右轴承载 watermark。
- checkpoint SLA 与 watermark SLA 使用不同虚线。
- 热力图分组为 `0-7 / 8-15 / 16-23`。
- 热力图后半为 `24-31 / 32-39 / 40-47`。
- behavior-job 行出现橙红连续区，必须由数据驱动。
- 迟到条分正常、迟到事件、丢弃事件三段。
- 窗口表展示 1、5、10、30、60 min。
- 窗口表同时展示闭合 P95 和丢弃率。
- Sink 卡展示 EPS、成功率、P95 和重试次数。
- Sink 卡 mini trend 必须随 snapshot 动态更新。
- 四个 Sink 为 ClickHouse、OpenSearch、NebulaGraph、MinIO。
- 右栏异常至少包含 behavior-job、Watermark、Checkpoint。
- 快速定位包含作业、Checkpoint、Watermark、Backpressure、日志。
- 修复建议包含并行度、压根因、Watermark、数据源和版本。
- 证据动作包含三类报告、质量报告和告警截图。

## 实现映射

| 模块 | Snapshot 字段 | 实现 |
|---|---|---|
| KPI | `flinkKpis` | 指标卡 + sparkline |
| 作业表 | `flinkJobRows` | 分页表 |
| 趋势 | `flinkCheckpointWatermarkTrend` | ECharts line |
| 热力图 | `flinkBackpressureRows` | ECharts heatmap |
| 迟到窗口 | `flinkLateTopicRows`, `flinkWindowRows` | 堆叠条 + 表 |
| 失败原因 | `flinkFailureRows` | Top 10 表 |
| Sink | `flinkSinkRows` | 四张质量卡 |
| 右栏 | `flinkRail*Rows` | 动作与 Drawer |

## 验收证据

- target：`evidence/ui-image-breakdowns/pages/data-quality-flink-quality/target.png`。
- overlay：同目录 `regions-overlay.png`。
- implementation：同目录 `implementation-r117-final.png`。
- diff / metrics：同目录 `diff-r117.png`、`metrics-r117.json`。
- business diff：同目录 `diff-business-r117.png`。
- runtime：同目录 `capture-meta-r117-final.json`。
- r271 ratio `0.12448878266629683 < 0.125`。
- r271 canvas count 11，runtime 错误为 0。
- Tab 几何和点击字段质量后的前三 Tab delta 均为 0。

## 差异清单

- 公共顶部实时值与静态目标不同，不归本页所有。
- 密集表格文字和 Windows 字体抗锯齿有残余 diff。
- ECharts 折线和热力图与静态目标存在采样差异。
- ratio 接近 0.125，后续优先精调线宽和列宽。
- 旧记录提到 React SVG/CSS，当前约束以动态 ECharts canvas 为准。
- typed fallback 必须标为仿真，不冒充 Flink REST 实时响应。

## 结论

Flink 质量页的目标区域、文本、组件、图标、token、动态 ECharts、分页和真实动作契约已经锁定。当前业务像素通过，严格像素精调及正式 Chrome extension 双门禁继续保留。

- 目标 PNG 只参与拆解和 diff，生产页面不得加载。
- 图表与表格必须共用同轮 snapshot，防止指标漂移。
- 业务像素和交互门需同时通过后才允许提升状态。
- 严格像素尚未通过时保持诚实差异记录。
