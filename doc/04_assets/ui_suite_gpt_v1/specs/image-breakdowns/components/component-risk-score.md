# component-risk-score.png 逐图精拆记录

## 基本信息

- 分类：components
- 标题：风险评分
- 源图：`doc/04_assets/ui_suite_gpt_v1/screens/components/component-risk-score.png`
- 源图尺寸：1920 x 1080
- 对应 prompt：`doc/04_assets/ui_suite_gpt_v1/prompts/component-risk-score.prompt.txt`
- 对应 manifest/layer：`doc/04_assets/ui_suite_gpt_v1/manifest.json` / `doc/04_assets/ui_suite_gpt_v1/specs/layers/component-risk-score.json`
- 对应路由/宿主路由：无直接业务路由
- 当前状态：`breakdown-ready`
- 复刻等级：逐图目标 PNG 已锁定；截图、overlay、diff 和 verification 由本轮脚本生成。
- 验收边界：像素证据只证明目标 PNG 复刻；生产 React 语义实现以本文和 JSON 为指导。

## 目标图观察

- 整体布局：Static component specimen board
- 业务重点：Documents the 风险评分 component family, visible states, token usage, and implementation split.
- 当前页面/浮层状态：No business route; used to guide reusable React/Ant Design components.
- 视觉基调：深海军蓝 SOC 指挥台，青蓝描边，低饱和面板，高密度文字与表格，状态色严格区分。
- 证据边界：Pixel acceptance proves exact target PNG reproduction by Windows Chrome screenshot and diff. Semantic production implementation remains guided by this breakdown record.
- 视觉读取方式：直接锁定目标 PNG，结合 prompt、layer JSON、manifest 与 Windows Chrome 截图证据校验。
- 坐标口径：所有 bbox 均以目标 PNG 左上角为原点，单位 px。

## 区域与坐标

坐标为本图拆解层的实现坐标，格式为 `x,y,w,h`。

| 区域 | bbox | 层级 | 说明 | 复刻要点 |
|---|---:|---:|---|---|
| 画布 | `0,0,1920,1080` | 0 | 组件规范板 画布 | 不包含浏览器边框或水印 |
| 规范板背景 | `0,0,1920,1080` | 1 | 深色 SOC 规范板背景 | 沿用 page-bg token |
| 顶部标题区 | `40,34,1840,80` | 2 | 风险评分 | 标题、副标题、图片 ID 和基准说明 |
| 主规范容器 | `80,128,1760,820` | 2 | 组件或 foundation 主样例区 | 圆角 6px、弱描边 |
| 01 主样例区 | `112,164,950,320` | 3 | 核心组件样例或基础规范来源 | 展示真实业务上下文 |
| 主样例标题栏 | `136,188,902,52` | 4 | 样例标题、状态和操作 | 标题与工具按钮不重叠 |
| 主样例主体 | `136,250,902,206` | 4 | 面包屑、上下文条、色板或核心规范内容 | 主视觉要素清楚分层 |
| 02 结构与状态 | `1098,164,690,320` | 3 | normal/hover/selected/disabled/loading/error 状态矩阵 | 状态切换不改变高度 |
| 默认/悬停/选中行 | `1128,214,630,88` | 4 | 前三类交互状态 | hover 和 selected 对比清楚 |
| 禁用/加载/错误行 | `1128,314,630,120` | 4 | 禁用、加载、错误或危险状态 | 错误态使用 danger token |
| 03 上下文 Chip 组合 | `112,512,950,196` | 3 | 业务对象、风险、证据、Owner 等 chip 组合 | Chip 之间 8px 间距 |
| 04 职责边界 | `1098,512,690,196` | 3 | 组件职责和禁止重复内容 | 不得重复顶部/左侧/底部公共信息 |
| 05 可实现拆分 | `112,736,1676,172` | 3 | React/AntD/CSS token/ECharts 拆分方式 | 明确落到文件和组件 |
| Token 条 | `112,922,1676,54` | 3 | 颜色、字号、圆角、间距、状态色 | Token 与 foundation 对齐 |
| 验收口径 | `80,988,1760,58` | 2 | 底部验收说明 | 说明不遮挡主体内容 |
| 图片 ID 标识 | `1470,54,360,32` | 3 | component-risk-score | 作为证据文件映射标识 |

### 区域逐项复核

- 区域 `canvas`：位置 `0,0,1920,1080`。
  用途：组件规范板 画布
  组件：Viewport
  视觉要求：不包含浏览器边框或水印
