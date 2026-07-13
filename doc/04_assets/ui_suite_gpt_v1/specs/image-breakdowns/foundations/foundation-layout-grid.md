# foundation-layout-grid.png 逐图精拆记录

## 基本信息

- 分类：foundations
- 图片 ID：foundation-layout-grid
- 源图：`doc/04_assets/ui_suite_gpt_v1/screens/foundations/foundation-layout-grid.png`
- 源图尺寸：1920 x 1080
- 对应 prompt：`doc/04_assets/ui_suite_gpt_v1/prompts/foundation-layout-grid.prompt.txt`
- 对应 layer：`doc/04_assets/ui_suite_gpt_v1/specs/layers/foundation-layout-grid.json`
- 对应 manifest：`doc/04_assets/ui_suite_gpt_v1/manifest.json`
- 对应路由：无
- 宿主路由：无
- 页面性质：设计系统 foundation 规范板
- 当前阶段：`pixel-accepted`
- 拆解目的：锁定 AppShell 公共区尺寸、内容起点、12 栅格、面板间距和紧凑组件尺寸
- 复刻证据目录：`evidence/ui-image-breakdowns/foundations/foundation-layout-grid/`
- 坐标系统：所有 bbox 均基于 1920 x 1080 目标 PNG
- OCR 方式：人工视觉读取并用局部裁剪校正
- 视觉基准：`screen.png`
- 生产边界：本图是 layout token 和栅格规范来源，不是独立业务路由
- 截图门禁：必须通过 Windows Chrome CDP 截图
- diff 门禁：target 与 implementation 必须生成 `diff.png` 和 `metrics.json`
- 辅助审查：需要智能体审查证据，主线程最终判断
- 验收状态：Windows Chrome 截图、视觉 diff、辅助审查和主线程判定已完成

## 目标图观察

- 整体是深色 SOC 规范板，顶部标题区高度约 73px。
- 主标题为 `Foundation Layout Grid`。
- 副标题写明公共区域实测：顶部 80px、左侧 166px、底部 83px。
- 右上角 `第一基准：screen.png` 表明所有尺寸基于当前态势大屏。
- 主体上方为一个大面板，编号 `01`。
- 大面板标题是 `基于 screen.png 的 AppShell 与内容栅格`。
- 大面板内部嵌入缩放版 `screen.png` 参考图。
- 参考图顶部以亮青色框标注 `Topbar 80`。
- 参考图左侧以蓝色框标注 `Sidebar 166`。
- 参考图内容区以橙色框标注 `Content`。
- 参考图底部以绿色框标注 `Statusbar 83`。
- 参考图内容区叠加 12 条竖向栅格线。
- 栅格线从内容区域顶部贯穿到状态栏上方。
- 栅格编号以蓝色小数字显示。
- 内容区从 x=166 y=80 开始，这是生产尺寸，不是缩略图坐标。
- 内容区生产尺寸为 1754 x 917。
- 底部状态栏生产坐标为 y=997 h=83。
- 左侧栏生产宽度为 166px。
- 顶部栏生产高度为 80px。
- 图中嵌入的 `screen.png` 仍保留完整 AppShell 内容。
- 顶部栏包含产品图标、系统名称、站点、时间、风险态势、告警、采集健康、数据质量和快捷入口。
- 左侧栏是单栏展开式导航，含底部用户区。
- 内容区左侧有综合态势与链路状态卡片。
- 内容区中间是园区数字孪生拓扑。
- 内容区下方是采集与流处理管道、证据与取证闭环、响应与反馈闭环。
- 内容区右侧为威胁态势总览、外联流向强度和运行底座。
- 底部状态栏包含固定状态指标和右侧全局动作图标。
- 主体下方为第二个面板，编号 `02`。
- 第二个面板标题为 `后续组件固定尺寸`。
- 第二个面板用三列文本列出固定尺寸。
- 第一列列出顶部栏、左侧栏、内容起点。
- 第二列列出内容区、底部栏、面板间距。
- 第三列列出面板圆角、按钮圆角、表格行高。
- 重要数值用青色、绿色和橙色强调。
- 本图最关键的不是业务图表，而是布局约束。
- 后续页面如果公共 AppShell 尺寸偏离，应按本图判定失败。
- 后续组件如果面板间距、圆角、表格行高不一致，也应回归本图。

