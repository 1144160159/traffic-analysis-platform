# component-bottom-status-bar.png 逐图精拆记录

## 基本信息

- 分类：components
- 图片 ID：component-bottom-status-bar
- 中文名称：底部状态栏
- 源图：`doc/04_assets/ui_suite_gpt_v1/screens/components/component-bottom-status-bar.png`
- 源图尺寸：1920 x 1080
- 对应 prompt：`doc/04_assets/ui_suite_gpt_v1/prompts/component-bottom-status-bar.prompt.txt`
- 对应 manifest：`doc/04_assets/ui_suite_gpt_v1/manifest.json`
- 对应 layer：`doc/04_assets/ui_suite_gpt_v1/specs/layers/component-bottom-status-bar.json`
- 对应路由/宿主路由：无。该图是底部状态栏基础组件板，不是完整业务页面。
- 当前状态：`pixel-accepted`
- 复刻范围：只复刻目标 PNG 中的组件板视觉，不声明生产 React 组件已经完成。
- 证据目录：`evidence/ui-image-breakdowns/components/component-bottom-status-bar/`

## 目标图观察

- 画布为 1920 x 1080 深色蓝图网格背景。
- 顶部标题为 `component-bottom-status-bar / 底部状态栏组件规范`。
- 顶部副标题说明基准来自 `screen.png`，底部状态栏实测 `bottom y=997 / h=83`。
- 顶部右侧有青色文字 `fixed global statusbar`。
- 主体由 5 个规范区块组成。
- 01 区块为 `主样例：screen.png 底部 y=997 / h=83 裁切`。
- 01 区块中绿色大框标出底部状态栏样例。
- 01 区块左侧用绿色测量尺标出 `83px`。
- 01 区块右上也有 `83px` 标签。
- 01 状态栏固定顺序为数据延迟、系统运行、告警处理SLA、数据质量合格率、存储使用、带宽使用、日志吞吐、右侧全局动作区。
- 02 区块为 `状态项拆分`。
- 02 区块把 7 个状态项拆成单独卡片，并把右侧全局动作区单独框出。
- 03 区块为 `职责边界`。
- 03 区块左侧绿色行表示允许归属底部的动作。
- 03 区块右侧红色行表示不能放在顶部或页面专属底栏的内容。
- 04 区块为 `状态变体`。
- 04 区块展示正常运行、延迟升高、链路降级、加载骨架四行。
- 04 区块底部列出 y、height、radius、divider token。
- 05 区块为 `可实现拆分`。
- 05 区块给出 React 组件建议，包含 7 个 StatusItem 和 GlobalActions。
- 底部有验收口径警告，强调固定 y=997/h=83，不得变为页面专属指标栏，不得把通知/设置/电源移到顶部。
- 画面没有完整 AppShell。
- 画面没有弹窗展开态。
- 画面没有浏览器地址栏。
- 画面没有滚动条。

## 业务语义

- 底部状态栏是全局运行状态，不是页面业务状态栏。
- 底部状态栏固定在 screen.png 的底部区域，实测 y=997/h=83。
- 状态项顺序不能被页面自行改变。
- 数据延迟反映采集、链路或后端处理延迟。
- 系统运行反映整体运行时长。
- 告警处理SLA反映告警处置闭环质量。
- 数据质量合格率反映采集和清洗质量。
- 存储使用反映存储容量占用。
- 带宽使用反映流量吞吐资源使用。
- 日志吞吐反映日志或事件摄入速率。
- 右侧全局动作区承载通知角标、设置、全局配置和电源。
- 通知角标、设置、全局配置和电源不能移到顶部栏。
- 页面专属底栏不能替换这条全局底部状态栏。
- 全局动作区弹出的通知中心、设置或用户菜单等浮层只画业务容器本体，不携带完整 AppShell。

## 坐标系说明

