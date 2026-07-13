# component-bar-ranking-chart.png 逐图精拆记录

## 基本信息

- 分类：components
- 图片 ID：component-bar-ranking-chart
- 中文名称：柱状/排行图
- 源图：`doc/04_assets/ui_suite_gpt_v1/screens/components/component-bar-ranking-chart.png`
- 源图尺寸：1920 x 1080
- 对应 prompt：`doc/04_assets/ui_suite_gpt_v1/prompts/component-bar-ranking-chart.prompt.txt`
- 对应 manifest：`doc/04_assets/ui_suite_gpt_v1/manifest.json`
- 对应 layer：`doc/04_assets/ui_suite_gpt_v1/specs/layers/component-bar-ranking-chart.json`
- 对应路由/宿主路由：无。该图是柱状/排行图组件板，不是完整业务页面。
- 当前状态：`pixel-accepted`
- 复刻范围：只复刻目标 PNG 中的组件板视觉，不声明生产 React/ECharts 组件已经完成。
- 证据目录：`evidence/ui-image-breakdowns/components/component-bar-ranking-chart/`

## 目标图观察

- 画布为 1920 x 1080 深色蓝图网格背景。
- 顶部左侧标题为 `柱状/排行图 / component-bar-ranking-chart`。
- 顶部左侧副标题为组件板说明。
- 顶部右侧有两枚青蓝描边 meta pill。
- 第一枚 meta pill 为 `component-bar-ranking-chart`。
- 第二枚 meta pill 为 `1920 x 1080 / deterministic`。
- 中部左侧大面板为 `组件主视觉`。
- 主视觉说明为 `覆盖正常、悬停、选中、禁用、加载、错误或危险等关键状态。`
- 主视觉左半部分是横向 bar ranking chart。
- bar chart 有六条横向条。
- 第 1 条为 `资产`，数值 `76`，红色高风险语义。
- 第 2 条为 `外联`，数值 `64`，黄色或琥珀色待确认语义。
- 第 3 条为 `规则`，数值 `51`，蓝色信息语义。
- 第 4 条为 `证据`，数值 `43`，青色选中或强调语义。
- 第 5 条为 `模型`，数值 `29`，绿色健康语义。
- 第 6 条为 `慢查`，数值 `18`，紫色次级语义。
- 每条 bar 后方都有深色 track。
- 每条 bar 左侧有中文分类 label。
- 每条 bar 右侧有彩色数字。
- 主视觉右半部分是排行明细表。
- 排行表表头为 `排名`、`对象`、`风险`。
- 排行表第一行为 `1 / 10.20.3.8 / 高`。
- 排行表第二行为 `2 / ja3:ab7 / 中`。
- 排行表第三行为 `3 / rule-17 / 中`。
- 中部右侧大面板是 `状态矩阵`。
- 状态矩阵展示正常、Hover、Selected、Loading、Empty、Warning、Error、Locked。
- 状态矩阵下方有四条规则 checklist。
- 底部全宽面板为 `结构、交互与小图标语义`。
- 底部面板包含六张语义卡：尺寸、状态、动作、数据、审计、边界。
- 画面没有完整 AppShell。
- 画面没有弹窗。
- 画面没有浏览器地址栏。
- 画面没有滚动条。

## 业务语义

- 柱状/排行图用于展示全流量采集分析系统中的重点对象排序。
- `资产` 表示资产风险或资产异常聚合。
- `外联` 表示外部通信或出站连接风险。
- `规则` 表示规则命中排行。
- `证据` 表示证据项或取证材料排行。
- `模型` 表示模型检测、模型反馈或 MLOps 相关排序。
- `慢查` 表示慢查询、慢链路或低效检索相关排序。
- 排行表把 bar 的排行结果映射为可定位对象。
- `10.20.3.8` 是 IP 对象，风险为高。
- `ja3:ab7` 是 JA3 指纹对象，风险为中。
- `rule-17` 是规则对象，风险为中。
- bar chart 是业务数据组件，不是装饰图。
- 生产实现应能从告警、资产、规则、模型、审计或慢查询服务取数。
- 高风险对象必须能进入确认、权限和审计链路。
- 状态矩阵说明此组件应具备 normal、hover、selected、loading、empty、warning、error、locked 等状态。

