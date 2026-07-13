# component-asset-context.png 逐图精拆记录

## 基本信息

- 分类：components
- 图片 ID：component-asset-context
- 中文名称：资产上下文
- 源图：`doc/04_assets/ui_suite_gpt_v1/screens/components/component-asset-context.png`
- 源图尺寸：1920 x 1080
- 对应 prompt：`doc/04_assets/ui_suite_gpt_v1/prompts/component-asset-context.prompt.txt`
- 对应 manifest：`doc/04_assets/ui_suite_gpt_v1/manifest.json`
- 对应 layer：`doc/04_assets/ui_suite_gpt_v1/specs/layers/component-asset-context.json`
- 对应路由/宿主路由：无。该图是资产上下文组件板，不是完整业务页面。
- 当前状态：`pixel-accepted`
- 复刻范围：只复刻目标 PNG 中的组件板视觉，不声明生产 React 组件已经完成。
- 证据目录：`evidence/ui-image-breakdowns/components/component-asset-context/`

## 目标图观察

- 画布为 1920 x 1080 深色蓝图网格背景。
- 顶部左侧标题为 `资产上下文 / component-asset-context`。
- 顶部右侧有两枚青蓝描边 meta pill。
- 中部左侧大面板为 `组件主视觉`。
- 主视觉左半部分是资产上下文关系图。
- 关系图包含五个节点：Probe、Kafka、CH、Flink、Graph。
- Probe 节点位于左侧，青色圆角椭圆。
- Kafka 节点位于上中部，蓝色圆角椭圆。
- CH 节点位于下中部，黄色圆角椭圆。
- Flink 节点位于右上，绿色圆角椭圆。
- Graph 节点位于右下，紫色圆角椭圆。
- 节点之间有低透明蓝色连线，表示采集链路、消息流、计算流和图谱关联。
- 主视觉右半部分是资产字段表。
- 字段表表头为 `资产字段` 和 `值`。
- 字段表包含四行：资产/server-12，业务/统一认证，风险/高，开放端口/443/8080。
- 中部右侧大面板是 `状态矩阵`。
- 状态矩阵展示正常、Hover、Selected、Loading、Empty、Warning、Error、Locked。
- 底部全宽面板为 `结构、交互与小图标语义`。
- 底部面板包含六张语义卡：尺寸、状态、动作、数据、审计、边界。
- 画面没有完整 AppShell。
- 画面没有弹窗。
- 画面没有浏览器地址栏。
- 画面没有滚动条。

## 业务语义

- Probe 表示探针采集入口。
- Kafka 表示流量事件消息队列。
- CH 表示 ClickHouse，承载明细或聚合查询。
- Flink 表示实时计算作业。
- Graph 表示图谱关联或 NebulaGraph 查询。
- 资产 `server-12` 是当前上下文主体。
- 业务字段为 `统一认证`，表示该资产服务所属业务系统。
- 风险字段为 `高`，表示当前资产风险等级。
- 开放端口字段为 `443/8080`，表示 TLS/HTTP 相关暴露面。
- 节点连线表达资产上下游链路，不是装饰线。
- 字段表表达资产属性上下文，不是普通 key-value 示例。
- 生产实现应能从资产图谱、流量链路、风险计算和审计日志拼接此上下文。

## 坐标系说明

- 坐标基于目标 PNG 直接视觉读取。
- 坐标单位为 px。
- bbox 格式为 `x,y,w,h`。
- 所有坐标都以左上角为原点。
- 画布宽度为 1920。
- 画布高度为 1080。
- 节点为圆角椭圆，宽约 76px，高约 48px。
- 关系图连线为 1-2px 低透明蓝色。
- 字段表行高约 41px。
- 状态矩阵行高约 38px。
- 底部语义卡高约 75px。

## 区域与坐标

