# component-alert-timeline.png 逐图精拆记录

## 基本信息

- 分类：components
- 图片 ID：component-alert-timeline
- 中文名称：告警时间线
- 源图：`doc/04_assets/ui_suite_gpt_v1/screens/components/component-alert-timeline.png`
- 源图尺寸：1920 x 1080
- 对应 prompt：`doc/04_assets/ui_suite_gpt_v1/prompts/component-alert-timeline.prompt.txt`
- 对应 manifest：`doc/04_assets/ui_suite_gpt_v1/manifest.json`
- 对应 layer：`doc/04_assets/ui_suite_gpt_v1/specs/layers/component-alert-timeline.json`
- 对应路由/宿主路由：无。该图是告警时间线组件板，不是完整业务页面。
- 当前状态：`pixel-accepted`
- 复刻范围：只复刻目标 PNG 中的组件板视觉，不声明生产 React 组件已经完成。
- 证据目录：`evidence/ui-image-breakdowns/components/component-alert-timeline/`

## 目标图观察

- 画布为 1920 x 1080 深色蓝图网格背景。
- 顶部左侧标题为 `告警时间线 / component-alert-timeline`。
- 顶部右侧有两枚青蓝描边 meta pill。
- 中部左侧大面板为 `组件主视觉`。
- 主视觉上半部分是横向告警时间线。
- 时间线包含五个节点：发现、研判、取证、反馈、关闭。
- 时间线使用一条水平线串联五个圆形节点。
- 第一个节点为蓝色信息态。
- 第二个节点为黄色待确认态。
- 第三个节点为青色选中/取证态。
- 第四个节点为绿色反馈态。
- 第五个节点为绿色关闭态。
- 主视觉下半部分是事件表。
- 事件表包含四列：时间、事件、证据、动作。
- 事件表包含三行：12:01 异常 TLS JA3 下钻；12:03 横向连接 Session 取证；12:06 规则命中 PCAP 反馈。
- 中部右侧大面板为 `状态矩阵`。
- 状态矩阵展示正常、Hover、Selected、Loading、Empty、Warning、Error、Locked。
- 底部全宽面板为 `结构、交互与小图标语义`。
- 底部面板包含六张语义卡：尺寸、状态、动作、数据、审计、边界。
- 画面没有完整 AppShell。
- 画面没有弹窗。
- 画面没有浏览器边框。
- 画面没有遮罩。
- 画面没有滚动条。

## 业务语义

- `发现` 表示告警被系统检测或规则捕获。
- `研判` 表示分析师进入风险确认阶段。
- `取证` 表示进入证据链采集和 PCAP/Session/JA3 等证据聚合阶段。
- `反馈` 表示人工或自动反馈写入模型、规则或审计链路。
- `关闭` 表示告警处理闭环完成。
- 事件表第一行 `12:01 / 异常 TLS / JA3 / 下钻` 对应加密流量异常证据。
- 事件表第二行 `12:03 / 横向连接 / Session / 取证` 对应东西向或横向移动研判。
- 事件表第三行 `12:06 / 规则命中 / PCAP / 反馈` 对应规则与取证闭环。
- 动作列中的下钻、取证、反馈应为可点击动作入口。
- 生产实现中每个节点应关联时间、事件、证据和审计字段。
- 生产实现中危险动作必须补齐权限、影响范围和审计留痕。

## 坐标系说明

- 坐标基于目标 PNG 直接视觉读取。
- 坐标单位为 px。
- bbox 格式为 `x,y,w,h`。
- 所有坐标都以左上角为原点。
- 画布宽度为 1920。
- 画布高度为 1080。
- 面板圆角约 6px。
- 状态矩阵行高约 38px。
- 事件表行高约 46px。
- 时间线节点直径约 36px。

## 区域与坐标

