# encrypted-traffic-evidence-center.png 逐图精拆记录

## 基本信息

- 分类：pages
- 标题：加密流量。
- 源图：`doc/04_assets/ui_suite_gpt_v1/screens/pages/encrypted-traffic-evidence-center.png`
- 源图尺寸：1920 x 1080
- 对应 prompt：`doc/04_assets/ui_suite_gpt_v1/prompts/encrypted-traffic.prompt.txt`
- 对应 manifest/layer：`doc/04_assets/ui_suite_gpt_v1/manifest.json`
- 对应路由/宿主路由：`/encrypted-traffic`
- 当前状态：`business-roi-rework-required-global-0125`；真实 API、四张证据 ECharts、五 Tab 共享几何、公共壳层复用、右侧业务控制组、单一证据 KPI 行、三列主工作区、选择联动、熵值/异常分契约和服务端审计请求已复验。r231 删除加密流量路由对主内容区和业务根容器的专属定位，使根业务区域与全局 `.taf-main` 内容起点严格对齐，同时恢复主栅格的全高可见状态。Windows Chrome 证明时间范围将 `start_time/end_time` 传给六个加密流量读接口，一键分析对非首条定位会话提交 `associate_analysis` 并返回 `200/action_id/recorded`，快速定位将会话表从 7 行收敛到目标 1 行。业务 ROI 仅评分 `content-root (260,80,1650,917)`；r231 的 `0.12906050692310234` 曾通过 `<0.13`，但已高于全局 `<0.125`，因此只允许在业务区回修，公共壳层保持不动。
- 复刻等级：逐图目标 PNG 已锁定；截图、overlay、diff 和 verification 由本轮脚本生成。
- 验收边界：像素证据只证明目标 PNG 复刻；生产 React 语义实现以本文和 JSON 为指导。

## 目标图观察

- 整体布局：Business AppShell page screenshot
- 业务重点：Topbar/sidebar/bottombar must remain aligned to screen.png while the central business area carries page-specific content.
- 当前页面/浮层状态：Current target state is the visible data-ready page state captured in the canonical PNG.
- 视觉基调：深海军蓝 SOC 指挥台，青蓝描边，低饱和面板，高密度文字与表格，状态色严格区分。
- 证据边界：Pixel acceptance proves exact target PNG reproduction by Windows Chrome screenshot and diff. Semantic production implementation remains guided by this breakdown record.
- 视觉读取方式：直接锁定目标 PNG，结合 prompt、layer JSON、manifest 与 Windows Chrome 截图证据校验。
- 坐标口径：所有 bbox 均以目标 PNG 左上角为原点，单位 px。

## 区域与坐标

坐标为本图拆解层的实现坐标，格式为 `x,y,w,h`。

| 区域 | bbox | 层级 | 说明 | 复刻要点 |
|---|---:|---:|---|---|
| 画布 | `0,0,1920,1080` | 0 | 1920x1080 单屏页面 | 不包含浏览器边框或外部装饰 |
| 顶部全局状态栏 | `0,0,1920,80` | 1 | 系统名、站点时间、运行指标与快捷入口 | 必须沿用 screen.png 公共区 |
| 左侧单栏导航 | `0,80,166,917` | 1 | 一级菜单和当前业务域二级菜单 | 不得恢复双栏导航 |
| 底部状态栏 | `0,997,1920,83` | 1 | 数据延迟、SLA、质量、存储、带宽、日志吞吐和全局动作 | 单层底部栏，右侧动作组固定 |
| 业务内容区 | `260,80,1650,917` | 1 | 页面业务工作区 | 从公共左侧导航右缘开始，按 12 栅格和 8px 间距组织 |
| 面包屑与上下文 | `198,96,1238,58` | 2 | 当前位置、对象上下文和状态摘要 | 不重复顶部站点/时间和用户信息 |
| 标题与主动作 | `198,154,1238,58` | 2 | 页面标题、主按钮、危险动作入口 | 危险动作需要权限、影响范围和审计提示 |
| 筛选工具栏 | `198,220,1238,58` | 2 | 搜索、筛选、时间窗和业务对象切换 | 控件高度稳定，不挤压标题 |
| 指标条 | `198,286,1238,104` | 2 | 本页面专属指标 | 指标名称不能与其他独立页面简单复用 |
| 主工作面板 | `198,398,818,366` | 2 | 主表格、图表、图谱或状态机 | 保持可扫描的信息密度 |
| 辅助面板 | `1024,398,412,366` | 2 | 证据、质量、排行或解释区 | 与主面板形成业务互补 |
| 下方明细区 | `198,772,1238,192` | 2 | 审计、证据、历史或闭环记录 | 行高约 32px，分页稳定 |
| 右侧闭环栏 | `1460,104,420,860` | 2 | 选中对象详情、处置动作、反馈学习和审计留痕 | 闭环动作不遮挡主工作区 |
| 右侧摘要 | `1484,128,372,168` | 3 | 对象状态、风险、Owner、Trace | 摘要字段固定对齐 |
| 右侧动作区 | `1484,312,372,172` | 3 | 处置、导出、反馈、审计 | 危险操作二次确认 |
| 右侧时间线 | `1484,500,372,430` | 3 | 事件进展和审计记录 | 时间线与底部状态栏不重叠 |

