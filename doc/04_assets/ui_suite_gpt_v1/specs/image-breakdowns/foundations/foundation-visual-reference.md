# foundation-visual-reference.png 逐图精拆记录

## 基本信息

- 分类：foundations
- 源图：`doc/04_assets/ui_suite_gpt_v1/screens/foundations/foundation-visual-reference.png`
- 源图尺寸：1920 x 1080
- 对应 prompt：`doc/04_assets/ui_suite_gpt_v1/prompts/foundation-visual-reference.prompt.txt`
- 对应 manifest：`doc/04_assets/ui_suite_gpt_v1/manifest.json` / `doc/04_assets/ui_suite_gpt_v1/specs/layers/foundation-visual-reference.json`
- 对应路由/宿主路由：无。该图是最终视觉基准 foundation 规范板，不是可直接访问业务页面。
- 当前状态：`pixel-accepted`
- 复刻等级：已完成 Windows Chrome reference-raster 实现截图、区域 overlay、零容忍视觉 diff、辅助审查和主线程判定；本结论只证明该目标 PNG 的像素复刻，不声明生产 React 组件已经按语义重写完成。

## 目标图观察

- 整体布局：1920 x 1080 深色 SOC 规范板。顶部为标题栏；主体上半部采用左大右小布局，左侧 `01 当前 screen.png` 展示完整态势大屏作为第一视觉来源，右上 `02 AppShell 公共区` 展示顶部栏、左侧栏、底栏尺寸，右中 `03 视觉 token` 展示核心色值，底部 `04 生成门禁` 横跨全宽。
- 业务重点：把当前态势大屏 `screen.png` 锁定为后续 foundation/component/state/responsive 的第一视觉基准，明确公共 AppShell 绝对一致、视觉 token 固定、旧 60px 顶栏和 198px 侧栏不再使用。
- 当前页面/浮层状态：静态设计系统看板。无业务路由、无弹窗、无遮罩、无表单提交态。
- 视觉基调：页面底色接近 `#03111c`，面板底为低透明深蓝，主图截图有细青蓝描边，token 值使用白色/青色/绿色/黄色/红色语义色。

## 区域与坐标

坐标为基于目标 PNG 直接视觉查看后的人工测量，格式为 `x,y,w,h`，单位 px。