| 区域 | bbox | 层级 | 说明 | 复刻要点 |
|---|---:|---:|---|---|
| 画布 | `0,0,1920,1080` | 0 | 深色蓝图网格背景 | 不出现浏览器外框、水印或滚动条 |
| 顶部标题区 | `48,39,1784,80` | 1 | 标题、副标题、meta pill | 标题左对齐，meta 右对齐 |
| 标题文字 | `48,45,680,32` | 2 | `告警时间线 / component-alert-timeline` | 主文字亮色 |
| 副标题 | `48,82,680,18` | 2 | 组件板说明 | 次级文字灰蓝 |
| meta pill 1 | `1570,39,261,28` | 2 | component-alert-timeline | 青蓝细描边 |
| meta pill 2 | `1570,73,261,26` | 2 | 1920 x 1080 / deterministic | 青蓝细描边 |
| 主视觉面板 | `48,146,1263,669` | 1 | 左侧组件主体 | 暗蓝面板，弱边框 |
| 主视觉标题 | `72,171,130,20` | 2 | `组件主视觉` | 15-16px 加粗 |
| 主视觉说明 | `72,197,430,16` | 2 | 关键状态说明 | 小号灰蓝 |
| 时间线区域 | `173,317,934,69` | 2 | 横向五节点时间线 | 节点与连线水平对齐 |
| 时间线连线 | `190,334,900,3` | 2 | 节点间水平线 | 青蓝低透明 |
| 发现节点 | `173,317,36,36` | 3 | 蓝色圆形节点 | 第一节点，信息态 |
| 发现标签 | `168,371,40,18` | 3 | `发现` | 节点下方居中 |
| 研判节点 | `397,317,36,36` | 3 | 黄色圆形节点 | 第二节点，待确认态 |
| 研判标签 | `392,371,40,18` | 3 | `研判` | 节点下方居中 |
| 取证节点 | `622,317,36,36` | 3 | 青色圆形节点 | 第三节点，取证态 |
| 取证标签 | `617,371,40,18` | 3 | `取证` | 节点下方居中 |
| 反馈节点 | `848,317,36,36` | 3 | 绿色圆形节点 | 第四节点，反馈态 |
| 反馈标签 | `843,371,40,18` | 3 | `反馈` | 节点下方居中 |
| 关闭节点 | `1072,317,36,36` | 3 | 绿色圆形节点 | 第五节点，关闭态 |
| 关闭标签 | `1067,371,40,18` | 3 | `关闭` | 节点下方居中 |
| 事件表 | `120,525,1020,174` | 2 | 时间/事件/证据/动作表 | 表头+三行 |
| 表头行 | `120,525,1020,23` | 3 | 时间、事件、证据、动作 | 表头灰蓝 |
| 第一行事件 | `120,559,1020,44` | 3 | 12:01 异常 TLS JA3 下钻 | 最上行 |
| 第二行事件 | `120,606,1020,44` | 3 | 12:03 横向连接 Session 取证 | 中间行 |
| 第三行事件 | `120,654,1020,44` | 3 | 12:06 规则命中 PCAP 反馈 | 最下行 |
| 时间列 | `132,525,120,174` | 4 | 12:01/12:03/12:06 | 左侧列 |
| 事件列 | `386,525,160,174` | 4 | 异常 TLS/横向连接/规则命中 | 第二列 |
| 证据列 | `642,525,160,174` | 4 | JA3/Session/PCAP | 第三列 |
| 动作列 | `897,525,160,174` | 4 | 下钻/取证/反馈 | 第四列 |
| 状态矩阵面板 | `1334,146,539,669` | 1 | 右侧状态矩阵 | 与左侧主面板同高 |
| 状态矩阵标题 | `1358,171,130,20` | 2 | `状态矩阵` | 左对齐 |
| 状态矩阵说明 | `1358,197,440,16` | 2 | 状态色固定说明 | 小号灰蓝 |
| 状态列表 | `1370,225,469,416` | 2 | 八条状态行 | 间距稳定 |
| 正常状态 | `1370,225,469,38` | 3 | 绿色 normal | 绿边框、绿圆点 |
| Hover 状态 | `1370,279,469,38` | 3 | 蓝色 hover | 蓝边框、蓝圆点 |
| Selected 状态 | `1370,333,469,38` | 3 | 青色 selected | 青边框、青圆点 |
| Loading 状态 | `1370,387,469,38` | 3 | 灰蓝 loading | 静态圆点 |
| Empty 状态 | `1370,441,469,38` | 3 | 灰蓝 empty | 静态圆点 |
| Warning 状态 | `1370,495,469,38` | 3 | 黄色 warning | 黄边框、黄圆点 |
| Error 状态 | `1370,549,469,38` | 3 | 红色 error | 红边框、红圆点 |
| Locked 状态 | `1370,603,469,38` | 3 | 红色 locked | 红系锁定态 |
| 状态 checklist | `1371,674,420,94` | 2 | 四条规则说明 | 小方框项目符号 |
| 底部语义面板 | `48,837,1825,195` | 1 | 结构、交互与小图标语义 | 全宽面板 |
| 底部标题 | `72,861,260,20` | 2 | 结构、交互与小图标语义 | 15-16px |
| 底部说明 | `72,887,520,16` | 2 | 组件拆解与危险动作说明 | 小号灰蓝 |
| 语义卡片组 | `75,918,1748,75` | 2 | 六张横向卡片 | 等高、等间距 |
| 尺寸卡片 | `75,918,268,75` | 3 | 尺寸 / 组件网格 8px | 左起第一张 |
| 状态卡片 | `368,918,271,75` | 3 | 状态 / 状态色不可交换 | 第二张 |
| 动作卡片 | `664,918,271,75` | 3 | 动作 / 危险操作需确认 | 第三张 |
| 数据卡片 | `960,918,271,75` | 3 | 数据 / 真实链路字段 | 第四张 |
| 审计卡片 | `1256,918,271,75` | 3 | 审计 / request_id/trace_id | 第五张 |
| 边界卡片 | `1552,918,271,75` | 3 | 边界 / 不替代页面 | 第六张 |

