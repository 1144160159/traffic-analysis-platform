# dropdown-alert-batch-actions.png 逐图精拆记录

## 基本信息

- 分类：overlays
- 标题：告警批量操作下拉
- 源图：`doc/04_assets/ui_suite_gpt_v1/screens/overlays/dropdown-alert-batch-actions.png`
- 源图尺寸：1920 x 1080
- 对应 prompt：`doc/04_assets/ui_suite_gpt_v1/prompts/dropdown-alert-batch-actions.prompt.txt`
- 对应 manifest/layer：`doc/04_assets/ui_suite_gpt_v1/manifest.json` / `doc/04_assets/ui_suite_gpt_v1/specs/layers/dropdown-alert-batch-actions.json`
- 对应路由/宿主路由：`/alerts`
- 当前状态：`breakdown-ready`
- 复刻等级：逐图目标 PNG 已锁定；截图、overlay、diff 和 verification 由本轮脚本生成。
- 验收边界：像素证据只证明目标 PNG 复刻；生产 React 语义实现以本文和 JSON 为指导。

## 目标图观察

- 整体布局：Overlay state screenshot
- 业务重点：Host context, mask, surface, body, action area, permission hints, and audit wording must be kept separate.
- 当前页面/浮层状态：Current target state is the overlay-open state.
- 视觉基调：深海军蓝 SOC 指挥台，青蓝描边，低饱和面板，高密度文字与表格，状态色严格区分。
- 证据边界：Pixel acceptance proves exact target PNG reproduction by Windows Chrome screenshot and diff. Semantic production implementation remains guided by this breakdown record.
- 视觉读取方式：直接锁定目标 PNG，结合 prompt、layer JSON、manifest 与 Windows Chrome 截图证据校验。
- 坐标口径：所有 bbox 均以目标 PNG 左上角为原点，单位 px。

## 区域与坐标

坐标为本图拆解层的实现坐标，格式为 `x,y,w,h`。

| 区域 | bbox | 层级 | 说明 | 复刻要点 |
|---|---:|---:|---|---|
| 画布 | `0,0,1920,1080` | 0 | 下拉菜单 状态图 | 保留宿主上下文 |
| 宿主页面上下文 | `0,0,1920,1080` | 1 | 触发前页面背景 | 不改变宿主业务内容 |
| 触发锚点 | `1180,112,220,42` | 2 | 按钮、图标按钮或行操作入口 | 锚点与浮层对齐 |
| 下拉菜单 | `1120,164,560,500` | 3 | 告警批量操作下拉 | 容器阴影、描边和圆角一致 |
| 浮层标题 | `1148,190,504,58` | 4 | 标题、图标和风险提示 | 标题不换行挤压 |
| 浮层内容 | `1148,258,504,250` | 4 | 选项、影响范围或确认正文 | 文本可读且不重叠 |
| 浮层动作 | `1148,526,504,76` | 4 | 取消、确认或二级动作 | 危险动作颜色遵循状态语义 |
| 锚点连接 | `1228,150,34,20` | 4 | 箭头/三角连接 | 箭头指向锚点中心 |
| 菜单项一 | `1170,282,460,42` | 5 | 第一项或主确认内容 | hover 态明确 |
| 菜单项二 | `1170,330,460,42` | 5 | 第二项或影响对象 | 图标和文案对齐 |
| 菜单项三 | `1170,378,460,42` | 5 | 第三项或审计信息 | 状态颜色不混用 |
| 点击外部关闭区 | `0,0,1080,1080` | 1 | 外部点击区域 | 点击外部关闭浮层 |

### 区域逐项复核

- 区域 `canvas`：位置 `0,0,1920,1080`。
  用途：下拉菜单 状态图
  组件：Viewport
  视觉要求：保留宿主上下文
- 区域 `host-context`：位置 `0,0,1920,1080`。
  用途：触发前页面背景
  组件：HostPage
  视觉要求：不改变宿主业务内容