### 区域逐项复核

- 区域 `canvas`：位置 `0,0,1920,1080`。
  用途：1920x1080 单屏页面
  组件：Viewport
  视觉要求：不包含浏览器边框或外部装饰
- 区域 `topbar`：位置 `0,0,1920,80`。
  用途：系统名、站点时间、运行指标与快捷入口
  组件：AppHeader / SiteTimeSelector / QuickEntry
  视觉要求：必须沿用 screen.png 公共区
- 区域 `sidebar`：位置 `0,80,166,917`。
  用途：一级菜单和当前业务域二级菜单
  组件：PrimarySidebar / SecondaryMenu / UserMenu
  视觉要求：不得恢复双栏导航
- 区域 `bottombar`：位置 `0,997,1920,83`。
  用途：数据延迟、SLA、质量、存储、带宽、日志吞吐和全局动作
  组件：BottomStatusBar
  视觉要求：单层底部栏，右侧动作组固定
- 区域 `content-root`：位置 `260,80,1650,917`。
  用途：页面业务工作区
  组件：PageContent
  视觉要求：按 12 栅格和 8px 间距组织
- 区域 `breadcrumb-context`：位置 `198,96,1238,58`。
  用途：当前位置、对象上下文和状态摘要
  组件：BreadcrumbContext / StatusTag
  视觉要求：不重复顶部站点/时间和用户信息
- 区域 `page-title-actions`：位置 `198,154,1238,58`。
  用途：页面标题、主按钮、危险动作入口
  组件：PageTitle / Button / ActionRail
  视觉要求：危险动作需要权限、影响范围和审计提示
- 区域 `filter-toolbar`：位置 `198,220,1238,58`。
  用途：搜索、筛选、时间窗和业务对象切换
  组件：Search / Select / DateRange / Segmented
  视觉要求：控件高度稳定，不挤压标题
- 区域 `metric-strip`：位置 `198,286,1238,104`。
  用途：本页面专属指标
  组件：MetricTile / StatusTag
  视觉要求：指标名称不能与其他独立页面简单复用
- 区域 `primary-panel`：位置 `198,398,818,366`。
  用途：主表格、图表、图谱或状态机
  组件：WorkPanel / DataTable / ECharts
  视觉要求：保持可扫描的信息密度
- 区域 `secondary-panel`：位置 `1024,398,412,366`。
  用途：证据、质量、排行或解释区
  组件：WorkPanel / DescriptionList / RankingList
  视觉要求：与主面板形成业务互补
- 区域 `lower-panel`：位置 `198,772,1238,192`。
  用途：审计、证据、历史或闭环记录
  组件：DataTable / TimelineStateMachine / EvidenceFileCard
  视觉要求：行高约 32px，分页稳定
- 区域 `right-rail`：位置 `1460,104,420,860`。
  用途：选中对象详情、处置动作、反馈学习和审计留痕
  组件：DescriptionList / ActionRail / FeedbackBlock
  视觉要求：闭环动作不遮挡主工作区
- 区域 `right-rail-summary`：位置 `1484,128,372,168`。
  用途：对象状态、风险、Owner、Trace
  组件：DescriptionList / StatusTag
  视觉要求：摘要字段固定对齐
- 区域 `right-rail-actions`：位置 `1484,312,372,172`。
  用途：处置、导出、反馈、审计
  组件：Button / IconButton / Popconfirm
  视觉要求：危险操作二次确认
- 区域 `right-rail-timeline`：位置 `1484,500,372,430`。
  用途：事件进展和审计记录
  组件：TimelineStateMachine
  视觉要求：时间线与底部状态栏不重叠

## 文本清单

OCR 辅助结果以本表人工校正值为准；实现时关键文案按 `must_match` 执行。