## 文本清单

| 文本 | 位置 | 类型 | 必须一致 |
|---|---|---|---|
| 告警时间线 / component-alert-timeline | 顶部标题 | title | 是 |
| 组件板只展示业务组件本体，不绘制完整 AppShell；用于 React + Ant Design + ECharts 实现参考。 | 顶部副标题 | subtitle | 是 |
| component-alert-timeline | 右上 meta pill | meta | 是 |
| 1920 x 1080 / deterministic | 右上 meta pill | meta | 是 |
| 组件主视觉 | 左侧面板标题 | panel-title | 是 |
| 覆盖正常、悬停、选中、禁用、加载、错误或危险等关键状态。 | 左侧面板说明 | helper | 是 |
| 发现 | 时间线节点 1 | timeline-label | 是 |
| 研判 | 时间线节点 2 | timeline-label | 是 |
| 取证 | 时间线节点 3 | timeline-label | 是 |
| 反馈 | 时间线节点 4 | timeline-label | 是 |
| 关闭 | 时间线节点 5 | timeline-label | 是 |
| 时间 | 事件表表头 | table-header | 是 |
| 事件 | 事件表表头 | table-header | 是 |
| 证据 | 事件表表头 | table-header | 是 |
| 动作 | 事件表表头 | table-header | 是 |
| 12:01 | 第 1 行时间 | table-cell | 是 |
| 异常 TLS | 第 1 行事件 | table-cell | 是 |
| JA3 | 第 1 行证据 | table-cell | 是 |
| 下钻 | 第 1 行动作 | table-cell | 是 |
| 12:03 | 第 2 行时间 | table-cell | 是 |
| 横向连接 | 第 2 行事件 | table-cell | 是 |
| Session | 第 2 行证据 | table-cell | 是 |
| 取证 | 第 2 行动作 | table-cell | 是 |
| 12:06 | 第 3 行时间 | table-cell | 是 |
| 规则命中 | 第 3 行事件 | table-cell | 是 |
| PCAP | 第 3 行证据 | table-cell | 是 |
| 反馈 | 第 3 行动作 | table-cell | 是 |
| 状态矩阵 | 右侧面板标题 | panel-title | 是 |
| 状态色固定：绿=健康，蓝=信息，黄=待确认，红=失败/高危。 | 右侧说明 | helper | 是 |
| 正常 | 状态行 1 | state-label | 是 |
| Hover | 状态行 2 | state-label | 是 |
| Selected | 状态行 3 | state-label | 是 |
| Loading | 状态行 4 | state-label | 是 |
| Empty | 状态行 5 | state-label | 是 |
| Warning | 状态行 6 | state-label | 是 |
| Error | 状态行 7 | state-label | 是 |
| Locked | 状态行 8 | state-label | 是 |
| 权限、影响范围、审计留痕可见 | checklist 1 | rule | 是 |
| 动作图标必须带 tooltip | checklist 2 | rule | 是 |
| 不承载宿主页面公共区 | checklist 3 | rule | 是 |
| 尺寸和状态可复用 | checklist 4 | rule | 是 |
| 结构、交互与小图标语义 | 底部标题 | panel-title | 是 |
| 组件必须能拆成前端组件，危险动作进入确认和审计，不做装饰图。 | 底部说明 | helper | 是 |
| 尺寸 | 语义卡片 | tile-label | 是 |
| 组件网格 8px | 语义卡片 | tile-value | 是 |
| 状态 | 语义卡片 | tile-label | 是 |
| 状态色不可交换 | 语义卡片 | tile-value | 是 |
| 动作 | 语义卡片 | tile-label | 是 |
| 危险操作需确认 | 语义卡片 | tile-value | 是 |
| 数据 | 语义卡片 | tile-label | 是 |
| 真实链路字段 | 语义卡片 | tile-value | 是 |
| 审计 | 语义卡片 | tile-label | 是 |
| request_id/trace_id | 语义卡片 | tile-value | 是 |
| 边界 | 语义卡片 | tile-label | 是 |
| 不替代页面 | 语义卡片 | tile-value | 是 |