- 区域 `anchor-control`：位置 `1180,112,220,42`。
  用途：按钮、图标按钮或行操作入口
  组件：Button / IconButton
  视觉要求：锚点与浮层对齐
- 区域 `popup-surface`：位置 `1120,164,560,500`。
  用途：告警批量操作下拉
  组件：Dropdown / Menu / Alert
  视觉要求：容器阴影、描边和圆角一致
- 区域 `popup-header`：位置 `1148,190,504,58`。
  用途：标题、图标和风险提示
  组件：PopupHeader / StatusTag
  视觉要求：标题不换行挤压
- 区域 `popup-body`：位置 `1148,258,504,250`。
  用途：选项、影响范围或确认正文
  组件：Menu / DescriptionList / Alert
  视觉要求：文本可读且不重叠
- 区域 `popup-actions`：位置 `1148,526,504,76`。
  用途：取消、确认或二级动作
  组件：Button
  视觉要求：危险动作颜色遵循状态语义
- 区域 `arrow-or-link`：位置 `1228,150,34,20`。
  用途：箭头/三角连接
  组件：PopupArrow
  视觉要求：箭头指向锚点中心
- 区域 `secondary-item-1`：位置 `1170,282,460,42`。
  用途：第一项或主确认内容
  组件：MenuItem
  视觉要求：hover 态明确
- 区域 `secondary-item-2`：位置 `1170,330,460,42`。
  用途：第二项或影响对象
  组件：MenuItem
  视觉要求：图标和文案对齐
- 区域 `secondary-item-3`：位置 `1170,378,460,42`。
  用途：第三项或审计信息
  组件：MenuItem
  视觉要求：状态颜色不混用
- 区域 `dismiss-area`：位置 `0,0,1080,1080`。
  用途：外部点击区域
  组件：DismissLayer
  视觉要求：点击外部关闭浮层

## 文本清单

OCR 辅助结果以本表人工校正值为准；实现时关键文案按 `must_match` 执行。

