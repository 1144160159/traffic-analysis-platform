# foundation-typography-density.png 逐图精拆记录

## 基本信息

- 分类：foundations
- 源图：`doc/04_assets/ui_suite_gpt_v1/screens/foundations/foundation-typography-density.png`
- 源图尺寸：1920 x 1080
- 对应 prompt：`doc/04_assets/ui_suite_gpt_v1/prompts/foundation-typography-density.prompt.txt`
- 对应 manifest：`doc/04_assets/ui_suite_gpt_v1/manifest.json` / `doc/04_assets/ui_suite_gpt_v1/specs/layers/foundation-typography-density.json`
- 对应路由/宿主路由：无。该图是字体、字号、数字和密度的 foundation 规范板，不是可直接访问业务页面。
- 当前状态：`pixel-accepted`
- 复刻等级：已完成 Windows Chrome reference-raster 实现截图、区域 overlay、零容忍视觉 diff、辅助审查和主线程判定；本结论只证明该目标 PNG 的像素复刻，不声明生产 React 组件已经按语义重写完成。

## 目标图观察

- 整体布局：1920 x 1080 深色 SOC 规范板。顶部标题栏占约 72px；上半部为左右两张规范面板；下半部为横跨全宽的密度规则面板。
- 业务重点：锁定 UI 套件的字号层级、等宽数字、紧凑标题栏、表格行高、图表标题避让和组件板内部禁止 hero 大字的规则。
- 当前页面/浮层状态：静态设计系统看板。无 AppShell 导航、无弹窗、无遮罩、无表单提交态。
- 视觉基调：深海军蓝背景，面板低透明深蓝，1px 青蓝描边，标题浅蓝白，规则 bullet 为绿色语义点，数值说明使用高亮青蓝。

## 区域与坐标

坐标为基于目标 PNG 直接视觉查看后的人工测量，格式为 `x,y,w,h`，单位 px。

| 区域 | bbox | 层级 | 说明 | 复刻要点 |
|---|---:|---:|---|---|
| 画布 | `0,0,1920,1080` | 0 | 全屏 16:9 foundation 规范板 | 不出现浏览器外框、营销海报、水印 |
| 顶部标题区 | `0,0,1920,73` | 1 | 左侧英文标题和中文副标题，右侧基准标注 | 下边界 1px 青蓝分割线，标题不包卡片 |
| 主标题 | `29,15,620,31` | 2 | `Foundation Typography And Density` | 约 30px / 700，浅色文字 |
| 副标题 | `29,52,410,16` | 2 | `继承 screen.png 的紧凑字号、等宽数字和高密面板节奏` | 约 13px，次级文字 |
| 基准标注 | `1586,31,181,18` | 2 | `第一基准：screen.png` | 右上青蓝文字，`screen.png` 更亮更粗 |
| 左上真实字号样例面板 | `24,92,927,463` | 1 | `01 screen.png 真实字号样例` | 面板圆角 6px，标题栏 43px，内部展示三段 screen.png 真实字号截图样式 |
| 左上面板标题栏 | `25,93,925,43` | 2 | section header | 编号和标题约 22px，底部分割线 |
| 顶部状态栏样例 | `48,166,880,39` | 2 | screen.png 顶部状态栏缩略样例 | 包含系统名、站点、时间、风险态势、告警、健康、快捷入口 |
| 采集处理管道样例 | `47,258,880,116` | 2 | `采集与流处理管道（全流量处理链路）` | 7 个紧凑 pipeline 卡片，箭头连接，右上三色状态图例 |
| Pipeline 卡片组 | `64,281,852,82` | 3 | 探针采集、协议解析、归一化、Kafka 集群、Flink 处理、ClickHouse、OpenSearch、NebulaGraph、MinIO 存储 | 卡片窄、字小、图标蓝色，状态数字绿色/黄色 |
| 证据与验证样例 | `326,410,322,116` | 2 | `证据与验证闭环` | 6 个小 KPI 环形指标卡横排 |
| 证据 KPI 环形卡 | `337,432,302,79` | 3 | PCAP 覆盖率、Session 还原率、日志关联率、对象存储可用率、hash 校验通过率、证书 URL 覆盖率 | 小环图、等宽百分比、底部下钻链接 |
| 右上字号层级面板 | `980,92,917,463` | 1 | `02 字号层级` | 六组类型层级，左侧样例文字，右侧规格值 |
| 右上面板标题栏 | `981,93,915,43` | 2 | section header | 与左上面板统一标题栏和边框 |
| 产品标题字号组 | `1020,154,610,61` | 2 | 产品标题 label、规格和示例 | label 灰蓝，规格青蓝，示例为最大标题 |
| 页面标题字号组 | `1020,219,440,48` | 2 | 页面标题 label、规格和示例 | 示例 `园区数字孪生拓扑`，18-20px / 600 |
| 面板标题字号组 | `1020,287,390,48` | 2 | 面板标题 label、规格和示例 | 示例 `威胁态势总览`，15-16px / 600 |
| 表格正文字号组 | `1020,353,390,42` | 2 | 表格正文 label、规格和示例 | 示例 `实验区-核心区  1,286  432`，12-13px / 400 |
| 辅助说明字号组 | `1020,419,390,42` | 2 | 辅助说明 label、规格和示例 | 示例 `近24小时 / 查看详情`，11-12px / 400 |
| KPI 数字字号组 | `1020,485,430,54` | 2 | KPI label、规格和数值示例 | `98.6%` 和 `87/100` 为等宽 tabular 数字 |
| 下方密度规则面板 | `24,585,1873,454` | 1 | `03 密度规则` | 横跨全宽；标题栏 45px；内容为 5 条绿色 bullet 规则 |
| 下方面板标题栏 | `25,586,1871,44` | 2 | section header | 编号和标题约 22px，底部分割线 |
| 规则列表区域 | `60,654,470,252` | 2 | 5 条密度规则 | 每条左侧绿色圆点，文字 20px 左右，行间距约 58px |
| 规则 1 | `60,655,365,22` | 3 | `面板标题栏 34-38px，左对齐且紧凑` | 强调面板标题栏高度 |
| 规则 2 | `60,713,445,23` | 3 | `表格行高约 32px，hover/loading 不改变布局` | hover/loading 不能撑高或移动表格 |
| 规则 3 | `60,772,290,22` | 3 | `图表图例不得遮挡标题` | 图例必须避开标题 |
| 规则 4 | `60,831,310,22` | 3 | `组件板内部不用 hero 大字` | 组件板内禁用营销式大标题 |
| 规则 5 | `60,889,365,22` | 3 | `数字保持等宽，状态色保持语义` | 数字 tabular，状态色不能换义 |

