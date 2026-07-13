# component-app-header.png 逐图精拆记录

## 基本信息

- 分类：components
- 图片 ID：component-app-header
- 中文名称：顶部状态栏
- 源图：`doc/04_assets/ui_suite_gpt_v1/screens/components/component-app-header.png`
- 源图尺寸：1920 x 1080
- 对应 prompt：`doc/04_assets/ui_suite_gpt_v1/prompts/component-app-header.prompt.txt`
- 对应 manifest：`doc/04_assets/ui_suite_gpt_v1/manifest.json`
- 对应 layer：`doc/04_assets/ui_suite_gpt_v1/specs/layers/component-app-header.json`
- 对应路由/宿主路由：无。该图是顶部状态栏组件规范板，不是完整业务页面。
- 当前状态：`pixel-accepted`
- 复刻范围：只复刻目标 PNG 中的顶部状态栏规范板视觉，不声明生产 React 组件已经完成。
- 证据目录：`evidence/ui-image-breakdowns/components/component-app-header/`

## 目标图观察

- 画布为 1920 x 1080 深色蓝图网格背景。
- 顶部标题为 `component-app-header / 顶部状态栏组件规范`。
- 标题下说明基准来自 `screen.png`，实测 topbar 为 80px。
- 右上角有青色标签 `current screen baseline`。
- 页面主体分为五个编号规范区。
- `01 主样例` 展示 screen.png 顶部 80px 裁切。
- `01` 区右上角有 `no user / no bell group` 标签。
- `02 顶部结构拆分` 展示顶部栏被拆成品牌、站点时间、风险告警、健康质量、快捷入口等片段。
- `02` 区右上角有 `fixed order` 标签。
- `03 职责边界：避免重复` 展示顶部保留和顶部禁止的边界。
- `03` 区右上角有 `must` 标签。
- `04 状态变体：只改变指标状态，不新增用户区` 展示正常、高危、加载、离线降级四行。
- `04` 区右上角有 `states` 标签。
- `05 与左侧/底部区域的关系` 展示顶部、左下用户身份区、底部动作区之间的归属关系。
- `05` 区右上角有 `ownership` 标签。
- 底部黄色验收口径明确禁止顶部出现用户头像、用户名、通知铃铛、设置或电源。
- 底部灰蓝说明保留 raw imagegen 版本作为追溯，当前最终 PNG 是 screen.png 裁切重组的确定性修正版。
- 整张图不是常规 AppShell，而是 AppHeader 组件规范说明板。
- 画面没有真实弹窗。
- 画面没有浏览器地址栏。
- 画面没有滚动条。

## 业务语义

- 顶部状态栏只承载运行态势和快捷入口。
- 顶部状态栏不得承载通知中心入口。
- 顶部状态栏不得承载用户头像、用户名、用户菜单。
- 顶部状态栏不得承载设置或电源动作。
- 告警数和关键告警是运行态势指标，不是通知入口。
- 用户身份归属左侧底部用户区。
- 通知、设置、电源等全局动作归属底部右侧动作区。
- 快捷入口固定为 PCAP 检索、资产检索、规则检测、脚本中心、帮助中心、更多应用。
- 主样例必须以 screen.png 顶部 80px 为基准。
- 状态变体只能改变指标状态，不得新增用户动作组。

## 坐标系说明

- 坐标基于目标 PNG 直接视觉读取。
- 坐标单位为 px。
- bbox 格式为 `x,y,w,h`。
- 所有坐标都以左上角为原点。
- 画布宽度为 1920。
- 画布高度为 1080。
- 主规范区使用青蓝细描边。
- 面板标题条高度约 34px。
- 顶部主样例中 topbar 裁切高度为 80px。
- 芯片圆角约 4px。
- 面板圆角约 4-6px。

## 区域与坐标

