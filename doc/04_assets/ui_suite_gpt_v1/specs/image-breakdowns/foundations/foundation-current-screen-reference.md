# foundation-current-screen-reference.png 逐图精拆记录

## 基本信息

- 分类：foundations
- 源图：`doc/04_assets/ui_suite_gpt_v1/screens/foundations/foundation-current-screen-reference.png`
- 源图尺寸：1920 x 1080
- 对应 prompt：无独立 prompt；该图以 `screens/pages/screen.png` 为当前态势大屏参考源。
- 对应 manifest：无独立 manifest item；该图用于约束后续 prompt 和页面复刻。
- 对应路由/宿主路由：无。该图是 generation reference 规范板，不是业务路由。
- 当前状态：`pixel-accepted`
- 复刻等级：已完成逐图视觉读取、结构化拆解、Windows Chrome evidence、零容忍 diff、辅助审查和主线程最终判断。

## 目标图观察

- 整体布局：1920 x 1080 深色规范板。顶部标题栏说明该图从当前态势大屏 `screen.png` 派生，左侧为大面积 `01 当前态势大屏主基准`，右侧为 `02 公共区域裁切` 和 `03 禁止漂移规则`，底部为 `04 后续组件顺序`。
- 业务重点：这张图不是新页面，而是后续 UI 生成和复刻的“公共区域不得漂移”规则板。它把 AppShell 顶栏、左侧栏、底部状态栏、组件密度、图标风格和视觉 token 固定下来。
- 当前页面/浮层状态：静态 foundation reference board。无业务数据刷新、无弹窗、无抽屉、无表单提交态。右侧裁切区展示公共区域缩略截图，右侧规则区展示不可偏移的尺寸规则。
- 视觉基调：延续第一张 foundation 的深色 SOC 样式；页面底色接近 `#03111c`，面板为深蓝半透明，边框为低透明青蓝，重点说明文字使用浅色，规则 bullet 使用绿色。
- 特殊点：左侧嵌入的态势大屏截图必须被视为第一参考图，不允许后续生成把顶栏高度、侧边栏宽度、底部状态栏坐标按旧模板漂移。

## 区域与坐标

坐标为基于目标 PNG 直接视觉查看后的人工测量，格式为 `x,y,w,h`，单位 px。