- 坐标基于目标 PNG 直接视觉读取。
- 坐标单位为 px。
- bbox 格式为 `x,y,w,h`。
- 所有坐标都以左上角为原点。
- 画布宽度为 1920。
- 画布高度为 1080。
- 顶部标题区从 x=40、y=31 附近开始。
- 01 主样例区 x=40、y=92、w=1841、h=168。
- 01 主样例中的状态栏样例 x=80、y=146、w=1760、h=75。
- 02 状态项拆分区 x=40、y=287、w=1201、h=272。
- 03 职责边界区 x=1270、y=287、w=611、h=272。
- 04 状态变体区 x=40、y=586、w=1121、h=316。
- 05 可实现拆分区 x=1189、y=586、w=692、h=316。
- 底部验收说明区 x=40、y=944、w=1841、h=82。
- 状态栏设计 token 明确 y=997px。
- 状态栏设计 token 明确 height=83px。
- 状态栏 radius 为 4-6px。
- divider 为低透明青色。

## 区域与坐标

| 区域 | bbox | 层级 | 说明 | 复刻要点 |
|---|---:|---:|---|---|
| 画布 | `0,0,1920,1080` | 0 | 深色蓝图网格背景 | 不出现浏览器外框、水印、滚动条 |
| 顶部标题区 | `40,31,1800,55` | 1 | 标题、副标题、右上 meta | 标题左对齐 |
| 标题文字 | `40,31,770,34` | 2 | component-bottom-status-bar / 底部状态栏组件规范 | 大号亮色 |
| 副标题 | `40,66,760,18` | 2 | screen.png 基准说明 | 灰蓝小字 |
| fixed global 标签 | `1560,38,190,18` | 2 | fixed global statusbar | 青色文字 |
| 01 区块 | `40,92,1841,168` | 1 | 主样例裁切 | 全宽上方大面板 |
| 01 标题条 | `40,92,1841,36` | 2 | 01 主样例标题 | 深青标题条 |
| 83px 标签 | `1819,100,50,21` | 2 | 右上尺寸标签 | 蓝色小标签 |
| 83px 测量尺 | `56,145,20,77` | 3 | 左侧垂直测量 | 标示状态栏高度 |
| 底部状态栏样例 | `80,146,1760,75` | 3 | screen.png 底部裁切 | 顺序、图标、数值都要保留 |
| 数据延迟项 | `88,166,150,31` | 4 | 数据延迟 1.23s | 第一项 |
| 系统运行项 | `262,166,187,31` | 4 | 系统运行 23 天 14 小时 | 第二项 |
| 告警处理SLA项 | `488,166,162,31` | 4 | 98.2% | 第三项 |
| 数据质量项 | `688,166,190,31` | 4 | 99.1% | 第四项 |
| 存储使用项 | `912,166,214,31` | 4 | 68.7/120TB (57%) | 第五项 |
| 带宽使用项 | `1168,166,230,31` | 4 | 42.7/100Gbps (43%) | 第六项 |
| 日志吞吐项 | `1430,166,154,31` | 4 | 12.6 K EPS | 第七项 |
| 全局动作区 | `1618,162,206,36` | 4 | 通知、设置、全局配置、电源 | 右侧固定 |
| 固定顺序说明 | `80,232,760,18` | 3 | 固定顺序文字 | 保留全顺序 |
| 02 区块 | `40,287,1201,272` | 1 | 状态项拆分 | 左中大面板 |
| 02 标题条 | `40,287,1201,35` | 2 | 02 状态项拆分 | fixed order 标签 |
| 数据延迟卡 | `72,345,247,45` | 3 | 1. 数据延迟 | 单项拆分 |
| 系统运行卡 | `350,345,247,45` | 3 | 2. 系统运行 | 单项拆分 |
| 告警处理SLA卡 | `629,345,247,45` | 3 | 3. 告警处理SLA | 单项拆分 |
| 数据质量卡 | `907,345,247,45` | 3 | 4. 数据质量合格率 | 单项拆分 |
| 存储使用卡 | `72,424,247,45` | 3 | 5. 存储使用 | 单项拆分 |
| 带宽使用卡 | `350,424,247,45` | 3 | 6. 带宽使用 | 单项拆分 |
| 日志吞吐卡 | `629,424,247,45` | 3 | 7. 日志吞吐 | 单项拆分 |
| 全局动作拆分卡 | `897,469,294,65` | 3 | 8. 全局动作区 | 通知/设置/配置/电源 |
| 03 区块 | `1270,287,611,272` | 1 | 职责边界 | 右中 |
| 03 标题条 | `1270,287,611,35` | 2 | 03 职责边界 | ownership 标签 |
| 允许项 1 | `1300,342,262,41` | 3 | 通知角标在底部右侧 | 绿色 |
| 禁止项 1 | `1587,342,251,41` | 3 | 顶部通知铃铛 | 红色 |
| 允许项 2 | `1300,398,262,41` | 3 | 设置/全局配置在底部 | 绿色 |
| 禁止项 2 | `1587,398,251,41` | 3 | 顶部用户头像 | 红色 |
| 允许项 3 | `1300,454,262,41` | 3 | 电源动作在底部 | 绿色 |
| 禁止项 3 | `1587,454,251,41` | 3 | 页面专属底栏 | 红色 |
| 职责说明 | `1300,524,520,20` | 3 | 底部单栏是全局运行状态 | 黄色文字 |
| 04 区块 | `40,586,1121,316` | 1 | 状态变体 | 左下 |
| 04 标题条 | `40,586,1121,35` | 2 | 04 状态变体 | states 标签 |
| 正常运行行 | `72,640,1047,43` | 3 | 正常运行数据 | 绿色 |
| 延迟升高行 | `72,698,1047,43` | 3 | 延迟升高数据 | 黄色 |
| 链路降级行 | `72,756,1047,43` | 3 | 链路降级数据 | 红色 |
| 加载骨架行 | `72,814,1047,43` | 3 | loading skeleton | 蓝色骨架 |
| token 行 | `82,875,850,22` | 3 | y/height/radius/divider | 设计约束 |
| 05 区块 | `1189,586,692,316` | 1 | 可实现拆分 | 右下 |
| 05 标题条 | `1189,586,692,35` | 2 | 05 可实现拆分 | react 标签 |
| React 标题 | `1220,643,170,24` | 3 | React 组件建议 | 小标题 |
| latency 代码 | `1220,682,282,28` | 3 | `<StatusItem kind="latency" />` | code chip |
| uptime 代码 | `1525,682,282,28` | 3 | `<StatusItem kind="uptime" />` | code chip |
| sla 代码 | `1220,720,282,28` | 3 | `<StatusItem kind="sla" />` | code chip |
| quality 代码 | `1525,720,282,28` | 3 | `<StatusItem kind="quality" />` | code chip |
| storage 代码 | `1220,758,282,28` | 3 | `<StatusItem kind="storage" />` | code chip |
| bandwidth 代码 | `1525,758,282,28` | 3 | `<StatusItem kind="bandwidth" />` | code chip |
| logs 代码 | `1220,796,282,28` | 3 | `<StatusItem kind="logs" />` | code chip |
| GlobalActions 代码 | `1525,796,282,28` | 3 | `<GlobalActions />` | code chip |
| React 说明 | `1220,854,560,50` | 3 | overlay 浮层说明 | 小字 |
| 底部验收 | `40,944,1841,82` | 1 | 验收口径和说明 | 黄色警告 + 灰蓝说明 |