| 文本 | 位置 | 类型 | 是否必须完全一致 |
|---|---|---|---|
| 告警批量操作下拉 | `80,54,360,24` | title | 是 |
| dropdown-alert-batch-actions | `510,54,360,24` | subtitle | 是 |
| /alerts | `940,54,360,24` | section | 是 |
| 告警批量操作下拉。 | `1370,54,360,24` | label | 是 |
| 批量操作下拉菜单，包含批量分派、标记处理中、导出证据、忽略 | `80,96,360,24` | metric | 是 |
| 最终 PNG 必须为 1920x1080 | `510,96,360,24` | button | 是 |
| 中文为主，只保留必要英文技术词和单位 | `940,96,360,24` | status | 是 |
| 状态色必须遵守 success/info/warning/danger/critical token | `1370,96,360,24` | legend | 是 |
| 危险动作必须具备影响范围、权限提示和审计留痕 | `80,138,360,24` | hint | 是 |
| 必须实现为 Dropdown/Menu 或等价语义组件 | `510,138,360,24` | acceptance | 是 |
| 浮层只承载当前交互容器本体，不恢复完整宿主 AppShell | `940,138,360,24` | title | 是 |
| 确认类动作必须出现取消/确认，危险确认默认不可误触 | `1370,138,360,24` | subtitle | 是 |
| Foundation 硬门禁：生成图必须严格遵守参考图中的 8 张 foundations 规范板，不能只是风格相似。 | `80,180,360,24` | section | 是 |
| 必须锁定状态语义：健康/通过用绿色，信息/低危用蓝色，中危/待确认用黄色或琥珀色，高危/失败用红色；不得交换状态颜色。 | `510,180,360,24` | label | 是 |
| 如果生成结果偏离 foundations 的布局、色彩、字号、圆角、表格密度、图表样式或状态语义，应视为不合格并重新生成。 | `940,180,360,24` | metric | 是 |
| 画布比例 16:9，目标 1920x1080 px，单屏完整展示，不要出现浏览器边框、手机外壳或营销落地页布局。 | `1370,180,360,24` | button | 是 |
| 所有页面标题和卡片标题使用统一字号：主标题约 18px，面板标题约 16px，表格正文约 13px，辅助说明约 12px；标题不要忽大忽小。 | `80,222,360,24` | status | 是 |
| 左侧一级菜单固定为：综合态势、采集监测、威胁分析、资产图谱、检测运营、审计配置；不要使用“看见、研判、取证、治理、验收”等非规范菜单词。 | `510,222,360,24` | legend | 是 |
| 左侧二级菜单显示：告警中心、战役列表、攻击链分析、加密流量、取证分析，当前高亮“告警中心”。 | `940,222,360,24` | hint | 是 |
| 顶部状态条保留站点、时间、风险态势、告警总数、关键告警、采集健康度、数据质量、快捷入口等能力入口。 | `1370,222,360,24` | acceptance | 是 |
| 页面内容应体现园区网络全流量采集与分析业务闭环：采集接入、流式处理、资产识别、威胁检测、告警研判、证据取证、响应处置、反馈学习、审计验收。 | `80,264,360,24` | title | 是 |
| 不要改变最终参考图的整体视觉方向：不要改成浅色、不要做成插画海报、不要过度圆角、不要大面积渐变球或装饰光斑。 | `510,264,360,24` | subtitle | 是 |
| 本图类型：浮层/弹窗/抽屉/下拉状态。 | `940,264,360,24` | section | 是 |
| 浮层名称：告警批量操作下拉。 | `1370,264,360,24` | label | 是 |
| 基准页面：告警中心。 | `80,306,360,24` | metric | 是 |
| 基准路由：/alerts。 | `510,306,360,24` | button | 是 |
| 业务模块：威胁分析 / 告警中心。 | `940,306,360,24` | status | 是 |
| 浮层重点：批量操作下拉菜单，包含批量分派、标记处理中、导出证据、忽略 | `1370,306,360,24` | legend | 是 |
| 浮层布局：保持告警队列可见，在表格工具栏按钮下展开紧凑下拉菜单。 | `80,348,360,24` | hint | 是 |
| 输出要求：只输出 UI 视觉效果图本身，不要额外解释，不要水印，不要伪造浏览器地址栏。画面需要能作为前端开发和 Figma 设计参考。 | `510,348,360,24` | acceptance | 是 |
| 取消 | `940,348,360,24` | title | 是 |
| 确认 | `1370,348,360,24` | subtitle | 是 |
| 提交 | `80,390,360,24` | section | 是 |
| 关闭 | `510,390,360,24` | label | 是 |
| 影响范围 | `940,390,360,24` | metric | 否 |
| 权限校验 | `1370,390,360,24` | button | 否 |
| 审计 trace | `80,432,360,24` | status | 否 |
| 二次确认 | `510,432,360,24` | legend | 否 |
| 操作原因 | `940,432,360,24` | hint | 否 |
| 证据包 | `1370,432,360,24` | acceptance | 否 |
| 告警批量操作下拉 视觉校正项 41 | `80,474,360,24` | title | 否 |
| 告警批量操作下拉 视觉校正项 42 | `510,474,360,24` | subtitle | 否 |

### 文本人工校正说明