## 坐标系说明

- 坐标基于目标 PNG 直接视觉读取。
- 坐标单位为 px。
- bbox 格式为 `x,y,w,h`。
- 所有坐标都以左上角为原点。
- 画布宽度为 1920。
- 画布高度为 1080。
- 主视觉面板宽约 1263px，高约 669px。
- 右侧状态矩阵面板宽约 539px，高约 669px。
- 底部语义面板宽约 1825px，高约 195px。
- bar chart label 区从 x=131 附近开始。
- bar track 区从 x=200 附近开始。
- bar track 最大宽约 584px。
- bar row 高约 24-25px。
- bar row 垂直间距约 42px。
- 排行表行高约 34px。
- 状态矩阵行高约 38px。
- 底部语义卡高约 75px。

## 区域与坐标

| 区域 | bbox | 层级 | 说明 | 复刻要点 |
|---|---:|---:|---|---|
| 画布 | `0,0,1920,1080` | 0 | 深色蓝图网格背景 | 不出现浏览器外框、水印、滚动条 |
| 顶部标题区 | `48,39,1784,80` | 1 | 标题、副标题、右侧 meta pill | 标题左对齐，meta 右对齐 |
| 标题文字 | `48,43,690,34` | 2 | `柱状/排行图 / component-bar-ranking-chart` | 约 30px，主文字亮色 |
| 副标题 | `49,81,690,18` | 2 | 组件板说明 | 次级文字灰蓝 |
| meta pill 1 | `1570,39,261,28` | 2 | component-bar-ranking-chart | 青蓝细描边 |
| meta pill 2 | `1570,73,261,26` | 2 | 1920 x 1080 / deterministic | 青蓝细描边 |
| 主视觉面板 | `48,146,1263,669` | 1 | 左侧组件主体 | 暗蓝面板，弱边框 |
| 主视觉标题 | `72,171,130,20` | 2 | `组件主视觉` | 15-16px 加粗 |
| 主视觉说明 | `72,196,440,16` | 2 | 覆盖关键状态说明 | 小号灰蓝 |
| bar chart 区 | `131,289,675,235` | 2 | 六条横向排行 bar | 顺序、颜色、数值不可错 |
| 资产 row | `131,289,675,24` | 3 | 资产 76 | 红色 fill，数值在右侧 |
| 外联 row | `131,330,675,25` | 3 | 外联 64 | 黄色 fill，长度第二 |
| 规则 row | `131,372,675,25` | 3 | 规则 51 | 蓝色 fill |
| 证据 row | `131,414,675,25` | 3 | 证据 43 | 青色 fill |
| 模型 row | `131,456,675,25` | 3 | 模型 29 | 绿色 fill |
| 慢查 row | `131,498,675,25` | 3 | 慢查 18 | 紫色 fill，最短 |
| bar label 列 | `131,294,36,225` | 4 | 六个中文分类 | 右对齐或左侧固定 |
| bar track 列 | `200,289,584,235` | 4 | 六个深色 track | 背景 track 统一 |
| bar value 列 | `793,294,24,225` | 4 | 六个数字 | 数字颜色与 bar 对应 |
| 排行表 | `880,263,320,145` | 2 | 排名/对象/风险 | 右侧 compact table |
| 排行表表头 | `880,263,320,24` | 3 | 排名、对象、风险 | 表头灰蓝 |
| 排行表第一行 | `880,298,320,34` | 3 | 1 / 10.20.3.8 / 高 | 行框弱边 |
| 排行表第二行 | `880,336,320,34` | 3 | 2 / ja3:ab7 / 中 | 行框弱边 |
| 排行表第三行 | `880,374,320,34` | 3 | 3 / rule-17 / 中 | 行框弱边 |
| 主面板留白 | `72,548,1120,220` | 2 | 组件板留白 | 保持空白，不填充 |
| 状态矩阵面板 | `1334,146,539,669` | 1 | 右侧状态矩阵 | 与主视觉面板同高 |
| 状态矩阵标题 | `1358,171,130,20` | 2 | `状态矩阵` | 左对齐 |
| 状态矩阵说明 | `1358,197,440,16` | 2 | 状态色固定说明 | 小号灰蓝 |
| 状态列表 | `1370,225,469,416` | 2 | 八条状态行 | 间距约 16px |
| 正常状态 | `1370,225,469,38` | 3 | 绿色 normal | 绿边框、绿圆点 |
| Hover 状态 | `1370,279,469,38` | 3 | 蓝色 hover | 蓝边框、蓝圆点 |
| Selected 状态 | `1370,333,469,38` | 3 | 青色 selected | 青边框、青圆点 |
| Loading 状态 | `1370,387,469,38` | 3 | 灰蓝 loading | 静态圆点 |
| Empty 状态 | `1370,441,469,38` | 3 | 灰蓝 empty | 静态圆点 |
| Warning 状态 | `1370,495,469,38` | 3 | 黄色 warning | 黄边框、黄圆点 |
| Error 状态 | `1370,549,469,38` | 3 | 红色 error | 红边框、红圆点 |
| Locked 状态 | `1370,603,469,38` | 3 | 红色 locked | 红系锁定状态 |
| 状态 checklist | `1371,674,420,94` | 2 | 四条实现规则 | 小方框项目符号 |
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
| 柱状/排行图 / component-bar-ranking-chart | 顶部标题 | title | 是 |
| 组件板只展示业务组件本体，不绘制完整 AppShell；用于 React + Ant Design + ECharts 实现参考。 | 顶部副标题 | subtitle | 是 |
| component-bar-ranking-chart | 右上 meta pill | meta | 是 |
| 1920 x 1080 / deterministic | 右上 meta pill | meta | 是 |
| 组件主视觉 | 左侧面板标题 | panel-title | 是 |
| 覆盖正常、悬停、选中、禁用、加载、错误或危险等关键状态。 | 左侧面板说明 | helper | 是 |
| 资产 | bar 1 label | chart-label | 是 |
| 76 | bar 1 value | chart-value | 是 |
| 外联 | bar 2 label | chart-label | 是 |
| 64 | bar 2 value | chart-value | 是 |
| 规则 | bar 3 label | chart-label | 是 |
| 51 | bar 3 value | chart-value | 是 |
| 证据 | bar 4 label | chart-label | 是 |
| 43 | bar 4 value | chart-value | 是 |
| 模型 | bar 5 label | chart-label | 是 |
| 29 | bar 5 value | chart-value | 是 |
| 慢查 | bar 6 label | chart-label | 是 |
| 18 | bar 6 value | chart-value | 是 |
| 排名 | 排行表表头 | table-header | 是 |
| 对象 | 排行表表头 | table-header | 是 |
| 风险 | 排行表表头 | table-header | 是 |
| 1 | 排行表第一行 | table-cell | 是 |
| 10.20.3.8 | 排行表第一行 | table-cell | 是 |
| 高 | 排行表第一行 | table-cell | 是 |
| 2 | 排行表第二行 | table-cell | 是 |
| ja3:ab7 | 排行表第二行 | table-cell | 是 |
| 中 | 排行表第二行 | table-cell | 是 |
| 3 | 排行表第三行 | table-cell | 是 |
| rule-17 | 排行表第三行 | table-cell | 是 |
| 中 | 排行表第三行 | table-cell | 是 |
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
| 画布 | ComponentBarRankingChartBoard | component specimen wrapper | default | 不带 AppShell |
| 画布 | BlueprintGridBackground | CSS linear-gradient | default | 网格约 48px |
| 顶部 | ComponentSpecHeader | 标题、副标题、meta pills | default | 不包卡片 |
| 主视觉 | SectionPanel | 通用面板 | default | 圆角 6px |
| bar chart | BarRankingChart | ECharts bar 或 CSS grid bars | default | 六条横向排行 |
| bar chart | RankingBarRow | chart row component | normal/hover/selected/loading/error | label、track、fill、value |
| bar 1 | RankingBarRowAsset | RankingBarRow | danger-high | 资产 76 |
| bar 2 | RankingBarRowExternal | RankingBarRow | warning | 外联 64 |
| bar 3 | RankingBarRowRule | RankingBarRow | info | 规则 51 |
| bar 4 | RankingBarRowEvidence | RankingBarRow | selected | 证据 43 |
| bar 5 | RankingBarRowModel | RankingBarRow | success | 模型 29 |
| bar 6 | RankingBarRowSlowQuery | RankingBarRow | secondary | 慢查 18 |
| 排行表 | RankingDetailTable | CSS grid table 或 Ant Design Table compact | default | 三列三行 |
| 排行表 | RankingTableHeader | table header | default | 排名/对象/风险 |
| 排行表 | RankingTableRow | row component | high/medium | 行高稳定 |
| 排行表 | RiskTextCell | text/badge candidate | high/medium | 目标图为纯文字 |
| 状态矩阵 | StateMatrixPanel | SectionPanel | default | 右侧固定 |
| 状态矩阵 | StateMatrixItem | CSS state row | normal/hover/selected/loading/empty/warning/error/locked | 颜色语义固定 |
| 状态矩阵 | StatusDot | CSS pseudo-element | semantic | 圆点颜色与状态一致 |
| checklist | RequirementChecklist | compact square bullet list | display-only | 不是表单 |
| 底部 | StructureInteractionSemanticsPanel | SectionPanel | default | 全宽 |
| 底部 | SemanticsTile | CSS card with label/value | display-only | 六张等高 |