| 区域 | bbox | 层级 | 说明 | 复刻要点 |
|---|---:|---:|---|---|
| 画布 | `0,0,1920,1080` | 0 | 全屏 16:9 foundation reference board | 不能出现浏览器外框、水印、滚动条 |
| 顶部标题区 | `0,0,1920,73` | 1 | 英文标题、中文说明、右侧基准标注 | 下边界 1px 青蓝分割线 |
| 主标题 | `29,15,574,31` | 2 | `Foundation Generation Reference` | 约 30px 粗体，浅色 |
| 副标题 | `29,53,512,14` | 2 | `由当前态势大屏 screen.png 派生；只作提示辅助，不能替代 screen.png` | 次级浅灰蓝，明确不能替代 screen |
| 基准标注 | `1586,31,181,18` | 2 | `第一基准：screen.png` | 右上 cyan，`screen.png` 更亮 |
| 主基准面板 | `24,92,1236,718` | 1 | `01 当前态势大屏主基准` | 本图最大面板，左侧完整嵌入当前大屏 |
| 主基准标题栏 | `25,93,1234,43` | 2 | section header | 标题 22px 左右，底部分割线 |
| 嵌入态势大屏画框 | `72,150,1141,642` | 2 | 当前大屏完整缩略参考 | 保留 80px 顶栏、166px 侧栏、底部状态栏 |
| 嵌入大屏顶栏 | `78,151,1130,44` | 3 | 系统名、站点时间、风险、告警、健康、质量、快捷入口 | 后续页面 Header 以此为准，不再按 60px 生成 |
| 嵌入大屏侧栏 | `79,196,91,502` | 3 | 单栏展开式导航和底部用户区 | 后续页面 Sidebar 以 166px 公共宽度规则为准 |
| 嵌入大屏主内容 | `175,196,750,471` | 3 | 覆盖率、拓扑、采集链路、证据与取证闭环 | 表格、图表、卡片密度作为后续页面密度源 |
| 嵌入大屏右侧态势列 | `929,196,275,471` | 3 | 威胁态势总览和运行底座 | 右侧小图表和列表密度不可放大 |
| 嵌入大屏底部状态栏 | `78,718,1130,50` | 3 | 数据延迟、系统运行、SLA、质量、存储、带宽、日志、动作 | 后续公共 Statusbar 以 y=997/h=83 规则推导 |
| 公共区域裁切面板 | `1290,92,607,249` | 1 | `02 公共区域裁切` | 右上示意公共顶栏、侧栏、状态栏裁切 |
| 公共区域裁切标题栏 | `1291,93,605,43` | 2 | section header | 与其他面板统一高度 |
| 顶栏裁切 | `1308,153,566,25` | 3 | 顶部公共 Header 的横向缩略图 | 系统名、风险、告警、健康、质量、快捷入口都可见 |
| 侧栏裁切 | `1352,215,24,116` | 3 | 左侧 Sidebar 的竖向缩略图 | 单栏图标/文字导航，不使用宽侧栏老模板 |
| 底部状态栏裁切 | `1444,284,426,22` | 3 | 底部公共 Statusbar 的横向缩略图 | 以绿色健康点和细分隔线为主 |
| 禁止漂移规则面板 | `1290,371,607,439` | 1 | `03 禁止漂移规则` | 六条 bullet 规则是本图核心文本 |
| 禁止漂移标题栏 | `1291,372,605,43` | 2 | section header | 标题左对齐，面板边框一致 |
| 规则列表 | `1318,435,522,292` | 2 | 六条绿色 bullet + 规则文字 | 行距约 55px，bullet 直径约 12px |
| 第一参考图规则 | `1318,434,404,23` | 3 | `第一参考图：screens/pages/screen.png` | 明确 screen.png 是第一参考图 |
| 顶部栏规则 | `1318,489,420,23` | 3 | `顶部栏 80px，不再按 60px 生成` | 锁定 Header 高度 |
| 左侧栏规则 | `1318,544,424,23` | 3 | `左侧栏 166px，不再按 198px 生成` | 锁定 Sidebar 宽度 |
| 底部状态栏规则 | `1318,599,369,23` | 3 | `底部状态栏 y=997，高 83px` | 锁定 Statusbar 坐标 |
| 组件继承规则 | `1318,653,480,23` | 3 | `组件板必须继承当前密度和图标风格` | 密度、图标不允许漂移 |
| 浮层公共区规则 | `1318,708,467,23` | 3 | `浮层可省略公共区，但保留视觉 token` | overlay 可裁掉公共区但不能丢 token |
| 后续组件顺序面板 | `24,840,1873,198` | 1 | `04 后续组件顺序` | 底部横跨全宽，用于指导后续组件拆解顺序 |
| 后续组件标题栏 | `25,841,1871,43` | 2 | section header | 分割线和标题样式与其他面板一致 |
| Header 顺序行 | `59,899,751,20` | 3 | Header 的组成顺序 | 系统名、站点时间、风险、告警、健康、质量、快捷入口 |
| Sidebar 顺序行 | `59,943,704,20` | 3 | Sidebar 的组成顺序 | 单栏展开式导航、六个固定业务域、底部用户区 |
| Statusbar 顺序行 | `59,986,873,20` | 3 | Statusbar 的组成顺序 | 数据延迟、系统运行、SLA、质量、存储、带宽、日志、全局动作 |

## 文本清单

