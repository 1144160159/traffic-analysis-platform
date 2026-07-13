# foundation-data-viz.png 逐图精拆记录

## 基本信息

- 分类：foundations
- 源图：`doc/04_assets/ui_suite_gpt_v1/screens/foundations/foundation-data-viz.png`
- 源图尺寸：1920 x 1080
- 对应 prompt：`doc/04_assets/ui_suite_gpt_v1/prompts/foundation-data-viz.prompt.txt`
- 对应 manifest：`doc/04_assets/ui_suite_gpt_v1/specs/layers/foundation-data-viz.json`
- 对应路由/宿主路由：无。该图是数据可视化规范板，不是业务路由。
- 当前状态：`pixel-accepted`
- 复刻等级：已完成逐图视觉读取、坐标测量、文本校正、组件/图标/token/交互拆解、Windows Chrome evidence、零容忍 diff、辅助审查和主线程最终判断。

## 目标图观察

- 整体布局：1920 x 1080 深色 foundation board。顶部是标题栏，主体是 2 x 2 四宫格面板：左上展示拓扑/图谱画布，右上展示威胁总览/雷达/地图，左下展示采集管道/迷你趋势，右下展示证据与响应环。
- 业务重点：锁定后续 ECharts/Canvas/SVG 的数据可视化风格。蓝色线路表示低透明网格和链路，黄色/橙色表示中危或告警，红色只用于风险，高密度小图必须保持可读。
- 当前页面/浮层状态：静态规范板。无 AppShell 导航、无业务交互、无弹窗、无抽屉、无表单提交态。
- 视觉基调：深海军蓝背景，四个面板统一 1px 青蓝边框和 6px 圆角。内嵌图均来自当前态势大屏的可视化样例，保留微缩后的密度和发光。
- 特殊点：每个面板都有底部 cyan 注释。注释是规则，不是按钮；不能被误作链接或导航。右下证据环图整体被放大并加暗化效果，强调圆环必须有状态和入口。

## 区域与坐标

坐标为基于目标 PNG 直接视觉查看后的人工测量，格式为 `x,y,w,h`，单位 px。