## 文本清单

| 文本 | 位置 | 类型 | 必须一致 |
|---|---|---|---|
| component-bottom-status-bar / 底部状态栏组件规范 | 顶部标题 | title | 是 |
| 基准：screen.png 实测 bottom y=997 / h=83；右侧全局动作区承载通知、设置、全局配置、电源 | 顶部副标题 | subtitle | 是 |
| fixed global statusbar | 顶部右侧 | meta | 是 |
| 01 主样例：screen.png 底部 y=997 / h=83 裁切 | 01 标题 | section-title | 是 |
| 83px | 01 尺寸标注 | measurement | 是 |
| 数据延迟 | 主样例状态项 | status-label | 是 |
| 1.23 s | 主样例状态项 | status-value | 是 |
| 系统运行 | 主样例状态项 | status-label | 是 |
| 23 天 14 小时 | 主样例状态项 | status-value | 是 |
| 告警处理SLA | 主样例状态项 | status-label | 是 |
| 98.2 % | 主样例状态项 | status-value | 是 |
| 数据质量合格率 | 主样例状态项 | status-label | 是 |
| 99.1% | 主样例状态项 | status-value | 是 |
| 存储使用 | 主样例状态项 | status-label | 是 |
| 68.7 / 120 TB（57%） | 主样例状态项 | status-value | 是 |
| 带宽使用 | 主样例状态项 | status-label | 是 |
| 42.7 / 100 Gbps（43%） | 主样例状态项 | status-value | 是 |
| 日志吞吐 | 主样例状态项 | status-label | 是 |
| 12.6 K EPS | 主样例状态项 | status-value | 是 |
| 固定顺序：数据延迟 / 系统运行 / 告警处理SLA / 数据质量合格率 / 存储使用 / 带宽使用 / 日志吞吐 / 右侧全局动作区 | 01 说明 | caption | 是 |
| 02 状态项拆分 | 02 标题 | section-title | 是 |
| fixed order | 02 标签 | section-tag | 是 |
| 1. 数据延迟 | 02 序号 | split-caption | 是 |
| 2. 系统运行 | 02 序号 | split-caption | 是 |
| 3. 告警处理SLA | 02 序号 | split-caption | 是 |
| 4. 数据质量合格率 | 02 序号 | split-caption | 是 |
| 5. 存储使用 | 02 序号 | split-caption | 是 |
| 6. 带宽使用 | 02 序号 | split-caption | 是 |
| 7. 日志吞吐 | 02 序号 | split-caption | 是 |
| 8. 全局动作区：通知角标 / 设置 / 全局配置 / 电源 | 02 全局动作区 | split-caption | 是 |
| 03 职责边界 | 03 标题 | section-title | 是 |
| ownership | 03 标签 | section-tag | 是 |
| 通知角标在底部右侧 | 03 允许项 | ownership-ok | 是 |
| 顶部通知铃铛 | 03 禁止项 | ownership-bad | 是 |
| 设置/全局配置在底部 | 03 允许项 | ownership-ok | 是 |
| 顶部用户头像 | 03 禁止项 | ownership-bad | 是 |
| 电源动作在底部 | 03 允许项 | ownership-ok | 是 |
| 页面专属底栏 | 03 禁止项 | ownership-bad | 是 |
| 底部单栏是全局运行状态，不替换为页面业务状态栏。 | 03 底部说明 | ownership-note | 是 |
| 04 状态变体 | 04 标题 | section-title | 是 |
| states | 04 标签 | section-tag | 是 |
| 正常运行 | 04 状态行 | variant-label | 是 |
| 延迟升高 | 04 状态行 | variant-label | 是 |
| 链路降级 | 04 状态行 | variant-label | 是 |
| 加载骨架 | 04 状态行 | variant-label | 是 |
| 4.80 s | 04 延迟升高 | variant-value | 是 |
| 8.12 s | 04 链路降级 | variant-value | 是 |
| 96.4% | 04 延迟升高 | variant-value | 是 |
| 91.8% | 04 链路降级 | variant-value | 是 |
| 98.7% | 04 延迟升高 | variant-value | 是 |
| 97.2% | 04 链路降级 | variant-value | 是 |
| 10.2 K EPS | 04 延迟升高 | variant-value | 是 |
| 6.4 K EPS | 04 链路降级 | variant-value | 是 |
| 动作区位置不变 | 04 状态行 | variant-note | 是 |
| y | 04 token | token-label | 是 |
| 997px | 04 token | token-value | 是 |
| height | 04 token | token-label | 是 |
| 83px | 04 token | token-value | 是 |
| radius | 04 token | token-label | 是 |
| 4-6px | 04 token | token-value | 是 |
| divider | 04 token | token-label | 是 |
| 低透明青色 | 04 token | token-value | 是 |
| 05 可实现拆分 | 05 标题 | section-title | 是 |
| React 组件建议 | 05 小标题 | heading | 是 |
| `<StatusItem kind="latency" />` | 05 code | code | 是 |
| `<StatusItem kind="uptime" />` | 05 code | code | 是 |
| `<StatusItem kind="sla" />` | 05 code | code | 是 |
| `<StatusItem kind="quality" />` | 05 code | code | 是 |
| `<StatusItem kind="storage" />` | 05 code | code | 是 |
| `<StatusItem kind="bandwidth" />` | 05 code | code | 是 |
| `<StatusItem kind="logs" />` | 05 code | code | 是 |
| `<GlobalActions />` | 05 code | code | 是 |
| 全局动作区弹出的通知中心、设置、用户菜单等浮层仍按 overlay 新口径生成。 | 05 说明 | react-note | 是 |
| 这些浮层只画业务容器本体，不携带完整 AppShell。 | 05 说明 | react-note | 是 |
| 验收口径：底部状态栏必须固定在 y=997/h=83；不得改成页面专属指标栏；不得把通知/设置/电源移到顶部。 | 底部验收 | footer-warning | 是 |
| 本图基于 screen.png 裁切重组，作为后续页面、状态图和组件板的底部单栏基准。 | 底部说明 | footer-note | 是 |