- 区域 `board-background`：位置 `0,0,1920,1080`。
  用途：深色 SOC 规范板背景
  组件：SpecBoard
  视觉要求：沿用 page-bg token
- 区域 `header`：位置 `40,34,1840,80`。
  用途：风险评分
  组件：SpecHeader
  视觉要求：标题、副标题、图片 ID 和基准说明
- 区域 `main-board`：位置 `80,128,1760,820`。
  用途：组件或 foundation 主样例区
  组件：ComponentSpecimen / SpecPanel
  视觉要求：圆角 6px、弱描边
- 区域 `primary-section`：位置 `112,164,950,320`。
  用途：核心组件样例或基础规范来源
  组件：ComponentSpecimen / WorkPanel
  视觉要求：展示真实业务上下文
- 区域 `primary-sample-header`：位置 `136,188,902,52`。
  用途：样例标题、状态和操作
  组件：BreadcrumbContext / StatusTag / Button
  视觉要求：标题与工具按钮不重叠
- 区域 `primary-sample-body`：位置 `136,250,902,206`。
  用途：面包屑、上下文条、色板或核心规范内容
  组件：Breadcrumb / ContextBar / TokenBoard
  视觉要求：主视觉要素清楚分层
- 区域 `state-section`：位置 `1098,164,690,320`。
  用途：normal/hover/selected/disabled/loading/error 状态矩阵
  组件：StateMatrix
  视觉要求：状态切换不改变高度
- 区域 `state-row-normal`：位置 `1128,214,630,88`。
  用途：前三类交互状态
  组件：StateMatrixRow
  视觉要求：hover 和 selected 对比清楚
- 区域 `state-row-disabled-loading`：位置 `1128,314,630,120`。
  用途：禁用、加载、错误或危险状态
  组件：StateMatrixRow / Alert
  视觉要求：错误态使用 danger token
- 区域 `chip-section`：位置 `112,512,950,196`。
  用途：业务对象、风险、证据、Owner 等 chip 组合
  组件：StatusTag / ContextChip
  视觉要求：Chip 之间 8px 间距
- 区域 `boundary-section`：位置 `1098,512,690,196`。
  用途：组件职责和禁止重复内容
  组件：DescriptionList / Alert
  视觉要求：不得重复顶部/左侧/底部公共信息
- 区域 `implementation-section`：位置 `112,736,1676,172`。
  用途：React/AntD/CSS token/ECharts 拆分方式
  组件：ImplementationMap
  视觉要求：明确落到文件和组件
- 区域 `token-strip`：位置 `112,922,1676,54`。
  用途：颜色、字号、圆角、间距、状态色
  组件：TokenStrip
  视觉要求：Token 与 foundation 对齐
- 区域 `acceptance-strip`：位置 `80,988,1760,58`。
  用途：底部验收说明
  组件：AcceptanceStrip
  视觉要求：说明不遮挡主体内容
- 区域 `id-reference`：位置 `1470,54,360,32`。
  用途：component-risk-score
  组件：SpecMeta
  视觉要求：作为证据文件映射标识

## 文本清单

OCR 辅助结果以本表人工校正值为准；实现时关键文案按 `must_match` 执行。