## 区域与坐标

坐标为基于目标 PNG 直接视觉查看和局部裁剪校正后的人工测量，格式为 `x,y,w,h`。

| 区域 | bbox | 层级 | 说明 | 复刻要点 |
|---|---:|---:|---|---|
| 画布 | `0,0,1920,1080` | 0 | 全屏 foundation 规范板 | 固定 16:9 |
| 顶部标题区 | `0,0,1920,73` | 1 | 标题、副标题、基准说明 | 底部一条细分割线 |
| 主面板 | `24,92,1873,793` | 1 | `01 AppShell 与内容栅格` | 大面板占据主体上半屏 |
| 主面板标题栏 | `25,93,1871,43` | 2 | section title | 标题栏高度 43px |
| AppShell 预览 | `318,138,1282,721` | 2 | 嵌入 screen.png | 带彩色测量框和栅格线 |
| Topbar 标注 | `320,139,1278,53` | 3 | 缩略图中的顶部栏框 | 对应生产 80px |
| Sidebar 标注 | `320,192,109,612` | 3 | 缩略图中的左侧栏框 | 对应生产 166px |
| Content 标注 | `429,192,850,612` | 3 | 内容左中区域 | 橙色框和栅格线 |
| 右侧闭环栏 | `1279,192,319,612` | 3 | 内容区右列 | 不是额外 AppShell |
| Statusbar 标注 | `320,805,1278,54` | 3 | 缩略图中的底部栏框 | 对应生产 y=997 h=83 |
| 12 栅格线 | `326,192,1268,613` | 4 | 竖向栅格覆盖 | 数字 1 到 12 |
| 顶部栏内容 | `327,143,1262,44` | 4 | 产品名、指标、快捷入口 | 公共顶部栏基准 |
| 左侧导航内容 | `330,198,92,600` | 4 | 单栏导航样例 | 含用户区 |
| 左列健康卡 | `439,216,167,165` | 4 | 综合态势与链路状态 | 8px 节奏 |
| 主拓扑区域 | `614,198,361,316` | 4 | 园区数字孪生拓扑 | 跨多列大图表 |
| 管道卡片行 | `438,390,834,98` | 4 | 采集与流处理管道 | 等宽卡片 + 8px 间距 |
| 证据闭环区域 | `438,501,479,281` | 4 | 证据与取证闭环 | 环形 KPI 卡 |
| 响应闭环区域 | `924,501,340,281` | 4 | 响应与反馈闭环 | 处置流程卡 |
| 右侧态势总览 | `1285,198,307,337` | 4 | 排行、雷达、地图、表格、donut | 右列高密度 |
| 右侧外联流向 | `1286,537,306,127` | 4 | 世界流向图 | 右列中部 |
| 右侧运行底座 | `1286,676,306,118` | 4 | 性能与渲染指标 | 右列底部 |
| 底部规格面板 | `24,904,1873,145` | 1 | `02 后续组件固定尺寸` | 单独规格表 |
| 规格面板标题栏 | `25,905,1871,43` | 2 | section title | 与主面板一致 |
| 固定尺寸数值表 | `52,952,1319,70` | 2 | 九个尺寸 token | 三列布局 |

## 文本清单