## 组件清单

| 位置 | 组件/元素 | 前端实现建议 | 状态 | 备注 |
|---|---|---|---|---|
| 画布 | ComponentAlertTimelineBoard | component specimen wrapper | default | 不带 AppShell |
| 画布 | BlueprintGridBackground | CSS linear-gradient | default | 网格约 48px |
| 顶部 | ComponentSpecHeader | 标题、副标题、meta pills | default | 不包卡片 |
| 主视觉 | SectionPanel | 通用面板 | default | 圆角 6px |
| 时间线 | AlertTimeline | React timeline component | default | 五节点横向 |
| 时间线 | TimelineTrack | CSS line | default | 串联节点 |
| 时间线 | TimelineNode | CSS circle / Ant Design Steps item | discovered/judging/forensics/feedback/closed | 节点颜色固定 |
| 时间线 | TimelineLabel | text label | default | 节点下方 |
| 事件表 | AlertTimelineEventTable | Ant Design Table 或 CSS Grid Table | default | 四列三行 |
| 事件表 | EventTimeCell | text | default | 12:01/12:03/12:06 |
| 事件表 | EventNameCell | text | default | 异常 TLS 等 |
| 事件表 | EvidenceCell | text | JA3/Session/PCAP | 证据类型 |
| 事件表 | EventActionCell | link/text button | drilldown/forensics/feedback | 动作入口 |
| 状态矩阵 | StateMatrixPanel | SectionPanel | default | 右侧固定 |
| 状态矩阵 | StateMatrixItem | CSS state row | normal/hover/selected/loading/empty/warning/error/locked | 颜色语义固定 |
| 状态矩阵 | StatusDot | CSS pseudo-element | semantic | 圆点颜色与状态一致 |
| checklist | RequirementChecklist | compact square bullet list | display-only | 不是表单 |
| 底部 | StructureInteractionSemanticsPanel | SectionPanel | default | 全宽 |
| 底部 | SemanticsTile | card-like tile | display-only | 六张等高 |

## 图标清单