| 区域 | bbox | 层级 | 说明 | 复刻要点 |
|---|---:|---:|---|---|
| 画布 | `0,0,1920,1080` | 0 | 深色蓝图网格背景 | 不出现浏览器外框或水印 |
| 顶部说明区 | `40,30,1840,50` | 1 | 标题、副标题、baseline 标签 | 无卡片包裹 |
| 页面标题 | `40,31,690,28` | 2 | component-app-header / 顶部状态栏组件规范 | 大号亮色 |
| 基准说明 | `40,67,820,18` | 2 | screen.png topbar=80px 说明 | 次级灰蓝 |
| baseline 标签 | `1570,38,190,18` | 2 | current screen baseline | 青色小标签 |
| 01 主样例面板 | `40,92,1840,168` | 1 | 主样例 screen.png 顶部裁切 | 顶部 80px 基准 |
| 01 面板标题 | `56,104,430,20` | 2 | 01 主样例：screen.png 顶部 80px 裁切 | 编号清晰 |
| no user 标签 | `1698,101,170,18` | 2 | no user / no bell group | 右上角 |
| 80px 标尺 | `56,146,25,74` | 3 | 左侧竖向标尺和 80px 文本 | 标出 topbar 高度 |
| 主样例 topbar | `80,146,1760,74` | 2 | screen.png 顶部状态栏裁切 | 只到快捷入口 |
| 品牌标题块 | `96,160,380,48` | 3 | shield logo + 系统名 | 不含用户信息 |
| 站点卡 | `482,155,100,57` | 3 | 站点/主校区/下拉 | 第一运行信息 |
| 时间卡 | `588,155,160,57` | 3 | 时间/2026-06-20 03:45:00 | 固定位置 |
| 风险态势卡 | `754,155,150,57` | 3 | 高风险 87/100 | 红色风险 |
| 告警总数卡 | `909,155,118,57` | 3 | 告警总数 128 24h | 黄色告警 |
| 关键告警卡 | `1032,155,114,57` | 3 | 关键告警 9 24h | 红色告警 |
| 采集健康卡 | `1152,155,188,57` | 3 | 采集健康度 98.6% 在线探针 24/25 | 绿色健康 |
| 数据质量卡 | `1347,155,140,57` | 3 | 数据质量 99.1% 合格率 | 绿色质量 |
| 快捷入口组 | `1515,160,310,48` | 3 | PCAP/资产/规则/脚本/帮助/更多 | 六个图标入口 |
| 主样例包含说明 | `80,230,900,18` | 2 | 包含字段列表 | 说明性小字 |
| 02 结构拆分面板 | `40,287,1201,269` | 1 | 顶部结构拆分 | 左中面板 |
| 02 面板标题 | `56,299,190,20` | 2 | 02 顶部结构拆分 | 编号标题 |
| fixed order 标签 | `1138,294,90,20` | 2 | fixed order | 右上角 |
| 品牌区切片 | `70,346,210,50` | 3 | 品牌区示意裁切 | 带 logo 和系统名 |
| 站点与时间切片 | `300,346,210,50` | 3 | 站点与时间裁切 | 站点 + 时间 |
| 风险与告警指标切片 | `530,350,210,48` | 3 | 风险、告警、关键告警 | 指标组 |
| 健康与质量切片 | `825,350,210,48` | 3 | 采集健康、数据质量 | 健康组 |
| 六个快捷入口切片 | `1080,356,190,43` | 3 | 快捷入口裁切 | PCAP 到更多 |
| 结构标签行 | `70,416,1080,20` | 3 | 品牌区/站点与时间/风险与告警指标/健康与质量/六个快捷入口 | 下方说明 |
| 固定顺序芯片链 | `78,470,1102,37` | 3 | 品牌到更多应用的固定顺序 | PCAP 芯片加粗 |
| 03 职责边界面板 | `1270,287,610,269` | 1 | 顶部职责边界 | 右中面板 |
| 03 面板标题 | `1286,299,270,20` | 2 | 03 职责边界：避免重复 | 编号标题 |
| must 标签 | `1818,294,50,20` | 2 | must | 右上角 |
| 顶部保留标题 | `1301,340,90,20` | 2 | 顶部保留 | 绿色 |
| 保留运行指标 | `1301,369,185,42` | 3 | 运行指标 | 绿色条 |
| 保留站点时间 | `1486,369,185,42` | 3 | 站点时间 | 绿色条 |
| 保留快捷入口 | `1670,369,230,42` | 3 | 快捷入口 | 绿色条 |
| 顶部禁止标题 | `1301,437,90,20` | 2 | 顶部禁止 | 红色 |
| 禁止通知铃铛 | `1301,466,220,42` | 3 | 通知铃铛 | 红色条 |
| 禁止用户头像菜单 | `1530,466,220,42` | 3 | 用户头像/菜单 | 红色条 |
| 禁止设置电源 | `1650,514,220,42` | 3 | 设置/电源 | 红色条 |
| 边界说明 | `1301,524,320,18` | 2 | 告警数是态势指标，不是通知中心入口。 | 灰蓝说明 |
| 04 状态变体面板 | `40,584,1120,318` | 1 | 四种状态变体 | 左下大面板 |
| 04 面板标题 | `56,596,520,20` | 2 | 04 状态变体：只改变指标状态，不新增用户区 | 编号标题 |
| states 标签 | `1090,593,58,20` | 2 | states | 右上角 |
| 正常状态行 | `72,644,1046,37` | 3 | 正常 + topbar 裁切 + 右端说明 | 绿色 |
| 高危状态行 | `72,697,1046,42` | 3 | 高危 + topbar 裁切 + 右端说明 | 红色 |
| 加载状态行 | `72,760,1046,37` | 3 | 加载 + skeleton 覆盖 | 蓝色 |
| 离线降级状态行 | `72,813,1046,37` | 3 | 离线降级 + 黄框裁切 | 黄色 |
| 状态 token 行 | `82,870,930,20` | 3 | topbar/font/border/radius | token 汇总 |
| 05 关系面板 | `1190,584,690,318` | 1 | 与左侧/底部区域关系 | 右下大面板 |
| 05 面板标题 | `1206,596,360,20` | 2 | 05 与左侧/底部区域的关系 | 编号标题 |
| ownership 标签 | `1782,593,88,20` | 2 | ownership | 右上角 |
| 顶部归属框 | `1245,650,560,20` | 3 | 顶部：无用户/通知动作组 | 青色长框 |
| 左下用户身份区框 | `1245,774,290,62` | 3 | 左下：用户身份区 | 绿色标注 |
| 底部全局动作区框 | `1285,821,250,58` | 3 | 12.6K EPS、通知、设置、电源 | 绿色框 |
| 用户卡示意 | `1605,744,210,137` | 3 | sec_analyst 安全分析师 在线 | 用户身份归属示意 |
| 底部验收线 | `40,944,1840,1` | 1 | 分隔线 | 青蓝 |
| 验收口径 | `40,968,1120,22` | 1 | 黄色验收口径 | 明确不合格条件 |
| 追溯说明 | `40,1008,860,18` | 1 | raw imagegen 追溯说明 | 灰蓝说明 |

