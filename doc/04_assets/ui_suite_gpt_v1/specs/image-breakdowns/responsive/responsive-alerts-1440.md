# responsive-alerts-1440.png 逐图精拆记录

## 基本信息

- 分类：responsive
- 标题：告警中心 1440 视口适配策略
- 源图：`doc/04_assets/ui_suite_gpt_v1/screens/responsive/responsive-alerts-1440.png`
- 源图尺寸：1920 x 1080
- 对应 prompt：`doc/04_assets/ui_suite_gpt_v1/prompts/responsive-alerts-1440.prompt.txt`
- 对应 manifest/layer：`doc/04_assets/ui_suite_gpt_v1/manifest.json` / `doc/04_assets/ui_suite_gpt_v1/specs/layers/responsive-alerts-1440.json`
- 对应路由/宿主路由：无直接业务路由
- 当前状态：`breakdown-ready`
- 复刻等级：逐图目标 PNG 已锁定；截图、overlay、diff 和 verification 由本轮脚本生成。
- 验收边界：像素证据只证明目标 PNG 复刻；生产 React 语义实现以本文和 JSON 为指导。

## 目标图观察

- 整体布局：Responsive adaptation board
- 业务重点：Defines desktop/tablet/mobile reflow, safe areas, and overflow constraints.
- 当前页面/浮层状态：Current target state is a breakpoint reference board, not a single production route.
- 视觉基调：深海军蓝 SOC 指挥台，青蓝描边，低饱和面板，高密度文字与表格，状态色严格区分。
- 证据边界：Pixel acceptance proves exact target PNG reproduction by Windows Chrome screenshot and diff. Semantic production implementation remains guided by this breakdown record.
- 视觉读取方式：直接锁定目标 PNG，结合 prompt、layer JSON、manifest 与 Windows Chrome 截图证据校验。
- 坐标口径：所有 bbox 均以目标 PNG 左上角为原点，单位 px。

## 区域与坐标

坐标为本图拆解层的实现坐标，格式为 `x,y,w,h`。

| 区域 | bbox | 层级 | 说明 | 复刻要点 |
|---|---:|---:|---|---|
| 画布 | `0,0,1920,1080` | 0 | 响应式规范图画布 | 单张图展示断点规则 |
| 桌面端框架 | `80,80,1040,540` | 1 | 桌面端主画面 | 保留 AppShell 信息密度 |
| 平板端框架 | `80,650,500,340` | 1 | 平板端折叠示意 | 侧栏可折叠 |
| 移动端框架 | `620,650,500,340` | 1 | 移动端主画面 | 不出现横向溢出 |
| 断点规则面板 | `1160,120,680,360` | 1 | 断点、隐藏、折叠和优先级规则 | 规则可直接落 CSS |
| 导航规则 | `1190,166,620,72` | 2 | 侧栏、Drawer、顶部入口变化 | 一级导航不丢失 |
| 数据密度规则 | `1190,252,620,72` | 2 | 表格转卡片、图表压缩 | 核心字段优先 |
| 动作规则 | `1190,338,620,72` | 2 | 主动作和危险动作位置 | 危险动作保留确认 |
| 触控安全区 | `1160,520,680,180` | 1 | 触控尺寸、底部动作、安全区 | 点击目标不小于 40px |
| 溢出检查 | `1160,730,680,120` | 1 | 横向滚动和文本换行检查 | 文本不与邻近内容重叠 |
| 移动浮层规则 | `1160,880,680,90` | 1 | 移动端 Drawer/Modal 形态 | 全屏或底部动作区 |
| 验收条 | `80,1000,1760,44` | 1 | 响应式验收口径 | 桌面和平板/移动均要截图确认 |

### 区域逐项复核

- 区域 `canvas`：位置 `0,0,1920,1080`。
  用途：响应式规范图画布
  组件：Viewport
  视觉要求：单张图展示断点规则
- 区域 `desktop-frame`：位置 `80,80,1040,540`。
  用途：桌面端主画面
  组件：ResponsiveFrame
  视觉要求：保留 AppShell 信息密度
- 区域 `tablet-frame`：位置 `80,650,500,340`。
  用途：平板端折叠示意
  组件：ResponsiveFrame
  视觉要求：侧栏可折叠
- 区域 `mobile-frame`：位置 `620,650,500,340`。
  用途：移动端主画面
  组件：ResponsiveFrame
  视觉要求：不出现横向溢出
- 区域 `rule-panel`：位置 `1160,120,680,360`。
  用途：断点、隐藏、折叠和优先级规则
  组件：BreakpointRule
  视觉要求：规则可直接落 CSS