| 文本 | 位置 | 类型 | 是否必须完全一致 |
|---|---|---|---|
| Foundation Generation Reference | 顶部标题区 | 主标题 | 是 |
| 由当前态势大屏 screen.png 派生；只作提示辅助，不能替代 screen.png | 顶部标题区 | 副标题 | 是 |
| 第一基准：screen.png | 顶部右侧 | 基准说明 | 是 |
| 01 当前态势大屏主基准 | 主基准面板标题 | 区块标题 | 是 |
| 园区网络全流量采集与分析系统 | 嵌入大屏顶栏 | 系统名 | 是 |
| 总览 主校区 | 嵌入大屏顶栏 | 站点选择 | 是 |
| 时间 2026-06-20 03:45:00 | 嵌入大屏顶栏 | 时间窗 | 是 |
| 风险态势 高风险 87/100 | 嵌入大屏顶栏 | 风险状态 | 是 |
| 告警总数 128 24h | 嵌入大屏顶栏 | 告警指标 | 是 |
| 关键告警 9 24h | 嵌入大屏顶栏 | 关键告警 | 是 |
| 采集健康度 98.6% 在线探针 24/25 | 嵌入大屏顶栏 | 健康指标 | 是 |
| 数据质量 99.1% 合格率 | 嵌入大屏顶栏 | 质量指标 | 是 |
| 综合态势 | 嵌入大屏侧栏 | 导航项 | 是 |
| 仪表盘 | 嵌入大屏侧栏 | 导航项 | 是 |
| 态势大屏 | 嵌入大屏侧栏 | 激活导航项 | 是 |
| 专题面板 | 嵌入大屏侧栏 | 导航项 | 是 |
| 采集监测 | 嵌入大屏侧栏 | 导航项 | 是 |
| 威胁分析 | 嵌入大屏侧栏 | 导航项 | 是 |
| 资产图谱 | 嵌入大屏侧栏 | 导航项 | 是 |
| 检测运营 | 嵌入大屏侧栏 | 导航项 | 是 |
| 审计配置 | 嵌入大屏侧栏 | 导航项 | 是 |
| sec_analyst | 嵌入大屏侧栏底部 | 用户名 | 是 |
| 02 公共区域裁切 | 右上裁切面板标题 | 区块标题 | 是 |
| 03 禁止漂移规则 | 右侧规则面板标题 | 区块标题 | 是 |
| 第一参考图：screens/pages/screen.png | 规则列表 | 规则 | 是 |
| 顶部栏 80px，不再按 60px 生成 | 规则列表 | 规则 | 是 |
| 左侧栏 166px，不再按 198px 生成 | 规则列表 | 规则 | 是 |
| 底部状态栏 y=997，高 83px | 规则列表 | 规则 | 是 |
| 组件板必须继承当前密度和图标风格 | 规则列表 | 规则 | 是 |
| 浮层可省略公共区，但保留视觉 token | 规则列表 | 规则 | 是 |
| 04 后续组件顺序 | 底部顺序面板标题 | 区块标题 | 是 |
| Header | 底部顺序面板 | 组件类别 | 是 |
| 系统名 / 站点时间 / 风险 / 告警 / 健康 / 质量 / 快捷入口 | Header 顺序行 | 组成顺序 | 是 |
| Sidebar | 底部顺序面板 | 组件类别 | 是 |
| 单栏展开式导航 / 六个固定业务域 / 底部用户区 | Sidebar 顺序行 | 组成顺序 | 是 |
| Statusbar | 底部顺序面板 | 组件类别 | 是 |
| 数据延迟 / 系统运行 / SLA / 质量 / 存储 / 带宽 / 日志 / 全局动作 | Statusbar 顺序行 | 组成顺序 | 是 |
| 数据延迟 1.23 s | 嵌入大屏底部状态栏 | 状态指标 | 是 |
| 系统运行 23 天 14 小时 | 嵌入大屏底部状态栏 | 状态指标 | 是 |
| 告警处置SLA 98.2% | 嵌入大屏底部状态栏 | 状态指标 | 是 |
| 数据质量合格率 99.1% | 嵌入大屏底部状态栏 | 状态指标 | 是 |
| 存储使用 68.7 / 120 TB | 嵌入大屏底部状态栏 | 状态指标 | 是 |
| 带宽使用 42.7 / 100 Gbps | 嵌入大屏底部状态栏 | 状态指标 | 是 |

## 组件清单

| 区域 | 组件/元素 | 实现方式 | 状态 | 备注 |
|---|---|---|---|---|
| 全图 | FoundationReferenceBoard | React 静态规范页或文档资产 | 默认 | 1920 x 1080 固定画布 |
| 顶部标题区 | SpecHeader | CSS flex/grid + typography token | 默认 | 与第一张 foundation header 对齐 |
| 主基准面板 | SectionPanel | WorkPanel 外框 + 标题栏 | 默认 | 右侧和底部面板沿用同一 panel token |
| 嵌入态势大屏 | CurrentScreenReferenceImage | raster reference 或缩略截图组件 | 默认 | 只作参考，不作为业务页面 DOM |
| 嵌入顶栏 | AppHeaderReference | 参考 `components/component-app-header.png` 和 `screen.png` | reference | 锁定 80px 顶栏规则 |
| 嵌入侧栏 | PrimarySidebarReference | 参考 `components/component-primary-sidebar.png` 和 `screen.png` | reference | 锁定 166px 左侧栏规则 |
| 嵌入底栏 | BottomStatusBarReference | 参考 `components/component-bottom-status-bar.png` 和 `screen.png` | reference | 锁定 y=997/h=83 |
| 公共区域裁切 | PublicRegionCropPanel | 三个 raster crop/thumbnail | 默认 | 顶栏、侧栏、状态栏三段必须都出现 |
| 禁止漂移规则 | RuleList | CSS bullet list | 默认 | bullet 使用 success green |
| 后续组件顺序 | ComponentOrderPanel | Description/List + tokenized text | 默认 | 三行 Header/Sidebar/Statusbar |
| Header 顺序行 | TokenizedInlineList | inline slash-separated text | 默认 | cyan 文本强调 |
| Sidebar 顺序行 | TokenizedInlineList | inline slash-separated text | 默认 | cyan 文本强调 |
| Statusbar 顺序行 | TokenizedInlineList | inline slash-separated text | 默认 | success green 文本强调 |

