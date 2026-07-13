# data-quality-replay-reconcile.png 逐图精拆记录

## 基本信息

- 分类：pages
- Page id：`data-quality-replay-reconcile`
- Route/state：`/data-quality?tab=replay-reconcile`
- Type：`menu-state`
- Parent：`data-quality`
- 源图：`doc/04_assets/ui_suite_gpt_v1/screens/pages/data-quality-replay-reconcile.png`
- 源图尺寸：1920 x 1080
- 证据目录：`evidence/ui-image-breakdowns/pages/data-quality-replay-reconcile`
- 当前状态：`business-pixel-accepted`
- 生产镜像：`traffic/web-ui:ui-data-quality-replay-reconcile-visual-20260709-r128`

## 目标图观察

- 左侧菜单保持 `采集监测 / 数据质量` 选中，页面内部 8 个数据质量 Tab 统一，`重放对账` active。
- 业务区顶部没有菜单路径；左侧写清楚页面 `数据质量`，右侧只保留刷新/自动刷新等业务动作。
- KPI 为 7 张重放对账专属卡片：对账通过率、待重放 DLQ、重放成功率、重复记录、幂等冲突、窗口差异率、验收包。
- 上排三面板：`DLQ 重放任务表`、`时间窗对账报告（近 24 小时）`、`幂等检查与重复检测`。
- 下排三面板：`差异样本与原因（近 24 小时）`、`重放链路状态`、`验收证据与导出`。
- 右侧栏四段：`重放对账异常`、`快速定位`、`修复建议`、`证据与报告`。

## 区域与坐标

| 区域 | bbox | 类型 | 说明 |
|---|---:|---|---|
| canvas | `0,0,1920,1080` | viewport | 画布 |
| topbar | `0,0,1920,80` | AppShell | 顶部全局状态栏 |
| sidebar | `0,80,190,917` | AppShell | 左侧导航 |
| bottombar | `0,997,1920,83` | AppShell | 底部状态栏 |
| business-root | `198,80,1722,917` | business | 业务内容区 |
| titlebar | `199,81,1505,74` | business | 标题和八 Tab |
| filterbar | `199,163,1505,32` | business | 筛选工具栏 |
| kpis | `199,203,1505,116` | business | 重放对账 KPI 指标条 |
| replayUpper | `199,327,1505,350` | business | 上排业务面板 |
| replayLower | `199,684,1505,312` | business | 下排业务面板 |
| taskTable | `206,364,561,304` | table | DLQ 重放任务表 |
| reconcileTrend | `791,364,484,304` | svg-chart | 时间窗对账报告趋势 |
| idempotency | `1298,364,398,304` | table | 幂等检查与重复检测 |
| differenceTable | `206,722,553,267` | table | 差异样本与原因表 |
| flow | `782,722,484,267` | react-flow | 重放链路状态 |
| evidence | `1290,722,406,267` | table/actions | 验收证据与导出 |
| rail | `1711,81,198,916` | business-rail | 右侧异常处置栏 |

## 文本清单

- 数据质量
- 质量总览
- Topic 健康
- Flink 质量
- 字段质量
- 存储质量
- 重放对账
- 质量报告
- 质量设置
- 时间范围
- 近 24 小时
- 2025-06-25 15:30:45 ~ 2025-06-26 15:30:45
- 自动刷新
- 刷新
- 30s
- 对账通过率 99.12%
- 待重放 DLQ 12,845
- 重放成功率 98.6%
- 重复记录 2,136
- 幂等冲突 47
- 窗口差异率 0.31%
- 验收包 8
- DLQ 重放任务表
- flow_original
- flow_enriched
- dns_logs
- asset_events
- threat_alerts
- pcap_index
- 时间窗对账报告（近 24 小时）
- 源端总数 3.21B
- 落库总数 3.20B
- 差异数量 9.95M
- 差异率 0.31%
- 阈值 1.00%
- 幂等检查与重复检测
- 幂等键一致性
- Hash 碰撞检测
- 重复 session_id
- 重复 alert_id
- 重放批次重叠
- 幂等冲突写入
- 差异样本与原因（近 24 小时）
- offset gap
- schema mismatch
- late event
- duplicate key
- sink timeout
- 重放链路状态
- DLQ / Kafka
- 重放作业（Flink）
- 去重过滤（幂等）
- 落库目标
- 重试队列
- 校验检查点
- 验收门禁
- 验收证据与导出
- 对账报告
- 重放日志
- 投递快照摘要
- 差异样本还原
- 审计记录
- 重放对账异常
- 差异率超阈值窗口
- 重放失败任务
- 幂等冲突告警
- 重复记录激增
- 快速定位
- 定位 DLQ Topic
- 定位重放作业
- 定位差异窗口
- 查看对账详情
- 修复建议
- 重放失败重试
- 扩容重放作业
- 补齐幂等规则
- 优化幂等字段索引
- 延长对账时间窗
- 证据与报告
- 导出对账报告
- 生成验收包
- 查看验收历史
- 审计操作日志

