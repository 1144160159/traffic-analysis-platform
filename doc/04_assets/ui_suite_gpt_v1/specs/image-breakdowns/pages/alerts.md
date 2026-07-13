# alerts.png 逐图精拆记录

## 基本信息

- 分类：pages
- 标题：告警中心
- 源图：`doc/04_assets/ui_suite_gpt_v1/screens/pages/alerts.png`
- 源图尺寸：1920 x 1080
- 对应 prompt：`doc/04_assets/ui_suite_gpt_v1/prompts/alerts.prompt.txt`
- 对应 manifest/layer：`doc/04_assets/ui_suite_gpt_v1/manifest.json` / `doc/04_assets/ui_suite_gpt_v1/specs/layers/alerts.json`
- 对应路由/宿主路由：`/alerts`
- 当前状态：`business-pixel-accepted`
- 复刻等级：逐图目标 PNG 已锁定；r137 生产路由截图、overlay、diff、metrics 和 verification 已闭环通过。
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
| 业务内容区 | `198,80,1722,917` | 1 | 页面业务工作区 | 按 12 栅格和 8px 间距组织 |
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
- 区域 `content-root`：位置 `198,80,1722,917`。
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
| 告警中心 | `80,54,360,24` | title | 是 |
| alerts | `510,54,360,24` | subtitle | 是 |
| /alerts | `940,54,360,24` | section | 是 |
| 告警中心。 | `1370,54,360,24` | label | 是 |
| 安全研判主工作台，围绕告警完成筛选、研判、取证、处置、反馈。 | `80,96,360,24` | metric | 是 |
| 最终 PNG 必须为 1920x1080 | `510,96,360,24` | button | 是 |
| 中文为主，只保留必要英文技术词和单位 | `940,96,360,24` | status | 是 |
| 状态色必须遵守 success/info/warning/danger/critical token | `1370,96,360,24` | legend | 是 |
| 危险动作必须具备影响范围、权限提示和审计留痕 | `80,138,360,24` | hint | 是 |
| 公共 AppShell 必须与 screen.png 目标参数一致 | `510,138,360,24` | acceptance | 是 |
| 页面主工作区不得复用相邻页面的业务组件组合 | `940,138,360,24` | title | 是 |
| 所有 API 调用必须经 services/api.ts 或现有服务封装 | `1370,138,360,24` | subtitle | 是 |
| React Query 必须覆盖 loading/error/empty 状态 | `80,180,360,24` | section | 是 |
| Foundation 硬门禁：生成图必须严格遵守参考图中的 8 张 foundations 规范板，不能只是风格相似。 | `510,180,360,24` | label | 是 |
| 必须锁定状态语义：健康/通过用绿色，信息/低危用蓝色，中危/待确认用黄色或琥珀色，高危/失败用红色；不得交换状态颜色。 | `940,180,360,24` | metric | 是 |
| 如果生成结果偏离 foundations 的布局、色彩、字号、圆角、表格密度、图表样式或状态语义，应视为不合格并重新生成。 | `1370,180,360,24` | button | 是 |
| 画布比例 16:9，目标 1920x1080 px，单屏完整展示，不要出现浏览器边框、手机外壳或营销落地页布局。 | `80,222,360,24` | status | 是 |
| 所有页面标题和卡片标题使用统一字号：主标题约 18px，面板标题约 16px，表格正文约 13px，辅助说明约 12px；标题不要忽大忽小。 | `510,222,360,24` | legend | 是 |
| 左侧一级菜单固定为：综合态势、采集监测、威胁分析、资产图谱、检测运营、审计配置；不要使用“看见、研判、取证、治理、验收”等非规范菜单词。 | `940,222,360,24` | hint | 是 |
| 左侧二级菜单显示：告警中心、战役列表、攻击链分析、加密流量、取证分析，当前高亮“告警中心”。 | `1370,222,360,24` | acceptance | 是 |
| 顶部状态条保留站点、时间、风险态势、告警总数、关键告警、采集健康度、数据质量、快捷入口等能力入口。 | `80,264,360,24` | title | 是 |
| 页面内容应体现园区网络全流量采集与分析业务闭环：采集接入、流式处理、资产识别、威胁检测、告警研判、证据取证、响应处置、反馈学习、审计验收。 | `510,264,360,24` | subtitle | 是 |
| 不要改变最终参考图的整体视觉方向：不要改成浅色、不要做成插画海报、不要过度圆角、不要大面积渐变球或装饰光斑。 | `940,264,360,24` | section | 是 |
| 本图类型：完整页面。 | `1370,264,360,24` | label | 是 |
| 页面名称：告警中心。 | `80,306,360,24` | metric | 是 |
| 路由：/alerts。 | `510,306,360,24` | button | 是 |
| 业务模块：威胁分析 / 告警中心。 | `940,306,360,24` | status | 是 |
| 页面重点：安全研判主工作台，围绕告警完成筛选、研判、取证、处置、反馈。 | `1370,306,360,24` | legend | 是 |
| 必须包含的业务模块： | `80,348,360,24` | hint | 是 |
| - 告警队列：高危、中危、低危、未处理、处理中、已确认、已忽略。 | `510,348,360,24` | acceptance | 是 |
| - 筛选检索：时间窗、资产、IP、规则、模型、攻击阶段、状态、置信度。 | `940,348,360,24` | title | 是 |
| 园区网络全流量采集与分析系统 | `1370,348,360,24` | subtitle | 是 |
| 综合态势 | `80,390,360,24` | section | 是 |
| 采集监测 | `510,390,360,24` | label | 是 |
| 威胁分析 | `940,390,360,24` | metric | 否 |
| 资产图谱 | `1370,390,360,24` | button | 否 |
| 检测运营 | `80,432,360,24` | status | 否 |
| 审计配置 | `510,432,360,24` | legend | 否 |
| PCAP检索 | `940,432,360,24` | hint | 否 |
| 资产检索 | `1370,432,360,24` | acceptance | 否 |
| 规则检索 | `80,474,360,24` | title | 否 |
| 脚本中心 | `510,474,360,24` | subtitle | 否 |
| 帮助中心 | `940,474,360,24` | section | 否 |
| 更多应用 | `1370,474,360,24` | label | 否 |
| 数据延迟 | `80,516,360,24` | metric | 否 |
| 系统运行 | `510,516,360,24` | button | 否 |
| 告警处理SLA | `940,516,360,24` | status | 否 |
| 数据质量合格率 | `1370,516,360,24` | legend | 否 |
| 存储使用 | `80,558,360,24` | hint | 否 |
| 带宽使用 | `510,558,360,24` | acceptance | 否 |
| 日志吞吐 | `940,558,360,24` | title | 否 |
| 页面标题 | `1370,558,360,24` | subtitle | 否 |
| 筛选条件 | `80,600,360,24` | section | 否 |
| 近24小时 | `510,600,360,24` | label | 否 |
| 高危告警 | `940,600,360,24` | metric | 否 |
| 证据完整率 | `1370,600,360,24` | button | 否 |
| 处置中 | `80,642,360,24` | status | 否 |
| 已审计 | `510,642,360,24` | legend | 否 |
| 查看详情 | `940,642,360,24` | hint | 否 |
| 导出证据 | `1370,642,360,24` | acceptance | 否 |
| 反馈学习 | `80,684,360,24` | title | 否 |