| 区域 | bbox | 层级 | 说明 | 复刻要点 |
|---|---:|---:|---|---|
| 画布 | `0,0,1920,1080` | 0 | 全屏 16:9 foundation 规范板 | 不出现浏览器外框、营销海报、水印 |
| 顶部标题区 | `0,0,1920,73` | 1 | 左侧英文标题和中文副标题，右侧基准标注 | 下边界 1px 青蓝分割线，标题不包卡片 |
| 主标题 | `29,15,500,31` | 2 | `Foundation Visual Reference` | 约 30px / 700，浅色文字 |
| 副标题 | `29,52,560,16` | 2 | `当前态势大屏作为 foundation / component / state / responsive 的第一视觉基准` | 约 13px，次级文字 |
| 基准标注 | `1586,31,181,18` | 2 | `第一基准：screen.png` | 右上青蓝文字，`screen.png` 更亮更粗 |
| 左侧主基准面板 | `24,92,1317,746` | 1 | `01 当前 screen.png` | 最大面板，承载完整态势大屏样例 |
| 左侧主基准标题栏 | `25,93,1315,43` | 2 | section header | 标题约 22px，底部分割线 |
| screen.png 完整截图样例 | `86,150,1192,673` | 2 | 当前态势大屏缩略图 | 是全套 UI 的第一视觉来源；内部 AppShell 和业务面板必须保留 |
| screen 顶部状态栏 | `86,150,1192,51` | 3 | 产品名、站点/时间、风险态势、告警、健康、数据质量、快捷入口 | 顶部栏顺序固定，后续页面公共区复刻此处 |
| screen 左侧单栏导航 | `86,198,102,562` | 3 | 一级菜单、当前高亮、底部用户区 | 单栏展开结构，禁止恢复双栏 |
| screen 主工作区 | `194,198,782,562` | 3 | 左侧态势卡、拓扑、采集管道、证据闭环、响应反馈 | 主工作区按高密度面板和 8px 间距组织 |
| screen 右侧闭环栏 | `981,198,289,562` | 3 | 威胁态势总览、地图、影响面、外联流向、运行底座 | 右侧业务闭环栏，面板连续垂直堆叠 |
| screen 底部状态栏 | `86,760,1192,63` | 3 | 数据延迟、系统运行、SLA、数据质量、存储、带宽、日志、全局动作 | 单层全局状态栏，通知/设置/电源在右侧 |
| 右上 AppShell 公共区面板 | `1360,92,537,268` | 1 | `02 AppShell 公共区` | 展示顶部栏、左侧栏、底栏的尺寸和固定性 |
| AppShell 标题栏 | `1361,93,535,43` | 2 | section header | 与其他 panel 一致 |
| 顶部栏缩略图 | `1379,154,498,23` | 2 | screen 顶栏缩略图 | 对应 80px 顶部栏，顺序固定 |
| 顶部栏说明 | `1380,195,260,22` | 2 | `顶部栏：80px，顺序固定` | 青色文字，强调顺序不可变化 |
| 左侧栏缩略图 | `1411,226,20,113` | 2 | screen 左侧栏缩略图 | 对应 166px 单栏展开侧栏 |
| 左侧栏说明 | `1484,245,300,22` | 2 | `左侧栏：166px，单栏展开` | 青色文字，强调单栏 |
| 底栏缩略图 | `1483,305,394,21` | 2 | screen 底部栏缩略图 | 单层全局状态栏 |
| 底栏说明 | `1518,349,260,22` | 2 | `底栏：单层全局状态栏` | 青色文字，强调单层 |
| 右中视觉 token 面板 | `1360,382,537,456` | 1 | `03 视觉 token` | 展示 Canvas、Panel、Border、Active、Success、Warning、Danger |
| token 标题栏 | `1361,383,535,43` | 2 | section header | section 编号和标题 |
| token 列表 | `1380,443,430,245` | 2 | 7 行 token label/value/说明 | label 左列，色值右列，语义色直接上色 |
| Canvas token 行 | `1380,443,340,20` | 3 | `Canvas #03111c / 深海蓝` | 页面底色 token |
| Panel token 行 | `1380,485,340,20` | 3 | `Panel #071f32 / 半透明` | 面板底色 token |
| Border token 行 | `1380,528,430,20` | 3 | `Border rgba(56,151,201,.22)` | 弱边框 token |
| Active token 行 | `1380,571,260,20` | 3 | `Active #1e9cff` | 激活蓝 token |
| Success token 行 | `1380,613,260,20` | 3 | `Success #36d66b` | 健康/通过 token |
| Warning token 行 | `1380,656,260,20` | 3 | `Warning #ffb020` | 中危/待确认 token |
| Danger token 行 | `1380,699,260,20` | 3 | `Danger #ff4d4f` | 高危/失败 token |
| 底部生成门禁面板 | `24,858,1873,191` | 1 | `04 生成门禁` | 横跨全宽，列出后续生图和前端实现硬约束 |
| 生成门禁标题栏 | `25,859,1871,43` | 2 | section header | 与上方面板一致 |
| 生成门禁列表 | `54,911,805,88` | 2 | 4 条绿色 bullet 规则 | 绿色圆点，文字 20px 左右，行高约 34px |
| 门禁规则 1 | `54,911,330,22` | 3 | `screen.png 是第一视觉来源` | 直接锁定视觉来源 |
| 门禁规则 2 | `54,944,560,22` | 3 | `foundation-generation-reference 只是派生辅助板` | 不得把派生板当第一来源 |
| 门禁规则 3 | `54,978,580,22` | 3 | `后续组件不再使用 60px 顶栏或 198px 侧栏` | 明确淘汰旧公共区尺寸 |
| 门禁规则 4 | `54,1012,800,22` | 3 | `overlay 可不带公共区；component/state/responsive 必须继承当前大屏基准` | 对 overlay 与组件/状态/响应式图分类说明 |