## 文本清单

| 文本 | 位置 | 类型 | 必须一致 |
|---|---|---|---|
| component-app-header / 顶部状态栏组件规范 | 顶部说明区 | title | 是 |
| 基准：screen.png 实测 topbar=80px；顶部只承载运行态势与快捷入口，不承载通知/用户动作组 | 顶部说明区 | subtitle | 是 |
| current screen baseline | 顶部右侧 | badge | 是 |
| 01 主样例：screen.png 顶部 80px 裁切 | 01 标题 | panel-title | 是 |
| no user / no bell group | 01 标签 | badge | 是 |
| 80px | 主样例标尺 | measure | 是 |
| 园区网络全流量采集与分析系统 | 主样例品牌 | topbar-title | 是 |
| 站点 / 主校区 | 主样例站点卡 | metric | 是 |
| 时间 / 2026-06-20 03:45:00 | 主样例时间卡 | metric | 是 |
| 风险态势 / 高风险 87/100 | 主样例风险卡 | metric | 是 |
| 告警总数 / 128 / 24h | 主样例告警卡 | metric | 是 |
| 关键告警 / 9 / 24h | 主样例关键告警 | metric | 是 |
| 采集健康度 / 98.6% / 在线探针 24/25 | 主样例健康卡 | metric | 是 |
| 数据质量 / 99.1% / 合格率 | 主样例质量卡 | metric | 是 |
| PCAP检索 | 快捷入口 | action | 是 |
| 资产检索 | 快捷入口 | action | 是 |
| 规则检测 | 快捷入口 | action | 是 |
| 脚本中心 | 快捷入口 | action | 是 |
| 帮助中心 | 快捷入口 | action | 是 |
| 更多应用 | 快捷入口 | action | 是 |
| 包含：产品标题 / 站点 / 时间 / 风险态势 / 告警总数 / 关键告警 / 采集健康 / 数据质量 / PCAP-资产-规则-脚本-帮助-更多应用 | 01 说明 | helper | 是 |
| 02 顶部结构拆分 | 02 标题 | panel-title | 是 |
| fixed order | 02 标签 | badge | 是 |
| 品牌区 | 02 标签 | label | 是 |
| 站点与时间 | 02 标签 | label | 是 |
| 风险与告警指标 | 02 标签 | label | 是 |
| 健康与质量 | 02 标签 | label | 是 |
| 六个快捷入口 | 02 标签 | label | 是 |
| 品牌 | 顺序芯片 | chip | 是 |
| 站点 | 顺序芯片 | chip | 是 |
| 时间 | 顺序芯片 | chip | 是 |
| 风险 | 顺序芯片 | chip | 是 |
| 告警 | 顺序芯片 | chip | 是 |
| 关键 | 顺序芯片 | chip | 是 |
| 健康 | 顺序芯片 | chip | 是 |
| 质量 | 顺序芯片 | chip | 是 |
| PCAP | 顺序芯片 | chip | 是 |
| 资产 | 顺序芯片 | chip | 是 |
| 规则 | 顺序芯片 | chip | 是 |
| 脚本 | 顺序芯片 | chip | 是 |
| 帮助 | 顺序芯片 | chip | 是 |
| 更多 | 顺序芯片 | chip | 是 |
| 03 职责边界：避免重复 | 03 标题 | panel-title | 是 |
| must | 03 标签 | badge | 是 |
| 顶部保留 | 03 小标题 | section-label | 是 |
| 运行指标 | 保留项 | allowed | 是 |
| 站点时间 | 保留项 | allowed | 是 |
| 快捷入口 | 保留项 | allowed | 是 |
| 顶部禁止 | 03 小标题 | section-label | 是 |
| 通知铃铛 | 禁止项 | forbidden | 是 |
| 用户头像/菜单 | 禁止项 | forbidden | 是 |
| 设置/电源 | 禁止项 | forbidden | 是 |
| 告警数是态势指标，不是通知中心入口。 | 03 说明 | helper | 是 |
| 04 状态变体：只改变指标状态，不新增用户区 | 04 标题 | panel-title | 是 |
| states | 04 标签 | badge | 是 |
| 正常 | 状态行 | state | 是 |
| 高危 | 状态行 | state | 是 |
| 加载 | 状态行 | state | 是 |
| 离线降级 | 状态行 | state | 是 |
| 右端仍止于快捷入口 | 状态行右侧 | rule | 是 |
| topbar | token | token | 是 |
| 80px | token | token | 是 |
| font | token | token | 是 |
| 12-24px | token | token | 是 |
| border | token | token | 是 |
| #3897c9 22% | token | token | 是 |
| radius | token | token | 是 |
| 4-6px | token | token | 是 |
| 05 与左侧/底部区域的关系 | 05 标题 | panel-title | 是 |
| ownership | 05 标签 | badge | 是 |
| 顶部：无用户/通知动作组 | 05 顶部框 | ownership | 是 |
| 左下：用户身份区 | 05 左下框 | ownership | 是 |
| 12.6 K EPS | 底部动作区 | footer-metric | 是 |
| sec_analyst | 用户卡 | user | 是 |
| 安全分析师 | 用户卡 | user-role | 是 |
| 在线 | 用户卡 | user-status | 是 |
| 验收口径：如果顶部出现用户头像、用户名、通知铃铛、设置或电源，视为重复并判定不合格。 | 底部黄字 | acceptance | 是 |
| 保留 raw imagegen 版本作为追溯；此最终 PNG 为 screen.png 裁切重组的确定性修正版。 | 底部灰字 | note | 是 |