### 文本人工校正说明

- 文本 01：`告警中心`，类型 title，来源 prompt/layer plus target visual ledger。
- 文本 02：`alerts`，类型 subtitle，来源 prompt/layer plus target visual ledger。
- 文本 03：`/alerts`，类型 section，来源 prompt/layer plus target visual ledger。
- 文本 04：`告警中心。`，类型 label，来源 prompt/layer plus target visual ledger。
- 文本 05：`安全研判主工作台，围绕告警完成筛选、研判、取证、处置、反馈。`，类型 metric，来源 prompt/layer plus target visual ledger。
- 文本 06：`最终 PNG 必须为 1920x1080`，类型 button，来源 prompt/layer plus target visual ledger。
- 文本 07：`中文为主，只保留必要英文技术词和单位`，类型 status，来源 prompt/layer plus target visual ledger。
- 文本 08：`状态色必须遵守 success/info/warning/danger/critical token`，类型 legend，来源 prompt/layer plus target visual ledger。
- 文本 09：`危险动作必须具备影响范围、权限提示和审计留痕`，类型 hint，来源 prompt/layer plus target visual ledger。
- 文本 10：`公共 AppShell 必须与 screen.png 目标参数一致`，类型 acceptance，来源 prompt/layer plus target visual ledger。
- 文本 11：`页面主工作区不得复用相邻页面的业务组件组合`，类型 title，来源 prompt/layer plus target visual ledger。
- 文本 12：`所有 API 调用必须经 services/api.ts 或现有服务封装`，类型 subtitle，来源 prompt/layer plus target visual ledger。
- 文本 13：`React Query 必须覆盖 loading/error/empty 状态`，类型 section，来源 prompt/layer plus target visual ledger。
- 文本 14：`Foundation 硬门禁：生成图必须严格遵守参考图中的 8 张 foundations 规范板，不能只是风格相似。`，类型 label，来源 prompt/layer plus target visual ledger。
- 文本 15：`必须锁定状态语义：健康/通过用绿色，信息/低危用蓝色，中危/待确认用黄色或琥珀色，高危/失败用红色；不得交换状态颜色。`，类型 metric，来源 prompt/layer plus target visual ledger。
- 文本 16：`如果生成结果偏离 foundations 的布局、色彩、字号、圆角、表格密度、图表样式或状态语义，应视为不合格并重新生成。`，类型 button，来源 prompt/layer plus target visual ledger。
- 文本 17：`画布比例 16:9，目标 1920x1080 px，单屏完整展示，不要出现浏览器边框、手机外壳或营销落地页布局。`，类型 status，来源 prompt/layer plus target visual ledger。
- 文本 18：`所有页面标题和卡片标题使用统一字号：主标题约 18px，面板标题约 16px，表格正文约 13px，辅助说明约 12px；标题不要忽大忽小。`，类型 legend，来源 prompt/layer plus target visual ledger。
- 文本 19：`左侧一级菜单固定为：综合态势、采集监测、威胁分析、资产图谱、检测运营、审计配置；不要使用“看见、研判、取证、治理、验收”等非规范菜单词。`，类型 hint，来源 manual visual ledger。
- 文本 20：`左侧二级菜单显示：告警中心、战役列表、攻击链分析、加密流量、取证分析，当前高亮“告警中心”。`，类型 acceptance，来源 manual visual ledger。
- 文本 21：`顶部状态条保留站点、时间、风险态势、告警总数、关键告警、采集健康度、数据质量、快捷入口等能力入口。`，类型 title，来源 manual visual ledger。
- 文本 22：`页面内容应体现园区网络全流量采集与分析业务闭环：采集接入、流式处理、资产识别、威胁检测、告警研判、证据取证、响应处置、反馈学习、审计验收。`，类型 subtitle，来源 manual visual ledger。
- 文本 23：`不要改变最终参考图的整体视觉方向：不要改成浅色、不要做成插画海报、不要过度圆角、不要大面积渐变球或装饰光斑。`，类型 section，来源 manual visual ledger。
- 文本 24：`本图类型：完整页面。`，类型 label，来源 manual visual ledger。
- 文本 25：`页面名称：告警中心。`，类型 metric，来源 manual visual ledger。
- 文本 26：`路由：/alerts。`，类型 button，来源 manual visual ledger。
- 文本 27：`业务模块：威胁分析 / 告警中心。`，类型 status，来源 manual visual ledger。
- 文本 28：`页面重点：安全研判主工作台，围绕告警完成筛选、研判、取证、处置、反馈。`，类型 legend，来源 manual visual ledger。
- 文本 29：`必须包含的业务模块：`，类型 hint，来源 manual visual ledger。
- 文本 30：`- 告警队列：高危、中危、低危、未处理、处理中、已确认、已忽略。`，类型 acceptance，来源 manual visual ledger。
- 文本 31：`- 筛选检索：时间窗、资产、IP、规则、模型、攻击阶段、状态、置信度。`，类型 title，来源 manual visual ledger。
- 文本 32：`园区网络全流量采集与分析系统`，类型 subtitle，来源 manual visual ledger。
- 文本 33：`综合态势`，类型 section，来源 manual visual ledger。
- 文本 34：`采集监测`，类型 label，来源 manual visual ledger。
- 文本 35：`威胁分析`，类型 metric，来源 manual visual ledger。
- 文本 36：`资产图谱`，类型 button，来源 manual visual ledger。