| 序号 | 文本 | 位置 | 类型 | 复刻要求 |
|---:|---|---|---|---|
| 1 | Foundation Layout Grid | 顶部标题区 | 主标题 | 必须完全一致 |
| 2 | 公共区域实测：顶部 80px / 左侧 166px / 底部 83px；内容区按 12 栅格组织 | 顶部标题区 | 副标题 | 必须完全一致 |
| 3 | 第一基准：screen.png | 顶部右侧 | 基准说明 | 必须完全一致 |
| 4 | 01 基于 screen.png 的 AppShell 与内容栅格 | 主面板标题 | 区块标题 | 必须完全一致 |
| 5 | Topbar 80 | 嵌入图顶部 | 测量标注 | 必须完全一致 |
| 6 | Sidebar 166 | 嵌入图左侧 | 测量标注 | 必须完全一致 |
| 7 | Content | 嵌入图内容区 | 测量标注 | 必须完全一致 |
| 8 | Statusbar 83 | 嵌入图底部 | 测量标注 | 必须完全一致 |
| 9 | 1 | 栅格线 | 栅格编号 | 必须保留 |
| 10 | 2 | 栅格线 | 栅格编号 | 必须保留 |
| 11 | 3 | 栅格线 | 栅格编号 | 必须保留 |
| 12 | 4 | 栅格线 | 栅格编号 | 必须保留 |
| 13 | 5 | 栅格线 | 栅格编号 | 必须保留 |
| 14 | 6 | 栅格线 | 栅格编号 | 必须保留 |
| 15 | 7 | 栅格线 | 栅格编号 | 必须保留 |
| 16 | 8 | 栅格线 | 栅格编号 | 必须保留 |
| 17 | 9 | 栅格线 | 栅格编号 | 必须保留 |
| 18 | 10 | 栅格线 | 栅格编号 | 必须保留 |
| 19 | 11 | 栅格线 | 栅格编号 | 必须保留 |
| 20 | 12 | 栅格线 | 栅格编号 | 必须保留 |
| 21 | 园区网络全流量采集与分析系统 | 嵌入顶部栏 | 产品标题 | 必须保持 screen.png 基准 |
| 22 | 站点 主校区 | 嵌入顶部栏 | 指标块 | 必须保持 |
| 23 | 时间 2026-06-20 03:45:00 | 嵌入顶部栏 | 指标块 | 必须保持 |
| 24 | 风险态势 高风险 87/100 | 嵌入顶部栏 | 指标块 | 必须保持 |
| 25 | 告警总数 128 | 嵌入顶部栏 | 指标块 | 必须保持 |
| 26 | 关键告警 9 | 嵌入顶部栏 | 指标块 | 必须保持 |
| 27 | 采集健康 98.6% | 嵌入顶部栏 | 指标块 | 必须保持 |
| 28 | 数据质量 99.1% | 嵌入顶部栏 | 指标块 | 必须保持 |
| 29 | PCAP检索 资产检索 规则检索 脚本中心 帮助中心 更多应用 | 嵌入顶部栏 | 快捷入口 | 必须保持顺序 |
| 30 | 综合态势与链路状态 | 嵌入内容左列 | 面板标题 | 必须保持 |
| 31 | 园区数字孪生拓扑 | 嵌入内容中心 | 面板标题 | 必须保持 |
| 32 | 威胁态势总览 | 右侧栏 | 面板标题 | 必须保持 |
| 33 | 采集与流处理管道（全流量处理链路） | 内容下方 | 面板标题 | 必须保持 |
| 34 | 证据与取证闭环 | 内容下方 | 面板标题 | 必须保持 |
| 35 | 响应与反馈闭环（近24小时） | 内容下方 | 面板标题 | 必须保持 |
| 36 | 外联流向强度（近24小时） | 右侧栏 | 面板标题 | 必须保持 |
| 37 | 运行底座（大屏性能与渲染） | 右侧栏 | 面板标题 | 必须保持 |
| 38 | 数据延迟 1.23s | 底部状态栏 | 状态指标 | 必须保持 |
| 39 | 系统运行 23天14小时 | 底部状态栏 | 状态指标 | 必须保持 |
| 40 | 告警处置SLA 98.2% | 底部状态栏 | 状态指标 | 必须保持 |
| 41 | 数据质量合格率 99.1% | 底部状态栏 | 状态指标 | 必须保持 |
| 42 | 存储使用 68.7/120 TB (57%) | 底部状态栏 | 状态指标 | 必须保持 |
| 43 | 带宽使用 42.7/100 Gbps (43%) | 底部状态栏 | 状态指标 | 必须保持 |
| 44 | 日志吞吐 12.6 K EPS | 底部状态栏 | 状态指标 | 必须保持 |
| 45 | 02 后续组件固定尺寸 | 底部规格面板 | 区块标题 | 必须完全一致 |
| 46 | 顶部栏 80px | 规格表 | 尺寸 token | 必须完全一致 |
| 47 | 左侧栏 166px | 规格表 | 尺寸 token | 必须完全一致 |
| 48 | 内容起点 x=166 y=80 | 规格表 | 尺寸 token | 必须完全一致 |
| 49 | 内容区 1754 x 917 | 规格表 | 尺寸 token | 必须完全一致 |
| 50 | 底部栏 y=997 h=83 | 规格表 | 尺寸 token | 必须完全一致 |
| 51 | 面板间距 8px rhythm | 规格表 | 尺寸 token | 必须完全一致 |
| 52 | 面板圆角 6px | 规格表 | 尺寸 token | 必须完全一致 |
| 53 | 按钮圆角 4px | 规格表 | 尺寸 token | 必须完全一致 |
| 54 | 表格行高 32px compact | 规格表 | 尺寸 token | 必须完全一致 |