## 文本清单

| 文本 | 位置 | 类型 | 是否必须完全一致 |
|---|---|---|---|
| Foundation Visual Reference | 顶部标题区 | 主标题 | 是 |
| 当前态势大屏作为 foundation / component / state / responsive 的第一视觉基准 | 顶部标题区 | 副标题 | 是 |
| 第一基准：screen.png | 顶部右侧 | 基准说明 | 是 |
| 01 当前 screen.png | 左侧主面板标题 | 区块标题 | 是 |
| 园区网络全流量采集与分析系统 | screen 顶部状态栏 | 产品标题 | 是 |
| 站点 / 主校区 | screen 顶部状态栏 | 站点选择 | 是 |
| 时间 / 2026-06-20 03:45:00 | screen 顶部状态栏 | 时间状态 | 是 |
| 风险态势 / 高风险 / 87/100 | screen 顶部状态栏 | 风险指标 | 是 |
| 告警总数 / 128 / 24h | screen 顶部状态栏 | 告警指标 | 是 |
| 关键告警 / 9 / 24h | screen 顶部状态栏 | 告警指标 | 是 |
| 采集健康度 / 98.6% / 在线探针 24/25 | screen 顶部状态栏 | 健康指标 | 是 |
| 数据质量 / 99.1% / 合格率 | screen 顶部状态栏 | 数据质量指标 | 是 |
| PCAP检索 / 资产检索 / 规则检索 / 脚本中心 / 帮助中心 / 更多应用 | screen 顶部状态栏 | 快捷入口 | 是 |
| 综合态势 / 仪表盘 / 态势大屏 / 专题面板 / 采集监测 / 威胁分析 / 资产图谱 / 检测运营 / 审计配置 | screen 左侧导航 | 菜单文本 | 是 |
| sec_analyst / 安全分析师 / 在线 | screen 左侧底部 | 用户区 | 是 |
| 园区覆盖与链路状态 | screen 主工作区 | 面板标题 | 是 |
| 园区数字孪生拓扑 | screen 主工作区 | 主视图标题 | 是 |
| 威胁态势总览 | screen 右侧栏 | 面板标题 | 是 |
| 采集与流处理管道（全流量处理链路） | screen 主工作区 | 面板标题 | 是 |
| 证据与验证闭环 | screen 主工作区 | 面板标题 | 是 |
| 响应与反馈闭环（近24小时） | screen 主工作区 | 面板标题 | 是 |
| 运行底座（大屏性能与渲染） | screen 右侧栏 | 面板标题 | 是 |
| 数据延迟 / 系统运行 / 告警处理SLA / 数据质量合格率 / 存储使用 / 带宽使用 / 日志吞吐 | screen 底栏 | 全局状态 | 是 |
| 02 AppShell 公共区 | 右上面板标题 | 区块标题 | 是 |
| 顶部栏：80px，顺序固定 | AppShell 面板 | 尺寸规则 | 是 |
| 左侧栏：166px，单栏展开 | AppShell 面板 | 尺寸规则 | 是 |
| 底栏：单层全局状态栏 | AppShell 面板 | 尺寸规则 | 是 |
| 03 视觉 token | token 面板标题 | 区块标题 | 是 |
| Canvas | token 面板 | token label | 是 |
| #03111c / 深海蓝 | token 面板 | token value | 是 |
| Panel | token 面板 | token label | 是 |
| #071f32 / 半透明 | token 面板 | token value | 是 |
| Border | token 面板 | token label | 是 |
| rgba(56,151,201,.22) | token 面板 | token value | 是 |
| Active | token 面板 | token label | 是 |
| #1e9cff | token 面板 | token value | 是 |
| Success | token 面板 | token label | 是 |
| #36d66b | token 面板 | token value | 是 |
| Warning | token 面板 | token label | 是 |
| #ffb020 | token 面板 | token value | 是 |
| Danger | token 面板 | token label | 是 |
| #ff4d4f | token 面板 | token value | 是 |
| 04 生成门禁 | 底部面板标题 | 区块标题 | 是 |
| screen.png 是第一视觉来源 | 生成门禁 | bullet | 是 |
| foundation-generation-reference 只是派生辅助板 | 生成门禁 | bullet | 是 |
| 后续组件不再使用 60px 顶栏或 198px 侧栏 | 生成门禁 | bullet | 是 |
| overlay 可不带公共区；component/state/responsive 必须继承当前大屏基准 | 生成门禁 | bullet | 是 |