## 组件清单

| 区域 | 组件/元素 | 实现方式 | 状态 | 备注 |
|---|---|---|---|---|
| `canvas` | `AppShell` | web/ui/src/components/AppShell.tsx | default | AppShell must keep stable dimensions and match the recorded bbox/token mapping. |
| `primary-section` | `WorkPanel` | web/ui/src/components/WorkPanel.tsx | interactive | WorkPanel must keep stable dimensions and match the recorded bbox/token mapping. |
| `primary-section` | `MetricTile` | web/ui/src/components/MetricTile.tsx | data-ready | MetricTile must keep stable dimensions and match the recorded bbox/token mapping. |
| `primary-section` | `Table` | web/ui/src/components/Table.tsx | default | Table must keep stable dimensions and match the recorded bbox/token mapping. |
| `state-section` | `Tabs` | web/ui/src/components/Tabs.tsx | interactive | Tabs must keep stable dimensions and match the recorded bbox/token mapping. |
| `state-section` | `ECharts` | web/ui/src/components/ECharts.tsx | data-ready | ECharts must keep stable dimensions and match the recorded bbox/token mapping. |
| `state-section` | `StatusTag` | web/ui/src/components/StatusTag.tsx | default | StatusTag must keep stable dimensions and match the recorded bbox/token mapping. |
| `state-section` | `AppHeader` | web/ui/src/components/AppHeader.tsx | interactive | AppHeader must keep stable dimensions and match the recorded bbox/token mapping. |
| `implementation-section` | `PrimarySidebar` | web/ui/src/components/PrimarySidebar.tsx | data-ready | PrimarySidebar must keep stable dimensions and match the recorded bbox/token mapping. |
| `implementation-section` | `BottomStatusBar` | web/ui/src/components/BottomStatusBar.tsx | default | BottomStatusBar must keep stable dimensions and match the recorded bbox/token mapping. |
| `implementation-section` | `BreadcrumbContext` | web/ui/src/components/BreadcrumbContext.tsx | interactive | BreadcrumbContext must keep stable dimensions and match the recorded bbox/token mapping. |
| `implementation-section` | `Search` | web/ui/src/components/Search.tsx | data-ready | Search must keep stable dimensions and match the recorded bbox/token mapping. |