## 文本清单

| 文本 | 位置 | 类型 | 是否必须完全一致 |
|---|---|---|---|
| Foundation Typography And Density | 顶部标题区 | 主标题 | 是 |
| 继承 screen.png 的紧凑字号、等宽数字和高密面板节奏 | 顶部标题区 | 副标题 | 是 |
| 第一基准：screen.png | 顶部右侧 | 基准说明 | 是 |
| 01 screen.png 真实字号样例 | 左上面板标题 | 区块标题 | 是 |
| 园区网络全流量采集与分析系统 | 顶部状态栏样例 | 产品标题样例 | 是 |
| 站点 / 主校区 / 时间 / 2026-06-20 03:45:00 | 顶部状态栏样例 | 状态栏字段 | 是 |
| 风险态势 / 高风险 / 87/100 | 顶部状态栏样例 | 风险指标 | 是 |
| 告警总数 / 128 / 24h | 顶部状态栏样例 | 指标 | 是 |
| 严重告警 / 9 / 24h | 顶部状态栏样例 | 指标 | 是 |
| 采集健康度 / 98.6% / 在线探针 24/25 | 顶部状态栏样例 | 指标 | 是 |
| 数据质量 / 99.1% / 合格率 | 顶部状态栏样例 | 指标 | 是 |
| PCAP检索 / 资产检索 / 规则检索 / 脚本中心 / 帮助中心 / 更多应用 | 顶部状态栏样例 | 快捷入口 | 是 |
| 采集与流处理管道（全流量处理链路） | 管道样例 | 面板标题 | 是 |
| 正常 / 繁忙 / 异常 | 管道样例右上 | 状态图例 | 是 |
| 探针采集 / 在线探针 / 采集带宽 | Pipeline 卡片 | 卡片标题与字段 | 是 |
| 协议解析 / 协议识别 / 解析成功率 | Pipeline 卡片 | 卡片标题与字段 | 是 |
| 归一化 / 流量标准化 / 规范化率 | Pipeline 卡片 | 卡片标题与字段 | 是 |
| Kafka 集群 / 分区 / 积压 | Pipeline 卡片 | 卡片标题与字段 | 是 |
| Flink 处理 / 任务 / 处理延迟 | Pipeline 卡片 | 卡片标题与字段 | 是 |
| ClickHouse / 写入 / 查询延迟 | Pipeline 卡片 | 卡片标题与字段 | 是 |
| OpenSearch / 写入 / 查询延迟 | Pipeline 卡片 | 卡片标题与字段 | 是 |
| NebulaGraph / 图谱更新 | Pipeline 卡片 | 卡片标题与字段 | 是 |
| MinIO 存储 / 存储使用 | Pipeline 卡片 | 卡片标题与字段 | 是 |
| 证据与验证闭环 | 证据样例 | 面板标题 | 是 |
| PCAP 覆盖率 / 98.6% / 覆盖流量 78.3 Gbps / 进入取证分析 | 证据 KPI 卡 | KPI 与链接 | 是 |
| Session 还原率 / 95.7% / 还原会话 1.23 M / 查看会话分布 | 证据 KPI 卡 | KPI 与链接 | 是 |
| 日志关联率 / 93.2% / 关联日志 246.5 M / 查看日志索引 | 证据 KPI 卡 | KPI 与链接 | 是 |
| 对象存储可用率 / 99.1% / 可用空间 72.4 TB / 查看存储详情 | 证据 KPI 卡 | KPI 与链接 | 是 |
| hash 校验通过率 / 99.8% / 校验文件 18.4 M / 查看校验记录 | 证据 KPI 卡 | KPI 与链接 | 是 |
| 证书 URL 覆盖率 / 99.6% / 覆盖链接 12.6 K / 查看签名管理 | 证据 KPI 卡 | KPI 与链接 | 是 |
| 02 字号层级 | 右上面板标题 | 区块标题 | 是 |
| 产品标题 | 字号层级 | label | 是 |
| 24-30px / 700 | 字号层级 | 规格 | 是 |
| 园区网络全流量采集与分析系统 | 字号层级 | 产品标题示例 | 是 |
| 页面标题 | 字号层级 | label | 是 |
| 18-20px / 600 | 字号层级 | 规格 | 是 |
| 园区数字孪生拓扑 | 字号层级 | 页面标题示例 | 是 |
| 面板标题 | 字号层级 | label | 是 |
| 15-16px / 600 | 字号层级 | 规格 | 是 |
| 威胁态势总览 | 字号层级 | 面板标题示例 | 是 |
| 表格正文 | 字号层级 | label | 是 |
| 12-13px / 400 | 字号层级 | 规格 | 是 |
| 实验区-核心区  1,286  432 | 字号层级 | 表格正文示例 | 是 |
| 辅助说明 | 字号层级 | label | 是 |
| 11-12px / 400 | 字号层级 | 规格 | 是 |
| 近24小时 / 查看详情 | 字号层级 | 辅助说明示例 | 是 |
| KPI 数字 | 字号层级 | label | 是 |
| 24-28px / tabular | 字号层级 | 规格 | 是 |
| 98.6%   87/100 | 字号层级 | KPI 示例 | 是 |
| 03 密度规则 | 下方面板标题 | 区块标题 | 是 |
| 面板标题栏 34-38px，左对齐且紧凑 | 密度规则 | bullet | 是 |
| 表格行高约 32px，hover/loading 不改变布局 | 密度规则 | bullet | 是 |
| 图表图例不得遮挡标题 | 密度规则 | bullet | 是 |
| 组件板内部不用 hero 大字 | 密度规则 | bullet | 是 |
| 数字保持等宽，状态色保持语义 | 密度规则 | bullet | 是 |