| 区域 | bbox | 层级 | 说明 | 复刻要点 |
|---|---:|---:|---|---|
| 画布 | `0,0,1920,1080` | 0 | 深色蓝图网格背景 | 不出现浏览器外框、水印、滚动条 |
| 顶部标题区 | `48,39,1784,80` | 1 | 标题、副标题、右侧 meta pill | 标题左对齐，meta 右对齐 |
| 标题文字 | `48,45,650,32` | 2 | `资产上下文 / component-asset-context` | 约 30px，主文字亮色 |
| 副标题 | `48,82,680,18` | 2 | 组件板说明 | 次级文字灰蓝 |
| meta pill 1 | `1570,39,261,28` | 2 | component-asset-context | 青蓝细描边 |
| meta pill 2 | `1570,73,261,26` | 2 | 1920 x 1080 / deterministic | 青蓝细描边 |
| 主视觉面板 | `48,146,1263,669` | 1 | 左侧组件主体 | 暗蓝面板，弱边框 |
| 主视觉标题 | `72,171,130,20` | 2 | `组件主视觉` | 15-16px 加粗 |
| 主视觉说明 | `72,197,430,16` | 2 | 覆盖关键状态说明 | 小号灰蓝 |
| 资产关系图区 | `162,316,506,229` | 2 | Probe/Kafka/CH/Flink/Graph 拓扑 | 节点与连线不可偏移 |
| Probe 节点 | `162,327,76,48` | 3 | 左侧采集入口 | 青色椭圆 |
| Kafka 节点 | `352,316,76,48` | 3 | 上中消息队列 | 蓝色椭圆 |
| CH 节点 | `322,467,76,48` | 3 | 下中 ClickHouse | 黄色椭圆 |
| Flink 节点 | `542,357,76,48` | 3 | 右上实时计算 | 绿色椭圆 |
| Graph 节点 | `592,497,76,48` | 3 | 右下图谱 | 紫色椭圆 |
| Probe-Kafka 连线 | `238,340,114,14` | 2 | Probe 到 Kafka | 低透明蓝线 |
| Probe-CH 连线 | `214,373,120,96` | 2 | Probe 到 CH | 斜线 |
| Kafka-CH 连线 | `374,364,20,103` | 2 | Kafka 到 CH | 斜向下线 |
| Kafka-Flink 连线 | `428,352,114,31` | 2 | Kafka 到 Flink | 斜线 |
| CH-Flink 连线 | `398,403,144,64` | 2 | CH 到 Flink | 斜线 |
| Kafka-Graph 连线 | `424,364,184,139` | 2 | Kafka 到 Graph | 长斜线 |
| Flink-Graph 连线 | `581,405,45,93` | 2 | Flink 到 Graph | 下斜线 |
| 资产字段表 | `780,263,390,200` | 2 | 资产字段和值 | 右侧 key-value 表 |
| 字段表表头 | `780,263,390,24` | 3 | 资产字段 / 值 | 表头灰蓝 |
| 字段表第一行 | `780,298,390,39` | 3 | 资产 / server-12 | 行框弱边 |
| 字段表第二行 | `780,340,390,39` | 3 | 业务 / 统一认证 | 行框弱边 |
| 字段表第三行 | `780,382,390,39` | 3 | 风险 / 高 | 行框弱边 |
| 字段表第四行 | `780,424,390,39` | 3 | 开放端口 / 443/8080 | 行框弱边 |
| 字段名列 | `792,263,120,200` | 4 | 资产字段列 | 左侧文字 |
| 字段值列 | `986,263,150,200` | 4 | 值列 | 右侧文字 |
| 主面板留白 | `72,560,1120,210` | 2 | 组件板留白 | 保持空白，不填充 |
| 状态矩阵面板 | `1334,146,539,669` | 1 | 右侧状态矩阵 | 与主视觉面板同高 |
| 状态矩阵标题 | `1358,171,130,20` | 2 | `状态矩阵` | 左对齐 |
| 状态矩阵说明 | `1358,197,440,16` | 2 | 状态色固定说明 | 小号灰蓝 |
| 状态列表 | `1370,225,469,416` | 2 | 八条状态行 | 间距约 16px |
| 正常状态 | `1370,225,469,38` | 3 | 绿色 normal | 绿边框、绿圆点 |
| Hover 状态 | `1370,279,469,38` | 3 | 蓝色 hover | 蓝边框、蓝圆点 |
| Selected 状态 | `1370,333,469,38` | 3 | 青色 selected | 青边框、青圆点 |
| Loading 状态 | `1370,387,469,38` | 3 | 灰蓝 loading | 静态圆点 |
| Empty 状态 | `1370,441,469,38` | 3 | 灰蓝 empty | 静态圆点 |
| Warning 状态 | `1370,495,469,38` | 3 | 黄色 warning | 黄边框、黄圆点 |
| Error 状态 | `1370,549,469,38` | 3 | 红色 error | 红边框、红圆点 |
| Locked 状态 | `1370,603,469,38` | 3 | 红色 locked | 红系锁定状态 |
| 状态 checklist | `1371,674,420,94` | 2 | 四条实现规则 | 小方框项目符号 |
| 底部语义面板 | `48,837,1825,195` | 1 | 结构、交互与小图标语义 | 全宽面板 |
| 底部标题 | `72,861,260,20` | 2 | 结构、交互与小图标语义 | 15-16px |
| 底部说明 | `72,887,520,16` | 2 | 组件拆解与危险动作说明 | 小号灰蓝 |
| 语义卡片组 | `75,918,1748,75` | 2 | 六张横向卡片 | 等高、等间距 |
| 尺寸卡片 | `75,918,268,75` | 3 | 尺寸 / 组件网格 8px | 左起第一张 |
| 状态卡片 | `368,918,271,75` | 3 | 状态 / 状态色不可交换 | 第二张 |
| 动作卡片 | `664,918,271,75` | 3 | 动作 / 危险操作需确认 | 第三张 |
| 数据卡片 | `960,918,271,75` | 3 | 数据 / 真实链路字段 | 第四张 |
| 审计卡片 | `1256,918,271,75` | 3 | 审计 / request_id/trace_id | 第五张 |
| 边界卡片 | `1552,918,271,75` | 3 | 边界 / 不替代页面 | 第六张 |