## 组件清单

| 位置 | 组件/元素 | 前端实现建议 | 状态 | 备注 |
|---|---|---|---|---|
| 画布 | ComponentAppHeaderSpecBoard | component specimen wrapper | default | 顶部状态栏规范板 |
| 背景 | BlueprintGridBackground | CSS linear-gradient | default | 网格约 32px |
| 顶部说明 | SpecPageTitle | text block | default | 不是应用 topbar |
| 01 | AppHeaderBaselinePanel | SectionPanel | baseline | 主样例面板 |
| 01 | TopbarCutout | cropped baseline topbar | baseline | 80px 裁切 |
| 01 | TopbarMeasureRuler | CSS ruler | measurement | 标出 80px |
| 主样例 | ProductBrandBlock | logo + product title | default | 品牌区 |
| 主样例 | RuntimeMetricCard | metric cards | normal/high/warning/success | 站点、时间、风险、告警、健康、质量 |
| 主样例 | QuickEntryGroup | icon buttons | default | 六个入口 |
| 02 | HeaderStructurePanel | SectionPanel | default | 结构拆分 |
| 02 | HeaderSegmentCutout | crop segment | default | 五个结构切片 |
| 02 | FixedOrderChipChain | chip sequence | fixed | 固定顺序 |
| 03 | ResponsibilityBoundaryPanel | SectionPanel | must | 保留/禁止边界 |
| 03 | AllowedItem | green rule bar | allowed | 运行指标、站点时间、快捷入口 |
| 03 | ForbiddenItem | red rule bar | forbidden | 通知、用户、设置电源 |
| 04 | HeaderStateVariantPanel | SectionPanel | states | 状态变体 |
| 04 | HeaderStateRow | sample row | normal/high/loading/degraded | 只改变指标状态 |
| 04 | HeaderTokenSummary | token row | display-only | topbar/font/border/radius |
| 05 | OwnershipPanel | SectionPanel | ownership | 区域归属 |
| 05 | OwnershipDiagram | diagram | default | 顶部/左下/底部关系 |
| 05 | UserIdentityCard | user card sample | online | 证明用户区不在顶部 |
| 底部 | AcceptanceRuleLine | text | critical | 最终验收口径 |