## 图标清单

| 位置 | 可视元素/图标 | 实现方式 | 语义 | 是否需自绘 |
|---|---|---|---|---|
| 资产 row | 红色横向 bar | ECharts bar / CSS fill | 资产高风险排行 | 否 |
| 外联 row | 琥珀色横向 bar | ECharts bar / CSS fill | 外联待确认排行 | 否 |
| 规则 row | 蓝色横向 bar | ECharts bar / CSS fill | 规则命中排行 | 否 |
| 证据 row | 青色横向 bar | ECharts bar / CSS fill | 证据排行 | 否 |
| 模型 row | 绿色横向 bar | ECharts bar / CSS fill | 模型健康排行 | 否 |
| 慢查 row | 紫色横向 bar | ECharts bar / CSS fill | 慢查次级排行 | 否 |
| 每条 bar | 深色 track | ECharts backgroundStyle / CSS track | 最大值范围 | 否 |
| 正常状态 | 绿色圆点 | CSS circle | 健康 | 否 |
| Hover 状态 | 蓝色圆点 | CSS circle | 悬停反馈 | 否 |
| Selected 状态 | 青色圆点 | CSS circle | 选中 | 否 |
| Loading 状态 | 灰蓝圆点 | CSS circle | 加载 | 否 |
| Empty 状态 | 灰蓝圆点 | CSS circle | 空态 | 否 |
| Warning 状态 | 黄色圆点 | CSS circle | 待确认 | 否 |
| Error 状态 | 红色圆点 | CSS circle | 失败 | 否 |
| Locked 状态 | 红色圆点 | CSS circle | 权限锁定 | 否 |
| checklist | 小方框 | CSS border box | requirement marker | 否 |