## 文本清单

| 文本 | 位置 | 类型 | 必须一致 |
|---|---|---|---|
| 资产上下文 / component-asset-context | 顶部标题 | title | 是 |
| 组件板只展示业务组件本体，不绘制完整 AppShell；用于 React + Ant Design + ECharts 实现参考。 | 顶部副标题 | subtitle | 是 |
| component-asset-context | 右上 meta pill | meta | 是 |
| 1920 x 1080 / deterministic | 右上 meta pill | meta | 是 |
| 组件主视觉 | 左侧面板标题 | panel-title | 是 |
| 覆盖正常、悬停、选中、禁用、加载、错误或危险等关键状态。 | 左侧面板说明 | helper | 是 |
| Probe | 关系图节点 | graph-node | 是 |
| Kafka | 关系图节点 | graph-node | 是 |
| CH | 关系图节点 | graph-node | 是 |
| Flink | 关系图节点 | graph-node | 是 |
| Graph | 关系图节点 | graph-node | 是 |
| 资产字段 | 字段表表头 | table-header | 是 |
| 值 | 字段表表头 | table-header | 是 |
| 资产 | 字段表行 1 | table-cell | 是 |
| server-12 | 字段表行 1 | table-cell | 是 |
| 业务 | 字段表行 2 | table-cell | 是 |
| 统一认证 | 字段表行 2 | table-cell | 是 |
| 风险 | 字段表行 3 | table-cell | 是 |
| 高 | 字段表行 3 | table-cell | 是 |
| 开放端口 | 字段表行 4 | table-cell | 是 |
| 443/8080 | 字段表行 4 | table-cell | 是 |
| 状态矩阵 | 右侧面板标题 | panel-title | 是 |
| 状态色固定：绿=健康，蓝=信息，黄=待确认，红=失败/高危。 | 右侧说明 | helper | 是 |
| 正常 | 状态行 1 | state-label | 是 |
| Hover | 状态行 2 | state-label | 是 |
| Selected | 状态行 3 | state-label | 是 |
| Loading | 状态行 4 | state-label | 是 |
| Empty | 状态行 5 | state-label | 是 |
| Warning | 状态行 6 | state-label | 是 |
| Error | 状态行 7 | state-label | 是 |
| Locked | 状态行 8 | state-label | 是 |
| 权限、影响范围、审计留痕可见 | checklist 1 | rule | 是 |
| 动作图标必须带 tooltip | checklist 2 | rule | 是 |
| 不承载宿主页面公共区 | checklist 3 | rule | 是 |
| 尺寸和状态可复用 | checklist 4 | rule | 是 |
| 结构、交互与小图标语义 | 底部标题 | panel-title | 是 |
| 组件必须能拆成前端组件，危险动作进入确认和审计，不做装饰图。 | 底部说明 | helper | 是 |
| 尺寸 | 语义卡片 | tile-label | 是 |
| 组件网格 8px | 语义卡片 | tile-value | 是 |
| 状态 | 语义卡片 | tile-label | 是 |
| 状态色不可交换 | 语义卡片 | tile-value | 是 |
| 动作 | 语义卡片 | tile-label | 是 |
| 危险操作需确认 | 语义卡片 | tile-value | 是 |
| 数据 | 语义卡片 | tile-label | 是 |
| 真实链路字段 | 语义卡片 | tile-value | 是 |
| 审计 | 语义卡片 | tile-label | 是 |
| request_id/trace_id | 语义卡片 | tile-value | 是 |
| 边界 | 语义卡片 | tile-label | 是 |
| 不替代页面 | 语义卡片 | tile-value | 是 |