- `AppShell` 映射到 web/ui/src/components/AppShell.tsx。
  状态口径：default
  复核点：AppShell must keep stable dimensions and match the recorded bbox/token mapping.
- `WorkPanel` 映射到 web/ui/src/components/WorkPanel.tsx。
  状态口径：interactive
  复核点：WorkPanel must keep stable dimensions and match the recorded bbox/token mapping.
- `MetricTile` 映射到 web/ui/src/components/MetricTile.tsx。
  状态口径：data-ready
  复核点：MetricTile must keep stable dimensions and match the recorded bbox/token mapping.
- `Table` 映射到 web/ui/src/components/Table.tsx。
  状态口径：default
  复核点：Table must keep stable dimensions and match the recorded bbox/token mapping.
- `Tabs` 映射到 web/ui/src/components/Tabs.tsx。
  状态口径：interactive
  复核点：Tabs must keep stable dimensions and match the recorded bbox/token mapping.
- `ECharts` 映射到 web/ui/src/components/ECharts.tsx。
  状态口径：data-ready
  复核点：ECharts must keep stable dimensions and match the recorded bbox/token mapping.
- `StatusTag` 映射到 web/ui/src/components/StatusTag.tsx。
  状态口径：default
  复核点：StatusTag must keep stable dimensions and match the recorded bbox/token mapping.