| 区域 | bbox | 层级 | 说明 | 复刻要点 |
|---|---:|---:|---|---|
| 画布 | `0,0,1920,1080` | 0 | 全屏 16:9 数据可视化规范板 | 不出现浏览器边框、水印、滚动条 |
| 顶部标题区 | `0,0,1920,73` | 1 | 标题、副标题、右侧基准标注 | 与其它 foundation 统一 |
| 主标题 | `29,15,534,31` | 2 | `Foundation Data Visualization` | 约 30px 粗体 |
| 副标题 | `29,53,440,14` | 2 | 图表风格说明 | 次级文字，不能变成营销文案 |
| 基准标注 | `1586,31,181,18` | 2 | `第一基准：screen.png` | 右上 cyan 标注 |
| 拓扑图谱面板 | `24,92,1007,464` | 1 | `01 拓扑 / 图谱画布` | 左上大面板，承载最大拓扑画布 |
| 拓扑面板标题栏 | `25,93,1005,43` | 2 | section header | 标题左对齐，底部细线 |
| 拓扑示例图 | `161,151,733,357` | 2 | 园区数字孪生拓扑 | 中心核心区、楼宇、探针、链路、罗盘 |
| 拓扑标题 | `170,154,120,14` | 3 | `园区数字孪生拓扑` | 微缩图内标题 |
| 拓扑顶部模式控件 | `786,151,96,17` | 3 | `2D / 3D / fullscreen` | 3D 选中，右上贴边 |
| 拓扑底部异常图例 | `164,468,452,27` | 3 | 异常链路位置和三条异常链路说明 | 红橙黄图例，不能丢 |
| 拓扑说明 | `82,525,186,15` | 2 | `克制发光，线条保持可读` | cyan 说明文字 |
| 威胁总览面板 | `1058,92,839,464` | 1 | `02 威胁总览 / 雷达 / 地图` | 右上面板，内嵌小态势总览 |
| 威胁面板标题栏 | `1059,93,837,43` | 2 | section header | 标题为 02 |
| 威胁总览缩略图 | `1361,152,232,356` | 2 | 威胁态势总览小仪表盘 | 四段图表：条形、雷达、地图、表格环图、外联流向 |
| 攻击阶段热度 | `1370,189,90,58` | 3 | 红橙黄绿水平条 | 风险色只表达风险 |
| 战役热度雷达 | `1461,189,126,58` | 3 | 雷达/散点图 | 高中低三色点 |
| 风险区域地图 | `1370,259,160,56` | 3 | 世界热力地图 | 红色只表达风险 |
| 异常影响表格环图 | `1370,325,216,76` | 3 | Top 5 表格 + donut | 右侧环图保留中心数值 |
| 外联流向地图 | `1370,414,215,78` | 3 | 世界流向弧线和速率表 | 蓝黄弧线，右侧 Gbps |
| 威胁说明 | `1116,525,145,15` | 2 | `风险色只表达风险` | cyan 说明文字 |
| 采集管道面板 | `24,585,1007,453` | 1 | `03 采集管道 / 迷你趋势` | 左下大面板，横向处理链路 |
| 采集面板标题栏 | `25,586,1005,43` | 2 | section header | 标题为 03 |
| 管道缩略图 | `43,754,970,128` | 2 | 采集与流处理管道 | 探针、协议、归一化、Kafka、Flink、ClickHouse、OpenSearch、NebulaGraph、MinIO |
| 管道图例 | `860,763,137,14` | 3 | 正常/繁忙/异常 | 绿/橙/红状态点 |
| 管道节点组 | `59,778,920,94` | 3 | 九个流程卡片和箭头 | 每张卡含图标、指标、迷你趋势 |
| 管道说明 | `82,1011,153,15` | 2 | `迷你图适配高密卡片` | cyan 说明文字 |
| 证据响应面板 | `1058,585,839,453` | 1 | `04 证据与响应环` | 右下面板，重点展示圆环状态 |
| 证据面板标题栏 | `1059,586,837,43` | 2 | section header | 标题为 04 |
| 证据环缩略图 | `1226,688,503,261` | 2 | 证据与取证闭环 | 六个圆环指标、入口按钮、局部放大暗化 |
| PCAP 覆盖率环 | `1245,732,62,82` | 3 | `98.6%` 环图 | 绿色圆环，含数据量 |
| Session 还原率环 | `1328,732,62,82` | 3 | `95.7%` 环图 | 绿色圆环 |
| 日志关联率环 | `1412,732,62,82` | 3 | `93.2%` 环图 | 绿色圆环，中心有红盾图标 |
| 对象存储归档环 | `1496,732,62,82` | 3 | `99.1%` 环图 | 绿色/蓝色圆环 |
| hash 快速追踪环 | `1580,732,62,82` | 3 | `99.8%` 环图 | 绿色圆环 |
| 签名 URL 可用率环 | `1662,732,62,82` | 3 | `99.6%` 环图 | 绿色圆环 |
| 证据入口行 | `1240,854,482,38` | 3 | 进入取证、学习、日志、校验、追踪等入口 | 每个入口都要保留可点击视觉 |
| 证据说明 | `1116,1011,169,15` | 2 | `圆环必须有状态与入口` | cyan 说明文字 |

## 文本清单