## 组件清单

| 组件 | bbox | 类型 | 说明 | 前端映射 |
|---|---:|---|---|---|
| FoundationBoardHeader | `0,0,1920,73` | layout header | 规范板标题栏 | 静态 foundation header |
| LayoutGridMainPanel | `24,92,1873,793` | panel | 主示例面板 | FoundationPanel |
| MainPanelTitlebar | `25,93,1871,43` | titlebar | 01 标题栏 | SectionTitle |
| AnnotatedAppShellPreview | `318,138,1282,721` | reference screenshot | 嵌入 screen.png | reference raster |
| TopbarMeasurementOverlay | `320,139,1278,53` | measurement overlay | Topbar 80 标注 | CSS token topbar-height |
| SidebarMeasurementOverlay | `320,192,109,612` | measurement overlay | Sidebar 166 标注 | CSS token sidebar-width |
| ContentGridOverlay | `429,192,1169,612` | grid overlay | Content 与 12 栅格 | CSS grid |
| StatusbarMeasurementOverlay | `320,805,1278,54` | measurement overlay | Statusbar 83 标注 | CSS token statusbar |
| EmbeddedAppTopbar | `327,143,1262,44` | appshell topbar | 缩放顶部栏 | AppTopbar |
| EmbeddedSidebar | `330,198,92,600` | appshell sidebar | 缩放左侧栏 | AppSidebar |
| EmbeddedMainTopology | `614,198,361,316` | chart panel | 主图表跨列 | ECharts/graph panel |
| EmbeddedPipelineRow | `438,390,834,98` | card row | 管道卡片行 | DataCardRow |
| EmbeddedRightSummaryColumn | `1285,198,307,596` | right rail | 右侧闭环栏 | RightSummaryColumn |
| EmbeddedBottomStatusbar | `320,805,1278,54` | statusbar | 缩放底部栏 | AppStatusbar |
| FixedSizeSpecPanel | `24,904,1873,145` | panel | 规格表面板 | FoundationPanel |
| FixedSizeSpecTable | `52,952,1319,70` | spec table | 九个尺寸 token | SpecGrid |

## 图标清单

| 图标 | bbox | 状态 | 语义 | 复刻要点 |
|---|---:|---|---|---|
| 产品盾牌图标 | `331,147,21,26` | brand | product-identity | 位于顶部栏最左 |
| 风险态势红盾 | `819,166,11,12` | danger | risk | 红色高风险语义 |
| 告警总数铃铛 | `929,166,12,12` | warning | alerts | 黄色告警 |
| 关键告警红色图标 | `1018,166,12,12` | danger | critical-alerts | 红色关键告警 |
| 采集健康绿圆 | `1110,166,12,12` | success | collection-health | 绿色健康 |
| 数据质量绿色数据库 | `1246,166,12,12` | success | data-quality | 绿色合格 |
| PCAP 快捷入口 | `1365,151,13,13` | action | pcap-search | 顶部快捷入口 |
| 资产快捷入口 | `1410,151,13,13` | action | asset-search | 顶部快捷入口 |
| 规则快捷入口 | `1453,151,13,13` | action | rule-search | 顶部快捷入口 |
| 脚本快捷入口 | `1497,151,13,13` | action | script-center | 顶部快捷入口 |
| 帮助快捷入口 | `1533,151,13,13` | action | help-center | 顶部快捷入口 |
| 更多快捷入口 | `1568,151,13,13` | action | more-apps | 顶部快捷入口 |
| 底部通知角标 | `1457,818,24,23` | alert-badge | notification | 位于底部状态栏 |
| 底部设置齿轮 | `1496,823,14,14` | action | settings | 位于底部全局动作组 |
| 底部电源 | `1564,823,14,14` | danger-capable | power-or-logout | 需要危险动作确认链 |