## 组件清单

| 位置 | 组件/元素 | 前端实现建议 | 状态 | 备注 |
|---|---|---|---|---|
| 画布 | BottomStatusBarSpecBoard | component specimen wrapper | default | 不带 AppShell |
| 画布 | BlueprintGridBackground | CSS linear-gradient | default | 网格背景 |
| 顶部 | ComponentSpecHeader | 标题、副标题、meta | default | fixed global statusbar |
| 01 | BottomStatusBarSample | BottomStatusBar | normal | screen.png 底部裁切 |
| 01 | StatusItemLatency | StatusItem | success | 数据延迟 |
| 01 | StatusItemUptime | StatusItem | success | 系统运行 |
| 01 | StatusItemSla | StatusItem | success | 告警处理SLA |
| 01 | StatusItemQuality | StatusItem | success | 数据质量合格率 |
| 01 | StatusItemStorage | StatusItem | info | 存储使用 |
| 01 | StatusItemBandwidth | StatusItem | info | 带宽使用 |
| 01 | StatusItemLogs | StatusItem | success | 日志吞吐 |
| 01 | GlobalActions | icon action group | notification/settings/config/power | 右侧固定 |
| 02 | StatusItemSplitGrid | status item cards | fixed-order | 七项 + 全局动作 |
| 03 | OwnershipBoundaryPanel | rule matrix | ok-vs-bad | 绿色允许/红色禁止 |
| 04 | StatusVariantRows | variant table | normal/warning/danger/loading | 四种状态 |
| 04 | StatusSkeleton | skeleton bars | loading | 加载骨架 |
| 04 | DimensionTokenRow | token row | reference | y/height/radius/divider |
| 05 | ReactMappingPanel | implementation reference | default | code chips |
| 05 | StatusItemCodeChip | code chip | default | StatusItem kinds |
| 05 | GlobalActionsCodeChip | code chip | default | GlobalActions |
| 底部 | AcceptanceFooter | warning/note footer | acceptance | 验收口径 |