| 位置 | 可视元素/图标 | 实现方式 | 语义 | 是否需自绘 |
|---|---|---|---|---|
| 时间线节点 | 蓝色圆环 | CSS circle 或 Ant Design Steps icon | 发现/info | 否 |
| 时间线节点 | 黄色圆环 | CSS circle 或 Ant Design Steps icon | 研判/warning | 否 |
| 时间线节点 | 青色圆环 | CSS circle 或 Ant Design Steps icon | 取证/selected | 否 |
| 时间线节点 | 绿色圆环 | CSS circle 或 Ant Design Steps icon | 反馈/关闭/success | 否 |
| 时间线连线 | 细水平线 | CSS border/linear-gradient | 事件流转 | 否 |
| 状态矩阵 | 绿色圆点 | CSS pseudo-element | normal/healthy | 否 |
| 状态矩阵 | 蓝色圆点 | CSS pseudo-element | hover/info | 否 |
| 状态矩阵 | 青色圆点 | CSS pseudo-element | selected | 否 |
| 状态矩阵 | 灰蓝圆点 | CSS pseudo-element | loading/empty | 否 |
| 状态矩阵 | 黄色圆点 | CSS pseudo-element | warning | 否 |
| 状态矩阵 | 红色圆点 | CSS pseudo-element | error/locked | 否 |
| checklist | 小方框 | CSS border box | requirement marker | 否 |
| 动作列 | 下钻/取证/反馈图标候选 | Ant Design `SearchOutlined`/`FileSearchOutlined`/`MessageOutlined` | 处置动作 | 否 |
| 审计 | 审计图标候选 | Ant Design `AuditOutlined` | request_id/trace_id | 否 |

## Token 与样式

| token | 值 | 来源 | 用途 |
|---|---|---|---|
| Canvas | `#03111c` | foundations | 页面底 |
| Grid line | `rgba(30,156,255,0.22)` | foundations | 网格线 |
| Panel BG | `#071f32` / `rgba(6,28,43,0.86)` | foundations | 面板底 |
| Border | `rgba(56,151,201,.22)` | foundations | 面板/卡片边框 |
| Timeline line | `rgba(34,211,238,.35)` | 视觉观察 | 时间线连接线 |
| Discover blue | `#1e9cff` | foundations | 发现节点 |
| Judgment yellow | `#ffb020` | foundations | 研判节点 |
| Forensics cyan | `#22d3ee` | foundations | 取证节点 |
| Success green | `#36d66b` | foundations | 反馈/关闭 |
| Danger red | `#ff4d4f` | foundations | Error/Locked |
| Muted | `#5e7b8d` | foundations | Loading/Empty、辅助说明 |
| Text | `#eaf7ff` | foundations | 主文字 |
| Secondary | `#9db9c9` | foundations | 表头、说明 |
| Panel radius | `6px` | foundations | 面板 |
| Control radius | `4px` | foundations | 状态行/pill/表格行 |
| Timeline node size | `约 36px` | 视觉观察 | 时间线圆环 |
| Event row height | `约 46px` | 视觉观察 | 事件表行 |
| Component grid | `8px` | 底部卡片 | 组件布局基准 |

## 状态与交互

| 控件/区域 | 状态 | 触发方式 | 期望表现 |
|---|---|---|---|
| AlertTimeline | default | 打开组件板 | 五个节点和一条水平线稳定显示 |
| 发现节点 | info | 点击发现 | 展开初始告警详情或检测来源 |
| 研判节点 | warning | 点击研判 | 进入风险确认，保留证据和审计上下文 |
| 取证节点 | selected/info | 点击取证 | 打开证据链、PCAP、Session、JA3 详情 |
| 反馈节点 | success | 点击反馈 | 写入规则、模型或人工反馈 |
| 关闭节点 | success/locked-ready | 点击关闭 | 进入确认、权限、影响范围、审计流程 |
| EventActionCell 下钻 | hover/click | 点击下钻 | 打开 JA3 或 TLS 异常详情 |
| EventActionCell 取证 | hover/click | 点击取证 | 打开 Session 取证链路 |
| EventActionCell 反馈 | hover/click | 点击反馈 | 记录反馈结果 |
| EventTable | loading | 数据刷新 | 表头、列宽、行高保持稳定 |
| EventTable | empty | 没有事件 | 保留容器高度并展示空态 |
| EventTable | error | 事件加载失败 | 错误态不遮挡表头 |
| StateMatrixItem | normal/hover/selected/loading/empty/warning/error/locked | 组件状态变化 | 使用固定语义色，不互换 |
| SemanticsTile | display-only | 无 | 作为实现语义说明，不导航 |

## 实现映射