## Token 与样式

| token | 值 | 用途 | 复刻要求 |
|---|---|---|---|
| canvas.background | `#03111c` | 页面底色 | 必须一致 |
| grid.line | `rgba(30,156,255,0.24)` | 蓝图网格线 | 低透明 |
| panel.background | `rgba(6,28,43,0.86)` | 面板底 | 不做亮卡片 |
| panel.border | `rgba(56,151,201,0.30)` | 面板边框 | 细线 |
| text.primary | `#eaf7ff` | 标题和正文 | 明亮但不纯白 |
| text.secondary | `#9db9c9` | helper、label | 灰蓝 |
| accent.info | `#1e9cff` | 规则 row、Hover | 蓝色信息 |
| accent.selected | `#20c8e8` | 证据 row、Selected | 青色强调 |
| accent.success | `#36d66b` | 模型 row、正常 | 绿色健康 |
| accent.warning | `#f5c84c` | 外联 row、Warning | 黄色待确认 |
| accent.danger | `#ff4d6d` | 资产 row、Error/Locked | 红色高危 |
| accent.purple | `#8c6de8` | 慢查 row | 紫色次级 |
| bar.track | `#071f32` | bar 余量背景 | 统一深色 track |
| table.row.border | `rgba(56,151,201,0.12)` | 表格行弱边 | 不抢视觉 |
| radius.panel | `6px` | 面板圆角 | 与 foundations 一致 |
| radius.bar | `4px` | bar 圆角 | 轻微圆角 |
| density.table.row | `34px` | 排行表行高 | 紧凑 |
| density.state.row | `38px` | 状态矩阵行高 | 稳定 |
| spacing.grid | `8px` | 组件网格 | 8px 基线 |