## 图标清单

| 位置 | 图标 | 图标库/实现 | 语义 | 是否需自绘 |
|---|---|---|---|---|
| 嵌入大屏系统名左侧 | Shield / 安全盾牌 | Ant Design `SafetyCertificateOutlined` 候选或自绘 | 系统安全标识 | 否 |
| 嵌入大屏导航 | Grid/Dashboard/Globe/Monitor/Setting 等 | Ant Design 图标组合 | 一级导航 | 否 |
| 嵌入大屏顶栏快捷入口 | PCAP 检索 | Ant Design `SearchOutlined`/`FileSearchOutlined` | 证据检索 | 否 |
| 嵌入大屏顶栏快捷入口 | 资产检索 | Ant Design `DatabaseOutlined`/`SearchOutlined` | 资产检索 | 否 |
| 嵌入大屏顶栏快捷入口 | 规则检索 | Ant Design `FileTextOutlined`/`SearchOutlined` | 规则检索 | 否 |
| 嵌入大屏顶栏快捷入口 | 脚本中心 | Ant Design `CodeOutlined` | 脚本入口 | 否 |
| 嵌入大屏顶栏快捷入口 | 帮助中心 | Ant Design `QuestionCircleOutlined` | 帮助入口 | 否 |
| 嵌入大屏顶栏快捷入口 | 更多应用 | Ant Design `AppstoreOutlined` | 应用集合 | 否 |
| 规则列表 bullet | 绿色圆点 | CSS circle | 不漂移规则锚点 | 否 |
| 底部状态栏 | 健康/告警/日志/设置/电源图标 | Ant Design + CSS badge | 状态和全局动作 | 否 |

## Token 与样式

| 项 | 值 | 来源 | 备注 |
|---|---|---|---|
| 页面底色 | `#03111c` | foundation token | 与第一张一致 |
| 面板底色 | `#071f32` / 低透明深蓝 | 视觉观察 | 主面板、右侧面板、底部面板一致 |
| 面板边框 | `rgba(56,151,201,.22)` | foundation token | 1px 青蓝 |
| 面板圆角 | `6px` | 视觉观察 | 外框统一 |
| 顶部标题色 | `#eaf7ff` | foundation token | 英文主标题 |
| 次级文字色 | `#9db9c9` | foundation token | 副标题和行 label |
| 弱文字色 | `#5e7b8d` | foundation token | 局部小字 |
| 激活蓝 | `#1e9cff` | foundation token | `screen.png`、顺序行中的 cyan |
| 成功绿 | `#36d66b` | foundation token | bullet 和 Statusbar 顺序行 |
| 警告橙 | `#ffb020` | 参考嵌入大屏 | 告警状态 token |
| 危险红 | `#ff4d4f` | 参考嵌入大屏 | 高风险状态 token |
| 顶栏高度规则 | `80px` | 禁止漂移规则 | 后续页面不得回到 60px |
| 左侧栏宽度规则 | `166px` | 禁止漂移规则 | 后续页面不得用 198px 旧宽度 |
| 底部状态栏规则 | `y=997, h=83px` | 禁止漂移规则 | 画布高度 1080 下固定 |
| 主基准嵌图宽度 | 约 `1141px` | 视觉测量 | 占左侧面板绝大部分 |
| 规则行距 | 约 `55px` | 视觉测量 | 六条 bullet 均匀纵向排布 |
| 底部顺序行高 | 约 `44px` | 视觉测量 | Header/Sidebar/Statusbar 三行 |
| 字体字重 | 标题 `700`，正文 `500-600` | 视觉观察 | 保持高密度但清晰 |

## 状态与交互