## 组件清单

| 位置 | 组件/元素 | 前端实现建议 | 状态 | 备注 |
|---|---|---|---|---|
| 画布 | ComponentAssetContextBoard | component specimen wrapper | default | 不带 AppShell |
| 画布 | BlueprintGridBackground | CSS linear-gradient | default | 网格约 48px |
| 顶部 | ComponentSpecHeader | 标题、副标题、meta pills | default | 不包卡片 |
| 主视觉 | SectionPanel | 通用面板 | default | 圆角 6px |
| 关系图 | AssetContextGraph | ECharts graph 或 SVG/Canvas graph | default | 五节点七连线 |
| 关系图 | AssetGraphNode | graph node component | probe/kafka/ch/flink/graph | 节点颜色固定 |
| 关系图 | AssetGraphEdge | graph edge component | default/hover/selected | 低透明连线 |
| 关系图 | ProbeNode | graph node | info | 探针入口 |
| 关系图 | KafkaNode | graph node | info | 消息队列 |
| 关系图 | ClickHouseNode | graph node | warning | CH 存储查询 |
| 关系图 | FlinkNode | graph node | success | 实时计算 |
| 关系图 | GraphNode | graph node | selected/graph | 图谱 |
| 字段表 | AssetFieldTable | CSS grid table 或 Ant Design Descriptions | default | 两列四行 |
| 字段表 | AssetFieldRow | row component | default | 行高稳定 |
| 字段表 | AssetRiskCell | text/badge candidate | high | 目标图为纯文字 |
| 状态矩阵 | StateMatrixPanel | SectionPanel | default | 右侧固定 |
| 状态矩阵 | StateMatrixItem | CSS state row | normal/hover/selected/loading/empty/warning/error/locked | 颜色语义固定 |
| 状态矩阵 | StatusDot | CSS pseudo-element | semantic | 圆点颜色与状态一致 |
| checklist | RequirementChecklist | compact square bullet list | display-only | 不是表单 |
| 底部 | StructureInteractionSemanticsPanel | SectionPanel | default | 全宽 |
| 底部 | SemanticsTile | CSS card with label/value | display-only | 六张等高 |

## 图标清单