## 图标清单

| 位置 | 可视元素/图标 | 实现方式 | 语义 | 是否需自绘 |
|---|---|---|---|---|
| 主样例品牌 | shield logo | 当前 screen.png 裁切或品牌 SVG | 产品品牌 | 是，沿用现有品牌 |
| 站点卡 | 下拉箭头 | Ant Design DownOutlined 或 CSS | 站点选择 | 否 |
| 风险态势 | 盾牌/火焰候选 | Ant Design 或现有图标 | 风险态势 | 否 |
| 告警总数 | 铃铛图标作为指标 | Ant Design BellOutlined within metric | 告警指标，不是通知入口 | 否 |
| 关键告警 | 红色告警图标 | Ant Design AlertOutlined | 关键告警指标 | 否 |
| 采集健康度 | 绿色状态图标 | Ant Design CheckCircleOutlined | 健康 | 否 |
| 数据质量 | 数据库图标 | Ant Design DatabaseOutlined | 数据质量 | 否 |
| PCAP检索 | 搜索/包图标 | Ant Design SearchOutlined | 快捷入口 | 否 |
| 资产检索 | 搜索图标 | Ant Design SearchOutlined | 快捷入口 | 否 |
| 规则检测 | 规则/文件图标 | Ant Design FileSearchOutlined | 快捷入口 | 否 |
| 脚本中心 | 文件/代码图标 | Ant Design CodeOutlined | 快捷入口 | 否 |
| 帮助中心 | 问号图标 | Ant Design QuestionCircleOutlined | 快捷入口 | 否 |
| 更多应用 | 全屏/九宫格图标 | Ant Design AppstoreOutlined | 快捷入口 | 否 |
| 03 绿色保留项 | check icon | Ant Design CheckCircleOutlined | 允许出现在顶部 | 否 |
| 03 红色禁止项 | ban icon | Ant Design StopOutlined | 顶部禁止项 | 否 |
| 05 底部动作区 | bell/settings/power icons | 底部区域图标 | 归属底部，不归属顶部 | 否 |
| 05 用户卡 | avatar icon | 用户区图标 | 归属左下用户身份区 | 否 |