| 控件/区域 | 状态 | 触发方式 | 期望表现 |
|---|---|---|---|
| Foundation reference board | static/reference | 打开图片或规范页 | 固定 1920 x 1080，不随内容滚动 |
| 主基准嵌入截图 | reference-only | 无 | 不作为可点击业务截图；只提供视觉基准 |
| 公共区域裁切 | reference-only | 无 | 展示顶栏、侧栏、底栏裁切，不提供 hover 态 |
| 禁止漂移规则 | static rule | 无 | 六条规则必须完整可读，bullet 不丢失 |
| Header 后续顺序 | implementation guidance | 组件实现阶段 | 按系统名/站点时间/风险/告警/健康/质量/快捷入口顺序实现 |
| Sidebar 后续顺序 | implementation guidance | 组件实现阶段 | 按单栏展开导航/六个固定业务域/底部用户区实现 |
| Statusbar 后续顺序 | implementation guidance | 组件实现阶段 | 按数据延迟/系统运行/SLA/质量/存储/带宽/日志/全局动作实现 |
| 后续页面复刻 | anti-drift gate | 页面生成/复刻 | 若公共区尺寸漂移，必须退回本图规则修正 |
| 浮层复刻 | overlay exception | 浮层/弹窗图 | 可省略公共区，但必须保留视觉 token |

## 实现映射

- 页面：无业务路由。可在开发态建立 `FoundationGenerationReferenceBoard`，用于设计系统文档和 prompt 约束。
- 组件：
  - `SpecHeader`：承载英文标题、中文说明、右上基准标注。
  - `SectionPanel`：承载 01/02/03/04 四个区块。
  - `CurrentScreenReferenceImage`：承载 `screens/pages/screen.png` 缩略基准。
  - `PublicRegionCropPanel`：承载顶栏、侧栏、底栏裁切缩略。
  - `RuleList`：承载六条禁止漂移规则。
  - `ComponentOrderPanel`：承载 Header/Sidebar/Statusbar 后续组件顺序。
- API/数据：无真实 API。本图所有内容均为静态规范和 reference，不触发接口。
- 样式：沿用 `foundation-color-status` 的 token；后续页面、组件、浮层都必须引用本图给出的公共区尺寸规则。
- 复刻方式：像素验收可使用 reference-raster 页面；生产组件实现仍必须按本记录拆解组件，不得只贴图替代业务 DOM。

## 验收证据

- URL：`http://10.0.5.8:44829/evidence/ui-image-breakdowns/foundations/foundation-current-screen-reference/implementation.html`
- 视口：`1920x1080`
- 目标图：`evidence/ui-image-breakdowns/foundations/foundation-current-screen-reference/target.png`
- 实现文件：`evidence/ui-image-breakdowns/foundations/foundation-current-screen-reference/implementation.html`
- 实现截图：`evidence/ui-image-breakdowns/foundations/foundation-current-screen-reference/implementation.png`
- diff 图：`evidence/ui-image-breakdowns/foundations/foundation-current-screen-reference/diff.png`
- 区域 overlay：`evidence/ui-image-breakdowns/foundations/foundation-current-screen-reference/regions-overlay.png`
- verification：`evidence/ui-image-breakdowns/foundations/foundation-current-screen-reference/verification.json`
- measurement：`evidence/ui-image-breakdowns/foundations/foundation-current-screen-reference/measurement.json`
- text ledger：`evidence/ui-image-breakdowns/foundations/foundation-current-screen-reference/text-ocr.txt`
- Chrome/CDP：必须来自 Windows Chrome CDP `http://127.0.0.1:9224`。
- 当前 mismatch ratio：`0.0`
- Windows Chrome 状态：`Chrome/150.0.7871.47`，DPR 1，页面无滚动、无 console/page/request 错误。

## 差异清单

| 类型 | 位置 | 当前 | 期望 | 状态 |
|---|---|---|---|---|
| evidence | full image | Windows Chrome implementation/diff/overlay/verification 已生成 | 必须生成 implementation、diff、overlay、verification | closed |
| diff | full image | `0.0` mismatch ratio | `<= 0.015` | closed |
| scope | production component implementation | 本图是规范板，不是业务路由 | 组件语义按本记录实现，像素证据可 reference-raster | documented |

## 结论

- 是否 pixel-accepted：是。
- 当前状态：`pixel-accepted`。
- 已完成：直接视觉读取、坐标测量、文本校正、组件/图标/token/交互拆解。
- 已关闭：Windows Chrome screenshot、视觉 diff、区域 overlay、verification、辅助智能体审查和主线程判定。
- 范围说明：该结论证明目标 PNG 像素复刻准确；生产 React 组件语义实现仍需依据本拆解记录落地。