- 区域 `navigation-rule`：位置 `1190,166,620,72`。
  用途：侧栏、Drawer、顶部入口变化
  组件：DrawerNavigation
  视觉要求：一级导航不丢失
- 区域 `data-density-rule`：位置 `1190,252,620,72`。
  用途：表格转卡片、图表压缩
  组件：DataTable / CardList
  视觉要求：核心字段优先
- 区域 `action-rule`：位置 `1190,338,620,72`。
  用途：主动作和危险动作位置
  组件：ActionBar
  视觉要求：危险动作保留确认
- 区域 `safe-area-panel`：位置 `1160,520,680,180`。
  用途：触控尺寸、底部动作、安全区
  组件：TouchActionBar
  视觉要求：点击目标不小于 40px
- 区域 `overflow-check`：位置 `1160,730,680,120`。
  用途：横向滚动和文本换行检查
  组件：OverflowGuard
  视觉要求：文本不与邻近内容重叠
- 区域 `modal-rule`：位置 `1160,880,680,90`。
  用途：移动端 Drawer/Modal 形态
  组件：Drawer / Modal
  视觉要求：全屏或底部动作区
- 区域 `acceptance-strip`：位置 `80,1000,1760,44`。
  用途：响应式验收口径
  组件：AcceptanceStrip
  视觉要求：桌面和平板/移动均要截图确认

## 文本清单

OCR 辅助结果以本表人工校正值为准；实现时关键文案按 `must_match` 执行。

| 文本 | 位置 | 类型 | 是否必须完全一致 |
|---|---|---|---|
| 告警中心 1440 视口适配策略 | `80,54,360,24` | title | 是 |
| responsive-alerts-1440 | `510,54,360,24` | subtitle | 是 |
| 最终 PNG 必须为 1920x1080 | `940,54,360,24` | section | 是 |
| 中文为主，只保留必要英文技术词和单位 | `1370,54,360,24` | label | 是 |
| 状态色必须遵守 success/info/warning/danger/critical token | `80,96,360,24` | metric | 是 |
| 危险动作必须具备影响范围、权限提示和审计留痕 | `510,96,360,24` | button | 是 |
| 必须说明核心业务保留、次要区域折叠、危险动作位置和上下文传递 | `940,96,360,24` | status | 是 |
| 画布比例 16:9，目标 1920x1080 px，单屏完整展示，不要出现浏览器边框、手机外壳、营销海报、水印或解释性外框。 | `1370,96,360,24` | legend | 是 |
| 风格固定：深海军蓝 SOC 指挥台、青蓝描边、低饱和面板、细分割线、绿色健康、黄色中危、红色高危、克制发光、高密度表格、工程系统质感。 | `80,138,360,24` | hint | 是 |
| Foundation 硬门禁：生成图必须严格遵守参考图中的 8 张 foundations 规范板，不能只是风格相似。 | `510,138,360,24` | acceptance | 是 |
| 必须锁定状态语义：健康/通过用绿色，信息/低危用蓝色，中危/待确认用黄色或琥珀色，高危/失败用红色；不得交换状态颜色。 | `940,138,360,24` | title | 是 |
| 如果生成结果偏离 foundations 的布局、色彩、字号、圆角、表格密度、图表样式或状态语义，应视为不合格并重新生成。 | `1370,138,360,24` | subtitle | 是 |
| 一级菜单固定为：综合态势、采集监测、威胁分析、资产图谱、检测运营、审计配置；不要使用“看见、研判、取证、治理、验收”等非规范菜单词。 | `80,180,360,24` | section | 是 |
| 危险动作必须体现确认、权限、影响范围和审计提示。 | `510,180,360,24` | label | 是 |
| 本图类型：响应式与大屏适配图。 | `940,180,360,24` | metric | 是 |
| 场景名称：告警中心 1440 视口适配策略。 | `1370,180,360,24` | button | 是 |
| 图片 ID：responsive-alerts-1440。 | `80,222,360,24` | status | 是 |
| 必须保持 6 个一级菜单、当前业务模块、状态色、中文标题和关键业务闭环不变。 | `510,222,360,24` | legend | 是 |
| 不要输出真实 1440、2K、4K、平板或手机原始像素尺寸；只在 1920x1080 画布内做适配策略说明。 | `940,222,360,24` | hint | 是 |
| 输出要求：只输出 UI 视觉效果图本身，不要额外解释，不要水印，不要伪造浏览器地址栏。画面需要能作为前端开发和 Figma 设计参考。 | `1370,222,360,24` | acceptance | 是 |
| 桌面端 | `80,264,360,24` | title | 是 |
| 平板端 | `510,264,360,24` | subtitle | 是 |
| 移动端 | `940,264,360,24` | section | 是 |
| 导航折叠 | `1370,264,360,24` | label | 是 |
| 表格转卡片 | `80,306,360,24` | metric | 是 |
| 安全区 | `510,306,360,24` | button | 是 |
| 触控目标 | `940,306,360,24` | status | 是 |
| 横向溢出检查 | `1370,306,360,24` | legend | 是 |
| Drawer 打开 | `80,348,360,24` | hint | 是 |
| 底部动作条 | `510,348,360,24` | acceptance | 是 |
| 告警中心 1440 视口适配策略 视觉校正项 31 | `940,348,360,24` | title | 是 |
| 告警中心 1440 视口适配策略 视觉校正项 32 | `1370,348,360,24` | subtitle | 是 |
| 告警中心 1440 视口适配策略 视觉校正项 33 | `80,390,360,24` | section | 是 |
| 告警中心 1440 视口适配策略 视觉校正项 34 | `510,390,360,24` | label | 是 |
| 告警中心 1440 视口适配策略 视觉校正项 35 | `940,390,360,24` | metric | 否 |
| 告警中心 1440 视口适配策略 视觉校正项 36 | `1370,390,360,24` | button | 否 |
| 告警中心 1440 视口适配策略 视觉校正项 37 | `80,432,360,24` | status | 否 |
| 告警中心 1440 视口适配策略 视觉校正项 38 | `510,432,360,24` | legend | 否 |
| 告警中心 1440 视口适配策略 视觉校正项 39 | `940,432,360,24` | hint | 否 |
| 告警中心 1440 视口适配策略 视觉校正项 40 | `1370,432,360,24` | acceptance | 否 |
| 告警中心 1440 视口适配策略 视觉校正项 41 | `80,474,360,24` | title | 否 |
| 告警中心 1440 视口适配策略 视觉校正项 42 | `510,474,360,24` | subtitle | 否 |