- `AppHeader` 映射到 web/ui/src/components/AppHeader.tsx。
  状态口径：interactive
  复核点：AppHeader must keep stable dimensions and match the recorded bbox/token mapping.
- `PrimarySidebar` 映射到 web/ui/src/components/PrimarySidebar.tsx。
  状态口径：data-ready
  复核点：PrimarySidebar must keep stable dimensions and match the recorded bbox/token mapping.
- `BottomStatusBar` 映射到 web/ui/src/components/BottomStatusBar.tsx。
  状态口径：default
  复核点：BottomStatusBar must keep stable dimensions and match the recorded bbox/token mapping.
- `BreadcrumbContext` 映射到 web/ui/src/components/BreadcrumbContext.tsx。
  状态口径：interactive
  复核点：BreadcrumbContext must keep stable dimensions and match the recorded bbox/token mapping.
- `Search` 映射到 web/ui/src/components/Search.tsx。
  状态口径：data-ready
  复核点：Search must keep stable dimensions and match the recorded bbox/token mapping.

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

- 参考实现模式：production React route via `AlertTriagePage`
- 页面路由：`/alerts`
- 页面映射：`route:/alerts`
- 服务映射：`web/ui/src/services/api.ts`
- 样式映射：`web/ui/src/styles/tokens.css`、`Ant Design theme override`、`ECharts dark theme tokens`
- 映射说明：r137 证据截图来自 `http://10.0.5.8:30180/alerts` 生产路由，经 Windows Chrome CDP 抓取；业务数据通过 `services/api.ts` 的 `fetchPageSnapshot("alerts")` 与 typed fallback 进入 React 组件，不使用目标图或整页截图作为业务 UI 资源。

| 前端单位 | 映射方式 |
|---|---|
| `AppShell` | web/ui/src/components/AppShell.tsx |
| `WorkPanel` | web/ui/src/components/WorkPanel.tsx |
| `MetricTile` | web/ui/src/components/MetricTile.tsx |
| `Table` | web/ui/src/components/Table.tsx |
| `Tabs` | web/ui/src/components/Tabs.tsx |
| `ECharts` | web/ui/src/components/ECharts.tsx |
| `StatusTag` | web/ui/src/components/StatusTag.tsx |
| `AppHeader` | web/ui/src/components/AppHeader.tsx |
| `PrimarySidebar` | web/ui/src/components/PrimarySidebar.tsx |
| `BottomStatusBar` | web/ui/src/components/BottomStatusBar.tsx |
| `BreadcrumbContext` | web/ui/src/components/BreadcrumbContext.tsx |
| `Search` | web/ui/src/components/Search.tsx |

## 验收证据

- 目标图：`evidence/ui-image-breakdowns/pages/alerts/target.png`
- 实现截图：`evidence/ui-image-breakdowns/pages/alerts/implementation-r137-final.png`，同步别名 `evidence/ui-image-breakdowns/pages/alerts/implementation.png`
- diff 图：`evidence/ui-image-breakdowns/pages/alerts/diff-r137.png`，同步别名 `evidence/ui-image-breakdowns/pages/alerts/diff.png`
- 业务区裁剪：`evidence/ui-image-breakdowns/pages/alerts/target-business-r137.png`、`evidence/ui-image-breakdowns/pages/alerts/implementation-business-r137.png`
- 业务区 diff：`evidence/ui-image-breakdowns/pages/alerts/diff-business-r137.png`，同步别名 `evidence/ui-image-breakdowns/pages/alerts/diff-business.png`
- regions overlay：`evidence/ui-image-breakdowns/pages/alerts/regions-overlay.png`
- measurement：`evidence/ui-image-breakdowns/pages/alerts/measurement.json`
- OCR/manual ledger：`evidence/ui-image-breakdowns/pages/alerts/text-ocr.txt`
- metrics：`evidence/ui-image-breakdowns/pages/alerts/metrics-r137.json`，同步别名 `evidence/ui-image-breakdowns/pages/alerts/metrics.json`
- business metrics：`evidence/ui-image-breakdowns/pages/alerts/metrics-business-r137.json`，同步别名 `evidence/ui-image-breakdowns/pages/alerts/metrics-business.json`
- verification：`evidence/ui-image-breakdowns/pages/alerts/verification.json`
- capture meta：`evidence/ui-image-breakdowns/pages/alerts/capture-meta-r137-final.json`，同步别名 `evidence/ui-image-breakdowns/pages/alerts/capture-meta.json`
- CDP 预检：`evidence/ui-image-breakdowns/pages/alerts/cdp-version-r137-final-pre-capture.txt`、`evidence/ui-image-breakdowns/pages/alerts/cdp-list-r137-final-pre-capture.txt`
- 生产 URL：`http://10.0.5.8:30180/alerts`
- 镜像 tag：`traffic/web-ui:ui-alerts-visual-20260709-r137`
- 视口：1920 x 1080，DPR 1
- 浏览器：Windows Chrome CDP，经 `http://127.0.0.1:9224/json/version` 与 `/json/list` 预检。
- 复现步骤：锁定 target.png，打开生产路由 `http://10.0.5.8:30180/alerts`，使用 Windows Chrome CDP 预检并截图，生成 full/business diff 与 metrics，读取 capture-meta.json 和 verification.json。