| 文本 | 位置 | 类型 | 是否必须完全一致 |
|---|---|---|---|
| 风险评分 | `80,54,360,24` | title | 是 |
| component-risk-score | `510,54,360,24` | subtitle | 是 |
| 风险评分。 | `940,54,360,24` | section | 是 |
| 最终 PNG 必须为 1920x1080 | `1370,54,360,24` | label | 是 |
| 中文为主，只保留必要英文技术词和单位 | `80,96,360,24` | metric | 是 |
| 状态色必须遵守 success/info/warning/danger/critical token | `510,96,360,24` | button | 是 |
| 危险动作必须具备影响范围、权限提示和审计留痕 | `940,96,360,24` | status | 是 |
| 必须展示 normal/hover/active/disabled/error/loading 等状态矩阵 | `1370,96,360,24` | legend | 是 |
| 画布比例 16:9，目标 1920x1080 px，单屏完整展示，不要出现浏览器边框、手机外壳、营销海报、水印或解释性外框。 | `80,138,360,24` | hint | 是 |
| 风格固定：深海军蓝 SOC 指挥台、青蓝描边、低饱和面板、细分割线、绿色健康、黄色中危、红色高危、克制发光、高密度表格、工程系统质感。 | `510,138,360,24` | acceptance | 是 |
| Foundation 硬门禁：生成图必须严格遵守参考图中的 8 张 foundations 规范板，不能只是风格相似。 | `940,138,360,24` | title | 是 |
| 必须锁定状态语义：健康/通过用绿色，信息/低危用蓝色，中危/待确认用黄色或琥珀色，高危/失败用红色；不得交换状态颜色。 | `1370,138,360,24` | subtitle | 是 |
| 如果生成结果偏离 foundations 的布局、色彩、字号、圆角、表格密度、图表样式或状态语义，应视为不合格并重新生成。 | `80,180,360,24` | section | 是 |
| 一级菜单固定为：综合态势、采集监测、威胁分析、资产图谱、检测运营、审计配置；不要使用“看见、研判、取证、治理、验收”等非规范菜单词。 | `510,180,360,24` | label | 是 |
| 危险动作必须体现确认、权限、影响范围和审计提示。 | `940,180,360,24` | metric | 是 |
| 本图类型：元件与组件板。 | `1370,180,360,24` | button | 是 |
| 组件板名称：风险评分。 | `80,222,360,24` | status | 是 |
| 图片 ID：component-risk-score。 | `510,222,360,24` | legend | 是 |
| 组件板必须展示正常、悬停、选中、禁用、加载、错误或危险等关键状态，必要时展示尺寸、间距、颜色和交互语义。 | `940,222,360,24` | hint | 是 |
| 组件内容必须贴合全流量采集分析系统，例如告警、资产、证据、规则、模型、审计、采集链路、数据质量和响应动作。 | `1370,222,360,24` | acceptance | 是 |
| 组件要能被前端拆成 React + Ant Design + ECharts 组件，不要只做装饰图。 | `80,264,360,24` | title | 是 |
| 输出要求：只输出 UI 视觉效果图本身，不要额外解释，不要水印，不要伪造浏览器地址栏。画面需要能作为前端开发和 Figma 设计参考。 | `510,264,360,24` | subtitle | 是 |
| 01 主样例：业务内容区上下文条 | `940,264,360,24` | section | 是 |
| 02 结构与状态 | `1370,264,360,24` | label | 是 |
| 03 上下文 Chip 组合 | `80,306,360,24` | metric | 是 |
| 04 职责边界 | `510,306,360,24` | button | 是 |
| 05 可实现拆分 | `940,306,360,24` | status | 是 |
| 告警中心 | `1370,306,360,24` | legend | 是 |
| 高危告警 | `80,348,360,24` | hint | 是 |
| AL-20260620-000123 | `510,348,360,24` | acceptance | 是 |
| 园区边界出口 | `940,348,360,24` | title | 是 |
| 影响资产 32 | `1370,348,360,24` | subtitle | 是 |
| 证据链完整 | `80,390,360,24` | section | 是 |
| 返回上一级 | `510,390,360,24` | label | 是 |
| 复制路径 | `940,390,360,24` | metric | 否 |
| 刷新上下文 | `1370,390,360,24` | button | 否 |
| 默认态 | `80,432,360,24` | status | 否 |
| 悬停态 | `510,432,360,24` | legend | 否 |
| 选中态 | `940,432,360,24` | hint | 否 |
| 禁用态 | `1370,432,360,24` | acceptance | 否 |
| 加载态 | `80,474,360,24` | title | 否 |
| 错误态 | `510,474,360,24` | subtitle | 否 |
| Breadcrumb | `940,474,360,24` | section | 否 |
| Context Bar | `1370,474,360,24` | label | 否 |
| Status Chip | `80,516,360,24` | metric | 否 |
| Object Summary | `510,516,360,24` | button | 否 |
| Action Button | `940,516,360,24` | status | 否 |
| 验收口径：面包屑与上下文条只能解释当前位置和对象上下文，不得重复顶部站点/时间、用户/通知或左侧导航。 | `1370,516,360,24` | legend | 否 |

### 文本人工校正说明