| 文本 | 位置 | 类型 | 是否必须完全一致 |
|---|---|---|---|
| 加密流量。 | `80,54,360,24` | title | 是 |
| encrypted-traffic-evidence-center | `510,54,360,24` | subtitle | 是 |
| /encrypted-traffic | `940,54,360,24` | section | 是 |
| 对 TLS、QUIC、VPN、隧道和未知加密外联进行可解释分析。 | `1370,54,360,24` | label | 是 |
| Foundation 硬门禁：生成图必须严格遵守参考图中的 8 张 foundations 规范板，不能只是风格相似。 | `80,96,360,24` | metric | 是 |
| 必须锁定状态语义：健康/通过用绿色，信息/低危用蓝色，中危/待确认用黄色或琥珀色，高危/失败用红色；不得交换状态颜色。 | `510,96,360,24` | button | 是 |
| 如果生成结果偏离 foundations 的布局、色彩、字号、圆角、表格密度、图表样式或状态语义，应视为不合格并重新生成。 | `940,96,360,24` | status | 是 |
| 画布比例 16:9，目标 1920x1080 px，单屏完整展示，不要出现浏览器边框、手机外壳或营销落地页布局。 | `1370,96,360,24` | legend | 是 |
| 所有页面标题和卡片标题使用统一字号：主标题约 18px，面板标题约 16px，表格正文约 13px，辅助说明约 12px；标题不要忽大忽小。 | `80,138,360,24` | hint | 是 |
| 左侧一级菜单固定为：综合态势、采集监测、威胁分析、资产图谱、检测运营、审计配置；不要使用“看见、研判、取证、治理、验收”等非规范菜单词。 | `510,138,360,24` | acceptance | 是 |
| 左侧二级菜单显示：告警中心、战役列表、攻击链分析、加密流量、取证分析，当前高亮“加密流量”。 | `940,138,360,24` | title | 是 |
| 顶部状态条保留站点、时间、风险态势、告警总数、关键告警、采集健康度、数据质量、快捷入口等能力入口。 | `1370,138,360,24` | subtitle | 是 |
| 页面内容应体现园区网络全流量采集与分析业务闭环：采集接入、流式处理、资产识别、威胁检测、告警研判、证据取证、响应处置、反馈学习、审计验收。 | `80,180,360,24` | section | 是 |
| 不要改变最终参考图的整体视觉方向：不要改成浅色、不要做成插画海报、不要过度圆角、不要大面积渐变球或装饰光斑。 | `510,180,360,24` | label | 是 |
| 本图类型：完整页面。 | `940,180,360,24` | metric | 是 |
| 页面名称：加密流量。 | `1370,180,360,24` | button | 是 |
| 路由：/encrypted-traffic。 | `80,222,360,24` | status | 是 |
| 业务模块：威胁分析 / 加密流量。 | `510,222,360,24` | legend | 是 |
| 页面重点：对 TLS、QUIC、VPN、隧道和未知加密外联进行可解释分析。 | `940,222,360,24` | hint | 是 |
| 必须包含的业务模块： | `1370,222,360,24` | acceptance | 是 |
| - 加密流量总览：TLS/QUIC 比例、未知 SNI、异常证书、可疑 JA3、外联目的地。 | `80,264,360,24` | title | 是 |
| - 指纹分析：JA3/JA3S、证书 issuer、SNI、ALPN、TLS 版本、密码套件。 | `510,264,360,24` | subtitle | 是 |
| 园区网络全流量采集与分析系统 | `940,264,360,24` | section | 是 |
| 综合态势 | `1370,264,360,24` | label | 是 |
| 采集监测 | `80,306,360,24` | metric | 是 |
| 威胁分析 | `510,306,360,24` | button | 是 |
| 资产图谱 | `940,306,360,24` | status | 是 |
| 检测运营 | `1370,306,360,24` | legend | 是 |
| 审计配置 | `80,348,360,24` | hint | 是 |
| PCAP检索 | `510,348,360,24` | acceptance | 是 |
| 资产检索 | `940,348,360,24` | title | 是 |
| 规则检索 | `1370,348,360,24` | subtitle | 是 |
| 脚本中心 | `80,390,360,24` | section | 是 |
| 帮助中心 | `510,390,360,24` | label | 是 |
| 更多应用 | `940,390,360,24` | metric | 否 |
| 数据延迟 | `1370,390,360,24` | button | 否 |
| 系统运行 | `80,432,360,24` | status | 否 |
| 告警处理SLA | `510,432,360,24` | legend | 否 |
| 数据质量合格率 | `940,432,360,24` | hint | 否 |
| 存储使用 | `1370,432,360,24` | acceptance | 否 |
| 带宽使用 | `80,474,360,24` | title | 否 |
| 日志吞吐 | `510,474,360,24` | subtitle | 否 |
| 页面标题 | `940,474,360,24` | section | 否 |
| 筛选条件 | `1370,474,360,24` | label | 否 |
| 近24小时 | `80,516,360,24` | metric | 否 |
| 高危告警 | `510,516,360,24` | button | 否 |
| 证据完整率 | `940,516,360,24` | status | 否 |
| 处置中 | `1370,516,360,24` | legend | 否 |
| 已审计 | `80,558,360,24` | hint | 否 |
| 查看详情 | `510,558,360,24` | acceptance | 否 |
| 导出证据 | `940,558,360,24` | title | 否 |
| 反馈学习 | `1370,558,360,24` | subtitle | 否 |

### 文本人工校正说明