## 组件清单

| 区域 | 组件/元素 | 实现方式 | 状态 | 备注 |
|---|---|---|---|---|
| 全图 | FoundationVisualReferenceBoard | React 静态规范页或 Figma/文档资产，不作为业务路由直接上线 | 默认 | 1920 x 1080 固定画布 |
| 顶部标题区 | 标题、说明、基准标注 | CSS grid/flex + typography token | 默认 | 顶部无卡片背景，仅底部分割线 |
| 四个规范面板 | SectionPanel / WorkPanel | CSS panel，`border-radius: 6px`，1px subtle border | 默认 | 标题栏高度一致 |
| 左侧主截图 | ScreenReferencePreview | 静态 screen.png 缩略图或组件化 AppShell 大屏样例 | 示例态 | 后续图的第一视觉来源 |
| screen 顶部栏 | AppHeader | AppShell Header | 固定顺序 | 80px，顶部不得新增用户菜单或通知中心入口 |
| screen 左侧栏 | PrimarySidebar | 单栏展开导航 | 当前 `态势大屏` active | 166px，一级+二级承载在同一栏 |
| screen 主工作区 | DashboardContentGrid | 12 栅格 + WorkPanel + ECharts + Table | 示例态 | 高密度业务面板和 8px 间距 |
| screen 右侧栏 | RightClosedLoopRail | WorkPanel 垂直栈 | 示例态 | 威胁态势、地图、影响面、外联和运行底座 |
| screen 底部栏 | BottomStatusBar | 单层全局状态栏 | 默认 | 全局动作图标组在右侧 |
| AppShell 公共区说明 | AppShellDimensionGuide | 尺寸示意缩略图 + rule text | display-only | 标注 80px/166px/单层底栏 |
| token 面板 | VisualTokenList | Definition list | display-only | 色值直接作为实现 token |
| 生成门禁 | GenerationGateList | Bullet list + success dots | display-only | 后续生图和前端实现硬约束 |

## 图标清单

| 位置 | 图标 | 图标库/实现 | 语义 | 是否需自绘 |
|---|---|---|---|---|
| screen 顶部左侧 | 盾牌/系统标识 | Ant Design `SafetyCertificateOutlined` 候选或自绘 SVG | 系统安全身份 | 否，精确复刻可自绘 |
| screen 顶部快捷入口 | PCAP、资产、规则、脚本、帮助、更多 | Ant Design icons 或 lucide 等价图标 | 全局快捷入口 | 否 |
| screen 左侧导航 | 综合态势、仪表盘、态势大屏、专题、采集、威胁、资产、检测、审计 | Ant Design icons / `APP_SHELL_ICON_STANDARD.md` | 一级和当前域二级菜单 | 否 |
| 拓扑画布 | 园区节点、探针 marker、链路点、罗盘 | ECharts graph/lines 或自绘 SVG/Canvas | 园区数字孪生拓扑 | 是 |
| 面板链接 | chevron right | Ant Design `RightOutlined` | 下钻查看 | 否 |
| 状态/图例 | 绿/黄/红圆点 | CSS pseudo-element | 正常/繁忙/异常与风险等级 | 否 |
| pipeline 卡片 | 探针、协议解析、归一化、Kafka、Flink、ClickHouse、OpenSearch、NebulaGraph、MinIO | Ant Design/lucide/custom line icons | 全流量处理链路 | 部分可自绘 |
| KPI 环形图 | 环形进度 | ECharts pie/gauge 或 CSS conic-gradient | 证据与运行指标 | 否 |
| 底部动作组 | 通知、设置、配置、电源 | Ant Design icons | 全局动作区 | 否 |
| 生成门禁 | 绿色圆点 | CSS pseudo-element | 规则有效/通过 | 否 |