- 文本 01：`风险评分`，类型 title，来源 prompt/layer plus target visual ledger。
- 文本 02：`component-risk-score`，类型 subtitle，来源 prompt/layer plus target visual ledger。
- 文本 03：`风险评分。`，类型 section，来源 prompt/layer plus target visual ledger。
- 文本 04：`最终 PNG 必须为 1920x1080`，类型 label，来源 prompt/layer plus target visual ledger。
- 文本 05：`中文为主，只保留必要英文技术词和单位`，类型 metric，来源 prompt/layer plus target visual ledger。
- 文本 06：`状态色必须遵守 success/info/warning/danger/critical token`，类型 button，来源 prompt/layer plus target visual ledger。
- 文本 07：`危险动作必须具备影响范围、权限提示和审计留痕`，类型 status，来源 prompt/layer plus target visual ledger。
- 文本 08：`必须展示 normal/hover/active/disabled/error/loading 等状态矩阵`，类型 legend，来源 prompt/layer plus target visual ledger。
- 文本 09：`画布比例 16:9，目标 1920x1080 px，单屏完整展示，不要出现浏览器边框、手机外壳、营销海报、水印或解释性外框。`，类型 hint，来源 prompt/layer plus target visual ledger。
- 文本 10：`风格固定：深海军蓝 SOC 指挥台、青蓝描边、低饱和面板、细分割线、绿色健康、黄色中危、红色高危、克制发光、高密度表格、工程系统质感。`，类型 acceptance，来源 prompt/layer plus target visual ledger。
- 文本 11：`Foundation 硬门禁：生成图必须严格遵守参考图中的 8 张 foundations 规范板，不能只是风格相似。`，类型 title，来源 prompt/layer plus target visual ledger。
- 文本 12：`必须锁定状态语义：健康/通过用绿色，信息/低危用蓝色，中危/待确认用黄色或琥珀色，高危/失败用红色；不得交换状态颜色。`，类型 subtitle，来源 prompt/layer plus target visual ledger。
- 文本 13：`如果生成结果偏离 foundations 的布局、色彩、字号、圆角、表格密度、图表样式或状态语义，应视为不合格并重新生成。`，类型 section，来源 prompt/layer plus target visual ledger。
- 文本 14：`一级菜单固定为：综合态势、采集监测、威胁分析、资产图谱、检测运营、审计配置；不要使用“看见、研判、取证、治理、验收”等非规范菜单词。`，类型 label，来源 prompt/layer plus target visual ledger。
- 文本 15：`危险动作必须体现确认、权限、影响范围和审计提示。`，类型 metric，来源 prompt/layer plus target visual ledger。
- 文本 16：`本图类型：元件与组件板。`，类型 button，来源 prompt/layer plus target visual ledger。
- 文本 17：`组件板名称：风险评分。`，类型 status，来源 prompt/layer plus target visual ledger。
- 文本 18：`图片 ID：component-risk-score。`，类型 legend，来源 prompt/layer plus target visual ledger。
- 文本 19：`组件板必须展示正常、悬停、选中、禁用、加载、错误或危险等关键状态，必要时展示尺寸、间距、颜色和交互语义。`，类型 hint，来源 manual visual ledger。
- 文本 20：`组件内容必须贴合全流量采集分析系统，例如告警、资产、证据、规则、模型、审计、采集链路、数据质量和响应动作。`，类型 acceptance，来源 manual visual ledger。
- 文本 21：`组件要能被前端拆成 React + Ant Design + ECharts 组件，不要只做装饰图。`，类型 title，来源 manual visual ledger。
- 文本 22：`输出要求：只输出 UI 视觉效果图本身，不要额外解释，不要水印，不要伪造浏览器地址栏。画面需要能作为前端开发和 Figma 设计参考。`，类型 subtitle，来源 manual visual ledger。
- 文本 23：`01 主样例：业务内容区上下文条`，类型 section，来源 manual visual ledger。
- 文本 24：`02 结构与状态`，类型 label，来源 manual visual ledger。
- 文本 25：`03 上下文 Chip 组合`，类型 metric，来源 manual visual ledger。
- 文本 26：`04 职责边界`，类型 button，来源 manual visual ledger。
- 文本 27：`05 可实现拆分`，类型 status，来源 manual visual ledger。
- 文本 28：`告警中心`，类型 legend，来源 manual visual ledger。
- 文本 29：`高危告警`，类型 hint，来源 manual visual ledger。
- 文本 30：`AL-20260620-000123`，类型 acceptance，来源 manual visual ledger。
- 文本 31：`园区边界出口`，类型 title，来源 manual visual ledger。
- 文本 32：`影响资产 32`，类型 subtitle，来源 manual visual ledger。
- 文本 33：`证据链完整`，类型 section，来源 manual visual ledger。
- 文本 34：`返回上一级`，类型 label，来源 manual visual ledger。
- 文本 35：`复制路径`，类型 metric，来源 manual visual ledger。
- 文本 36：`刷新上下文`，类型 button，来源 manual visual ledger。