- 文本 01：`加密流量。`，类型 title，来源 prompt/layer plus target visual ledger。
- 文本 02：`encrypted-traffic-evidence-center`，类型 subtitle，来源 prompt/layer plus target visual ledger。
- 文本 03：`/encrypted-traffic`，类型 section，来源 prompt/layer plus target visual ledger。
- 文本 04：`对 TLS、QUIC、VPN、隧道和未知加密外联进行可解释分析。`，类型 label，来源 prompt/layer plus target visual ledger。
- 文本 05：`Foundation 硬门禁：生成图必须严格遵守参考图中的 8 张 foundations 规范板，不能只是风格相似。`，类型 metric，来源 prompt/layer plus target visual ledger。
- 文本 06：`必须锁定状态语义：健康/通过用绿色，信息/低危用蓝色，中危/待确认用黄色或琥珀色，高危/失败用红色；不得交换状态颜色。`，类型 button，来源 prompt/layer plus target visual ledger。
- 文本 07：`如果生成结果偏离 foundations 的布局、色彩、字号、圆角、表格密度、图表样式或状态语义，应视为不合格并重新生成。`，类型 status，来源 prompt/layer plus target visual ledger。
- 文本 08：`画布比例 16:9，目标 1920x1080 px，单屏完整展示，不要出现浏览器边框、手机外壳或营销落地页布局。`，类型 legend，来源 prompt/layer plus target visual ledger。
- 文本 09：`所有页面标题和卡片标题使用统一字号：主标题约 18px，面板标题约 16px，表格正文约 13px，辅助说明约 12px；标题不要忽大忽小。`，类型 hint，来源 prompt/layer plus target visual ledger。
- 文本 10：`左侧一级菜单固定为：综合态势、采集监测、威胁分析、资产图谱、检测运营、审计配置；不要使用“看见、研判、取证、治理、验收”等非规范菜单词。`，类型 acceptance，来源 prompt/layer plus target visual ledger。
- 文本 11：`左侧二级菜单显示：告警中心、战役列表、攻击链分析、加密流量、取证分析，当前高亮“加密流量”。`，类型 title，来源 prompt/layer plus target visual ledger。
- 文本 12：`顶部状态条保留站点、时间、风险态势、告警总数、关键告警、采集健康度、数据质量、快捷入口等能力入口。`，类型 subtitle，来源 prompt/layer plus target visual ledger。
- 文本 13：`页面内容应体现园区网络全流量采集与分析业务闭环：采集接入、流式处理、资产识别、威胁检测、告警研判、证据取证、响应处置、反馈学习、审计验收。`，类型 section，来源 prompt/layer plus target visual ledger。
- 文本 14：`不要改变最终参考图的整体视觉方向：不要改成浅色、不要做成插画海报、不要过度圆角、不要大面积渐变球或装饰光斑。`，类型 label，来源 prompt/layer plus target visual ledger。
- 文本 15：`本图类型：完整页面。`，类型 metric，来源 prompt/layer plus target visual ledger。
- 文本 16：`页面名称：加密流量。`，类型 button，来源 prompt/layer plus target visual ledger。
- 文本 17：`路由：/encrypted-traffic。`，类型 status，来源 prompt/layer plus target visual ledger。
- 文本 18：`业务模块：威胁分析 / 加密流量。`，类型 legend，来源 prompt/layer plus target visual ledger。
- 文本 19：`页面重点：对 TLS、QUIC、VPN、隧道和未知加密外联进行可解释分析。`，类型 hint，来源 manual visual ledger。
- 文本 20：`必须包含的业务模块：`，类型 acceptance，来源 manual visual ledger。
- 文本 21：`- 加密流量总览：TLS/QUIC 比例、未知 SNI、异常证书、可疑 JA3、外联目的地。`，类型 title，来源 manual visual ledger。
- 文本 22：`- 指纹分析：JA3/JA3S、证书 issuer、SNI、ALPN、TLS 版本、密码套件。`，类型 subtitle，来源 manual visual ledger。
- 文本 23：`园区网络全流量采集与分析系统`，类型 section，来源 manual visual ledger。
- 文本 24：`综合态势`，类型 label，来源 manual visual ledger。
- 文本 25：`采集监测`，类型 metric，来源 manual visual ledger。
- 文本 26：`威胁分析`，类型 button，来源 manual visual ledger。
- 文本 27：`资产图谱`，类型 status，来源 manual visual ledger。
- 文本 28：`检测运营`，类型 legend，来源 manual visual ledger。
- 文本 29：`审计配置`，类型 hint，来源 manual visual ledger。
- 文本 30：`PCAP检索`，类型 acceptance，来源 manual visual ledger。
- 文本 31：`资产检索`，类型 title，来源 manual visual ledger。
- 文本 32：`规则检索`，类型 subtitle，来源 manual visual ledger。
- 文本 33：`脚本中心`，类型 section，来源 manual visual ledger。
- 文本 34：`帮助中心`，类型 label，来源 manual visual ledger。
- 文本 35：`更多应用`，类型 metric，来源 manual visual ledger。
- 文本 36：`数据延迟`，类型 button，来源 manual visual ledger。

## 组件清单

| 区域 | 组件/元素 | 实现方式 | 状态 | 备注 |
|---|---|---|---|---|
| `canvas` | `AppHeader` | web/ui/src/components/AppHeader.tsx | default | AppHeader must keep stable dimensions and match the recorded bbox/token mapping. |
| `primary-section` | `PrimarySidebar` | web/ui/src/components/PrimarySidebar.tsx | interactive | PrimarySidebar must keep stable dimensions and match the recorded bbox/token mapping. |
| `primary-section` | `BottomStatusBar` | web/ui/src/components/BottomStatusBar.tsx | data-ready | BottomStatusBar must keep stable dimensions and match the recorded bbox/token mapping. |
| `primary-section` | `BreadcrumbContext` | web/ui/src/components/BreadcrumbContext.tsx | default | BreadcrumbContext must keep stable dimensions and match the recorded bbox/token mapping. |
| `state-section` | `Search` | web/ui/src/components/Search.tsx | interactive | Search must keep stable dimensions and match the recorded bbox/token mapping. |
| `state-section` | `DateRange` | web/ui/src/components/DateRange.tsx | data-ready | DateRange must keep stable dimensions and match the recorded bbox/token mapping. |
| `state-section` | `MetricTile` | web/ui/src/components/MetricTile.tsx | default | MetricTile must keep stable dimensions and match the recorded bbox/token mapping. |
| `state-section` | `DataTable` | web/ui/src/components/DataTable.tsx | interactive | DataTable must keep stable dimensions and match the recorded bbox/token mapping. |
| `implementation-section` | `EChartsPanel` | ECharts option builder under web/ui/src/components | data-ready | EChartsPanel must keep stable dimensions and match the recorded bbox/token mapping. |
| `implementation-section` | `RightRail` | web/ui/src/components/RightRail.tsx | default | RightRail must keep stable dimensions and match the recorded bbox/token mapping. |
| `implementation-section` | `ActionRail` | web/ui/src/components/ActionRail.tsx | interactive | ActionRail must keep stable dimensions and match the recorded bbox/token mapping. |
| `implementation-section` | `FeedbackBlock` | web/ui/src/components/FeedbackBlock.tsx | data-ready | FeedbackBlock must keep stable dimensions and match the recorded bbox/token mapping. |