| 文本 | 位置 | 类型 | 是否必须完全一致 |
|---|---|---|---|
| Foundation Data Visualization | 顶部标题区 | 主标题 | 是 |
| 图表风格来自态势大屏：低透明网格、蓝青主色、红黄只用于风险 | 顶部标题区 | 副标题 | 是 |
| 第一基准：screen.png | 顶部右侧 | 基准说明 | 是 |
| 01 拓扑 / 图谱画布 | 左上面板标题 | 区块标题 | 是 |
| 园区数字孪生拓扑 | 拓扑图内 | 图标题 | 是 |
| 核心链路 / 汇聚链路 / 异常链路 / 探针位置 | 拓扑图例 | 图例 | 是 |
| 2D / 3D | 拓扑右上 | 模式控件 | 是 |
| 核心区 | 拓扑中心 | 节点标签 | 是 |
| 数学区 / 图书馆 / 实验楼 / 办公区 / 宿舍区 / 数据中心 / 汇聚区B | 拓扑图内 | 节点标签 | 是 |
| 异常链路位置 | 拓扑底部 | 图例标题 | 是 |
| 教学区-核心区链路 / 宿舍区-汇聚区B链路 / 体育馆-汇聚区A链路 | 拓扑底部 | 图例文本 | 是 |
| 进入拓扑详情 | 拓扑右下 | 入口文本 | 是 |
| 克制发光，线条保持可读 | 左上面板底部 | 规则说明 | 是 |
| 02 威胁总览 / 雷达 / 地图 | 右上面板标题 | 区块标题 | 是 |
| 威胁态势总览 | 威胁缩略图 | 图表组标题 | 是 |
| 近24小时 | 威胁缩略图右上 | 时间窗 | 是 |
| 攻击阶段热度 | 威胁缩略图 | 图标题 | 是 |
| 战役维度盘 | 威胁缩略图 | 图标题 | 是 |
| 风险区域密度 | 威胁缩略图 | 图标题 | 是 |
| 异常链路影响面（Top 5） | 威胁缩略图 | 图标题 | 是 |
| 外联流向强度（近24小时） | 威胁缩略图 | 图标题 | 是 |
| 风险色只表达风险 | 右上面板底部 | 规则说明 | 是 |
| 03 采集管道 / 迷你趋势 | 左下面板标题 | 区块标题 | 是 |
| 采集与流处理管道（全流量处理链路） | 管道缩略图 | 图标题 | 是 |
| 正常 / 繁忙 / 异常 | 管道缩略图右上 | 状态图例 | 是 |
| 探针采集 / 协议解析 / 归一化 / Kafka 集群 / Flink 处理 / ClickHouse / OpenSearch / NebulaGraph / MinIO 存储 | 管道节点 | 流程节点 | 是 |
| 在线探针 24/25 / 协议识别 58种 / 流量标准化 / 分区 48/48 / 任务 58 / 写入 78.3Gbps / 写入 12.6K EPS / 图谱更新 2.1K/s / 存储使用 72.4TB/120TB | 管道节点 | 指标文本 | 是 |
| 迷你图适配高密卡片 | 左下面板底部 | 规则说明 | 是 |
| 04 证据与响应环 | 右下面板标题 | 区块标题 | 是 |
| 证据与取证闭环（近24小时） | 证据缩略图 | 图标题 | 是 |
| PCAP 覆盖率 / Session 还原率 / 日志关联率 / 对象存储归档 / hash 快速追踪 / 签名 URL 可用率 | 证据环图 | 指标标题 | 是 |
| 98.6% / 95.7% / 93.2% / 99.1% / 99.8% / 99.6% | 证据环图 | 指标数值 | 是 |
| 进入取证分析 / 查看日志详情 / 查看校验管理 / 查看签名管理 | 证据缩略图 | 入口文本 | 是 |
| 圆环必须有状态与入口 | 右下面板底部 | 规则说明 | 是 |

## 组件清单

| 区域 | 组件/元素 | 实现方式 | 状态 | 备注 |
|---|---|---|---|---|
| 全图 | FoundationDataVizBoard | React 静态规范页或文档资产 | 默认 | 1920 x 1080 固定画布 |
| 顶部标题区 | SpecHeader | CSS grid/flex + typography token | 默认 | 与其它 foundation header 对齐 |
| 四个规范面板 | SectionPanel | WorkPanel 外框 + 标题栏 | 默认 | 6px 圆角、1px cyan border |
| 拓扑图谱 | TopologyGraphPreview | ECharts graph/lines 或 Canvas/SVG/raster reference | 示例态 | 蓝、绿、橙三类链路 |
| 威胁总览 | ThreatDashboardPreview | 多图表组合 | 示例态 | bar/radar/map/table/donut/flow map |
| 采集管道 | PipelineMiniTrend | 横向流程卡 + sparkline | 示例态 | 卡片高密度，箭头连接 |
| 证据响应环 | EvidenceResponseRing | 多个 ring progress + action entries | 示例态 | 圆环必须配入口 |
| 规则说明 | CaptionText | cyan caption | 静态 | 四个面板底部说明 |
| 状态图例 | StatusLegend | CSS dot + label | 默认 | 正常/繁忙/异常 |
| 入口链接 | TextLink | Ant Design `Button type=link` 或 anchor | default/hover/focus | 入口只存在于缩略图内 |