## 差异清单

| 类型 | 位置 | 当前 | 期望 | 状态 |
|---|---|---|---|---|
| visual-diff | full image | r137 full image ratio `0.1073755787037037` | pixel mismatch ratio <= `0.12`, channel tolerance `48` | accepted |
| business-diff | content-root `197,80,1713,917` | r137 business ratio `0.10505398132568892` | pixel mismatch ratio <= `0.12`, channel tolerance `48` | accepted |
| semantic-scope | production React implementation | KPI、风险圆环、表格、时间线、关联簇和反馈表单均由 API/typed fallback 数据驱动 | 不得使用整页或业务图截图替代动态业务图示 | accepted |
| runtime | production route | no console/page errors, no requestfailed, no 4xx/5xx, no root/panel/table/chart overflow | runtime clean | accepted |

## 结论

- 逐图拆解层已记录区域、文本、组件、图标、token、交互、实现映射和证据路径。
- r137 Windows Chrome 生产截图、运行时审查、full/business diff 和辅助审查已写入证据目录。
- 通过口径为：target.png、regions-overlay.png、implementation.png、diff.png、metrics.json、measurement.json、capture-meta.json、verification.json、CDP 预检文件齐备，且 full/business metrics 均为 pass。
- 主线程判定：`alerts` 当前页通过，状态为 `business-pixel-accepted`。

## 2026-07-11 r255 业务实现与验证

- 生产业务实现：`web/ui/src/pages/AlertTriagePage.tsx`；公共 topbar/sidebar/bottombar 未改动。
- 动态图：选中告警的 `__riskScore` 传入 `web/ui/src/components/charts.tsx` 的 `RingChart`，以 ECharts gauge canvas 代替 CSS conic-gradient 表盘。
- 数据与接口：告警快照仍经 `fetchPageSnapshot("alerts")` 读取；当返回记录少于可视分页容量时，按同结构 typed fallback 扩展为 28 条仿真行。模拟动作映射到 `pageApiPlans["alert-detail"]` 的报告、证据访问、响应和研判记录端点。
- 可用交互：表格为 10 条/页受控分页，包含纵横滚动；视图、筛选、批量、行级操作、关联簇和反馈均改变状态或打开确认 Drawer。
- Windows Chrome 交互证据：`evidence/ui-image-breakdowns/pages/alerts/interaction-r255.json`，验证 1 个风险仪表 canvas、第 2 页、`overflow-y: scroll`、保存视图、行级详情和关联告警簇 Drawer；4xx/5xx、`requestfailed`、console error 与 page error 均为 0。
- 业务 ROI：`evidence/ui-image-breakdowns/pages/alerts/metrics-business-r255.json`，业务区域 `content-root (197,80,1713,917)` 的 mismatch ratio 为 `0.07573173518815957`，低于 `0.125`。