- `AppHeader` 映射到 web/ui/src/components/AppHeader.tsx。
  状态口径：default
  复核点：AppHeader must keep stable dimensions and match the recorded bbox/token mapping.
- `PrimarySidebar` 映射到 web/ui/src/components/PrimarySidebar.tsx。
  状态口径：interactive
  复核点：PrimarySidebar must keep stable dimensions and match the recorded bbox/token mapping.
- `BottomStatusBar` 映射到 web/ui/src/components/BottomStatusBar.tsx。
  状态口径：data-ready
  复核点：BottomStatusBar must keep stable dimensions and match the recorded bbox/token mapping.
- `BreadcrumbContext` 映射到 web/ui/src/components/BreadcrumbContext.tsx。
  状态口径：default
  复核点：BreadcrumbContext must keep stable dimensions and match the recorded bbox/token mapping.
- `Search` 映射到 web/ui/src/components/Search.tsx。
  状态口径：interactive
  复核点：Search must keep stable dimensions and match the recorded bbox/token mapping.
- `DateRange` 映射到 web/ui/src/components/DateRange.tsx。
  状态口径：data-ready
  复核点：DateRange must keep stable dimensions and match the recorded bbox/token mapping.
- `MetricTile` 映射到 web/ui/src/components/MetricTile.tsx。
  状态口径：default
  复核点：MetricTile must keep stable dimensions and match the recorded bbox/token mapping.
- `DataTable` 映射到 web/ui/src/components/DataTable.tsx。
  状态口径：interactive
  复核点：DataTable must keep stable dimensions and match the recorded bbox/token mapping.
- `EChartsPanel` 映射到 ECharts option builder under web/ui/src/components。
  状态口径：data-ready
  复核点：EChartsPanel must keep stable dimensions and match the recorded bbox/token mapping.
- `RightRail` 映射到 web/ui/src/components/RightRail.tsx。
  状态口径：default
  复核点：RightRail must keep stable dimensions and match the recorded bbox/token mapping.
- `ActionRail` 映射到 web/ui/src/components/ActionRail.tsx。
  状态口径：interactive
  复核点：ActionRail must keep stable dimensions and match the recorded bbox/token mapping.
- `FeedbackBlock` 映射到 web/ui/src/components/FeedbackBlock.tsx。
  状态口径：data-ready
  复核点：FeedbackBlock must keep stable dimensions and match the recorded bbox/token mapping.

## 图标清单

| 位置 | 图标 | 图标库/实现 | 语义 | 是否需自绘 |
|---|---|---|---|---|
| breadcrumb/context header | `HomeOutlined` | Ant Design Icons | 一级位置或业务域入口 | 否 |
| breadcrumb/context header | `RightOutlined` | Ant Design Icons | 面包屑层级分隔 | 否 |
| breadcrumb/context header | `ReloadOutlined` | Ant Design Icons | 刷新上下文 | 否 |
| toolbar/action area | `CopyOutlined` | Ant Design Icons | 复制路径或对象 ID | 否 |
| toolbar/action area | `WarningOutlined` | Ant Design Icons | 风险/异常 | 否 |
| toolbar/action area | `CheckCircleOutlined` | Ant Design Icons | 通过/健康 | 否 |
| status/audit area | `DatabaseOutlined` | Ant Design Icons | 资产/证据对象 | 否 |
| status/audit area | `AuditOutlined` | Ant Design Icons | 审计留痕 | 否 |
| status/audit area | `FilterOutlined` | Ant Design Icons | 筛选上下文 | 否 |
| status/audit area | `DownloadOutlined` | Ant Design Icons | 导出/下载证据 | 否 |

- 图标 01：`HomeOutlined` 位于 `120,146,24,24`，语义为 一级位置或业务域入口。
- 图标 02：`RightOutlined` 位于 `194,194,24,24`，语义为 面包屑层级分隔。
- 图标 03：`ReloadOutlined` 位于 `268,146,24,24`，语义为 刷新上下文。
- 图标 04：`CopyOutlined` 位于 `342,194,24,24`，语义为 复制路径或对象 ID。
- 图标 05：`WarningOutlined` 位于 `416,146,24,24`，语义为 风险/异常。
- 图标 06：`CheckCircleOutlined` 位于 `490,194,24,24`，语义为 通过/健康。
- 图标 07：`DatabaseOutlined` 位于 `564,146,24,24`，语义为 资产/证据对象。
- 图标 08：`AuditOutlined` 位于 `638,194,24,24`，语义为 审计留痕。
- 图标 09：`FilterOutlined` 位于 `712,146,24,24`，语义为 筛选上下文。
- 图标 10：`DownloadOutlined` 位于 `786,194,24,24`，语义为 导出/下载证据。