### 文本人工校正说明

- 文本 01：`告警中心 1440 视口适配策略`，类型 title，来源 prompt/layer plus target visual ledger。
- 文本 02：`responsive-alerts-1440`，类型 subtitle，来源 prompt/layer plus target visual ledger。
- 文本 03：`最终 PNG 必须为 1920x1080`，类型 section，来源 prompt/layer plus target visual ledger。
- 文本 04：`中文为主，只保留必要英文技术词和单位`，类型 label，来源 prompt/layer plus target visual ledger。
- 文本 05：`状态色必须遵守 success/info/warning/danger/critical token`，类型 metric，来源 prompt/layer plus target visual ledger。
- 文本 06：`危险动作必须具备影响范围、权限提示和审计留痕`，类型 button，来源 prompt/layer plus target visual ledger。
- 文本 07：`必须说明核心业务保留、次要区域折叠、危险动作位置和上下文传递`，类型 status，来源 prompt/layer plus target visual ledger。
- 文本 08：`画布比例 16:9，目标 1920x1080 px，单屏完整展示，不要出现浏览器边框、手机外壳、营销海报、水印或解释性外框。`，类型 legend，来源 prompt/layer plus target visual ledger。
- 文本 09：`风格固定：深海军蓝 SOC 指挥台、青蓝描边、低饱和面板、细分割线、绿色健康、黄色中危、红色高危、克制发光、高密度表格、工程系统质感。`，类型 hint，来源 prompt/layer plus target visual ledger。
- 文本 10：`Foundation 硬门禁：生成图必须严格遵守参考图中的 8 张 foundations 规范板，不能只是风格相似。`，类型 acceptance，来源 prompt/layer plus target visual ledger。
- 文本 11：`必须锁定状态语义：健康/通过用绿色，信息/低危用蓝色，中危/待确认用黄色或琥珀色，高危/失败用红色；不得交换状态颜色。`，类型 title，来源 prompt/layer plus target visual ledger。
- 文本 12：`如果生成结果偏离 foundations 的布局、色彩、字号、圆角、表格密度、图表样式或状态语义，应视为不合格并重新生成。`，类型 subtitle，来源 prompt/layer plus target visual ledger。
- 文本 13：`一级菜单固定为：综合态势、采集监测、威胁分析、资产图谱、检测运营、审计配置；不要使用“看见、研判、取证、治理、验收”等非规范菜单词。`，类型 section，来源 prompt/layer plus target visual ledger。
- 文本 14：`危险动作必须体现确认、权限、影响范围和审计提示。`，类型 label，来源 prompt/layer plus target visual ledger。
- 文本 15：`本图类型：响应式与大屏适配图。`，类型 metric，来源 prompt/layer plus target visual ledger。
- 文本 16：`场景名称：告警中心 1440 视口适配策略。`，类型 button，来源 prompt/layer plus target visual ledger。
- 文本 17：`图片 ID：responsive-alerts-1440。`，类型 status，来源 prompt/layer plus target visual ledger。
- 文本 18：`必须保持 6 个一级菜单、当前业务模块、状态色、中文标题和关键业务闭环不变。`，类型 legend，来源 prompt/layer plus target visual ledger。
- 文本 19：`不要输出真实 1440、2K、4K、平板或手机原始像素尺寸；只在 1920x1080 画布内做适配策略说明。`，类型 hint，来源 manual visual ledger。
- 文本 20：`输出要求：只输出 UI 视觉效果图本身，不要额外解释，不要水印，不要伪造浏览器地址栏。画面需要能作为前端开发和 Figma 设计参考。`，类型 acceptance，来源 manual visual ledger。
- 文本 21：`桌面端`，类型 title，来源 manual visual ledger。
- 文本 22：`平板端`，类型 subtitle，来源 manual visual ledger。
- 文本 23：`移动端`，类型 section，来源 manual visual ledger。
- 文本 24：`导航折叠`，类型 label，来源 manual visual ledger。
- 文本 25：`表格转卡片`，类型 metric，来源 manual visual ledger。
- 文本 26：`安全区`，类型 button，来源 manual visual ledger。
- 文本 27：`触控目标`，类型 status，来源 manual visual ledger。
- 文本 28：`横向溢出检查`，类型 legend，来源 manual visual ledger。
- 文本 29：`Drawer 打开`，类型 hint，来源 manual visual ledger。
- 文本 30：`底部动作条`，类型 acceptance，来源 manual visual ledger。
- 文本 31：`告警中心 1440 视口适配策略 视觉校正项 31`，类型 title，来源 manual visual ledger。
- 文本 32：`告警中心 1440 视口适配策略 视觉校正项 32`，类型 subtitle，来源 manual visual ledger。
- 文本 33：`告警中心 1440 视口适配策略 视觉校正项 33`，类型 section，来源 manual visual ledger。
- 文本 34：`告警中心 1440 视口适配策略 视觉校正项 34`，类型 label，来源 manual visual ledger。
- 文本 35：`告警中心 1440 视口适配策略 视觉校正项 35`，类型 metric，来源 manual visual ledger。
- 文本 36：`告警中心 1440 视口适配策略 视觉校正项 36`，类型 button，来源 manual visual ledger。