## 图示/图标/背景分类

| 类型 | 实现方式 | 数据来源 | 刷新节奏 | 禁止方式 |
|---|---|---|---|---|
| 时间窗对账报告 | React SVG 折线 + 柱状 | `snapshot.visuals.dataQuality.replayReconcileTrend` / typed fallback | React Query 30s | 禁止截图替代 |
| 重放链路状态 | React/CSS 节点与数据驱动边图例 | `replayFlowNodes` / `replayFlowEdges` | React Query 30s | 禁止整图截图 |
| 表格/证据列表 | React 数据表格/按钮 | `replayTaskRows`、`replayDifferenceRows`、`replayEvidenceRows` | React Query 30s | 禁止静态图片 |
| 独立图标 | Ant Design icons | 组件库 | 跟随组件渲染 | 禁止整卡截图 |
| 背景装饰 | 无业务动态图背景资源 | CSS 面板样式 | N/A | 禁止含业务 UI 的背景图 |

## 数据映射

- KPI：`replayKpis`
- DLQ 任务：`replayTaskRows`
- 对账趋势：`replayReconcileTrend`，字段 `times/sourceTotal/sinkTotal/diffCount/diffRate/diffRateThreshold`
- 趋势汇总：`replayReconcileSummary`
- 幂等检测：`replayIdempotencyRows`
- 差异样本：`replayDifferenceRows`
- 链路状态：`replayFlowNodes`、`replayFlowEdges`
- 验收证据：`replayEvidenceRows`
- 右侧栏：`replayRailAlerts`、`replayRailLocateRows`、`replayRailRepairRows`、`replayRailEvidenceRows`

## 验证结果

- Windows Chrome CDP：`http://127.0.0.1:9224`
- 生产 URL：`http://10.0.5.8:30180/data-quality?tab=replay-reconcile&__codex_ui_breakdown_production=1&__capture=r128-final&windowsCdpEvidenceTs=1783555335274`
- Viewport：1920 x 1080
- Runtime：0 console error、0 page error、0 requestfailed、0 HTTP 4xx/5xx、0 overflow
- 全图 diff：`0.08337432484567901`，阈值 `0.13`，tolerance `90`
- 业务区 diff：`0.08638923824975904`，阈值 `0.13`，tolerance `90`

## 主线程判断

`business-pixel-accepted`。业务模块、Tab 选中态、右侧栏、文本完整性和动态业务图示均通过；diff 热点集中在字体抗锯齿、SVG 曲线/柱形动态绘制和表格微排版，符合当前页 alpha 规则。

## r271 最新生产验收

- Windows Chrome canvas `7`，业务动作 endpoint/audit Drawer 通过，运行时错误为 `0`。
- 逐路由与真实点击字段质量的 Tab 几何差均为 `0`；业务 ROI：`0.08187963325341308 < 0.125`；证据：`metrics-business-r271.json`、`diff-business-r271.png`、`../data-quality/interaction-r271-replay-reconcile.png`。

## 组件清单