## Token 与样式

| 项 | 值 | 来源 | 备注 |
|---|---|---|---|
| `page-bg` | `#03111c` | foundation-color-status | 页面底色 |
| `shell-bg` | `#061827` | foundation-color-status | 顶部/左侧/底部框架底色 |
| `panel-bg` | `rgba(6,28,43,0.86)` | foundation-color-status | 业务面板底色 |
| `panel-strong-bg` | `#071f32` | foundation-color-status | 强调面板底色 |
| `border-weak` | `rgba(56,151,201,0.22)` | foundation-color-status | 弱描边 |
| `active-blue` | `#1e9cff` | foundation-color-status | 激活态、链接、主按钮 |
| `text-primary` | `#eaf7ff` | foundation-color-status | 主文字 |
| `text-secondary` | `#9db9c9` | foundation-color-status | 次级文字 |
| `text-muted` | `#5e7b8d` | foundation-color-status | 弱文字 |
| `success` | `#36d66b` | foundation-color-status | 健康/通过 |
| `info` | `#18a8ff` | foundation-color-status | 信息/低危 |
| `warning` | `#ffb020` | foundation-color-status | 中危/需确认 |
| `danger` | `#ff4d4f` | foundation-color-status | 高危/失败 |
| `critical` | `#ff2d2d` | foundation-color-status | 严重/关键 |
| `panel-radius` | `6px` | foundation-layout-density | 面板圆角 |
| `control-radius` | `4px` | foundation-layout-density | 按钮/输入圆角 |
| `table-row-height` | `32px` | foundation-layout-density | 高密度表格行高 |
| `panel-gap` | `8px` | foundation-layout-density | 业务区面板间距 |

### Token 实现约束

- `page-bg`：值 `#03111c`，用于 页面底色。
- `shell-bg`：值 `#061827`，用于 顶部/左侧/底部框架底色。
- `panel-bg`：值 `rgba(6,28,43,0.86)`，用于 业务面板底色。
- `panel-strong-bg`：值 `#071f32`，用于 强调面板底色。
- `border-weak`：值 `rgba(56,151,201,0.22)`，用于 弱描边。
- `active-blue`：值 `#1e9cff`，用于 激活态、链接、主按钮。
- `text-primary`：值 `#eaf7ff`，用于 主文字。
- `text-secondary`：值 `#9db9c9`，用于 次级文字。
- `text-muted`：值 `#5e7b8d`，用于 弱文字。
- `success`：值 `#36d66b`，用于 健康/通过。
- `info`：值 `#18a8ff`，用于 信息/低危。
- `warning`：值 `#ffb020`，用于 中危/需确认。
- `danger`：值 `#ff4d4f`，用于 高危/失败。
- `critical`：值 `#ff2d2d`，用于 严重/关键。
- `panel-radius`：值 `6px`，用于 面板圆角。
- `control-radius`：值 `4px`，用于 按钮/输入圆角。
- `table-row-height`：值 `32px`，用于 高密度表格行高。
- `panel-gap`：值 `8px`，用于 业务区面板间距。
- 字体密度：产品标题约 24px，页面标题 18-20px，面板标题 15-16px，表格正文 12-13px。
- 间距密度：面板间距 8px，内部栅格按 8px 倍数，按钮圆角 4px，面板圆角 6px。
- 图表样式：ECharts 深色透明背景，网格线低透明青色，图例不能遮挡标题。

## 状态与交互

| 控件/区域 | 状态 | 触发方式 | 期望表现 |
|---|---|---|---|
| `target-loaded` | ready | open image route in Windows Chrome CDP | 1920x1080 content is visible without browser chrome or scroll shift |
| `hover-primary-control` | hover | pointer hover on primary action or chip | border/text/background changes follow active-blue token and layout size remains stable |
| `keyboard-focus` | focus-visible | Tab key moves to first interactive control | focus ring is visible and does not move neighboring text |
| `disabled-control` | disabled | permission or missing-selection state | control is dimmed, non-clickable, and still readable |
| `loading-state` | loading | data refresh or submit starts | spinner/skeleton appears inside fixed container |
| `error-state` | error | request failure or validation failure | danger token appears with action guidance and trace/audit wording |
| `selected-context` | selected | row/node/chip selected | selected state is visually distinct and updates right rail or context bar |
| `danger-action-confirm` | confirming | dangerous action clicked | confirmation includes permission, impact scope, and audit trace |

- 交互 `target-loaded`：状态 ready。
  触发：open image route in Windows Chrome CDP
  期望：1920x1080 content is visible without browser chrome or scroll shift
- 交互 `hover-primary-control`：状态 hover。
  触发：pointer hover on primary action or chip
  期望：border/text/background changes follow active-blue token and layout size remains stable
- 交互 `keyboard-focus`：状态 focus-visible。
  触发：Tab key moves to first interactive control
  期望：focus ring is visible and does not move neighboring text
- 交互 `disabled-control`：状态 disabled。
  触发：permission or missing-selection state
  期望：control is dimmed, non-clickable, and still readable