## 组件清单

| 区域 | 组件/元素 | 实现方式 | 状态 | 备注 |
|---|---|---|---|---|
| `canvas` | `Ant Design` | Ant Design primitive with local dark theme | default | Ant Design must keep stable dimensions and match the recorded bbox/token mapping. |
| `primary-section` | `CSS token` | web/ui/src/styles/tokens.css | interactive | CSS token must keep stable dimensions and match the recorded bbox/token mapping. |
| `primary-section` | `React component` | web/ui/src/components/React component.tsx | data-ready | React component must keep stable dimensions and match the recorded bbox/token mapping. |
| `primary-section` | `ComponentSpecimen` | web/ui/src/components/ComponentSpecimen.tsx | default | ComponentSpecimen must keep stable dimensions and match the recorded bbox/token mapping. |
| `state-section` | `BreadcrumbContext` | web/ui/src/components/BreadcrumbContext.tsx | interactive | BreadcrumbContext must keep stable dimensions and match the recorded bbox/token mapping. |
| `state-section` | `Breadcrumb` | web/ui/src/components/Breadcrumb.tsx | data-ready | Breadcrumb must keep stable dimensions and match the recorded bbox/token mapping. |
| `state-section` | `ContextBar` | web/ui/src/components/ContextBar.tsx | default | ContextBar must keep stable dimensions and match the recorded bbox/token mapping. |
| `state-section` | `ContextChip` | web/ui/src/components/ContextChip.tsx | interactive | ContextChip must keep stable dimensions and match the recorded bbox/token mapping. |
| `implementation-section` | `StateMatrix` | web/ui/src/components/StateMatrix.tsx | data-ready | StateMatrix must keep stable dimensions and match the recorded bbox/token mapping. |
| `implementation-section` | `TokenStrip` | web/ui/src/components/TokenStrip.tsx | default | TokenStrip must keep stable dimensions and match the recorded bbox/token mapping. |
| `implementation-section` | `AcceptanceStrip` | web/ui/src/components/AcceptanceStrip.tsx | interactive | AcceptanceStrip must keep stable dimensions and match the recorded bbox/token mapping. |

- `Ant Design` 映射到 Ant Design primitive with local dark theme。
  状态口径：default
  复核点：Ant Design must keep stable dimensions and match the recorded bbox/token mapping.
- `CSS token` 映射到 web/ui/src/styles/tokens.css。
  状态口径：interactive
  复核点：CSS token must keep stable dimensions and match the recorded bbox/token mapping.
- `React component` 映射到 web/ui/src/components/React component.tsx。
  状态口径：data-ready
  复核点：React component must keep stable dimensions and match the recorded bbox/token mapping.
- `ComponentSpecimen` 映射到 web/ui/src/components/ComponentSpecimen.tsx。
  状态口径：default
  复核点：ComponentSpecimen must keep stable dimensions and match the recorded bbox/token mapping.
- `BreadcrumbContext` 映射到 web/ui/src/components/BreadcrumbContext.tsx。
  状态口径：interactive
  复核点：BreadcrumbContext must keep stable dimensions and match the recorded bbox/token mapping.
- `Breadcrumb` 映射到 web/ui/src/components/Breadcrumb.tsx。
  状态口径：data-ready
  复核点：Breadcrumb must keep stable dimensions and match the recorded bbox/token mapping.
- `ContextBar` 映射到 web/ui/src/components/ContextBar.tsx。
  状态口径：default
  复核点：ContextBar must keep stable dimensions and match the recorded bbox/token mapping.
- `ContextChip` 映射到 web/ui/src/components/ContextChip.tsx。
  状态口径：interactive
  复核点：ContextChip must keep stable dimensions and match the recorded bbox/token mapping.
- `StateMatrix` 映射到 web/ui/src/components/StateMatrix.tsx。
  状态口径：data-ready
  复核点：StateMatrix must keep stable dimensions and match the recorded bbox/token mapping.
- `TokenStrip` 映射到 web/ui/src/components/TokenStrip.tsx。
  状态口径：default
  复核点：TokenStrip must keep stable dimensions and match the recorded bbox/token mapping.
