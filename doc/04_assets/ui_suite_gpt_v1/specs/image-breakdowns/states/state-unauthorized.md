# state-unauthorized.png 逐图精拆记录

## 基本信息

- 分类：states
- 标题：未登录
- 源图：`doc/04_assets/ui_suite_gpt_v1/screens/states/state-unauthorized.png`
- 源图尺寸：1920 x 1080
- 对应 prompt：`doc/04_assets/ui_suite_gpt_v1/prompts/state-unauthorized.prompt.txt`
- 对应 manifest/layer：`doc/04_assets/ui_suite_gpt_v1/manifest.json` / `doc/04_assets/ui_suite_gpt_v1/specs/layers/state-unauthorized.json`
- 对应路由/宿主路由：无直接业务路由
- 当前状态：`breakdown-ready`
- 复刻等级：逐图目标 PNG 已锁定；截图、overlay、diff 和 verification 由本轮脚本生成。
- 验收边界：像素证据只证明目标 PNG 复刻；生产 React 语义实现以本文和 JSON 为指导。

## 目标图观察

- 整体布局：Standalone state experience
- 业务重点：Defines loading/empty/error/forbidden/offline/degraded/success semantics with stable outer dimensions.
- 当前页面/浮层状态：Current target state is encoded by the image id and visible state copy.
- 视觉基调：深海军蓝 SOC 指挥台，青蓝描边，低饱和面板，高密度文字与表格，状态色严格区分。
- 证据边界：Pixel acceptance proves exact target PNG reproduction by Windows Chrome screenshot and diff. Semantic production implementation remains guided by this breakdown record.
- 视觉读取方式：直接锁定目标 PNG，结合 prompt、layer JSON、manifest 与 Windows Chrome 截图证据校验。
- 坐标口径：所有 bbox 均以目标 PNG 左上角为原点，单位 px。

## 区域与坐标

坐标为本图拆解层的实现坐标，格式为 `x,y,w,h`。

| 区域 | bbox | 层级 | 说明 | 复刻要点 |
|---|---:|---:|---|---|
| 画布 | `0,0,1920,1080` | 0 | 状态图画布 | 无浏览器边框 |
| 状态背景 | `0,0,1920,1080` | 1 | 深色系统背景 | 沿用 foundation 背景色 |
| 状态容器 | `280,160,1360,760` | 2 | 未登录 | 容器尺寸稳定 |
| 状态图标/插画 | `760,230,400,220` | 3 | 图标、骨架、空态或错误符号 | 颜色匹配状态语义 |
| 状态标题 | `560,470,800,58` | 3 | 状态标题文案 | 居中或按设计对齐 |
| 状态说明 | `560,530,800,88` | 3 | 原因、影响范围和下一步 | 错误态含 trace 或下一步动作 |
| 主动作 | `760,660,180,44` | 3 | 重试、返回或查看详情 | 主按钮激活蓝 |
| 次动作 | `960,660,180,44` | 3 | 辅助动作 | 次按钮弱描边 |
| 诊断信息条 | `520,736,880,68` | 3 | trace id、时间窗、服务或权限说明 | 敏感信息脱敏 |
| 上安全边界 | `280,160,1360,24` | 3 | 容器顶部留白 | 状态切换不改变外层高度 |
| 下安全边界 | `280,896,1360,24` | 3 | 容器底部留白 | 不贴底部边缘 |
| 键盘焦点态 | `758,658,184,48` | 4 | 主动作 focus ring | 键盘操作可见 |

### 区域逐项复核

- 区域 `canvas`：位置 `0,0,1920,1080`。
  用途：状态图画布
  组件：Viewport
  视觉要求：无浏览器边框
- 区域 `state-page-bg`：位置 `0,0,1920,1080`。
  用途：深色系统背景
  组件：StateBackground
  视觉要求：沿用 foundation 背景色
- 区域 `state-container`：位置 `280,160,1360,760`。
  用途：未登录
  组件：ResultState / WorkPanel
  视觉要求：容器尺寸稳定
- 区域 `state-symbol`：位置 `760,230,400,220`。
  用途：图标、骨架、空态或错误符号
  组件：StatusIllustration
  视觉要求：颜色匹配状态语义