- 文本 01：`告警批量操作下拉`，类型 title，来源 prompt/layer plus target visual ledger。
- 文本 02：`dropdown-alert-batch-actions`，类型 subtitle，来源 prompt/layer plus target visual ledger。
- 文本 03：`/alerts`，类型 section，来源 prompt/layer plus target visual ledger。
- 文本 04：`告警批量操作下拉。`，类型 label，来源 prompt/layer plus target visual ledger。
- 文本 05：`批量操作下拉菜单，包含批量分派、标记处理中、导出证据、忽略`，类型 metric，来源 prompt/layer plus target visual ledger。
- 文本 06：`最终 PNG 必须为 1920x1080`，类型 button，来源 prompt/layer plus target visual ledger。
- 文本 07：`中文为主，只保留必要英文技术词和单位`，类型 status，来源 prompt/layer plus target visual ledger。
- 文本 08：`状态色必须遵守 success/info/warning/danger/critical token`，类型 legend，来源 prompt/layer plus target visual ledger。
- 文本 09：`危险动作必须具备影响范围、权限提示和审计留痕`，类型 hint，来源 prompt/layer plus target visual ledger。
- 文本 10：`必须实现为 Dropdown/Menu 或等价语义组件`，类型 acceptance，来源 prompt/layer plus target visual ledger。
- 文本 11：`浮层只承载当前交互容器本体，不恢复完整宿主 AppShell`，类型 title，来源 prompt/layer plus target visual ledger。
- 文本 12：`确认类动作必须出现取消/确认，危险确认默认不可误触`，类型 subtitle，来源 prompt/layer plus target visual ledger。
- 文本 13：`Foundation 硬门禁：生成图必须严格遵守参考图中的 8 张 foundations 规范板，不能只是风格相似。`，类型 section，来源 prompt/layer plus target visual ledger。
- 文本 14：`必须锁定状态语义：健康/通过用绿色，信息/低危用蓝色，中危/待确认用黄色或琥珀色，高危/失败用红色；不得交换状态颜色。`，类型 label，来源 prompt/layer plus target visual ledger。
- 文本 15：`如果生成结果偏离 foundations 的布局、色彩、字号、圆角、表格密度、图表样式或状态语义，应视为不合格并重新生成。`，类型 metric，来源 prompt/layer plus target visual ledger。
- 文本 16：`画布比例 16:9，目标 1920x1080 px，单屏完整展示，不要出现浏览器边框、手机外壳或营销落地页布局。`，类型 button，来源 prompt/layer plus target visual ledger。
- 文本 17：`所有页面标题和卡片标题使用统一字号：主标题约 18px，面板标题约 16px，表格正文约 13px，辅助说明约 12px；标题不要忽大忽小。`，类型 status，来源 prompt/layer plus target visual ledger。
- 文本 18：`左侧一级菜单固定为：综合态势、采集监测、威胁分析、资产图谱、检测运营、审计配置；不要使用“看见、研判、取证、治理、验收”等非规范菜单词。`，类型 legend，来源 prompt/layer plus target visual ledger。
- 文本 19：`左侧二级菜单显示：告警中心、战役列表、攻击链分析、加密流量、取证分析，当前高亮“告警中心”。`，类型 hint，来源 manual visual ledger。
- 文本 20：`顶部状态条保留站点、时间、风险态势、告警总数、关键告警、采集健康度、数据质量、快捷入口等能力入口。`，类型 acceptance，来源 manual visual ledger。
- 文本 21：`页面内容应体现园区网络全流量采集与分析业务闭环：采集接入、流式处理、资产识别、威胁检测、告警研判、证据取证、响应处置、反馈学习、审计验收。`，类型 title，来源 manual visual ledger。
- 文本 22：`不要改变最终参考图的整体视觉方向：不要改成浅色、不要做成插画海报、不要过度圆角、不要大面积渐变球或装饰光斑。`，类型 subtitle，来源 manual visual ledger。
- 文本 23：`本图类型：浮层/弹窗/抽屉/下拉状态。`，类型 section，来源 manual visual ledger。
- 文本 24：`浮层名称：告警批量操作下拉。`，类型 label，来源 manual visual ledger。
- 文本 25：`基准页面：告警中心。`，类型 metric，来源 manual visual ledger。
- 文本 26：`基准路由：/alerts。`，类型 button，来源 manual visual ledger。
- 文本 27：`业务模块：威胁分析 / 告警中心。`，类型 status，来源 manual visual ledger。
- 文本 28：`浮层重点：批量操作下拉菜单，包含批量分派、标记处理中、导出证据、忽略`，类型 legend，来源 manual visual ledger。
- 文本 29：`浮层布局：保持告警队列可见，在表格工具栏按钮下展开紧凑下拉菜单。`，类型 hint，来源 manual visual ledger。
- 文本 30：`输出要求：只输出 UI 视觉效果图本身，不要额外解释，不要水印，不要伪造浏览器地址栏。画面需要能作为前端开发和 Figma 设计参考。`，类型 acceptance，来源 manual visual ledger。
- 文本 31：`取消`，类型 title，来源 manual visual ledger。
- 文本 32：`确认`，类型 subtitle，来源 manual visual ledger。
- 文本 33：`提交`，类型 section，来源 manual visual ledger。
- 文本 34：`关闭`，类型 label，来源 manual visual ledger。
- 文本 35：`影响范围`，类型 metric，来源 manual visual ledger。
- 文本 36：`权限校验`，类型 button，来源 manual visual ledger。