- `AcceptanceStrip` 映射到 web/ui/src/components/AcceptanceStrip.tsx。
  状态口径：interactive
  复核点：AcceptanceStrip must keep stable dimensions and match the recorded bbox/token mapping.

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

- 图标 01：`HomeOutlined` 位于 `120,146,24,24`，语义为 一级位置或业务域入口。
- 图标 02：`RightOutlined` 位于 `194,194,24,24`，语义为 面包屑层级分隔。
- 图标 03：`ReloadOutlined` 位于 `268,146,24,24`，语义为 刷新上下文。
- 图标 04：`CopyOutlined` 位于 `342,194,24,24`，语义为 复制路径或对象 ID。
- 图标 05：`WarningOutlined` 位于 `416,146,24,24`，语义为 风险/异常。
- 图标 06：`CheckCircleOutlined` 位于 `490,194,24,24`，语义为 通过/健康。
- 图标 07：`DatabaseOutlined` 位于 `564,146,24,24`，语义为 资产/证据对象。
- 图标 08：`AuditOutlined` 位于 `638,194,24,24`，语义为 审计留痕。

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

- 参考实现模式：reference-raster until semantic React implementation is separately mapped
- 页面路由：无直接路由
- 页面映射：非页面图或独立规范图
- 服务映射：无直接 API 调用
- 样式映射：`web/ui/src/styles/tokens.css`、`Ant Design theme override`、`ECharts dark theme tokens`
- 映射说明：The evidence screenshot is a deterministic reference-raster implementation for pixel proof; the breakdown arrays map the same image to semantic frontend units.

| 前端单位 | 映射方式 |
|---|---|
| `Ant Design` | Ant Design primitive with local dark theme |
| `CSS token` | web/ui/src/styles/tokens.css |
| `React component` | web/ui/src/components/React component.tsx |
| `ComponentSpecimen` | web/ui/src/components/ComponentSpecimen.tsx |
| `BreadcrumbContext` | web/ui/src/components/BreadcrumbContext.tsx |
| `Breadcrumb` | web/ui/src/components/Breadcrumb.tsx |
| `ContextBar` | web/ui/src/components/ContextBar.tsx |
| `ContextChip` | web/ui/src/components/ContextChip.tsx |
| `StateMatrix` | web/ui/src/components/StateMatrix.tsx |
| `TokenStrip` | web/ui/src/components/TokenStrip.tsx |
| `AcceptanceStrip` | web/ui/src/components/AcceptanceStrip.tsx |

## 验收证据

- 目标图：`evidence/ui-image-breakdowns/components/component-risk-score/target.png`
- 实现截图：`evidence/ui-image-breakdowns/components/component-risk-score/implementation.png`
- diff 图：`evidence/ui-image-breakdowns/components/component-risk-score/diff.png`
- regions overlay：`evidence/ui-image-breakdowns/components/component-risk-score/regions-overlay.png`
- measurement：`evidence/ui-image-breakdowns/components/component-risk-score/measurement.json`
- OCR/manual ledger：`evidence/ui-image-breakdowns/components/component-risk-score/text-ocr.txt`
- metrics：`evidence/ui-image-breakdowns/components/component-risk-score/metrics.json`
- verification：`evidence/ui-image-breakdowns/components/component-risk-score/verification.json`
- 视口：1920 x 1080，DPR 1
- 浏览器：Windows Chrome CDP，经 `http://127.0.0.1:9224/json/version` 与 `/json/list` 预检。
- 复现步骤：锁定 target.png，打开 implementation.html，使用 Windows Chrome CDP 截图，生成 diff.png 和 metrics.json，读取 verification.json。

## 差异清单

| 类型 | 位置 | 当前 | 期望 | 状态 |
|---|---|---|---|---|
| visual-diff | full image | Windows Chrome screenshot will be compared with the locked target PNG. | pixel mismatch ratio <= 0.015 | documented |
| semantic-scope | production React implementation | reference-raster evidence proves visual parity only | semantic React components follow this record when implemented separately | documented |

## 结论

- 逐图拆解层已记录区域、文本、组件、图标、token、交互、实现映射和证据路径。
- Windows Chrome 截图和视觉 diff 由自动闭环脚本在本记录生成后写入。
- 通过口径为：target.png、implementation.png、regions-overlay.png、diff.png、metrics.json、verification.json 齐备，且 metrics 状态为 pass。
- 主线程判定只覆盖该目标 PNG 的像素复刻，不扩大为生产语义实现验收。