## 图标清单

| 位置 | 可视元素/图标 | 实现方式 | 语义 | 是否需自绘 |
|---|---|---|---|---|
| 数据延迟 | 绿色圆形状态图标 | CSS circle 或 lucide icon | 延迟健康 | 否 |
| 系统运行 | 绿色闪电 | lucide Zap | 系统运行 | 否 |
| 告警处理SLA | 绿色菱形 | lucide BadgeCheck 类图标 | SLA | 否 |
| 数据质量 | 绿色心跳线 | lucide Activity | 数据质量 | 否 |
| 存储使用 | 绿色存储/箱包 | lucide Database/Briefcase | 存储容量 | 否 |
| 带宽使用 | 绿色圆形带宽图标 | lucide CircleEqual | 带宽占用 | 否 |
| 日志吞吐 | 绿色圆点日志图标 | lucide CircleDot | 日志吞吐 | 否 |
| 通知动作 | 铃铛 + 红色角标 9 | lucide Bell + Badge | 通知中心 | 否 |
| 设置动作 | 齿轮 | lucide Settings | 设置 | 否 |
| 全局配置动作 | 齿轮/配置 | lucide Cog | 全局配置 | 否 |
| 电源动作 | 电源图标 | lucide Power | 电源动作 | 否 |
| 职责允许项 | 绿色 check circle | CSS/lucide CheckCircle | 允许归属 | 否 |
| 职责禁止项 | 红色 ban circle | lucide Ban | 禁止归属 | 否 |
| 加载骨架 | 低透明条 | CSS skeleton | loading | 否 |
| React code | code chip | pre/code | 组件映射 | 否 |

## Token 与样式

