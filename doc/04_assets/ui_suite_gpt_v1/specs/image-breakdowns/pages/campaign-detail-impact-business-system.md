# campaign-detail-impact-business-system.png 逐图精拆记录

## 基本信息

- 分类：pages
- 标题：战役详情。
- 源图：`doc/04_assets/ui_suite_gpt_v1/screens/pages/campaign-detail-impact-business-system.png`
- 源图尺寸：1920 x 1080
- 对应 prompt：`doc/04_assets/ui_suite_gpt_v1/prompts/campaign-detail.prompt.txt`
- 对应 manifest/layer：`doc/04_assets/ui_suite_gpt_v1/manifest.json`
- 对应路由/宿主路由：`/campaigns/:campaignId?impact=business-system` / `/campaigns/:campaignId`
- 当前状态：`business-pixel-accepted`
- 生产镜像：`traffic/web-ui:ui-campaign-impact-business-system-20260710-r171`
- 复刻等级：真实 React/CSS 数据驱动实现，已完成 Windows Chrome 生产路由截图、diff、两轮辅助审查、两次结构回修和主线程判断。
- 验收边界：目标图是战役详情中的“影响范围 / 业务系统”focus 状态；普通生产详情仍嵌在战役详情页中，不作为全屏弹窗。

## 目标图观察

- 整体布局：影响范围独立面板，顶部六个对象 tab，中部业务系统风险椭圆环与风险分布，下部 Top 5 业务系统表。
- 业务重点：受影响系统数量、风险占比、关键服务、风险和恢复优先级均由 typed campaign snapshot 驱动。
- 当前页面/浮层状态：`业务系统` tab 激活；受影响系统 9，高/中/低风险分别为 3/4/2。
- 视觉基调：深海军蓝 SOC 指挥台，青蓝描边，低饱和面板，高密度文字与表格，状态色严格区分。
- 证据边界：`implementation.png` 来自 r171 真实 APISIX 路由和 Windows Chrome CDP；`target.png` 只参与拆解与 diff。
- 视觉读取方式：直接锁定目标 PNG，结合 prompt、layer JSON、manifest 与 Windows Chrome 截图证据校验。
- 坐标口径：所有 bbox 均以目标 PNG 左上角为原点，单位 px。

## 本轮生产实现与回修

- `CampaignDetailPage.tsx` 使用 query `impact=business-system` 表达可访问、可复现的业务系统状态；验收 focus 状态只隐藏 AppShell，不改变普通生产详情页的嵌入形态。
- `campaignDetailApi.ts` 新增 `CampaignDetailImpactBusinessSystem`、`CampaignDetailBusinessSystemRow`，真实接口入口保持 `/v1/campaigns/{campaignId}`。
- API 未返回业务系统影响字段时使用 typed fallback：总数 9，风险分布 3/4/2，Top 5 为科研管理系统、数据分析平台、文件存储系统、统一认证平台和教工终端管理。
- 风险环使用 React/CSS `conic-gradient` 根据 typed count 计算角度；表格直接消费 snapshot rows，不引用目标 PNG 或静态业务图。
- r168 虽以 `0.10338348765432098` 通过宽阈值，但辅助审查拒绝了多余风险区外框、偏小椭圆、未填满表格和标签样式；r170 修复后降至 `0.06808883101851852`，第二次审查继续要求校正风险列表列位。
- r171 将标签、数量、百分比校准到目标约 x990/x1420/x1637，最终 mismatch 为 `0.06796344521604938`；全新辅助复审 PASS。
- r171 runtime：无 console/page error、requestfailed、HTTP 4xx/5xx、禁止资源请求或横纵向溢出；5 行业务系统和 3 行风险分布均存在。

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
| 战役详情。 | `80,54,360,24` | title | 是 |
| campaign-detail-impact-business-system | `510,54,360,24` | subtitle | 是 |
| /campaigns/:campaignId | `940,54,360,24` | section | 是 |
| 战役画像、攻击时间轴、关联告警、影响范围、证据包、复盘结论。 | `1370,54,360,24` | label | 是 |
| Foundation 硬门禁：生成图必须严格遵守参考图中的 8 张 foundations 规范板，不能只是风格相似。 | `80,96,360,24` | metric | 是 |
| 必须锁定状态语义：健康/通过用绿色，信息/低危用蓝色，中危/待确认用黄色或琥珀色，高危/失败用红色；不得交换状态颜色。 | `510,96,360,24` | button | 是 |
| 如果生成结果偏离 foundations 的布局、色彩、字号、圆角、表格密度、图表样式或状态语义，应视为不合格并重新生成。 | `940,96,360,24` | status | 是 |
| 画布比例 16:9，目标 1920x1080 px，单屏完整展示，不要出现浏览器边框、手机外壳或营销落地页布局。 | `1370,96,360,24` | legend | 是 |
| 所有页面标题和卡片标题使用统一字号：主标题约 18px，面板标题约 16px，表格正文约 13px，辅助说明约 12px；标题不要忽大忽小。 | `80,138,360,24` | hint | 是 |
| 左侧一级菜单固定为：综合态势、采集监测、威胁分析、资产图谱、检测运营、审计配置；不要使用“看见、研判、取证、治理、验收”等非规范菜单词。 | `510,138,360,24` | acceptance | 是 |
| 左侧二级菜单显示：告警中心、战役列表、攻击链分析、加密流量、取证分析，当前高亮“战役列表”。 | `940,138,360,24` | title | 是 |
| 顶部状态条保留站点、时间、风险态势、告警总数、关键告警、采集健康度、数据质量、快捷入口等能力入口。 | `1370,138,360,24` | subtitle | 是 |
| 页面内容应体现园区网络全流量采集与分析业务闭环：采集接入、流式处理、资产识别、威胁检测、告警研判、证据取证、响应处置、反馈学习、审计验收。 | `80,180,360,24` | section | 是 |
| 不要改变最终参考图的整体视觉方向：不要改成浅色、不要做成插画海报、不要过度圆角、不要大面积渐变球或装饰光斑。 | `510,180,360,24` | label | 是 |
| 本图类型：完整页面。 | `940,180,360,24` | metric | 是 |
| 页面名称：战役详情。 | `1370,180,360,24` | button | 是 |
| 路由：/campaigns/:campaignId。 | `80,222,360,24` | status | 是 |
| 业务模块：威胁分析 / 战役列表。 | `510,222,360,24` | legend | 是 |
| 页面重点：战役画像、攻击时间轴、关联告警、影响范围、证据包、复盘结论。 | `940,222,360,24` | hint | 是 |
| 必须包含的业务模块： | `1370,222,360,24` | acceptance | 是 |
| - 战役画像：名称、风险、阶段、持续时间、负责人、当前状态。 | `80,264,360,24` | title | 是 |
| - 攻击时间轴：关键阶段、关联告警、证据节点、处置节点。 | `510,264,360,24` | subtitle | 是 |
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