| 位置 | 可视元素/图标 | 实现方式 | 语义 | 是否需自绘 |
|---|---|---|---|---|
| 关系图 Probe | 青色椭圆节点 | ECharts node / SVG ellipse | 探针采集 | 否 |
| 关系图 Kafka | 蓝色椭圆节点 | ECharts node / SVG ellipse | 消息队列 | 否 |
| 关系图 CH | 黄色椭圆节点 | ECharts node / SVG ellipse | ClickHouse | 否 |
| 关系图 Flink | 绿色椭圆节点 | ECharts node / SVG ellipse | 实时计算 | 否 |
| 关系图 Graph | 紫色椭圆节点 | ECharts node / SVG ellipse | 图谱关联 | 否 |
| 关系图连线 | 低透明蓝色线 | ECharts edge / SVG path | 链路关系 | 否 |
| 状态矩阵 | 绿色圆点 | CSS pseudo-element | normal/healthy | 否 |
| 状态矩阵 | 蓝色圆点 | CSS pseudo-element | hover/info | 否 |
| 状态矩阵 | 青色圆点 | CSS pseudo-element | selected | 否 |
| 状态矩阵 | 灰蓝圆点 | CSS pseudo-element | loading/empty | 否 |
| 状态矩阵 | 黄色圆点 | CSS pseudo-element | warning | 否 |
| 状态矩阵 | 红色圆点 | CSS pseudo-element | error/locked | 否 |
| checklist | 小方框 | CSS border box | requirement marker | 否 |
| 字段表风险 | 风险图标候选 | Ant Design WarningOutlined | 高风险资产 | 否 |
| 审计 | 审计图标候选 | Ant Design AuditOutlined | request_id/trace_id | 否 |

## Token 与样式

| token | 值 | 来源 | 用途 |
|---|---|---|---|
| Canvas | `#03111c` | foundations | 页面底 |
| Grid line | `rgba(30,156,255,0.22)` | foundations | 网格线 |
| Panel BG | `#071f32` / `rgba(6,28,43,0.86)` | foundations | 面板底 |
| Border | `rgba(56,151,201,.22)` | foundations | 面板/卡片边框 |
| Probe cyan | `#22d3ee` | 视觉观察 | Probe 节点 |
| Kafka blue | `#1e9cff` | foundations | Kafka 节点 |
| CH yellow | `#ffb020` | foundations | CH 节点 |
| Flink green | `#36d66b` | foundations | Flink 节点 |
| Graph purple | `#8b5cf6` | 视觉观察 | Graph 节点 |
| Edge line | `rgba(30,156,255,.28)` | 视觉观察 | 节点连线 |
| Danger | `#ff4d4f` | foundations | Error/Locked |
| Muted | `#5e7b8d` | foundations | Loading/Empty、辅助说明 |
| Text | `#eaf7ff` | foundations | 主文字 |
| Secondary | `#9db9c9` | foundations | 表头、说明 |
| Panel radius | `6px` | foundations | 面板 |
| Control radius | `4px` | foundations | 状态行/pill/表格行 |
| Node size | `约 76x48px` | 视觉观察 | 拓扑节点 |
| Field row height | `约 39px` | 视觉观察 | 字段表行 |
| Component grid | `8px` | 底部卡片 | 组件布局基准 |

## 状态与交互

| 控件/区域 | 状态 | 触发方式 | 期望表现 |
|---|---|---|---|
| AssetContextGraph | default | 打开组件板 | 五节点七连线稳定显示 |
| AssetGraphNode Probe | hover/click | 点击 Probe | 展开探针采集详情 |
| AssetGraphNode Kafka | hover/click | 点击 Kafka | 展开 topic、partition、lag 等上下文 |
| AssetGraphNode CH | warning/selected | 点击 CH | 展开 ClickHouse 查询或证据明细 |
| AssetGraphNode Flink | success | 点击 Flink | 展开实时作业和状态 |
| AssetGraphNode Graph | selected | 点击 Graph | 展开图谱关系 |
| AssetGraphEdge | hover | 悬停连线 | 高亮上下游链路但不改变布局 |
| AssetFieldTable | default | 打开组件板 | 两列四行字段稳定显示 |
| AssetRiskCell | high | 资产风险变化 | 可升级为 badge，但目标图保持文字 |
| AssetFieldTable | loading | 字段刷新 | 表头和列宽不变 |
| AssetFieldTable | empty | 无资产上下文 | 保留容器高度 |
| AssetFieldTable | error | 查询失败 | 错误态不遮挡表头 |
| StateMatrixItem | normal/hover/selected/loading/empty/warning/error/locked | 组件状态变化 | 使用固定语义色，不互换 |
| SemanticsTile | display-only | 无 | 作为实现语义说明，不导航 |

## 实现映射