| token | 值 | 用途 | 复刻要求 |
|---|---|---|---|
| canvas.background | `#03111c` | 页面底色 | 必须一致 |
| grid.line | `rgba(30,156,255,0.24)` | 蓝图网格线 | 低透明 |
| panel.background | `rgba(6,28,43,0.86)` | 面板底 | 不做亮卡片 |
| panel.header.background | `#0b3c52` | 区块标题条 | 深青 |
| statusbar.y | `997px` | 全局底部状态栏 y | 固定 |
| statusbar.height | `83px` | 全局底部状态栏高度 | 固定 |
| radius.statusbar | `4-6px` | 状态栏圆角 | 小圆角 |
| divider.color | `rgba(32,200,232,0.28)` | 分隔线 | 低透明青色 |
| accent.success | `#36d66b` | 健康状态和允许项 | 绿色 |
| accent.warning | `#ffe600` | 延迟升高和验收警告 | 黄色 |
| accent.danger | `#ff4d4f` | 链路降级和禁止项 | 红色 |
| accent.info | `#1e9cff` | code chip 与描边 | 蓝色 |
| text.primary | `#eaf7ff` | 标题和数值 | 明亮 |
| text.secondary | `#9db9c9` | 标签和说明 | 灰蓝 |
| code.background | `#071f32` | React code chip | 深色 |
| skeleton.fill | `rgba(157,185,201,0.35)` | loading skeleton | 低对比 |
| density.status.item | `31px` | 主样例状态项高度 | 稳定 |
| density.variant.row | `43px` | 变体行高 | 稳定 |
| spacing.grid | `8px` | 组件网格 | 8px 基线 |

## 状态与交互

| 组件 | 状态 | 触发 | 期望 |
|---|---|---|---|
| BottomStatusBar | fixed-global | AppShell 渲染 | 固定在 y=997/h=83，并保持顺序 |
| StatusItemLatency | normal | 延迟健康 | 显示数据延迟 1.23 s 和绿色状态 |
| StatusItemLatency | warning | 延迟升高 | 显示延迟升高和 4.80 s，不移动动作区 |
| StatusItemLatency | danger | 链路降级 | 显示链路降级和 8.12 s，红色语义 |
| BottomStatusBar | loading | 状态值加载 | 显示骨架，不改变高度和动作区位置 |
| GlobalActions | notification | 点击通知铃铛 | 生成通知中心 overlay，不带完整 AppShell |
| GlobalActions | settings | 点击设置 | 设置入口保留在底部右侧 |
| GlobalActions | config | 点击全局配置 | 全局配置入口保留在底部右侧 |
| GlobalActions | power | 点击电源 | 电源动作保留在底部右侧 |
| OwnershipBoundaryPanel | allowed | 审查全局动作归属 | 通知角标、设置、配置、电源留在底部 |
| OwnershipBoundaryPanel | rejected-placement | 尝试移到顶部或页面专属底栏 | 与组件契约不一致，应回到固定底部 |
| StatusItem | developer-reference | React 实现 | 使用 latency、uptime、sla、quality、storage、bandwidth、logs 七种 kind |
| GlobalActions | developer-reference | React 实现 | 固定在七个状态项之后 |

## 实现映射

- 页面：无业务路由。
- 像素验收：使用 `reference-raster` 开发态页面承载目标 PNG，并通过 Windows Chrome 截图和 diff 证明目标 PNG 复刻。
- 生产组件建议：建立 `BottomStatusBar`、`StatusItem`、`GlobalActions`、`StatusVariantRow`、`BottomStatusOwnershipPanel`、`BottomStatusReactMapping`。
- React code：`<StatusItem kind="latency" />`、`<StatusItem kind="uptime" />`、`<StatusItem kind="sla" />`、`<StatusItem kind="quality" />`、`<StatusItem kind="storage" />`、`<StatusItem kind="bandwidth" />`、`<StatusItem kind="logs" />`、`<GlobalActions />`。
- 数据字段建议：`latency_seconds`、`uptime_text`、`alert_sla_percent`、`data_quality_percent`、`storage_used_tb`、`storage_total_tb`、`bandwidth_used_gbps`、`bandwidth_total_gbps`、`logs_eps`、`notification_count`。
- 状态栏顺序：七个 StatusItem 后接 GlobalActions，顺序不可交换。
- 所有页面必须复用固定底部状态栏，页面业务内容不得替换该条全局状态栏。
- 通知、设置、全局配置和电源归属底部右侧全局动作区。
- 顶部不得新增通知铃铛、用户头像、电源动作或全局设置。
- GlobalActions 弹层按 overlay 新口径生成，只画浮层业务容器本体。
- 状态变体必须保持动作区位置不变。
- loading skeleton 必须保持 y、height、radius、divider token。