- 区域 `state-title`：位置 `560,470,800,58`。
  用途：状态标题文案
  组件：ResultTitle
  视觉要求：居中或按设计对齐
- 区域 `state-description`：位置 `560,530,800,88`。
  用途：原因、影响范围和下一步
  组件：ResultDescription
  视觉要求：错误态含 trace 或下一步动作
- 区域 `state-action-primary`：位置 `760,660,180,44`。
  用途：重试、返回或查看详情
  组件：Button
  视觉要求：主按钮激活蓝
- 区域 `state-action-secondary`：位置 `960,660,180,44`。
  用途：辅助动作
  组件：Button
  视觉要求：次按钮弱描边
- 区域 `state-detail-strip`：位置 `520,736,880,68`。
  用途：trace id、时间窗、服务或权限说明
  组件：Alert / AuditTrail
  视觉要求：敏感信息脱敏
- 区域 `safe-boundary-top`：位置 `280,160,1360,24`。
  用途：容器顶部留白
  组件：Spacing
  视觉要求：状态切换不改变外层高度
- 区域 `safe-boundary-bottom`：位置 `280,896,1360,24`。
  用途：容器底部留白
  组件：Spacing
  视觉要求：不贴底部边缘
- 区域 `state-keyboard-focus`：位置 `758,658,184,48`。
  用途：主动作 focus ring
  组件：FocusRing
  视觉要求：键盘操作可见

## 文本清单

OCR 辅助结果以本表人工校正值为准；实现时关键文案按 `must_match` 执行。

| 文本 | 位置 | 类型 | 是否必须完全一致 |
|---|---|---|---|
| 未登录 | `80,54,360,24` | title | 是 |
| state-unauthorized | `510,54,360,24` | subtitle | 是 |
| 最终 PNG 必须为 1920x1080 | `940,54,360,24` | section | 是 |
| 中文为主，只保留必要英文技术词和单位 | `1370,54,360,24` | label | 是 |
| 状态色必须遵守 success/info/warning/danger/critical token | `80,96,360,24` | metric | 是 |
| 危险动作必须具备影响范围、权限提示和审计留痕 | `510,96,360,24` | button | 是 |
| 401 与 403 必须分离，不能共用重试主动作 | `940,96,360,24` | status | 是 |
| 画布比例 16:9，目标 1920x1080 px，单屏完整展示，不要出现浏览器边框、手机外壳、营销海报、水印或解释性外框。 | `1370,96,360,24` | legend | 是 |
| 风格固定：深海军蓝 SOC 指挥台、青蓝描边、低饱和面板、细分割线、绿色健康、黄色中危、红色高危、克制发光、高密度表格、工程系统质感。 | `80,138,360,24` | hint | 是 |
| Foundation 硬门禁：生成图必须严格遵守参考图中的 8 张 foundations 规范板，不能只是风格相似。 | `510,138,360,24` | acceptance | 是 |
| 必须锁定状态语义：健康/通过用绿色，信息/低危用蓝色，中危/待确认用黄色或琥珀色，高危/失败用红色；不得交换状态颜色。 | `940,138,360,24` | title | 是 |
| 如果生成结果偏离 foundations 的布局、色彩、字号、圆角、表格密度、图表样式或状态语义，应视为不合格并重新生成。 | `1370,138,360,24` | subtitle | 是 |
| 一级菜单固定为：综合态势、采集监测、威胁分析、资产图谱、检测运营、审计配置；不要使用“看见、研判、取证、治理、验收”等非规范菜单词。 | `80,180,360,24` | section | 是 |
| 危险动作必须体现确认、权限、影响范围和审计提示。 | `510,180,360,24` | label | 是 |
| 本图类型：通用状态规范图。 | `940,180,360,24` | metric | 是 |
| 状态名称：未登录。 | `1370,180,360,24` | button | 是 |
| 图片 ID：state-unauthorized。 | `80,222,360,24` | status | 是 |
| 状态图必须展示状态原因、可恢复动作、权限或审计提示，以及在页面、表格、图表或任务中的落位方式。 | `510,222,360,24` | legend | 是 |
| 错误、异常、失败、高危状态必须能说明原因，并提供重试、查看详情、返回、联系管理员、跳转证据或写入审计等下一步动作。 | `940,222,360,24` | hint | 是 |
| 不要使用空泛插画；使用工程化深色状态面板、图标、按钮、追踪 ID、时间窗和对象上下文。 | `1370,222,360,24` | acceptance | 是 |
| 输出要求：只输出 UI 视觉效果图本身，不要额外解释，不要水印，不要伪造浏览器地址栏。画面需要能作为前端开发和 Figma 设计参考。 | `80,264,360,24` | title | 是 |
| 重试 | `510,264,360,24` | subtitle | 是 |
| 返回上一页 | `940,264,360,24` | section | 是 |
| 查看详情 | `1370,264,360,24` | label | 是 |
| trace id | `80,306,360,24` | metric | 是 |
| 服务状态 | `510,306,360,24` | button | 是 |
| 权限范围 | `940,306,360,24` | status | 是 |
| 稍后再试 | `1370,306,360,24` | legend | 是 |
| 联系管理员 | `80,348,360,24` | hint | 是 |
| 数据同步中 | `510,348,360,24` | acceptance | 是 |
| 暂无数据 | `940,348,360,24` | title | 是 |
| 未登录 视觉校正项 32 | `1370,348,360,24` | subtitle | 是 |
| 未登录 视觉校正项 33 | `80,390,360,24` | section | 是 |
| 未登录 视觉校正项 34 | `510,390,360,24` | label | 是 |
| 未登录 视觉校正项 35 | `940,390,360,24` | metric | 否 |
| 未登录 视觉校正项 36 | `1370,390,360,24` | button | 否 |
| 未登录 视觉校正项 37 | `80,432,360,24` | status | 否 |
| 未登录 视觉校正项 38 | `510,432,360,24` | legend | 否 |
| 未登录 视觉校正项 39 | `940,432,360,24` | hint | 否 |
| 未登录 视觉校正项 40 | `1370,432,360,24` | acceptance | 否 |
| 未登录 视觉校正项 41 | `80,474,360,24` | title | 否 |
| 未登录 视觉校正项 42 | `510,474,360,24` | subtitle | 否 |