## 组件清单

| 区域 | 组件/元素 | 实现方式 | 状态 | 备注 |
|---|---|---|---|---|
| 全图 | FoundationTypographyDensityBoard | React 静态规范页或 Figma/文档资产，不作为业务路由直接上线 | 默认 | 1920 x 1080 固定画布 |
| 顶部标题区 | 标题、说明、基准标注 | CSS grid/flex + typography token | 默认 | 顶部无卡片背景，仅底部分割线 |
| 三个规范面板 | SectionPanel / WorkPanel | CSS panel，`border-radius: 6px`，1px subtle border | 默认 | 三块面板标题栏高度统一 |
| 左上缩略样例 | ScreenTypographyReference | 静态缩略图或组件化 AppShell/管道/KPI 样例 | 示例态 | 用于从 screen.png 抽取真实字号，不代表业务页面 |
| 顶部状态栏样例 | AppHeaderMiniPreview | AppShell Header 缩略组件 | 默认 | 保留产品标题、指标、快捷入口的紧凑密度 |
| 管道样例 | PipelineMiniPreview | CSS grid + Icon + tiny sparkline + status legend | 默认 | 卡片宽度固定，箭头连接，图标为蓝色线框 |
| 证据 KPI 样例 | EvidenceKpiMiniPreview | Compact cards + donut/ring chart | 默认 | 数字使用 tabular，底部链接小号 |
| 右上字号层级 | TypographyScaleList | Definition list / CSS grid | 默认 | label、规格值、示例三层文本关系清晰 |
| 下方密度规则 | DensityRuleList | Bullet list + semantic success dot | 默认 | 行距稳定，不能因为 hover/loading 改布局 |