## Token 与样式

| Token | 值 | 用途 | 约束 |
|---|---|---|---|
| canvas-width | `1920px` | 目标画布宽度 | 所有桌面图一致 |
| canvas-height | `1080px` | 目标画布高度 | 所有桌面图一致 |
| topbar-height | `80px` | 生产顶部栏高度 | 不允许随页面变化 |
| sidebar-width | `166px` | 生产左侧栏宽度 | 单栏展开式 |
| statusbar-y | `997px` | 生产底部栏 y 坐标 | 必须对齐底部 |
| statusbar-height | `83px` | 生产底部栏高度 | 单层状态栏 |
| content-origin | `x=166 y=80` | 内容区起点 | 不覆盖公共区 |
| content-size | `1754 x 917` | 内容区可用范围 | 不含顶部/左侧/底部 |
| grid-columns | `12` | 内容栅格列数 | 桌面端基准 |
| panel-gap | `8px rhythm` | 面板间距 | 所有业务区一致 |
| panel-radius | `6px` | 面板圆角 | 避免大圆角卡片 |
| button-radius | `4px` | 按钮圆角 | 紧凑按钮 |
| table-row-height | `32px compact` | 表格行高 | 高密度表格 |
| page-bg | `#03111c` | 页面底色 | foundation 背景 |
| panel-bg | `#071f32` | 面板底色 | 低饱和深蓝 |
| border-weak | `rgba(56,151,201,.22)` | 边框/分割线 | 细线 |
| active-blue | `#1e9cff` | 激活态和栅格数字 | 不替代状态色 |
| success-green | `#36d66b` | 底部栏和健康状态 | 只用于健康/通过 |

## 状态与交互

- Topbar 是固定公共区，不随业务页面调整高度。
- Sidebar 是固定公共区，不随业务页面调整宽度。
- Statusbar 是固定公共区，不随业务页面拆成两层。
- Content 从 `x=166 y=80` 开始，不能压到顶部栏或侧栏。
- Content 高度止于底部状态栏上方。
- 内容区按 12 栅格组织。
- 右侧闭环栏属于内容栅格，不是第二套 AppShell。
- 面板间距使用 8px rhythm。
- 面板圆角使用 6px。
- 按钮圆角使用 4px。
- 表格行高使用 32px compact。
- 顶部快捷入口固定为 PCAP检索、资产检索、规则检索、脚本中心、帮助中心、更多应用。
- 通知、设置、电源仍归属底部状态栏右侧。
- 生产实现的 resize 不能改变公共区 token。
- 若移动端/平板端另有响应式规范，应以 responsive foundation 单独处理。
- 本图只锁定桌面 1920x1080 AppShell。
- 业务面板可以换内容，但不得破坏公共区尺寸。
- 大图表可跨多列，但仍需贴齐栅格线。
- 卡片行必须等高、等宽或按栅格跨度计算。
- 右侧栏不能遮挡主拓扑，也不能突破内容区边界。
- 底部规格表中的数值优先级高于临时页面样式。

## 实现映射