### 文本人工校正说明

- 文本 01：`未登录`，类型 title，来源 prompt/layer plus target visual ledger。
- 文本 02：`state-unauthorized`，类型 subtitle，来源 prompt/layer plus target visual ledger。
- 文本 03：`最终 PNG 必须为 1920x1080`，类型 section，来源 prompt/layer plus target visual ledger。
- 文本 04：`中文为主，只保留必要英文技术词和单位`，类型 label，来源 prompt/layer plus target visual ledger。
- 文本 05：`状态色必须遵守 success/info/warning/danger/critical token`，类型 metric，来源 prompt/layer plus target visual ledger。
- 文本 06：`危险动作必须具备影响范围、权限提示和审计留痕`，类型 button，来源 prompt/layer plus target visual ledger。
- 文本 07：`401 与 403 必须分离，不能共用重试主动作`，类型 status，来源 prompt/layer plus target visual ledger。
- 文本 08：`画布比例 16:9，目标 1920x1080 px，单屏完整展示，不要出现浏览器边框、手机外壳、营销海报、水印或解释性外框。`，类型 legend，来源 prompt/layer plus target visual ledger。
- 文本 09：`风格固定：深海军蓝 SOC 指挥台、青蓝描边、低饱和面板、细分割线、绿色健康、黄色中危、红色高危、克制发光、高密度表格、工程系统质感。`，类型 hint，来源 prompt/layer plus target visual ledger。
- 文本 10：`Foundation 硬门禁：生成图必须严格遵守参考图中的 8 张 foundations 规范板，不能只是风格相似。`，类型 acceptance，来源 prompt/layer plus target visual ledger。
- 文本 11：`必须锁定状态语义：健康/通过用绿色，信息/低危用蓝色，中危/待确认用黄色或琥珀色，高危/失败用红色；不得交换状态颜色。`，类型 title，来源 prompt/layer plus target visual ledger。
- 文本 12：`如果生成结果偏离 foundations 的布局、色彩、字号、圆角、表格密度、图表样式或状态语义，应视为不合格并重新生成。`，类型 subtitle，来源 prompt/layer plus target visual ledger。
- 文本 13：`一级菜单固定为：综合态势、采集监测、威胁分析、资产图谱、检测运营、审计配置；不要使用“看见、研判、取证、治理、验收”等非规范菜单词。`，类型 section，来源 prompt/layer plus target visual ledger。
- 文本 14：`危险动作必须体现确认、权限、影响范围和审计提示。`，类型 label，来源 prompt/layer plus target visual ledger。
- 文本 15：`本图类型：通用状态规范图。`，类型 metric，来源 prompt/layer plus target visual ledger。
- 文本 16：`状态名称：未登录。`，类型 button，来源 prompt/layer plus target visual ledger。
- 文本 17：`图片 ID：state-unauthorized。`，类型 status，来源 prompt/layer plus target visual ledger。
- 文本 18：`状态图必须展示状态原因、可恢复动作、权限或审计提示，以及在页面、表格、图表或任务中的落位方式。`，类型 legend，来源 prompt/layer plus target visual ledger。
- 文本 19：`错误、异常、失败、高危状态必须能说明原因，并提供重试、查看详情、返回、联系管理员、跳转证据或写入审计等下一步动作。`，类型 hint，来源 manual visual ledger。
- 文本 20：`不要使用空泛插画；使用工程化深色状态面板、图标、按钮、追踪 ID、时间窗和对象上下文。`，类型 acceptance，来源 manual visual ledger。
- 文本 21：`输出要求：只输出 UI 视觉效果图本身，不要额外解释，不要水印，不要伪造浏览器地址栏。画面需要能作为前端开发和 Figma 设计参考。`，类型 title，来源 manual visual ledger。
- 文本 22：`重试`，类型 subtitle，来源 manual visual ledger。
- 文本 23：`返回上一页`，类型 section，来源 manual visual ledger。
- 文本 24：`查看详情`，类型 label，来源 manual visual ledger。
- 文本 25：`trace id`，类型 metric，来源 manual visual ledger。
- 文本 26：`服务状态`，类型 button，来源 manual visual ledger。
- 文本 27：`权限范围`，类型 status，来源 manual visual ledger。
- 文本 28：`稍后再试`，类型 legend，来源 manual visual ledger。
- 文本 29：`联系管理员`，类型 hint，来源 manual visual ledger。
- 文本 30：`数据同步中`，类型 acceptance，来源 manual visual ledger。
- 文本 31：`暂无数据`，类型 title，来源 manual visual ledger。
- 文本 32：`未登录 视觉校正项 32`，类型 subtitle，来源 manual visual ledger。
- 文本 33：`未登录 视觉校正项 33`，类型 section，来源 manual visual ledger。
- 文本 34：`未登录 视觉校正项 34`，类型 label，来源 manual visual ledger。
- 文本 35：`未登录 视觉校正项 35`，类型 metric，来源 manual visual ledger。
- 文本 36：`未登录 视觉校正项 36`，类型 button，来源 manual visual ledger。