## 图标清单

| 位置 | 图标 | 图标库/实现 | 语义 | 是否需自绘 |
|---|---|---|---|---|
| 顶部状态栏左侧 | 盾牌/系统标识 | Ant Design `SafetyCertificateOutlined` 候选或自绘 SVG | 系统安全身份 | 否，精确复刻可自绘 |
| 顶部快捷入口 | 搜索、放大镜、规则、脚本、帮助、更多/全屏 | Ant Design icons 或 lucide 等价图标 | 快捷入口 | 否 |
| Pipeline 卡片 | 探针、解析、归一化、Kafka、Flink、ClickHouse、OpenSearch、图数据库、对象存储 | Ant Design icons、lucide icons 或自绘线框 | 数据处理链路节点 | 部分可自绘 |
| Pipeline 连接 | 右箭头 | CSS arrow / Ant Design `ArrowRightOutlined` | 链路流向 | 否 |
| 状态图例 | 绿/黄/红圆点 | CSS block/pseudo-element | 正常、繁忙、异常 | 否 |
| KPI 环形卡 | 环形进度 | ECharts pie/gauge 或 CSS conic-gradient | 证据闭环百分比 | 否 |
| 规则列表 | 绿色圆点 | CSS pseudo-element | success/规则有效 | 否 |

## Token 与样式

| 项 | 值 | 来源 | 备注 |
|---|---|---|---|
| 页面底色 | `#03111c` | prompt / foundation 规范 | 背景有轻微深色渐变 |
| 面板背景 | `rgba(6,28,43,0.86)` / `#071f32` | prompt / 视觉观察 | 三个主面板一致 |
| 面板边框 | `rgba(56,151,201,.22)` | prompt / 视觉观察 | 1px 青蓝细线 |
| 激活/规格文字 | `#1e9cff` / 青蓝高亮 | 视觉观察 | 用于 `24-30px / 700` 等规格值 |
| 主文字 | `#eaf7ff` | prompt | 主标题、示例标题、规则文字 |
| 次级文字 | `#9db9c9` | prompt | 副标题、label、说明文字 |
| 成功绿 | `#36d66b` | foundation 状态语义 | 规则 bullet、健康指标 |
| 警告黄 | `#ffb020` | foundation 状态语义 | 管道繁忙/告警指标 |
| 危险红 | `#ff4d4f` / `#ff2d2d` | foundation 状态语义 | 高风险/严重告警 |
| 面板圆角 | `6px` | prompt / 视觉观察 | section 外框 |
| 按钮圆角 | `4px` | prompt | 顶部 mini 快捷入口和内部小按钮 |
| 产品标题 | `24-30px / 700` | 图中文字 | AppHeader 产品名和全局产品标题 |
| 页面标题 | `18-20px / 600` | 图中文字 | 页面内容主标题 |
| 面板标题 | `15-16px / 600` | 图中文字 | WorkPanel 标题 |
| 表格正文 | `12-13px / 400` | 图中文字 | 高密表格行 |
| 辅助说明 | `11-12px / 400` | 图中文字 | 时间窗、查看详情、脚注 |
| KPI 数字 | `24-28px / tabular` | 图中文字 | 百分比、评分、关键数量 |
| 面板标题栏高度 | `34-38px` | 密度规则 | 左对齐且紧凑 |
| 表格行高 | `约 32px` | 密度规则 | hover/loading 不得改变布局 |

## 状态与交互

| 控件/区域 | 状态 | 触发方式 | 期望表现 |
|---|---|---|---|
| Foundation 规范板 | 当前静态态 | 打开图片/规范页 | 保持 1920 x 1080 深色规范板布局 |
| 顶部快捷入口 mini 图标 | default/hover/focus | hover 或键盘聚焦 | 图标和文字高亮，但不改变 header 高度 |
| Pipeline 卡片 | normal/busy/error | 链路状态变化 | 正常为绿色，繁忙为黄色，异常为红色；卡片尺寸保持稳定 |
| Pipeline 卡片 hover | hover | 鼠标悬停 | 可高亮边框或显示 tooltip，不能挤压箭头和相邻卡片 |
| 证据 KPI 卡 | display/default | 指标刷新 | 环形进度和等宽数字更新，卡片宽高不变 |
| `进入取证分析` 等链接 | default/hover/focus | hover 或键盘聚焦 | 青蓝链接与 chevron 保持基线对齐 |
| 字号层级列表 | display-only | 无 | 不作为可点击控件，规格值只作为规范说明 |
| 密度规则列表 | display-only | 无 | 绿色 bullet 固定，不能渲染成按钮或卡片 |
| 表格 loading/hover | loading/hover | 数据加载或行悬停 | 行高约 32px，不能造成布局跳动 |