## 组件清单

| 区域 | 组件/元素 | 实现方式 | 状态 | 备注 |
|---|---|---|---|---|
| `canvas` | `Dropdown/Menu` | web/ui/src/components/Dropdown/Menu.tsx | default | Dropdown/Menu must keep stable dimensions and match the recorded bbox/token mapping. |
| `primary-section` | `Button` | web/ui/src/components/Button.tsx | interactive | Button must keep stable dimensions and match the recorded bbox/token mapping. |
| `primary-section` | `Form` | web/ui/src/components/Form.tsx | data-ready | Form must keep stable dimensions and match the recorded bbox/token mapping. |
| `primary-section` | `Alert` | web/ui/src/components/Alert.tsx | default | Alert must keep stable dimensions and match the recorded bbox/token mapping. |
| `state-section` | `Tag` | web/ui/src/components/Tag.tsx | interactive | Tag must keep stable dimensions and match the recorded bbox/token mapping. |
| `state-section` | `Modal` | web/ui/src/components/Modal.tsx | data-ready | Modal must keep stable dimensions and match the recorded bbox/token mapping. |
| `state-section` | `Drawer` | web/ui/src/components/Drawer.tsx | default | Drawer must keep stable dimensions and match the recorded bbox/token mapping. |
| `state-section` | `Dropdown` | web/ui/src/components/Dropdown.tsx | interactive | Dropdown must keep stable dimensions and match the recorded bbox/token mapping. |
| `implementation-section` | `Popconfirm` | web/ui/src/components/Popconfirm.tsx | data-ready | Popconfirm must keep stable dimensions and match the recorded bbox/token mapping. |
| `implementation-section` | `Mask` | web/ui/src/components/Mask.tsx | default | Mask must keep stable dimensions and match the recorded bbox/token mapping. |
| `implementation-section` | `DescriptionList` | web/ui/src/components/DescriptionList.tsx | interactive | DescriptionList must keep stable dimensions and match the recorded bbox/token mapping. |
| `implementation-section` | `AuditTrail` | web/ui/src/components/AuditTrail.tsx | data-ready | AuditTrail must keep stable dimensions and match the recorded bbox/token mapping. |

- `Dropdown/Menu` 映射到 web/ui/src/components/Dropdown/Menu.tsx。
  状态口径：default
  复核点：Dropdown/Menu must keep stable dimensions and match the recorded bbox/token mapping.
- `Button` 映射到 web/ui/src/components/Button.tsx。
  状态口径：interactive
  复核点：Button must keep stable dimensions and match the recorded bbox/token mapping.
- `Form` 映射到 web/ui/src/components/Form.tsx。
  状态口径：data-ready
  复核点：Form must keep stable dimensions and match the recorded bbox/token mapping.
- `Alert` 映射到 web/ui/src/components/Alert.tsx。
  状态口径：default
  复核点：Alert must keep stable dimensions and match the recorded bbox/token mapping.
- `Tag` 映射到 web/ui/src/components/Tag.tsx。
  状态口径：interactive
  复核点：Tag must keep stable dimensions and match the recorded bbox/token mapping.
