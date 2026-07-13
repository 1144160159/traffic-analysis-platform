# data-quality-storage-quality 页面拆解

## 锁定信息

- page-id: `data-quality-storage-quality`
- queue order: 12
- type: `menu-state`
- parent: `data-quality`
- route: `/data-quality?tab=storage-quality`
- target UI: `doc/04_assets/ui_suite_gpt_v1/screens/pages/data-quality-storage-quality.png`
- evidence: `evidence/ui-image-breakdowns/pages/data-quality-storage-quality/`
- production URL: `http://10.0.5.8:30180/data-quality?tab=storage-quality&__codex_ui_breakdown_production=1&__capture=r126-final`
- viewport: 1920 x 1080
- image tag: `traffic/web-ui:ui-data-quality-storage-quality-visual-20260708-r126`
- status: `business-pixel-accepted`

## 目标图读取

当前图是数据质量页面的 `存储质量` Tab 态。公共 AppShell 保持采集监测菜单选中，二级菜单高亮 `数据质量`；业务区顶部不展示菜单路径，只展示页面标题 `数据质量`、8 个统一 Tab、筛选工具栏和右侧刷新动作。

业务区采用统一数据质量外壳：

| 区域 | bbox | 说明 |
|---|---:|---|
| 业务内容区 | `198,80,1722,917` | 不含左侧菜单、顶部状态栏、底部状态栏 |
| 标题与八 Tab | `199,81,1505,74` | `存储质量` Tab 选中 |
| 筛选工具栏 | `199,163,1505,32` | 时间范围、日期窗、自动刷新、刷新 |
| KPI 指标条 | `199,203,1505,116` | 7 个存储专属指标 |
| 上排业务面板 | `199,327,1505,350` | 组件健康表、写入趋势、容量水位 |
| 下排业务面板 | `199,684,1505,312` | 失败写入、链路全景、副本/对象健康 |
| 右侧处置栏 | `1711,81,198,916` | 异常、快速定位、修复建议、证据报告 |

## 文本清单

必须保留的目标文案：

- `数据质量`
- `质量总览`
- `Topic 健康`
- `Flink 质量`
- `字段质量`
- `存储质量`
- `重放对账`
- `质量报告`
- `质量设置`
- `时间范围`
- `近 24 小时`
- `2025-06-25 15:30:45 ~ 2025-06-26 15:30:45`
- `自动刷新`
- `刷新`
- `存储质量分 93/100`
- `写入成功率 99.84%`
- `写入延迟 P95 420 ms`
- `失败写入 186 条`
- `索引滞后 2.1 s`
- `归档成功率 99.7%`
- `容量水位 72.6%`
- `存储组件健康总览`
- `写入速率与延迟趋势（近 24 小时）`
- `容量与水位趋势（近 7 天）`
- `失败写入与原因列表（近 24 小时）`
- `索引与归档链路（写入链路全景）`
- `副本、分片与对象健康`
- `存储质量异常（近 24 小时）`
- `快速定位`
- `修复建议`
- `证据与报告`

关键存储对象：`ClickHouse`、`OpenSearch`、`NebulaGraph`、`MinIO`。

## 组件与图示分类

| 类型 | 内容 | 实现方式 | 禁止方式 |
|---|---|---|---|
| 业务动态图示 | 写入速率与延迟趋势 | typed fallback 数据 + React SVG polyline | 截图替代 |
| 业务动态图示 | 容量与水位趋势 | typed fallback 数据 + React SVG polygon/polyline | 截图替代 |
| 业务动态图示 | 索引与归档链路 | typed fallback 数据 + React/CSS 节点和边标签 | 截图替代 |
| 业务动态图示 | OpenSearch 索引健康环 | typed fallback 数据 + React SVG donut | 截图替代 |
| 表格 | 组件健康、失败写入、分区/对象健康 | typed fallback 数据 + React 表格网格 | 整表图片 |
| 独立图标 | KPI 分数盾牌、右栏动作图标 | Ant Design icons | 整卡图片 |

## 数据来源与映射

数据通过 `fetchPageSnapshot(route.id)` 进入 `snapshot.visuals.dataQuality`，后端缺口时使用 `web/ui/src/services/mockData.ts` 的 typed fallback。