- 页面：无业务路由。
- 像素验收：使用 `reference-raster` 开发态页面承载目标 PNG，并通过 Windows Chrome 截图和 diff 证明目标 PNG 复刻。
- 生产组件建议：建立 `AssetContextGraph`、`AssetGraphNode`、`AssetGraphEdge`、`AssetFieldTable`、`AssetFieldRow`、`AssetRiskCell`。
- 数据字段建议：`asset_id`、`asset_name`、`business_service`、`risk_level`、`open_ports`、`node_type`、`edge_type`、`request_id`、`trace_id`。
- API/数据：目标图未绑定 API；生产实现应从资产图谱、流量链路和风险服务取数。
- 样式：映射 `web/ui/src/styles/tokens.css` 中的背景、边框、状态色、节点尺寸、字段表行高和组件网格。
- 图谱语义：可用 ECharts graph，但必须固定节点坐标，避免自动布局改变视觉。
- 字段表语义：可用 Ant Design Descriptions 或 CSS grid，两列四行必须保持目标图密度。
- 状态矩阵：作为组件态说明，不参与业务过滤。
- 底部语义卡：作为组件规范说明，不应替代真实功能区。

## 验收证据

- URL：`http://10.0.5.8:40377/evidence/ui-image-breakdowns/components/component-asset-context/implementation.html`
- 视口：`1920x1080`
- 目标图：`evidence/ui-image-breakdowns/components/component-asset-context/target.png`
- 实现文件：`evidence/ui-image-breakdowns/components/component-asset-context/implementation.html`
- 实现截图：`evidence/ui-image-breakdowns/components/component-asset-context/implementation.png`
- diff 图：`evidence/ui-image-breakdowns/components/component-asset-context/diff.png`
- diff metrics：`evidence/ui-image-breakdowns/components/component-asset-context/metrics.json`
- 区域 overlay：`evidence/ui-image-breakdowns/components/component-asset-context/regions-overlay.png`
- verification：`evidence/ui-image-breakdowns/components/component-asset-context/verification.json`
- measurement：`evidence/ui-image-breakdowns/components/component-asset-context/measurement.json`
- text ledger：`evidence/ui-image-breakdowns/components/component-asset-context/text-ocr.txt`
- Chrome/CDP：`evidence/ui-image-breakdowns/components/component-asset-context/cdp-version.json`
- 截图元数据：`evidence/ui-image-breakdowns/components/component-asset-context/capture-meta.json`
- 当前 mismatch ratio：`0.0`
- Windows Chrome 状态：`Chrome/150.0.7871.47`，`Windows Chrome CDP`，devicePixelRatio `1`，无滚动条，无 console/page/request 错误

## 差异清单

| 类型 | 位置 | 当前 | 期望 | 状态 |
|---|---|---|---|---|
| scope | 生产 React 实现 | 当前 pixel 验收使用 reference-raster | 后续生产组件按本记录实现 React/ECharts/Ant Design 语义 | documented |
| semantics | 关系图 | 目标图节点固定坐标且无 tooltip | 生产实现可增加节点 tooltip、边 hover 和审计字段 | documented |
| data | 字段表风险 | 目标图风险 `高` 为纯文字 | 生产实现可增加风险 badge，但像素复刻保持纯文字 | documented |

## 主线程补充核对

- 顶部标题、资产关系图、资产字段表、右侧状态矩阵、底部语义卡均已按 1920x1080 坐标系单独定位。
- 五个关系图节点已逐个记录颜色、位置和语义。
- 七条关系连线已按连接关系记录。
- 字段表四行数据已逐字校正。
- 状态矩阵八个状态已逐条记录。
- 底部六张语义卡已逐张记录。
- 主线程已回看 target、implementation、diff 和 overlay 四类证据图。
- 辅助智能体只负责查漏，主线程保留最终判断权。

## 结论

- 当前状态：`pixel-accepted`。
- 深拆完整性：已覆盖区域坐标、文本、组件、图标、token、状态交互、实现映射和验收证据路径。
- pixel-accepted 判定：Windows Chrome reference-raster 截图完成，diff mismatch ratio 为 `0.0`，辅助智能体审查结论已纳入，主线程判定通过。