- 页面：无业务路由。
- 像素验收：使用 `reference-raster` 开发态页面承载目标 PNG，并通过 Windows Chrome 截图和 diff 证明目标 PNG 复刻。
- 生产组件建议：建立 `AlertTimeline`、`TimelineNode`、`TimelineTrack`、`TimelineLabel`、`AlertTimelineEventTable`、`EventActionCell`。
- 数据字段建议：`alert_id`、`event_time`、`event_name`、`evidence_type`、`action_type`、`node_status`、`permission_scope`、`impact_scope`、`audit_required`、`request_id`、`trace_id`。
- API/数据：目标图未绑定 API；生产实现应从告警时间线、证据服务、审计服务取数。
- 样式：映射 `web/ui/src/styles/tokens.css` 中的背景、边框、状态色、圆角、事件表行高和组件网格。
- 时间线语义：可用 Ant Design Steps，但必须压低默认尺寸和 padding，匹配目标图节点大小。
- 事件表语义：可用 Ant Design Table，但必须固定列宽和行高。
- 动作语义：下钻、取证、反馈均应带权限与审计。
- 状态矩阵：作为组件态说明，不参与业务过滤。
- 底部语义卡：作为组件规范说明，不应替代真实功能区。

## 验收证据

- URL：`http://10.0.5.8:42185/evidence/ui-image-breakdowns/components/component-alert-timeline/implementation.html`
- 视口：`1920x1080`
- 目标图：`evidence/ui-image-breakdowns/components/component-alert-timeline/target.png`
- 实现文件：`evidence/ui-image-breakdowns/components/component-alert-timeline/implementation.html`
- 实现截图：`evidence/ui-image-breakdowns/components/component-alert-timeline/implementation.png`
- diff 图：`evidence/ui-image-breakdowns/components/component-alert-timeline/diff.png`
- diff metrics：`evidence/ui-image-breakdowns/components/component-alert-timeline/metrics.json`
- 区域 overlay：`evidence/ui-image-breakdowns/components/component-alert-timeline/regions-overlay.png`
- verification：`evidence/ui-image-breakdowns/components/component-alert-timeline/verification.json`
- measurement：`evidence/ui-image-breakdowns/components/component-alert-timeline/measurement.json`
- text ledger：`evidence/ui-image-breakdowns/components/component-alert-timeline/text-ocr.txt`
- Chrome/CDP：`evidence/ui-image-breakdowns/components/component-alert-timeline/cdp-version.json`
- 截图元数据：`evidence/ui-image-breakdowns/components/component-alert-timeline/capture-meta.json`
- 当前 mismatch ratio：`0.0`
- Windows Chrome 状态：`Chrome/150.0.7871.47`，`Windows Chrome CDP`，`1920x1080`，DPR `1`

## 差异清单

| 类型 | 位置 | 当前 | 期望 | 状态 |
|---|---|---|---|---|
| scope | 生产 React 实现 | 当前 pixel 验收使用 reference-raster | 后续生产组件按本记录实现 React/Ant Design 语义 | documented |
| semantics | 时间线节点 | 目标图使用圆环和文字，不展示具体时间戳 | 生产实现可补充 tooltip 显示时间、证据和审计链路 | documented |
| interaction | 关闭节点 | 目标图只显示关闭节点，没有确认弹窗 | 生产实现点击关闭时需权限、影响范围和审计 | documented |

## 主线程补充核对

- 顶部标题、左侧时间线、事件表、右侧状态矩阵、底部语义卡均已按 1920x1080 坐标系单独定位。
- 五个时间线节点已按颜色和标签逐个记录。
- 事件表三行数据已逐字校正。
- 状态矩阵八个状态已逐条记录。
- 底部六张语义卡已逐张记录。
- 已逐项回看 target、implementation、diff 和 overlay 四类证据图。
- 辅助智能体只负责查漏，主线程保留最终判断权。

## 结论

- 当前状态：`pixel-accepted`。
- 深拆完整性：已覆盖区域坐标、文本、组件、图标、token、状态交互、实现映射和验收证据路径。
- pixel-accepted 判定：Windows Chrome 截图与目标图一致，diff mismatch ratio 为 `0.0`，overlay 区域覆盖与记录一致。