## 图标清单

| 位置 | 图标 | 图标库/实现 | 语义 | 是否需自绘 |
|---|---|---|---|---|
| 拓扑中心 | 六边形安全节点 | Ant Design `SafetyCertificateOutlined` 或自绘 SVG | 核心区 | 否 |
| 拓扑探针 | 绿色 pin/圆点 | ECharts symbol 或自绘 SVG | 探针位置 | 是 |
| 拓扑罗盘 | N/W/E/S compass | 自绘 SVG/Canvas | 方位辅助 | 是 |
| 拓扑全屏 | Fullscreen icon | Ant Design `FullscreenOutlined` | 放大查看 | 否 |
| 管道节点 | shield/search/flow/network/storage 图标 | Ant Design Icons + 自绘业务线图标 | 链路阶段 | 否 |
| 管道箭头 | right arrow | CSS/SVG arrow | 处理流向 | 否 |
| 状态图例 | 三色状态点 | CSS circle | 正常/繁忙/异常 | 否 |
| 证据圆环中心 | shield/hash/link icon | Ant Design Icons 或 CSS symbol | 证据类型 | 否 |
| 入口链接 | chevron right | Ant Design `RightOutlined` | 下钻入口 | 否 |

## Token 与样式

| 项 | 值 | 来源 | 备注 |
|---|---|---|---|
| 页面底色 | `#03111c` | foundation token | 全局背景 |
| 面板底色 | `#071f32` / rgba 深蓝 | 视觉观察 | 四个面板一致 |
| 面板边框 | `rgba(56,151,201,.22)` | foundation token | 1px cyan |
| 图表网格线 | 低透明 cyan | prompt / 视觉观察 | 不抢标题 |
| 拓扑主链路蓝 | `#1e9cff` | active token | 核心链路和发光 |
| 探针/健康绿 | `#36d66b` | success token | 探针和正常状态 |
| 中危/繁忙橙 | `#ffb020` | warning token | 繁忙和中危 |
| 风险红 | `#ff4d4f` / `#ff2d2d` | danger/critical token | 只用于风险 |
| 主文字 | `#eaf7ff` | text token | 标题和关键数字 |
| 次级文字 | `#9db9c9` | secondary token | 图例和说明 |
| caption cyan | `#1e9cff` | active token | 四个面板底部说明 |
| 面板圆角 | `6px` | foundation token | 所有 section |
| 图表容器圆角 | `4px` | 视觉观察 | 内嵌图边框 |
| 字体密度 | 11-16px | prompt | 微缩图必须可读 |
| sparkline 高度 | 8-16px | 视觉观察 | 管道卡片内迷你趋势 |

## 状态与交互

| 控件/区域 | 状态 | 触发方式 | 期望表现 |
|---|---|---|---|
| Foundation data viz board | static/reference | 打开图片或规范页 | 固定 1920 x 1080，不随内容滚动 |
| 拓扑画布 | reference-only | 无 | 保持克制发光和链路可读，不作为真实拓扑交互 |
| 2D/3D 控件 | selected sample | 视觉示例 | 3D 为选中态，2D 非激活 |
| 威胁总览图表 | reference-only | 无 | 风险色只表达风险，不混用成功色 |
| 管道节点卡 | normal/busy/error sample | 数据状态变化 | 绿/橙/红分别表达正常、繁忙、异常 |
| 证据圆环 | progress/state sample | 数据状态变化 | 圆环必须同时有状态、数值和入口 |
| 入口链接 | default/hover/focus | hover 或键盘聚焦 | cyan 链接和 chevron 高亮，不改变布局 |
| 后续图表实现 | loading/empty/error/offline | 业务页面实现阶段 | 必须保留稳定容器高度和清晰状态说明 |

## 实现映射