## Token 与样式

| token | 值 | 来源 | 用途 |
|---|---|---|---|
| Canvas | `#03111c` | foundations | 页面底 |
| Grid line | `rgba(30,156,255,0.22)` | foundations | 背景网格 |
| Panel BG | `#071f32` / `rgba(6,28,43,0.86)` | foundations | 面板底 |
| Border | `rgba(56,151,201,.22)` | foundations | 面板/卡片边框 |
| Strong cyan | `#00d5ff` | 视觉观察 | 标题、标签、描边 |
| Text | `#eaf7ff` | foundations | 主文字 |
| Secondary | `#9db9c9` | foundations | 说明文字 |
| Success | `#36d66b` | foundations | 允许项、正常、在线 |
| Warning | `#ffb020` | foundations | 验收口径、高危/离线边界 |
| Danger | `#ff4d4f` | foundations | 禁止项、高危 |
| Topbar height | `80px` | 图中标尺 | 顶部状态栏高度 |
| Font range | `12-24px` | 状态 token 行 | 顶部字体范围 |
| Border alpha | `#3897c9 22%` | 状态 token 行 | topbar 边框 |
| Radius | `4-6px` | 状态 token 行 | topbar 圆角 |
| Panel radius | `4-6px` | 视觉观察 | 规范面板 |
| Chip radius | `4px` | 视觉观察 | 标签/顺序芯片 |
| Grid unit | `8px` | foundations | 间距 |

## 状态与交互

| 控件/区域 | 状态 | 触发方式 | 期望表现 |
|---|---|---|---|
| AppHeader | baseline | 打开规范板 | 80px 高度，右端止于快捷入口 |
| ProductBrandBlock | default | 无 | 展示 logo 和系统名 |
| SiteSelector | default/hover | 点击站点 | 可展开站点，但不影响用户区归属 |
| TimeIndicator | realtime | 时间刷新 | 卡片宽度稳定 |
| RiskMetric | normal/high | 风险变化 | 只改变指标颜色和文案 |
| AlertMetric | warning/high | 告警数变化 | 仍作为态势指标，不成为通知入口 |
| HealthMetric | success/degraded/loading | 采集健康刷新 | 保持卡片位置 |
| QualityMetric | success/degraded/loading | 数据质量刷新 | 保持卡片位置 |
| QuickEntryGroup | default/hover/selected | 点击入口 | PCAP、资产、规则、脚本、帮助、更多顺序固定 |
| HeaderStateRow | normal | 正常状态 | 保持绿色/健康指标 |
| HeaderStateRow | high | 高危状态 | 风险和告警指标变红，不新增用户区 |
| HeaderStateRow | loading | 加载状态 | 使用 skeleton 覆盖指标卡，不改变结构 |
| HeaderStateRow | degraded | 离线降级 | 使用黄色/降级状态，不新增用户区 |
| ForbiddenItem | forbidden | 设计审查 | 顶部出现通知铃铛、用户头像、设置、电源即不合格 |
| OwnershipDiagram | display-only | 无 | 表达顶部、左下、底部职责分离 |

## 实现映射