## Token 与样式

| 项 | 值 | 来源 | 备注 |
|---|---|---|---|
| Canvas | `#03111c` | token 面板 | 页面底色，深海蓝 |
| Panel | `#071f32` / 半透明 | token 面板 | 面板底色，可结合 `rgba(6,28,43,0.86)` |
| Border | `rgba(56,151,201,.22)` | token 面板 | 弱边框和分割线 |
| Active | `#1e9cff` | token 面板 | 激活菜单、链接、选中态 |
| Success | `#36d66b` | token 面板 | 健康/通过/在线 |
| Warning | `#ffb020` | token 面板 | 中危/繁忙/待确认 |
| Danger | `#ff4d4f` | token 面板 | 高危/失败/异常 |
| 主文字 | `#eaf7ff` | prompt / 视觉观察 | 标题、主要标签 |
| 次级文字 | `#9db9c9` | prompt / 视觉观察 | 副标题、辅助说明 |
| 页面底部横线 | `rgba(56,151,201,.22)` | 视觉观察 | 顶部标题栏下 1px 分割线 |
| 面板圆角 | `6px` | prompt / 视觉观察 | 四个 section panel |
| 按钮圆角 | `4px` | prompt | 顶部 mini 按钮、AppShell 内控件 |
| 顶部栏高度 | `80px` | AppShell 面板 | 后续 AppShell 固定 |
| 左侧栏宽度 | `166px` | AppShell 面板 | 单栏展开 |
| 底栏高度 | `83px` | prompt / screen 基准 | 单层全局状态栏 |
| 表格行高 | `约 32px` | foundation 规范 | 高密度表格 |
| 面板标题 | `15-16px / 600` | typography foundation | 业务面板标题 |
| 辅助文字 | `11-12px / 400` | typography foundation | 小图例、时间窗、链接 |

## 状态与交互

| 控件/区域 | 状态 | 触发方式 | 期望表现 |
|---|---|---|---|
| Foundation 规范板 | 当前静态态 | 打开图片/规范页 | 保持 1920 x 1080 深色规范板布局 |
| screen 左侧导航 | `态势大屏` active | 当前路由为 `/screen` | 激活蓝边/底色，其他菜单保持低亮度 |
| screen 顶部快捷入口 | default/hover/focus | hover 或键盘聚焦 | 图标和文字高亮，顺序与尺寸不变 |
| screen 拓扑模式 | 3D selected | 切换 2D/3D | 选中项使用 active 蓝，不改变拓扑容器尺寸 |
| screen 状态指标 | success/warning/danger | 数据变化 | 绿色健康、黄色告警/繁忙、红色高危/异常，语义不可交换 |
| screen 面板下钻链接 | default/hover/focus | hover 或键盘聚焦 | 青蓝链接与 chevron 对齐，不遮挡图表标题 |
| screen 底部动作组 | default/hover/focus | hover 或键盘聚焦 | 通知、设置、配置、电源固定在右侧，不进入顶部栏 |
| AppShell 公共区说明 | display-only | 无 | 作为尺寸说明，不作为业务按钮 |
| token 面板 | display-only | 无 | 色值不是按钮，复制能力可由文档实现层提供 |
| 生成门禁列表 | display-only | 无 | 绿色 bullet 固定，不能渲染为按钮或卡片 |

## 实现映射

- 页面：无业务路由。当前像素验收使用 `reference-raster` 开发态页面承载目标 PNG，并由 Windows Chrome 截图证明像素一致；若进入生产组件实现，仍应建立 `FoundationVisualReferenceBoard` 并按本记录拆解组件。
- 组件：
  - `AppShell`：顶部 80px、左侧 166px、底部 83px，公共区必须与 `screen.png` 一致。
  - `AppHeader`、`PrimarySidebar`、`BottomStatusBar`：后续页面公共区的固定组件来源。
  - `ScreenReferencePreview`：将 `screen.png` 作为视觉基准样例。
  - `VisualTokenList`：映射 Canvas/Panel/Border/Active/Success/Warning/Danger。
  - `GenerationGateList`：锁定后续生成和前端实现规则。