| UI 模块 | 字段 |
|---|---|
| KPI | `storageKpis[]` |
| 组件健康表 | `storageComponentRows[][]` |
| 写入趋势 | `storageTrend.times/clickhouse/opensearch/nebula/minio/latencyP95/latencySla` |
| 容量水位 | `storageCapacityTrend.days/clickhouse/opensearch/nebula/minio/threshold` |
| 失败写入 | `storageFailureRows[][]` |
| 链路全景 | `storagePipelineRows[]` |
| 副本/分片/对象健康 | `storageReplicaRows[][]`、`storageIndexHealth[]`、`storagePartitionRows[][]`、`storageObjectRows[][]` |
| 右侧栏 | `storageRailAlerts[][]`、`storageRailLocateRows[]`、`storageRailRepairRows[]`、`storageRailEvidenceRows[]` |

刷新节奏沿用数据质量页 React Query：`refetchInterval: 30_000`，分页表格除外。

## 实现文件

- `web/ui/src/pages/DataQualityPage.tsx`
- `web/ui/src/services/mockData.ts`
- `web/ui/src/styles/pages.css`
- `deployments/kubernetes/applications/web-ui.yaml`

## 证据与验收

| 证据 | 路径 |
|---|---|
| target | `evidence/ui-image-breakdowns/pages/data-quality-storage-quality/target.png` |
| overlay | `evidence/ui-image-breakdowns/pages/data-quality-storage-quality/regions-overlay.png` |
| implementation | `evidence/ui-image-breakdowns/pages/data-quality-storage-quality/implementation-r126-final.png` |
| implementation alias | `evidence/ui-image-breakdowns/pages/data-quality-storage-quality/implementation.png` |
| diff | `evidence/ui-image-breakdowns/pages/data-quality-storage-quality/diff-r126.png` |
| diff alias | `evidence/ui-image-breakdowns/pages/data-quality-storage-quality/diff.png` |
| metrics | `evidence/ui-image-breakdowns/pages/data-quality-storage-quality/metrics-r126.json` |
| metrics alias | `evidence/ui-image-breakdowns/pages/data-quality-storage-quality/metrics.json` |
| business diff | `evidence/ui-image-breakdowns/pages/data-quality-storage-quality/diff-business-r126.png` |
| business metrics | `evidence/ui-image-breakdowns/pages/data-quality-storage-quality/metrics-business-r126.json` |
| runtime | `evidence/ui-image-breakdowns/pages/data-quality-storage-quality/capture-meta-r126-final.json` |
| runtime alias | `evidence/ui-image-breakdowns/pages/data-quality-storage-quality/capture-meta.json` |
| verification | `evidence/ui-image-breakdowns/pages/data-quality-storage-quality/verification.json` |

Diff 结果：

- full ratio: `0.10324749228395062`, threshold `0.13`, status `pass`
- business ratio: `0.11260840213948174`, threshold `0.13`, status `pass`
- runtime: 0 console error, 0 page error, 0 requestfailed, 0 HTTP 4xx/5xx, 0 business overflow

主线程判断：业务内容与目标图在 alpha 门禁下通过；差异集中于 SVG 折线密度、抗锯齿和表格微排版，业务模块、文本、选中态、右侧栏和动态图示均符合当前页要求。

## r271 最新生产验收

- Windows Chrome canvas `9`，业务动作 endpoint/audit Drawer 通过，运行时错误为 `0`。
- 逐路由与真实点击字段质量的 Tab 几何差均为 `0`；业务 ROI：`0.0939867289310064 < 0.125`；证据：`metrics-business-r271.json`、`diff-business-r271.png`、`../data-quality/interaction-r271-storage-quality.png`。

## 基本信息

- id：`data-quality-storage-quality`；路由：`/data-quality?tab=storage-quality`。
- 目标画布 1920 x 1080；业务 ROI 为 `196,64,1716,913`。
- 公共顶部、侧栏和底部只读，不改变其几何。
- 对象为 ClickHouse、OpenSearch、NebulaGraph 和 MinIO。

## 目标图观察

- 七 KPI 为质量分、成功率、P95、失败写入、索引滞后、归档成功率、容量水位。
- 上排是组件健康表、写入趋势、容量面积图。
- 下排是失败写入表、API SVG 写入链路、副本分片对象健康。
- 右栏为异常、快速定位、修复建议、证据报告。
- 四组件使用固定系列色，健康/告警同时保留文字。
- 图表使用动态 ECharts，拓扑保留 API SVG。

## 区域与坐标