- 页面：无业务路由。
- 像素验收：使用 `reference-raster` 开发态页面承载目标 PNG，并通过 Windows Chrome 截图和 diff 证明目标 PNG 复刻。
- 生产组件建议：建立 `AppHeader`、`ProductBrandBlock`、`RuntimeMetricCard`、`QuickEntryGroup`、`HeaderStateVariant`、`HeaderOwnershipGuard`。
- 数据字段建议：`site_name`、`current_time`、`risk_score`、`alert_count_24h`、`critical_alert_count_24h`、`probe_online_count`、`probe_total_count`、`data_quality_rate`。
- 禁止字段：顶部组件不得接收 `user_name`、`avatar_url`、`notification_count`、`settings_action`、`power_action`。
- 快捷入口：`pcap_search`、`asset_search`、`rule_search`、`script_center`、`help_center`、`more_apps`。
- API/数据：目标图未绑定 API；生产实现应接入实时态势指标服务和全局快捷入口配置。
- 样式：映射 `web/ui/src/styles/tokens.css` 中的背景、边框、状态色、字体密度、topbar 高度和圆角。
- 验收守卫：可增加静态 lint 或视觉 contract，检测顶部是否出现用户头像、用户名、通知铃铛、设置、电源。
- 与左侧/底部关系：用户身份只在左侧底部，通知/设置/电源只在底部右侧。

## 验收证据

- URL：`http://10.0.5.8:39425/evidence/ui-image-breakdowns/components/component-app-header/implementation.html`
- 视口：`1920x1080`
- 目标图：`evidence/ui-image-breakdowns/components/component-app-header/target.png`
- 实现文件：`evidence/ui-image-breakdowns/components/component-app-header/implementation.html`
- 实现截图：`evidence/ui-image-breakdowns/components/component-app-header/implementation.png`
- diff 图：`evidence/ui-image-breakdowns/components/component-app-header/diff.png`
- diff metrics：`evidence/ui-image-breakdowns/components/component-app-header/metrics.json`
- 区域 overlay：`evidence/ui-image-breakdowns/components/component-app-header/regions-overlay.png`
- verification：`evidence/ui-image-breakdowns/components/component-app-header/verification.json`
- measurement：`evidence/ui-image-breakdowns/components/component-app-header/measurement.json`
- text ledger：`evidence/ui-image-breakdowns/components/component-app-header/text-ocr.txt`
- Chrome/CDP：`evidence/ui-image-breakdowns/components/component-app-header/cdp-version.json`
- 截图元数据：`evidence/ui-image-breakdowns/components/component-app-header/capture-meta.json`
- 当前 mismatch ratio：`0.0`
- Windows Chrome 状态：`Chrome/150.0.7871.47`，`Windows Chrome CDP`，`1920x1080`，DPR `1`

## 差异清单

| 类型 | 位置 | 当前 | 期望 | 状态 |
|---|---|---|---|---|
| scope | 生产 React 实现 | 当前 pixel 验收使用 reference-raster | 后续生产组件按本记录实现 React/Ant Design 语义 | documented |
| semantics | 主样例 | 当前图使用 screen.png 顶部裁切 | 生产实现必须由真实 AppHeader 组件输出相同结构 | documented |
| guardrail | 顶部禁止项 | 当前图是规范说明，不执行自动检测 | 生产代码需增加顶部禁用用户/通知/设置电源的守卫 | documented |

## 主线程补充核对

- 顶部标题、01 主样例、02 结构拆分、03 职责边界、04 状态变体、05 归属关系和底部验收口径均已单独定位。
- 主样例 topbar 的 80px 标尺和右端快捷入口边界已记录。
- 结构拆分中的固定顺序芯片链已逐项记录。
- 职责边界中顶部保留和顶部禁止项已逐项记录。
- 状态变体四行已按正常、高危、加载、离线降级记录。
- 底部黄色验收口径已逐字记录。
- 已逐项回看 target、implementation、diff 和 overlay 四类证据图。
- 辅助智能体只负责查漏，主线程保留最终判断权。

## 结论

- 当前状态：`pixel-accepted`。
- 深拆完整性：已覆盖区域坐标、文本、组件、图标、token、状态交互、实现映射和验收证据路径。
- pixel-accepted 判定：Windows Chrome 截图与目标图一致，diff mismatch ratio 为 `0.0`，overlay 区域覆盖与记录一致。