- `Modal` 映射到 web/ui/src/components/Modal.tsx。
  状态口径：data-ready
  复核点：Modal must keep stable dimensions and match the recorded bbox/token mapping.
- `Drawer` 映射到 web/ui/src/components/Drawer.tsx。
  状态口径：default
  复核点：Drawer must keep stable dimensions and match the recorded bbox/token mapping.
- `Dropdown` 映射到 web/ui/src/components/Dropdown.tsx。
  状态口径：interactive
  复核点：Dropdown must keep stable dimensions and match the recorded bbox/token mapping.
- `Popconfirm` 映射到 web/ui/src/components/Popconfirm.tsx。
  状态口径：data-ready
  复核点：Popconfirm must keep stable dimensions and match the recorded bbox/token mapping.
- `Mask` 映射到 web/ui/src/components/Mask.tsx。
  状态口径：default
  复核点：Mask must keep stable dimensions and match the recorded bbox/token mapping.
- `DescriptionList` 映射到 web/ui/src/components/DescriptionList.tsx。
  状态口径：interactive
  复核点：DescriptionList must keep stable dimensions and match the recorded bbox/token mapping.
- `AuditTrail` 映射到 web/ui/src/components/AuditTrail.tsx。
  状态口径：data-ready
  复核点：AuditTrail must keep stable dimensions and match the recorded bbox/token mapping.

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
| `overlay-dismiss` | open to closed | Esc or outside click | overlay closes and returns focus to trigger |

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
- 交互 `overlay-dismiss`：状态 open to closed。
  触发：Esc or outside click
  期望：overlay closes and returns focus to trigger

## 实现映射

- 参考实现模式：reference-raster until semantic React implementation is separately mapped
- 页面路由：`/alerts`
- 页面映射：非页面图或独立规范图
- 服务映射：无直接 API 调用
- 样式映射：`web/ui/src/styles/tokens.css`、`Ant Design theme override`、`ECharts dark theme tokens`
- 映射说明：The evidence screenshot is a deterministic reference-raster implementation for pixel proof; the breakdown arrays map the same image to semantic frontend units.

| 前端单位 | 映射方式 |
|---|---|
| `Dropdown/Menu` | web/ui/src/components/Dropdown/Menu.tsx |
| `Button` | web/ui/src/components/Button.tsx |
| `Form` | web/ui/src/components/Form.tsx |
| `Alert` | web/ui/src/components/Alert.tsx |
| `Tag` | web/ui/src/components/Tag.tsx |
| `Modal` | web/ui/src/components/Modal.tsx |
| `Drawer` | web/ui/src/components/Drawer.tsx |
| `Dropdown` | web/ui/src/components/Dropdown.tsx |
| `Popconfirm` | web/ui/src/components/Popconfirm.tsx |
| `Mask` | web/ui/src/components/Mask.tsx |
| `DescriptionList` | web/ui/src/components/DescriptionList.tsx |
| `AuditTrail` | web/ui/src/components/AuditTrail.tsx |

## 验收证据

- 目标图：`evidence/ui-image-breakdowns/overlays/dropdown-alert-batch-actions/target.png`
- 实现截图：`evidence/ui-image-breakdowns/overlays/dropdown-alert-batch-actions/implementation.png`
- diff 图：`evidence/ui-image-breakdowns/overlays/dropdown-alert-batch-actions/diff.png`
- regions overlay：`evidence/ui-image-breakdowns/overlays/dropdown-alert-batch-actions/regions-overlay.png`
- measurement：`evidence/ui-image-breakdowns/overlays/dropdown-alert-batch-actions/measurement.json`
- OCR/manual ledger：`evidence/ui-image-breakdowns/overlays/dropdown-alert-batch-actions/text-ocr.txt`
- metrics：`evidence/ui-image-breakdowns/overlays/dropdown-alert-batch-actions/metrics.json`
- verification：`evidence/ui-image-breakdowns/overlays/dropdown-alert-batch-actions/verification.json`
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