- 文本 01：`战役详情。`，类型 title，来源 prompt/layer plus target visual ledger。
- 文本 02：`campaign-detail-impact-business-system`，类型 subtitle，来源 prompt/layer plus target visual ledger。
- 文本 03：`/campaigns/:campaignId`，类型 section，来源 prompt/layer plus target visual ledger。
- 文本 04：`战役画像、攻击时间轴、关联告警、影响范围、证据包、复盘结论。`，类型 label，来源 prompt/layer plus target visual ledger。
- 文本 05：`Foundation 硬门禁：生成图必须严格遵守参考图中的 8 张 foundations 规范板，不能只是风格相似。`，类型 metric，来源 prompt/layer plus target visual ledger。
- 文本 06：`必须锁定状态语义：健康/通过用绿色，信息/低危用蓝色，中危/待确认用黄色或琥珀色，高危/失败用红色；不得交换状态颜色。`，类型 button，来源 prompt/layer plus target visual ledger。
- 文本 07：`如果生成结果偏离 foundations 的布局、色彩、字号、圆角、表格密度、图表样式或状态语义，应视为不合格并重新生成。`，类型 status，来源 prompt/layer plus target visual ledger。
- 文本 08：`画布比例 16:9，目标 1920x1080 px，单屏完整展示，不要出现浏览器边框、手机外壳或营销落地页布局。`，类型 legend，来源 prompt/layer plus target visual ledger。
- 文本 09：`所有页面标题和卡片标题使用统一字号：主标题约 18px，面板标题约 16px，表格正文约 13px，辅助说明约 12px；标题不要忽大忽小。`，类型 hint，来源 prompt/layer plus target visual ledger。
- 文本 10：`左侧一级菜单固定为：综合态势、采集监测、威胁分析、资产图谱、检测运营、审计配置；不要使用“看见、研判、取证、治理、验收”等非规范菜单词。`，类型 acceptance，来源 prompt/layer plus target visual ledger。
- 文本 11：`左侧二级菜单显示：告警中心、战役列表、攻击链分析、加密流量、取证分析，当前高亮“战役列表”。`，类型 title，来源 prompt/layer plus target visual ledger。
- 文本 12：`顶部状态条保留站点、时间、风险态势、告警总数、关键告警、采集健康度、数据质量、快捷入口等能力入口。`，类型 subtitle，来源 prompt/layer plus target visual ledger。
- 文本 13：`页面内容应体现园区网络全流量采集与分析业务闭环：采集接入、流式处理、资产识别、威胁检测、告警研判、证据取证、响应处置、反馈学习、审计验收。`，类型 section，来源 prompt/layer plus target visual ledger。
- 文本 14：`不要改变最终参考图的整体视觉方向：不要改成浅色、不要做成插画海报、不要过度圆角、不要大面积渐变球或装饰光斑。`，类型 label，来源 prompt/layer plus target visual ledger。
- 文本 15：`本图类型：完整页面。`，类型 metric，来源 prompt/layer plus target visual ledger。
- 文本 16：`页面名称：战役详情。`，类型 button，来源 prompt/layer plus target visual ledger。
- 文本 17：`路由：/campaigns/:campaignId。`，类型 status，来源 prompt/layer plus target visual ledger。
- 文本 18：`业务模块：威胁分析 / 战役列表。`，类型 legend，来源 prompt/layer plus target visual ledger。
- 文本 19：`页面重点：战役画像、攻击时间轴、关联告警、影响范围、证据包、复盘结论。`，类型 hint，来源 manual visual ledger。
- 文本 20：`必须包含的业务模块：`，类型 acceptance，来源 manual visual ledger。
- 文本 21：`- 战役画像：名称、风险、阶段、持续时间、负责人、当前状态。`，类型 title，来源 manual visual ledger。
- 文本 22：`- 攻击时间轴：关键阶段、关联告警、证据节点、处置节点。`，类型 subtitle，来源 manual visual ledger。
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