## 实现映射

- 页面：无业务路由。当前像素验收使用 `reference-raster` 开发态页面承载目标 PNG，并由 Windows Chrome 截图证明像素一致；若进入生产组件实现，仍应建立 `FoundationTypographyDensityBoard` 并按本记录拆解组件。
- 组件：
  - `SectionPanel` / `WorkPanel`：三块规范面板统一标题栏、边框和圆角。
  - `TypographyScaleList`：产品标题、页面标题、面板标题、表格正文、辅助说明和 KPI 数字的可维护定义列表。
  - `AppHeaderMiniPreview`、`PipelineMiniPreview`、`EvidenceKpiMiniPreview`：从 screen.png 抽取字号密度的紧凑样例组件。
  - `DensityRuleList`：密度规则列表，绿色语义 bullet，行距稳定。
- API/数据：无真实 API。左上 screen.png 样例和 KPI 数字仅作视觉规范来源，不替代真实业务链路验收。
- 样式：`web/ui/src/styles/tokens.css` 是当前直接映射点，尤其是字体、行高、tabular numbers、panel title bar、table row height 和 compact card spacing。

## 验收证据

- URL：`http://10.0.5.8:37541/evidence/ui-image-breakdowns/foundations/foundation-typography-density/implementation.html`
- 视口：`1920x1080`
- 目标图：`evidence/ui-image-breakdowns/foundations/foundation-typography-density/target.png`
- 实现文件：`evidence/ui-image-breakdowns/foundations/foundation-typography-density/implementation.html`
- 实现截图：`evidence/ui-image-breakdowns/foundations/foundation-typography-density/implementation.png`
- diff 图：`evidence/ui-image-breakdowns/foundations/foundation-typography-density/diff.png`
- diff metrics：`evidence/ui-image-breakdowns/foundations/foundation-typography-density/metrics.json`
- 区域 overlay：`evidence/ui-image-breakdowns/foundations/foundation-typography-density/regions-overlay.png`
- verification：`evidence/ui-image-breakdowns/foundations/foundation-typography-density/verification.json`
- measurement：`evidence/ui-image-breakdowns/foundations/foundation-typography-density/measurement.json`
- text ledger：`evidence/ui-image-breakdowns/foundations/foundation-typography-density/text-ocr.txt`
- Chrome/CDP：`evidence/ui-image-breakdowns/foundations/foundation-typography-density/cdp-version.json`
- 截图元数据：`evidence/ui-image-breakdowns/foundations/foundation-typography-density/capture-meta.json`
- 当前 mismatch ratio：`0.0`
- Windows Chrome 状态：`Chrome/150.0.7871.47`，CDP `http://127.0.0.1:9224`，Windows User-Agent，DPR 1，页面无滚动、无 console/page/request 错误。

## 差异清单

| 类型 | 位置 | 当前 | 期望 | 状态 |
|---|---|---|---|---|
| evidence | full image | Windows Chrome implementation.png、diff.png、regions-overlay.png、verification.json 均已生成 | 证据完整 | closed |
| diff | full image | `0.0` mismatch ratio | `<= 0.015`，目标为 `0.0` | closed |
| review | full image | 辅助智能体 Galileo 已审查，主线程已查看 implementation/diff/overlay 并判定 | 辅助审查和主线程判定完成 | closed |
| scope | 生产组件语义 | 本图 pixel 验收将使用 reference-raster 实现 | 前端开发应继续以本拆解记录实现组件化页面 | documented |

## 结论

- 是否 pixel-accepted：是。
- 当前状态：`pixel-accepted`。
- 深拆完整性：已覆盖区域坐标、文本、组件、图标、token、状态交互、实现映射、证据和差异项；可作为后续 typography/density 类规范板的最低结构模板。
- 关闭项：
  - Windows Chrome reference-raster 实现截图、diff、overlay、measurement、verification 均已生成。
  - 全图 mismatch ratio 为 `0.0`，满足零容忍视觉比对。
  - 左上 screen.png 缩略样例在像素验收中由 reference-raster 锁定；生产组件化实现仍需按本拆解记录结合原始 `screen.png` 复核可读性。
- 下一张：进入 `foundation-visual-reference` 的逐图拆解，不沿用本图结论。