- 页面：无业务路由。可在开发态建立 `FoundationDataVizBoard`，用于设计系统文档和 ECharts theme 验收。
- 组件：
  - `TopologyGraphPreview`：拓扑/图谱图表基准。
  - `ThreatDashboardPreview`：条形图、雷达、地图、表格、环图、流向图组合基准。
  - `PipelineMiniTrend`：采集管道卡片和 sparkline 基准。
  - `EvidenceResponseRing`：证据闭环圆环和入口基准。
  - `StatusLegend`、`CaptionText`、`TextLink`：辅助组件。
- API/数据：无真实 API。所有数值来自目标图静态样例，不触发接口。
- 样式：沿用 `foundation-color-status` 的 token，并为 ECharts/Canvas/SVG 规定低透明网格、克制发光和风险色使用边界。
- 复刻方式：像素验收可使用 reference-raster；生产组件实现仍必须按本记录拆解图表和状态。

## 图表落地约束

- 拓扑图谱：节点标签不能遮挡主链路，探针 marker 使用成功绿，异常链路只使用橙/红。
- 雷达/散点：点位半径保持小尺寸，图例必须贴近图表右侧，不能遮挡面板标题。
- 风险地图：红色热区仅表示风险密度，不得用于健康、通过或普通流量。
- 流向地图：弧线必须保留方向感和低透明尾迹，右侧 Gbps 表格保持右对齐。
- 管道卡片：每个节点卡必须包含图标、阶段名称、关键指标、状态文字和 sparkline。
- 管道箭头：箭头只表示处理方向，不能被误作可点击按钮。
- 圆环图：每个圆环都必须显示指标名、百分比、状态图标和下钻入口。
- 证据入口：入口视觉可以轻量，但必须能映射到取证分析、日志详情、校验管理或签名管理。
- Loading/empty/error：真实业务页面落地时，图表容器高度必须稳定，不能因状态切换造成布局跳动。
- 无权限/offline/degraded：状态色仍遵守 foundation-color-status 的语义，不得用红色表达普通信息。

## 验收证据

- URL：`http://10.0.5.8:46777/evidence/ui-image-breakdowns/foundations/foundation-data-viz/implementation.html`
- 视口：`1920x1080`
- 目标图：`evidence/ui-image-breakdowns/foundations/foundation-data-viz/target.png`
- 实现文件：`evidence/ui-image-breakdowns/foundations/foundation-data-viz/implementation.html`
- 实现截图：`evidence/ui-image-breakdowns/foundations/foundation-data-viz/implementation.png`
- diff 图：`evidence/ui-image-breakdowns/foundations/foundation-data-viz/diff.png`
- 区域 overlay：`evidence/ui-image-breakdowns/foundations/foundation-data-viz/regions-overlay.png`
- verification：`evidence/ui-image-breakdowns/foundations/foundation-data-viz/verification.json`
- measurement：`evidence/ui-image-breakdowns/foundations/foundation-data-viz/measurement.json`
- text ledger：`evidence/ui-image-breakdowns/foundations/foundation-data-viz/text-ocr.txt`
- Chrome/CDP：必须来自 Windows Chrome CDP `http://127.0.0.1:9224`。
- 当前 mismatch ratio：`0.0`
- Windows Chrome 状态：`Chrome/150.0.7871.47`，DPR 1，页面无滚动、无 console/page/request 错误。

## 差异清单

| 类型 | 位置 | 当前 | 期望 | 状态 |
|---|---|---|---|---|
| evidence | full image | Windows Chrome implementation/diff/overlay/verification 已生成 | 必须生成 implementation、diff、overlay、verification | closed |
| diff | full image | `0.0` mismatch ratio | `<= 0.015` | closed |
| scope | production chart implementation | 本图是规范板，不是业务路由 | 组件语义按本记录实现，像素证据可 reference-raster | documented |

## 结论

- 是否 pixel-accepted：是。
- 当前状态：`pixel-accepted`。
- 已完成：直接视觉读取、坐标测量、文本校正、组件/图标/token/交互拆解。
- 已关闭：Windows Chrome screenshot、视觉 diff、区域 overlay、verification、辅助智能体审查和主线程判定。
- 范围说明：该结论证明目标 PNG 像素复刻准确；生产 ECharts/Canvas/SVG 组件语义实现仍需依据本拆解记录落地。