| 区域 | bbox | 内容 |
|---|---:|---|
| 画布 | `0,0,1920,1080` | 完整截图 |
| 业务根 | `196,64,1716,913` | ROI |
| 标题 Tab | `197,64,1710,74` | 八 Tab |
| 筛选栏 | `197,145,1710,31` | 时间与刷新 |
| KPI | `197,182,1710,108` | 七指标 |
| 组件健康 | `197,300,646,296` | 四组件表 |
| 写入趋势 | `850,300,397,296` | ECharts line |
| 容量趋势 | `1254,300,429,296` | ECharts area |
| 失败写入 | `197,605,540,340` | 分页表 |
| 写入链路 | `744,605,458,340` | API SVG |
| 健康组合 | `1210,605,473,340` | 副本/索引/分片/对象 |
| 右侧栏 | `1692,300,215,645` | 闭环动作 |

## 图标清单

- 质量分使用盾牌图标。
- 刷新使用 `ReloadOutlined` 并有 loading。
- 组件使用 `DatabaseOutlined` 或既有产品图标。
- 健康使用 `CheckCircleFilled`。
- 写入异常使用 `WarningFilled`。
- 报告使用 `ExportOutlined`。

## 组件清单

- `DataQualityTabs`：统一八 Tab 与 query 同步。
- `StorageComponentHealthTable`：四存储组件健康表。
- `StorageWriteTrend`：四序列速率和 P95 动态 ECharts。
- `StorageCapacityTrend`：七日容量水位动态 ECharts。
- `StorageFailureTable`：失败写入分页表。
- `StoragePipelineFlow`：API 驱动 SVG 写入链路。
- `StorageReplicaHealth`：副本、索引、分片和对象健康。
- `StorageQualitySideRail`：异常、定位、修复和证据动作。

## Token 与样式

- `canvas-bg #020e18`；`panel-bg #031723`；`border #0c4261`。
- `active #168cff` 用于存储质量 Tab 和动作。
- ClickHouse `#2498ff`；OpenSearch `#70c448`。
- NebulaGraph `#f5a20b`；MinIO `#b45ae8`。
- `danger #f0443e` 用于失败写入。
- 正文 12px，标题 15px，圆角 4px，间距 8px。
- 表格、图表和链路面板固定高度。

## 状态与交互

- 点击存储质量 Tab 更新 query，八 Tab 几何不变。
- 自动刷新 30 秒轮询，手动刷新真实 refetch。
- 点击组件详情打开健康 Drawer。
- 写入趋势 tooltip 显示四组件速率、P95 和 800ms 阈值。
- 容量趋势 tooltip 显示日期、容量和 90% 阈值。
- 数据更新后验证 ECharts canvas 重绘。
- 失败写入表必须分页，翻页高度不变。
- 点击链路节点过滤组件、队列或归档状态。
- API SVG 不改成普通图表。
- 右栏按钮必须有 Drawer、API 或下载反馈。
- 看不全时局部滚动，禁止横向溢出。

## 实现映射

| 模块 | Snapshot 字段 | 实现 |
|---|---|---|
| KPI | `storageKpis` | 指标卡 + sparkline |
| 组件表 | `storageComponentRows` | 健康表 |
| 写入趋势 | `storageTrend` | ECharts line |
| 容量趋势 | `storageCapacityTrend` | ECharts area |
| 失败写入 | `storageFailureRows` | 分页表 |
| 写入链路 | `storagePipelineRows` | API SVG |
| 健康组合 | `storageReplica/Index/Partition/ObjectRows` | 表和环图 |
| 右栏 | `storageRail*Rows` | Drawer 与动作 |

## 差异清单

- 目标面积曲线与生产 ECharts 抗锯齿有细微差异。
- 密集表格在 Windows 字体下有字宽差异。
- 公共顶部实时数据不归本 Tab 所有。
- typed fallback 必须标注仿真，不能称为实时存储 API。
- ratio 已通过 0.125，不宣称严格逐像素一致。

## 验收证据

- target：`evidence/ui-image-breakdowns/pages/data-quality-storage-quality/target.png`。
- overlay：同目录 `regions-overlay.png`。
- implementation：同目录 `implementation-r126-final.png`。
- diff / metrics：同目录 `diff-r126.png`、`metrics-r126.json`。
- r126 business ratio `0.11260840213948174`。
- r271 business ratio `0.0939867289310064 < 0.125`。
- runtime 错误为 0，canvas count 9，动作和审计 Drawer 通过。

## 结论

存储质量页已补齐动态图表、API SVG、分页表格、真实动作、字段映射、token 和差异边界，可进入后续全栈开发和智能体复检。