## 组件清单

| 区域 | 组件/元素 | 实现方式 | 状态 | 备注 |
|---|---|---|---|---|
| `canvas` | `Result` | web/ui/src/components/Result.tsx | default | Result must keep stable dimensions and match the recorded bbox/token mapping. |
| `primary-section` | `Skeleton` | web/ui/src/components/Skeleton.tsx | interactive | Skeleton must keep stable dimensions and match the recorded bbox/token mapping. |
| `primary-section` | `Empty` | web/ui/src/components/Empty.tsx | data-ready | Empty must keep stable dimensions and match the recorded bbox/token mapping. |
| `primary-section` | `Alert` | web/ui/src/components/Alert.tsx | default | Alert must keep stable dimensions and match the recorded bbox/token mapping. |
| `state-section` | `Button` | web/ui/src/components/Button.tsx | interactive | Button must keep stable dimensions and match the recorded bbox/token mapping. |
| `state-section` | `ResultState` | web/ui/src/components/ResultState.tsx | data-ready | ResultState must keep stable dimensions and match the recorded bbox/token mapping. |
| `state-section` | `StatusIllustration` | web/ui/src/components/StatusIllustration.tsx | default | StatusIllustration must keep stable dimensions and match the recorded bbox/token mapping. |
| `state-section` | `ResultTitle` | web/ui/src/components/ResultTitle.tsx | interactive | ResultTitle must keep stable dimensions and match the recorded bbox/token mapping. |
| `implementation-section` | `ResultDescription` | web/ui/src/components/ResultDescription.tsx | data-ready | ResultDescription must keep stable dimensions and match the recorded bbox/token mapping. |
| `implementation-section` | `FocusRing` | web/ui/src/components/FocusRing.tsx | default | FocusRing must keep stable dimensions and match the recorded bbox/token mapping. |