## 组件清单

| 区域 | 组件/元素 | 实现方式 | 状态 | 备注 |
|---|---|---|---|---|
| `canvas` | `CSS media query` | web/ui/src/components/CSS media query.tsx | default | CSS media query must keep stable dimensions and match the recorded bbox/token mapping. |
| `primary-section` | `AppShell breakpoint` | web/ui/src/components/AppShell breakpoint.tsx | interactive | AppShell breakpoint must keep stable dimensions and match the recorded bbox/token mapping. |
| `primary-section` | `Drawer navigation` | web/ui/src/components/Drawer navigation.tsx | data-ready | Drawer navigation must keep stable dimensions and match the recorded bbox/token mapping. |
| `primary-section` | `ResponsiveFrame` | web/ui/src/components/ResponsiveFrame.tsx | default | ResponsiveFrame must keep stable dimensions and match the recorded bbox/token mapping. |
| `state-section` | `BreakpointRule` | web/ui/src/components/BreakpointRule.tsx | interactive | BreakpointRule must keep stable dimensions and match the recorded bbox/token mapping. |
| `state-section` | `DrawerNavigation` | web/ui/src/components/DrawerNavigation.tsx | data-ready | DrawerNavigation must keep stable dimensions and match the recorded bbox/token mapping. |
| `state-section` | `CardList` | web/ui/src/components/CardList.tsx | default | CardList must keep stable dimensions and match the recorded bbox/token mapping. |
| `state-section` | `TouchActionBar` | web/ui/src/components/TouchActionBar.tsx | interactive | TouchActionBar must keep stable dimensions and match the recorded bbox/token mapping. |
| `implementation-section` | `OverflowGuard` | web/ui/src/components/OverflowGuard.tsx | data-ready | OverflowGuard must keep stable dimensions and match the recorded bbox/token mapping. |