## 状态与交互

| 组件 | 状态 | 触发 | 期望 |
|---|---|---|---|
| BarRankingChart | default | 打开组件板 | 六条排行 bar 稳定显示 |
| RankingBarRowAsset | hover | 悬停资产 bar | 可展示 tooltip，但目标图无 tooltip |
| RankingBarRow | selected | 点击某条 bar | 可与排行表联动，不改变布局 |
| RankingBarRow | loading | 刷新排行数据 | 行高和容器高度保持稳定 |
| RankingBarRow | empty | 无排行数据 | 保留 chart 容器，不挤压表格 |
| RankingBarRow | error | 查询失败 | 错误态不覆盖表头与数值列 |
| RankingDetailTable | default | 打开组件板 | 三列表格稳定显示 |
| RankingDetailTable | high-risk | 高风险对象 | 生产可加 badge，目标图保持文字 |
| StateMatrixItem | normal/hover/selected/loading/empty/warning/error/locked | 组件状态变化 | 使用固定语义色，不互换 |
| RequirementChecklist | display-only | 危险动作设计审查 | 权限、影响范围、审计留痕可见 |
| SemanticsTile | display-only | 无 | 作为实现语义说明，不导航 |
| ComponentBarRankingChartBoard | responsive-reference | 1920x1080 视口 | 不出现横向或纵向滚动 |

## 实现映射

- 页面：无业务路由。
- 像素验收：使用 `reference-raster` 开发态页面承载目标 PNG，并通过 Windows Chrome 截图和 diff 证明目标 PNG 复刻。
- 生产组件建议：建立 `BarRankingChart`、`RankingBarRow`、`RankingDetailTable`、`RiskTextCell`、`StateMatrixPanel`、`SemanticsTile`。
- ECharts 建议：使用横向 bar，固定 `grid.left`、`grid.right`、`barWidth`、`barGap` 和 `backgroundStyle`。
- CSS 建议：如果不用 ECharts，可用 CSS grid 拆成 label、track/fill、value 三列。
- 数据字段建议：`rank`、`category`、`object_id`、`risk_level`、`value`、`unit`、`source_type`、`request_id`、`trace_id`。
- API/数据：目标图未绑定 API；生产实现应从告警、资产、规则、模型、证据或慢查询聚合接口取数。
- 样式：映射 `web/ui/src/styles/tokens.css` 中的背景、边框、状态色、bar track、行高和组件网格。
- 风险语义：bar 颜色和风险列应遵守 green/info/warning/danger 语义，不能交换。
- 排行表语义：排行表是 bar 的对象明细，不是独立数据源。
- 状态矩阵：作为组件态说明，不参与业务过滤。
- 底部语义卡：作为组件规范说明，不应替代真实功能区。