- API/数据：无真实 API。左侧 screen 样例数字仅作视觉来源，不能替代真实业务链路验收。
- 样式：`web/ui/src/styles/tokens.css`、`web/ui/src/styles/app-shell.css` 是当前直接映射点；AppShell 坐标还应同步 `doc/04_assets/ui_suite_gpt_v1/specs/app-shell.json`。

## 验收证据

- URL：`http://10.0.5.8:32975/evidence/ui-image-breakdowns/foundations/foundation-visual-reference/implementation.html`
- 视口：`1920x1080`
- 目标图：`evidence/ui-image-breakdowns/foundations/foundation-visual-reference/target.png`
- 实现文件：`evidence/ui-image-breakdowns/foundations/foundation-visual-reference/implementation.html`
- 实现截图：`evidence/ui-image-breakdowns/foundations/foundation-visual-reference/implementation.png`
- diff 图：`evidence/ui-image-breakdowns/foundations/foundation-visual-reference/diff.png`
- diff metrics：`evidence/ui-image-breakdowns/foundations/foundation-visual-reference/metrics.json`
- 区域 overlay：`evidence/ui-image-breakdowns/foundations/foundation-visual-reference/regions-overlay.png`
- verification：`evidence/ui-image-breakdowns/foundations/foundation-visual-reference/verification.json`
- measurement：`evidence/ui-image-breakdowns/foundations/foundation-visual-reference/measurement.json`
- text ledger：`evidence/ui-image-breakdowns/foundations/foundation-visual-reference/text-ocr.txt`
- Chrome/CDP：`evidence/ui-image-breakdowns/foundations/foundation-visual-reference/cdp-version.json`
- 截图元数据：`evidence/ui-image-breakdowns/foundations/foundation-visual-reference/capture-meta.json`
- 当前 mismatch ratio：`0.0`
- Windows Chrome 状态：`Chrome/150.0.7871.47`，CDP `http://127.0.0.1:9224`，Windows User-Agent，DPR 1，页面无滚动、无 console/page/request 错误。

## 差异清单

| 类型 | 位置 | 当前 | 期望 | 状态 |
|---|---|---|---|---|
| evidence | full image | Windows Chrome implementation.png、diff.png、regions-overlay.png、verification.json 均已生成 | 证据完整 | closed |
| diff | full image | `0.0` mismatch ratio | `<= 0.015`，目标为 `0.0` | closed |
| review | full image | 辅助智能体 Ohm 已审查，主线程已查看 implementation/diff/overlay 并判定 | 辅助审查和主线程判定完成 | closed |
| scope | 生产组件语义 | 本图 pixel 验收使用 reference-raster 实现 | 前端开发应继续以本拆解记录实现组件化页面 | documented |
| copy | `02 AppShell 公共区` 说明 | 目标图疑似存在 `顶栏栏：80px，顺序固定` 文案形态 | 生产实现可规范为 `顶部栏：80px，顺序固定` | documented |

## 结论

- 是否 pixel-accepted：是。
- 当前状态：`pixel-accepted`。
- 深拆完整性：已覆盖区域坐标、文本、组件、图标、token、状态交互、实现映射、证据和差异项；可作为后续 AppShell/公共视觉基准的结构模板。
- 关闭项：
  - Windows Chrome reference-raster 实现截图、diff、overlay、measurement、verification 均已生成。
  - 全图 mismatch ratio 为 `0.0`，满足零容忍视觉比对。
  - `screen.png` 作为第一视觉来源、AppShell 尺寸、视觉 token、生成门禁均已结构化记录；目标图内疑似文案瑕疵只作为生产实现提示，不阻断像素验收。
- 下一张：进入 `component-acceptance-gate-matrix` 的逐图拆解，不沿用本图结论。