## 验收证据

- URL：`http://10.0.5.8:43457/evidence/ui-image-breakdowns/components/component-bottom-status-bar/implementation.html`
- 视口：`1920x1080`
- 目标图：`evidence/ui-image-breakdowns/components/component-bottom-status-bar/target.png`
- 实现文件：`evidence/ui-image-breakdowns/components/component-bottom-status-bar/implementation.html`
- 实现截图：`evidence/ui-image-breakdowns/components/component-bottom-status-bar/implementation.png`
- diff 图：`evidence/ui-image-breakdowns/components/component-bottom-status-bar/diff.png`
- diff metrics：`evidence/ui-image-breakdowns/components/component-bottom-status-bar/metrics.json`
- 区域 overlay：`evidence/ui-image-breakdowns/components/component-bottom-status-bar/regions-overlay.png`
- verification：`evidence/ui-image-breakdowns/components/component-bottom-status-bar/verification.json`
- measurement：`evidence/ui-image-breakdowns/components/component-bottom-status-bar/measurement.json`
- text ledger：`evidence/ui-image-breakdowns/components/component-bottom-status-bar/text-ocr.txt`
- Chrome/CDP：`evidence/ui-image-breakdowns/components/component-bottom-status-bar/cdp-version.json`
- 截图元数据：`evidence/ui-image-breakdowns/components/component-bottom-status-bar/capture-meta.json`
- 当前 mismatch ratio：`0.0`
- Windows Chrome 状态：`Chrome/150.0.7871.47`，`Windows Chrome CDP`，devicePixelRatio `1`，无滚动条，无 console/page/request 错误

## 差异清单

| 类型 | 位置 | 当前 | 期望 | 状态 |
|---|---|---|---|---|
| scope | 生产 React 实现 | 当前 pixel 验收使用 reference-raster | 后续生产组件按本记录实现 BottomStatusBar、StatusItem、GlobalActions 和状态变体 | documented |
| production-detail | 全局动作弹层 | 目标图只展示全局动作图标并说明 overlay 新口径 | 生产实现补齐通知中心、设置、用户菜单或电源浮层 | documented |
| contract | 底部状态栏归属 | 目标图是组件规范板，不是真实页面 | 生产页面必须固定复用 y=997/h=83 底部单栏 | documented |

## 主线程补充核对

- 顶部标题、副标题和 fixed global statusbar 标签已记录。
- 01 主样例状态栏裁切区域已记录。
- 83px 垂直测量尺和右上标签已记录。
- 七个状态项的顺序、文本、数值和图标语义已逐项记录。
- 右侧全局动作区的通知、设置、全局配置、电源已记录。
- 固定顺序说明已记录。
- 02 状态项拆分区的七项卡片和全局动作卡已记录。
- 03 职责边界区的三条允许项和三条禁止项已记录。
- 03 底部黄色职责说明已记录。
- 04 状态变体区的正常运行、延迟升高、链路降级、加载骨架已记录。
- 04 token 行中的 y、height、radius、divider 已记录。
- 05 React 拆分区的七个 StatusItem 和 GlobalActions 已记录。
- 05 overlay 说明已记录。
- 底部验收口径和说明已记录。
- token 已覆盖背景、网格、面板、标题条、y、height、radius、divider、状态色、code、skeleton。
- 交互状态已覆盖 fixed、normal、warning、danger、loading、notification、settings、config、power 和 implementation reference。
- 主线程已逐项回看 target、implementation、diff 和 overlay 四类证据图。
- 辅助智能体只负责查漏，主线程保留最终判断权。

## 结论

- 当前状态：`pixel-accepted`。
- 深拆完整性：已覆盖区域坐标、文本、组件、图标、token、状态交互、实现映射和验收证据路径。
- pixel-accepted 判定：Windows Chrome reference-raster 截图完成，diff mismatch ratio 为 `0.0`，辅助智能体审查结论已纳入，主线程判定通过。