- 交互 `loading-state`：状态 loading。
  触发：data refresh or submit starts
  期望：spinner/skeleton appears inside fixed container
- 交互 `error-state`：状态 error。
  触发：request failure or validation failure
  期望：danger token appears with action guidance and trace/audit wording
- 交互 `selected-context`：状态 selected。
  触发：row/node/chip selected
  期望：selected state is visually distinct and updates right rail or context bar
- 交互 `danger-action-confirm`：状态 confirming。
  触发：dangerous action clicked
  期望：confirmation includes permission, impact scope, and audit trace

## 实现映射

- 参考实现模式：语义 React 生产页面，真实 API 优先，空集合才使用类型化仿真回退。
- 页面路由：`/encrypted-traffic`
- 页面映射：`route:/encrypted-traffic`
- 服务映射：`web/ui/src/services/api.ts`
- 样式映射：`web/ui/src/styles/tokens.css`、`Ant Design theme override`、`ECharts dark theme tokens`
- 映射说明：`EvidenceCenterContent` 使用 `/v1/encrypted-traffic/evidence` 的 Session、PCAP 索引和时间桶；`PcapPacketTrendChart` 与 `EvidenceClosureRingChart` 是 ECharts 组件。选中的 Session 同步到证书详情、握手元数据和右侧审计动作。当前线上三类主集合为空时，页面明确显示“仿真数据（API 空）”。

| 前端单位 | 映射方式 |
|---|---|
| `AppHeader` | web/ui/src/components/AppHeader.tsx |
| `PrimarySidebar` | web/ui/src/components/PrimarySidebar.tsx |
| `BottomStatusBar` | web/ui/src/components/BottomStatusBar.tsx |
| `BreadcrumbContext` | web/ui/src/components/BreadcrumbContext.tsx |
| `Search` | web/ui/src/components/Search.tsx |
| `DateRange` | web/ui/src/components/DateRange.tsx |
| `MetricTile` | web/ui/src/components/MetricTile.tsx |
| `DataTable` | web/ui/src/components/DataTable.tsx |
| `EChartsPanel` | ECharts option builder under web/ui/src/components |
| `RightRail` | web/ui/src/components/RightRail.tsx |
| `ActionRail` | web/ui/src/components/ActionRail.tsx |
| `FeedbackBlock` | web/ui/src/components/FeedbackBlock.tsx |

## 验收证据

- 目标图：`evidence/ui-image-breakdowns/pages/encrypted-traffic-evidence-center/target.png`
- 实现截图：`evidence/ui-image-breakdowns/pages/encrypted-traffic-evidence-center/implementation.png`
- diff 图：`evidence/ui-image-breakdowns/pages/encrypted-traffic-evidence-center/diff.png`
- regions overlay：`evidence/ui-image-breakdowns/pages/encrypted-traffic-evidence-center/regions-overlay.png`
- measurement：`evidence/ui-image-breakdowns/pages/encrypted-traffic-evidence-center/measurement.json`
- OCR/manual ledger：`evidence/ui-image-breakdowns/pages/encrypted-traffic-evidence-center/text-ocr.txt`
- metrics：`evidence/ui-image-breakdowns/pages/encrypted-traffic-evidence-center/metrics.json`
- verification：`evidence/ui-image-breakdowns/pages/encrypted-traffic-evidence-center/verification.json`
- r206 稳定 Windows Chrome 运行证据：`evidence/ui-image-breakdowns/pages/encrypted-traffic-evidence-center/normal-route-r206-stable-runtime.json`；两次 11 秒等待后的截图均为 `0.10848717206790123`，五个 Tab 均为固定 `88 x 29.99px`，选中 `s-23a9b7d4c1e8` 后内侧证据锚点与右侧快捷定位同步更新。
- r207 API 契约运行证据：`evidence/ui-image-breakdowns/pages/encrypted-traffic-evidence-center/normal-route-r207-api-contract-runtime.json`；Payload entropy 不可用时 API 显式返回 `entropy_available=false`，并与 `anomaly_trend` 分离。
- r211 五 Tab 共享布局运行证据：`evidence/ui-image-breakdowns/pages/encrypted-traffic-evidence-center/normal-route-r211-five-tab-layout-runtime.json`；Windows Chrome 实际点击总览、指纹分析、隧道检测、外联画像、证据中心后，标题栏、KPI、主区、右栏及五个按钮轨道坐标完全一致，证据中心长内容仅在主区内部滚动。
- r212 业务控制组运行证据：`evidence/ui-image-breakdowns/pages/encrypted-traffic-evidence-center/normal-route-r212-business-controls-runtime.json`；公共 topbar/sidebar/bottombar 与仪表盘几何一致，五个 Tab 的业务控制组固定于标题栏右侧，顺序为时间范围、刷新、一键分析、指纹详情、证书详情，指纹详情 Drawer 已实际打开并关闭。
- r212 严格像素运行证据：`evidence/ui-image-breakdowns/pages/encrypted-traffic-evidence-center/normal-route-r212-strict-pixel-runtime.json`；两次 Windows Chrome 生产捕获稳定于 `0.121069–0.121071`，业务阈值 `0.35` 通过，严格目标 `0.015` 未通过。公共壳层差异按用户约束记录，业务主区和证据栏仍为后续回修重点。
- r214 业务回修运行证据：`evidence/ui-image-breakdowns/pages/encrypted-traffic-evidence-center/normal-route-r214-business-rework-runtime.json`；重复 KPI 已合并，三块主面板同排、底部详情位于业务区单屏、PCAP 近场动作与时间范围/一键分析均已在 Windows Chrome 复验。两次生产 capture 为 `0.120231–0.120240`，业务阈值通过，严格像素仍未接受。
- r219 业务 ROI 严格像素证据：`evidence/ui-image-breakdowns/pages/encrypted-traffic-evidence-center/capture-meta.json`、`metrics.json` 和 `diff.png`；Windows Chrome 生产截图以 `content-root (260,80,1650,917)` 的 `1,513,050` 像素作为唯一评分范围，`0.13239945804831302` 对严格目标 `<=0.015` 未通过。整图 `0.11728636188271604` 仅保留为诊断值，公共 topbar/sidebar/bottombar 不参与评分或红色 diff 覆盖。
- r219 验收包：`doc/02_acceptance/02-regression/ui-visual-interaction/encrypted-traffic-evidence-center-r219/`
- r207 验收包：`doc/02_acceptance/02-regression/ui-visual-interaction/encrypted-traffic-evidence-center-r207/`
- r194 历史 ECharts、选择联动与审计写回：`evidence/ui-image-breakdowns/pages/encrypted-traffic-evidence-center/normal-route-r194-echarts-interactions-runtime.json`
- r194 历史真实证据 API：`evidence/ui-image-breakdowns/pages/encrypted-traffic-evidence-center/r194-evidence-api-runtime.json`
- r194 历史业务验收包：`doc/02_acceptance/02-regression/ui-visual-interaction/encrypted-traffic-evidence-center-r194/`
- r195 模块图表复验：`evidence/ui-image-breakdowns/pages/encrypted-traffic-evidence-center/normal-route-r195-module-echarts-runtime.json`，总览的协议/趋势与 JA3 散点、指纹分析的 JA3 散点、隧道检测的 JA3 散点与心跳序列均由 ECharts canvas 渲染。
- 视口：1920 x 1080，DPR 1
- 浏览器：Windows Chrome CDP，经 `http://127.0.0.1:9224/json/version` 与 `/json/list` 预检。
- 复现步骤：锁定 target.png，打开 APISIX 生产路由 `/encrypted-traffic?tab=evidence-center`，使用 Windows Chrome CDP 截图，生成 diff.png 和 metrics.json，读取 verification.json。