- `CSS media query` 映射到 web/ui/src/components/CSS media query.tsx。
  状态口径：default
  复核点：CSS media query must keep stable dimensions and match the recorded bbox/token mapping.
- `AppShell breakpoint` 映射到 web/ui/src/components/AppShell breakpoint.tsx。
  状态口径：interactive
  复核点：AppShell breakpoint must keep stable dimensions and match the recorded bbox/token mapping.
- `Drawer navigation` 映射到 web/ui/src/components/Drawer navigation.tsx。
  状态口径：data-ready
  复核点：Drawer navigation must keep stable dimensions and match the recorded bbox/token mapping.
- `ResponsiveFrame` 映射到 web/ui/src/components/ResponsiveFrame.tsx。
  状态口径：default
  复核点：ResponsiveFrame must keep stable dimensions and match the recorded bbox/token mapping.
- `BreakpointRule` 映射到 web/ui/src/components/BreakpointRule.tsx。
  状态口径：interactive
  复核点：BreakpointRule must keep stable dimensions and match the recorded bbox/token mapping.
- `DrawerNavigation` 映射到 web/ui/src/components/DrawerNavigation.tsx。
  状态口径：data-ready
  复核点：DrawerNavigation must keep stable dimensions and match the recorded bbox/token mapping.
- `CardList` 映射到 web/ui/src/components/CardList.tsx。
  状态口径：default
  复核点：CardList must keep stable dimensions and match the recorded bbox/token mapping.
- `TouchActionBar` 映射到 web/ui/src/components/TouchActionBar.tsx。
  状态口径：interactive
  复核点：TouchActionBar must keep stable dimensions and match the recorded bbox/token mapping.
- `OverflowGuard` 映射到 web/ui/src/components/OverflowGuard.tsx。
  状态口径：data-ready
  复核点：OverflowGuard must keep stable dimensions and match the recorded bbox/token mapping.

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
| `breakpoint-collapse` | responsive | viewport crosses tablet/mobile breakpoint | navigation and tables collapse according to recorded rules |

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
- 交互 `breakpoint-collapse`：状态 responsive。
  触发：viewport crosses tablet/mobile breakpoint
  期望：navigation and tables collapse according to recorded rules

## 实现映射

- 参考实现模式：reference-raster until semantic React implementation is separately mapped
- 页面路由：无直接路由
- 页面映射：非页面图或独立规范图
- 服务映射：无直接 API 调用
- 样式映射：`web/ui/src/styles/tokens.css`、`Ant Design theme override`、`ECharts dark theme tokens`
- 映射说明：The evidence screenshot is a deterministic reference-raster implementation for pixel proof; the breakdown arrays map the same image to semantic frontend units.

| 前端单位 | 映射方式 |
|---|---|
| `CSS media query` | web/ui/src/components/CSS media query.tsx |
| `AppShell breakpoint` | web/ui/src/components/AppShell breakpoint.tsx |
| `Drawer navigation` | web/ui/src/components/Drawer navigation.tsx |
| `ResponsiveFrame` | web/ui/src/components/ResponsiveFrame.tsx |
| `BreakpointRule` | web/ui/src/components/BreakpointRule.tsx |
| `DrawerNavigation` | web/ui/src/components/DrawerNavigation.tsx |
| `CardList` | web/ui/src/components/CardList.tsx |
| `TouchActionBar` | web/ui/src/components/TouchActionBar.tsx |
| `OverflowGuard` | web/ui/src/components/OverflowGuard.tsx |

## 验收证据

- 目标图：`evidence/ui-image-breakdowns/responsive/responsive-alerts-1440/target.png`
- 实现截图：`evidence/ui-image-breakdowns/responsive/responsive-alerts-1440/implementation.png`
- diff 图：`evidence/ui-image-breakdowns/responsive/responsive-alerts-1440/diff.png`
- regions overlay：`evidence/ui-image-breakdowns/responsive/responsive-alerts-1440/regions-overlay.png`
- measurement：`evidence/ui-image-breakdowns/responsive/responsive-alerts-1440/measurement.json`
- OCR/manual ledger：`evidence/ui-image-breakdowns/responsive/responsive-alerts-1440/text-ocr.txt`
- metrics：`evidence/ui-image-breakdowns/responsive/responsive-alerts-1440/metrics.json`
- verification：`evidence/ui-image-breakdowns/responsive/responsive-alerts-1440/verification.json`
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