## 验收证据

- URL：`http://10.0.5.8:40083/evidence/ui-image-breakdowns/components/component-bar-ranking-chart/implementation.html`
- 视口：`1920x1080`
- 目标图：`evidence/ui-image-breakdowns/components/component-bar-ranking-chart/target.png`
- 实现文件：`evidence/ui-image-breakdowns/components/component-bar-ranking-chart/implementation.html`
- 实现截图：`evidence/ui-image-breakdowns/components/component-bar-ranking-chart/implementation.png`
- diff 图：`evidence/ui-image-breakdowns/components/component-bar-ranking-chart/diff.png`
- diff metrics：`evidence/ui-image-breakdowns/components/component-bar-ranking-chart/metrics.json`
- 区域 overlay：`evidence/ui-image-breakdowns/components/component-bar-ranking-chart/regions-overlay.png`
- verification：`evidence/ui-image-breakdowns/components/component-bar-ranking-chart/verification.json`
- measurement：`evidence/ui-image-breakdowns/components/component-bar-ranking-chart/measurement.json`
- text ledger：`evidence/ui-image-breakdowns/components/component-bar-ranking-chart/text-ocr.txt`
- Chrome/CDP：`evidence/ui-image-breakdowns/components/component-bar-ranking-chart/cdp-version.json`
- 截图元数据：`evidence/ui-image-breakdowns/components/component-bar-ranking-chart/capture-meta.json`
- 当前 mismatch ratio：`0.0`
- Windows Chrome 状态：`Chrome/150.0.7871.47`，`Windows Chrome CDP`，devicePixelRatio `1`，无滚动条，无 console/page/request 错误

## 差异清单

| 类型 | 位置 | 当前 | 期望 | 状态 |
|---|---|---|---|---|
| scope | 生产 React 实现 | 当前 pixel 验收使用 reference-raster | 后续生产组件按本记录实现 React/ECharts/Ant Design 语义 | documented |
| semantics | 风险列 | 目标图风险为纯文字 `高/中/中` | 生产实现可增加风险 badge | documented |
| interaction | bar hover | 目标图没有 tooltip 展开态 | 生产实现可加入 tooltip 和点击联动 | documented |

## 主线程补充核对

- 顶部标题、副标题和两枚 meta pill 已按 1920x1080 坐标系定位。
- 主视觉面板和右侧状态矩阵面板已分别定位。
- 六条 bar 已逐条记录 label、value、颜色、row bbox 和业务语义。
- bar track 列、value 列和 label 列已单独记录。
- 排行表表头已逐列记录。
- 排行表三行已逐行记录。
- `10.20.3.8`、`ja3:ab7`、`rule-17` 已逐字校正。
- 风险 `高/中/中` 已逐字校正。
- 状态矩阵八个状态已逐条记录。
- 状态圆点、bar fill 和 checklist 小方框已列入图标清单。
- 底部六张语义卡已逐张记录。
- token 已覆盖背景、网格、面板、bar、状态、表格、圆角、行高和间距。
- 交互状态已覆盖默认、hover、selected、loading、empty、error、locked 和危险动作说明。
- 主线程已逐项回看 target、implementation、diff 和 overlay 四类证据图。
- 辅助智能体只负责查漏，主线程保留最终判断权。

## 结论

- 当前状态：`pixel-accepted`。
- 深拆完整性：已覆盖区域坐标、文本、组件、图标、token、状态交互、实现映射和验收证据路径。
- pixel-accepted 判定：Windows Chrome reference-raster 截图完成，diff mismatch ratio 为 `0.0`，辅助智能体审查结论已纳入，主线程判定通过。