## 差异清单

| 类型 | 位置 | 当前 | 期望 | 状态 |
|---|---|---|---|---|
| business-visual-diff | `content-root (260,80,1650,917)` | r219 Windows Chrome ROI ratio `0.13239945804831302` | `<= 0.35` at tolerance `64` | pass |
| strict-pixel-diff | `content-root (260,80,1650,917)` | r219 ROI ratio `0.13239945804831302` | `<= 0.015` at tolerance `64` | rework-required |
| shared-shell-baseline | topbar/sidebar/bottombar | public shell deliberately matches dashboard under user instruction | target PNG uses a different public shell composition | excluded-from-strict-score |
| API-first data | `/api/v1/encrypted-traffic/evidence` | r207 returns 200 and当前 Session、PCAP、PCAP 趋势和 payload entropy 主集合为空 | fields map directly when rows arrive; all-empty mode is visibly labeled | documented-nonblocking |
| entropy semantics | `/api/v1/encrypted-traffic/evidence` | `entropy_available=false` and `entropy_trend=[]`; `anomaly_trend` is separate | Payload entropy must never be fabricated from anomaly scores | pass |
| forensic-task-dispatch | `/api/v1/encrypted-traffic/evidence-actions` | 保全等动作已持久化请求和审计事件，但未连接外部取证任务服务 | 外部任务投递和终态应作为独立集成呈现 | documented-follow-up |

## 结论

- 逐图拆解层已记录区域、文本、组件、图标、token、交互、实现映射和证据路径。
- r214 将证据 KPI 合并到共享的单一业务 KPI 行，使会话表、PCAP 索引、证据锚点三块主面板同排，并使底部详情保持在业务区单屏可见；公共 topbar/sidebar/bottombar 不再为本页特化。
- 五个 Tab 的位置、大小和间距固定；PCAP 趋势、完整度环图、会话熵分和右栏完整度均由 ECharts canvas 绘制；默认详情保留真实适配的 Issuer 与 ClientHello。
- 选择 `s-23a9b7d4c1e8` 后，内侧证据锚点与右侧快捷定位同步切换；运行期间没有控制台、页面、请求或横向溢出错误。保全操作以“请求已写入审计”的真实语义呈现，未伪称外部任务已完成。
- 独立审查确认，公共壳层差异不属于本页业务开发范围。r219 保持比较器的 ROI 规则：公共壳层只保留在整图诊断中，不会影响严格像素结果；后续只在右侧证据栏、会话表密度、底部摘要和总体 KPI 的业务区范围内继续收敛。
- 业务视觉门禁使用 `0.35` 阈值通过；严格像素门禁在相同的 `content-root` ROI 上以 `0.015` 为目标，本页尚未获得 `pixel-accepted` 判定。
- 主线程结论区分生产语义验收与严格像素验收，不会以仿真回退或宽松阈值伪称实时或像素完成。