- `DataQualityTabs`：八等分固定槽位，重放对账激活。
- `ReplayKpiStrip`：七指标与动态 sparkline。
- `ReplayTaskTable`：DLQ 任务、成功率、失败数、幂等状态和操作。
- `ReplayReconcileTrend`：源端、落库、差异量和差异率动态 ECharts。
- `ReplayIdempotencyTable`：幂等键、Hash、重复 ID、批次和冲突。
- `ReplayDifferenceTable`：差异样本及处置。
- `ReplayFlowStatus`：API SVG 数据流、重试流、失败流和控制流。
- `ReplayEvidenceExport`：PDF、日志、JSON、样本和审计导出。
- `ReplayReconcileSideRail`：异常、定位、修复和证据动作。

## 图标清单

- 重放任务使用 `PlayCircleOutlined`。
- 刷新使用 `ReloadOutlined` 并防重复提交。
- 通过状态使用 `CheckCircleFilled` 和文字。
- 冲突与重试使用琥珀 `WarningFilled`。
- 验收文件使用 `FileDoneOutlined`。
- 导出使用 `ExportOutlined` 并反馈结果。

## Token 与样式

- `canvas-bg #020e18`；`panel-bg #031723`；`border #0c4261`。
- `active #168cff` 用于激活 Tab、链接和操作。
- `flow #1da1f2` 表示正常数据流。
- `success #54c84d` 表示通过和落库确认。
- `warning #f5a20b` 表示重试和幂等冲突。
- `danger #f0443e` 表示失败和阈值。
- 主文字 `#dbe6ef`，次级文字 `#8299aa`。
- 正文 12px，面板标题 15px，圆角 4px，间距 8px。

## 状态与交互

- 点击重放对账 Tab 更新 query，八 Tab 几何不变。
- 自动刷新 30 秒轮询，关闭后停止请求。
- 手动刷新重取任务、趋势、幂等、差异、链路和证据。
- 点击任务详情打开 Drawer，点击重放提交可审计任务。
- 任务表和差异表必须分页，翻页后高度稳定。
- 趋势粒度切换后更新 ECharts option 和 canvas。
- tooltip 显示源端、落库、差异量、差异率和阈值。
- 点击幂等行下钻到冲突样本。
- 点击链路节点过滤任务或差异，保留 API SVG。
- 点击 PDF、日志、JSON、样本、记录导出真实文件。
- 右栏动作必须打开流程，不能是假按钮。

## 实现映射

| 模块 | Snapshot 字段 | 实现 |
|---|---|---|
| KPI | `replayKpis` | 指标卡 + sparkline |
| 任务 | `replayTaskRows` | 分页表 |
| 趋势 | `replayReconcileTrend` | ECharts line/bar |
| 幂等 | `replayIdempotencyRows` | 状态表 |
| 差异 | `replayDifferenceRows` | 分页表 |
| 链路 | `replayFlowNodes`, `replayFlowEdges` | API SVG |
| 证据 | `replayEvidenceRows` | 导出动作 |
| 右栏 | `replayRail*Rows` | Drawer 与动作 |

## 验收证据

- target：`evidence/ui-image-breakdowns/pages/data-quality-replay-reconcile/target.png`。
- implementation：同目录 `implementation-r128-final.png`。
- diff / metrics：同目录 `diff-r128.png`、`metrics-r128.json`。
- r128 ratio `0.08638923824975904`；r271 ratio `0.08187963325341308`。
- runtime 错误为 0，业务区无溢出。
- r271 canvas count 7，动作 endpoint 和审计 Drawer 通过。

## 差异清单

- 静态曲线与生产 ECharts 采样、抗锯齿有细微差异。
- 公共顶部实时值不纳入业务区精调。
- typed fallback 必须标注仿真，不能冒充实时 DLQ 数据。
- API SVG 拓扑按原动态实现保留，不替换为 ECharts。
- ratio 远低于 0.125，后续只做微排版精调。

## 结论

重放对账页的动态 ECharts、API SVG、分页表格、可审计动作、字段映射和证据边界已补齐，可直接驱动后续全栈实现与智能体复检。