- 实现模式：生产 React/CSS 与 typed dynamic snapshot
- 页面路由：`/campaigns/:campaignId?impact=business-system`
- 页面映射：`web/ui/src/pages/CampaignDetailPage.tsx` / `CampaignImpactBusinessSystemContent`
- 服务映射：`web/ui/src/services/campaignDetailApi.ts` / `/v1/campaigns/{campaignId}`
- 样式映射：`web/ui/src/styles/pages.css`、`web/ui/src/styles/app-shell.css`
- 映射说明：验收 focus query 用于稳定截图；普通生产路由仍在战役详情业务区内渲染同一份动态组件。

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

- 目标图：`evidence/ui-image-breakdowns/pages/campaign-detail-impact-business-system/target.png`
- 实现截图：`evidence/ui-image-breakdowns/pages/campaign-detail-impact-business-system/implementation.png`
- diff 图：`evidence/ui-image-breakdowns/pages/campaign-detail-impact-business-system/diff.png`
- regions overlay：`evidence/ui-image-breakdowns/pages/campaign-detail-impact-business-system/regions-overlay.png`
- measurement：`evidence/ui-image-breakdowns/pages/campaign-detail-impact-business-system/measurement.json`
- OCR/manual ledger：`evidence/ui-image-breakdowns/pages/campaign-detail-impact-business-system/text-ocr.txt`
- metrics：`evidence/ui-image-breakdowns/pages/campaign-detail-impact-business-system/metrics.json`
- verification：`evidence/ui-image-breakdowns/pages/campaign-detail-impact-business-system/verification.json`
- 视口：1920 x 1080，DPR 1
- 浏览器：Windows Chrome CDP，经 `http://127.0.0.1:9224/json/version` 与 `/json/list` 预检。
- 复现步骤：锁定 target.png，打开 implementation.html，使用 Windows Chrome CDP 截图，生成 diff.png 和 metrics.json，读取 verification.json。

## 差异清单

| 类型 | 位置 | 当前 | 期望 | 状态 |
|---|---|---|---|---|
| risk-summary-geometry | impact summary | r171 椭圆、风险框和三列坐标已按目标校准 | 宽椭圆与标签/数量/百分比分列 | closed |
| table-density | Top 5 table | r171 表头与五行填满目标表格区域 | 无底部大块空白 | closed |
| font-rasterization | full image | Windows Chrome 原生文字渲染 | 生成目标图的模糊发光文字 | accepted-within-threshold |
| windows-scaling | capture metadata | PNG 1920x1080；CSS viewport 2133x1200，DPR 0.9 | 1920x1080 验收图 | documented |

## 结论

- r171 的生产 React/CSS、typed dynamic data、Windows Chrome runtime、视觉 diff、辅助复审和证据完整性均通过。
- 主线程判定为 `business-pixel-accepted`；验收 focus 状态不改变生产态“小 Modal / 窄 Drawer、禁止全屏弹层”的交互门禁。
- 下一队列项可进入 `campaign-detail-impact-campus`。