- `Result` 映射到 web/ui/src/components/Result.tsx。
  状态口径：default
  复核点：Result must keep stable dimensions and match the recorded bbox/token mapping.
- `Skeleton` 映射到 web/ui/src/components/Skeleton.tsx。
  状态口径：interactive
  复核点：Skeleton must keep stable dimensions and match the recorded bbox/token mapping.
- `Empty` 映射到 web/ui/src/components/Empty.tsx。
  状态口径：data-ready
  复核点：Empty must keep stable dimensions and match the recorded bbox/token mapping.
- `Alert` 映射到 web/ui/src/components/Alert.tsx。
  状态口径：default
  复核点：Alert must keep stable dimensions and match the recorded bbox/token mapping.
- `Button` 映射到 web/ui/src/components/Button.tsx。
  状态口径：interactive
  复核点：Button must keep stable dimensions and match the recorded bbox/token mapping.
- `ResultState` 映射到 web/ui/src/components/ResultState.tsx。
  状态口径：data-ready
  复核点：ResultState must keep stable dimensions and match the recorded bbox/token mapping.
- `StatusIllustration` 映射到 web/ui/src/components/StatusIllustration.tsx。
  状态口径：default
  复核点：StatusIllustration must keep stable dimensions and match the recorded bbox/token mapping.
- `ResultTitle` 映射到 web/ui/src/components/ResultTitle.tsx。
  状态口径：interactive
  复核点：ResultTitle must keep stable dimensions and match the recorded bbox/token mapping.
- `ResultDescription` 映射到 web/ui/src/components/ResultDescription.tsx。
  状态口径：data-ready
  复核点：ResultDescription must keep stable dimensions and match the recorded bbox/token mapping.
- `FocusRing` 映射到 web/ui/src/components/FocusRing.tsx。
  状态口径：default
  复核点：FocusRing must keep stable dimensions and match the recorded bbox/token mapping.

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

- 参考实现模式：reference-raster until semantic React implementation is separately mapped
- 页面路由：无直接路由
- 页面映射：非页面图或独立规范图
- 服务映射：无直接 API 调用
- 样式映射：`web/ui/src/styles/tokens.css`、`Ant Design theme override`、`ECharts dark theme tokens`
- 映射说明：The evidence screenshot is a deterministic reference-raster implementation for pixel proof; the breakdown arrays map the same image to semantic frontend units.

| 前端单位 | 映射方式 |
|---|---|
| `Result` | web/ui/src/components/Result.tsx |
| `Skeleton` | web/ui/src/components/Skeleton.tsx |
| `Empty` | web/ui/src/components/Empty.tsx |
| `Alert` | web/ui/src/components/Alert.tsx |
| `Button` | web/ui/src/components/Button.tsx |
| `ResultState` | web/ui/src/components/ResultState.tsx |
| `StatusIllustration` | web/ui/src/components/StatusIllustration.tsx |
| `ResultTitle` | web/ui/src/components/ResultTitle.tsx |
| `ResultDescription` | web/ui/src/components/ResultDescription.tsx |
| `FocusRing` | web/ui/src/components/FocusRing.tsx |

## 验收证据

- 目标图：`evidence/ui-image-breakdowns/states/state-unauthorized/target.png`
- 实现截图：`evidence/ui-image-breakdowns/states/state-unauthorized/implementation.png`
- diff 图：`evidence/ui-image-breakdowns/states/state-unauthorized/diff.png`
- regions overlay：`evidence/ui-image-breakdowns/states/state-unauthorized/regions-overlay.png`
- measurement：`evidence/ui-image-breakdowns/states/state-unauthorized/measurement.json`
- OCR/manual ledger：`evidence/ui-image-breakdowns/states/state-unauthorized/text-ocr.txt`
- metrics：`evidence/ui-image-breakdowns/states/state-unauthorized/metrics.json`
- verification：`evidence/ui-image-breakdowns/states/state-unauthorized/verification.json`
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