- `AppShell` 使用固定 1920x1080 桌面基准。
- `AppTopbar` 高度固定为 80px。
- `AppSidebar` 宽度固定为 166px。
- `AppStatusbar` 固定为 y=997 h=83。
- `MainContent` 使用 `left:166px; top:80px; width:1754px; height:917px` 的基准关系。
- `ContentGrid` 应实现 12 列桌面布局。
- `FoundationPanel` 应使用 6px 圆角和弱青蓝边框。
- `PanelGap` 应使用 8px rhythm。
- `CompactTable` 应使用 32px 行高。
- `Button` 应使用 4px 圆角。
- `RightSummaryColumn` 应在内容区内按栅格定位。
- `GlobalActionIcons` 应固定在底部状态栏右侧。
- `TopQuickEntries` 应固定在顶部栏右侧。
- `SidebarUserZone` 应固定在左侧栏底部。
- 对响应式页面，不得把桌面公共区尺寸误套到移动端；应转到 responsive foundation。
- 对浮层/弹窗，公共区可作为背景上下文，但弹窗本身不改变 AppShell 尺寸。
- 对页面截图复刻，先复用本图尺寸 token，再处理业务内容。

## 验收证据

- 源图：`doc/04_assets/ui_suite_gpt_v1/screens/foundations/foundation-layout-grid.png`
- target：`evidence/ui-image-breakdowns/foundations/foundation-layout-grid/target.png`
- implementation：`evidence/ui-image-breakdowns/foundations/foundation-layout-grid/implementation.png`
- diff：`evidence/ui-image-breakdowns/foundations/foundation-layout-grid/diff.png`
- regions overlay：`evidence/ui-image-breakdowns/foundations/foundation-layout-grid/regions-overlay.png`
- measurement：`evidence/ui-image-breakdowns/foundations/foundation-layout-grid/measurement.json`
- text ledger：`evidence/ui-image-breakdowns/foundations/foundation-layout-grid/text-ocr.txt`
- metrics：`evidence/ui-image-breakdowns/foundations/foundation-layout-grid/metrics.json`
- capture metadata：`evidence/ui-image-breakdowns/foundations/foundation-layout-grid/capture-meta.json`
- verification：`evidence/ui-image-breakdowns/foundations/foundation-layout-grid/verification.json`
- 浏览器要求：Windows Chrome CDP `http://127.0.0.1:9224`
- 截图要求：1920 x 1080，DPR 1
- diff 要求：对 target 与 implementation 生成 `diff.png`
- 审查要求：辅助智能体检查证据，主线程最终判断
- 证据边界：reference-raster 证明目标 PNG 复刻，生产 CSS/React 实现仍需按本记录落地

## 差异清单

| 类型 | 位置 | 当前记录 | 验收要求 | 状态 |
|---|---|---|---|---|
| 视觉截图 | 全图 | Windows Chrome 截图由像素门禁生成 | 必须有 implementation.png | closed |
| 视觉差异 | 全图 | diff 由像素门禁生成 | mismatch ratio 达到门禁 | closed |
| 区域覆盖 | 全图 | JSON 已记录 24 个区域 | overlay 覆盖标题、主面板、AppShell、规格表 | closed |
| 文本校正 | 全图 | 已人工校正 54 条文本 | text ledger 同步记录 | closed |
| 栅格线 | 内容区 | 已记录 12 栅格 | 不得漏掉编号和纵向线 | closed |
| 尺寸 token | 底部规格表 | 已记录 9 个固定尺寸 | 后续页面必须继承 | closed |
| 生产边界 | React 实现 | reference-raster 只证明像素复刻 | 语义实现按本记录落地 | documented |

## 结论

- `foundation-layout-grid.png` 是桌面 AppShell 布局和栅格规范板。
- 它锁定了 topbar 80px、sidebar 166px、statusbar y=997 h=83。
- 它锁定了内容起点 `x=166 y=80`。
- 它锁定了内容区 `1754 x 917`。
- 它锁定了 12 栅格组织方式。
- 它锁定了 8px 面板间距、6px 面板圆角、4px 按钮圆角和 32px 表格行高。
- 它把右侧闭环栏定义为内容栅格的一部分，而不是第二套导航。
- 它把通知、设置、电源动作固定在底部状态栏右侧。
- 本记录已完成区域、文本、组件、图标、token、交互和实现映射拆解。
- Windows Chrome 截图、视觉 diff、辅助审查和主线程判定已经完成。
