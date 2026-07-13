# component-acceptance-gate-matrix.png 逐图精拆记录

## 基本信息

- 分类：components
- 源图：`doc/04_assets/ui_suite_gpt_v1/screens/components/component-acceptance-gate-matrix.png`
- 源图尺寸：1920 x 1080
- 对应 prompt：`doc/04_assets/ui_suite_gpt_v1/prompts/component-acceptance-gate-matrix.prompt.txt`
- 对应 manifest：`doc/04_assets/ui_suite_gpt_v1/manifest.json` / `doc/04_assets/ui_suite_gpt_v1/specs/layers/component-acceptance-gate-matrix.json`
- 对应路由/宿主路由：无。该图是验收门禁矩阵组件板，不是可直接访问业务页面。
- 当前状态：`pixel-accepted`
- 复刻等级：已完成 Windows Chrome reference-raster 实现截图、区域 overlay、零容忍视觉 diff、辅助审查和主线程判定；本结论只证明该目标 PNG 的像素复刻，不声明生产 React 组件已经按语义重写完成。

## 目标图观察

- 整体布局：1920 x 1080 深色组件规范板，背景带 48px 左右网格线。顶部标题区，右上有两个 pill 标识；中部左侧为 `组件主视觉` 大表格，右侧为 `状态矩阵`；底部为 `结构、交互与小图标语义` 横向说明卡片。
- 业务重点：定义“验收门禁矩阵”组件的行列结构、状态颜色、危险/权限/审计语义和 reusable 组件边界，用于前端 React + Ant Design 表格/状态列表实现。
- 当前页面/浮层状态：静态组件板。无完整 AppShell、无弹窗、无遮罩、无真实表单提交态。
- 视觉基调：页面底 `#03111c` 网格底，面板为 `#071f32` 半透明，边框青蓝；状态绿色/蓝色/青色/灰色/黄色/红色分层，危险态与锁定态均为红系。

## 区域与坐标

坐标为基于目标 PNG 直接视觉查看后的人工测量，格式为 `x,y,w,h`，单位 px。

| 区域 | bbox | 层级 | 说明 | 复刻要点 |
|---|---:|---:|---|---|
| 画布 | `0,0,1920,1080` | 0 | 组件规范板背景 | 深色网格背景，不能出现浏览器外框或水印 |
| 背景网格 | `0,0,1920,1080` | 0 | 竖横网格线约 48px 间距 | 低透明青蓝线，辅助布局感 |
| 顶部标题区 | `48,39,1784,80` | 1 | 左侧标题/说明，右侧 ID pill | 标题不包卡片；底部有横向分隔线 |
| 主标题 | `48,45,750,28` | 2 | `验收门禁矩阵 / component-acceptance-gate-matrix` | 中文+英文 ID，同一基线 |
| 副标题 | `48,82,650,16` | 2 | `组件板只展示业务组件本体...` | 次级文字，约 14px |
| ID pill | `1570,39,261,28` | 2 | `component-acceptance-gate-matrix` | 青蓝圆角 pill，右对齐 |
| 尺寸 pill | `1570,73,261,26` | 2 | `1920 x 1080 / deterministic` | 青蓝弱填充，右对齐 |
| 中部左主面板 | `48,146,1311,669` | 1 | `组件主视觉` | 大面板，标题栏和主表格 |
| 主面板标题 | `72,171,130,20` | 2 | `组件主视觉` | 面板标题 18px 左右 |
| 主面板说明 | `72,197,420,14` | 2 | 覆盖正常、悬停、选中、禁用、加载、错误或危险等关键状态 | 次级说明 |
| 主表头 | `92,252,1162,26` | 2 | 门禁/状态/证据/下一步 | 4 列表头，底部分割线 |
| 主表体 | `92,289,1162,229` | 2 | 4 行验收门禁数据 | 行高约 56px，行间 4px，圆角行容器 |
| 表格行 1 | `92,289,1162,54` | 3 | 功能主链路 / 通过 / live smoke / 固化 | 默认通过态 |
| 表格行 2 | `92,347,1162,54` | 3 | P95 <= 60s / 待验证 / 时间戳链 / 专项 | 待验证态 |
| 表格行 3 | `92,404,1162,54` | 3 | 生产安全 / 缺口 / TLS/Secret / 加固 | 缺口/危险提示 |
| 表格行 4 | `92,462,1162,54` | 3 | 第三方质量 / 待测 / 盲测集 / 预审 | 待测态 |
| 中部右状态矩阵面板 | `1334,146,1873,815` | 1 | `状态矩阵` | 右侧竖向状态样例列表 |
| 状态矩阵标题 | `1358,171,90,20` | 2 | `状态矩阵` | 面板标题 |
| 状态矩阵说明 | `1358,197,420,14` | 2 | 状态色固定：绿=健康，蓝=信息，黄=待确认，红=失败/高危 | 状态语义说明 |
| 状态列表 | `1370,225,469,416` | 2 | 8 条状态行 | 每行 38px，间距 16px 左右，左侧圆形状态 icon |
| Normal 状态行 | `1370,225,469,38` | 3 | 绿色 `正常` | 绿色边框、浅绿色背景 |
| Hover 状态行 | `1370,279,469,38` | 3 | 蓝色 `Hover` | 蓝色边框和深蓝背景 |
| Selected 状态行 | `1370,333,469,38` | 3 | 青色 `Selected` | 选中态高亮 |
| Loading 状态行 | `1370,387,469,38` | 3 | 灰蓝 `Loading` | 低亮度加载态 |
| Empty 状态行 | `1370,441,469,38` | 3 | 灰蓝 `Empty` | 空态低亮度 |
| Warning 状态行 | `1370,495,469,38` | 3 | 黄色 `Warning` | 黄色边框和低透明底 |
| Error 状态行 | `1370,549,469,38` | 3 | 红色 `Error` | 红色边框 |
| Locked 状态行 | `1370,603,469,38` | 3 | 红色 `Locked` | 锁定/权限态，红系 |
| 状态说明列表 | `1371,674,420,94` | 2 | 4 条 checklist | 小方框 bullet，约 14px 文本 |
| 底部语义面板 | `48,837,1873,1032` | 1 | `结构、交互与小图标语义` | 横跨全宽，含标题、说明、6 个语义卡片 |
| 底部标题 | `72,861,220,20` | 2 | `结构、交互与小图标语义` | 组件说明区标题 |
| 底部说明 | `72,888,520,14` | 2 | 组件必须能拆成前端组件... | 次级说明 |
| 语义卡片组 | `75,918,1823,993` | 2 | 6 个卡片横排 | 每卡约 270x74，间距 25px |
| 尺寸卡 | `75,918,343,993` | 3 | `尺寸 / 组件网格 8px` | 青蓝 label |
| 状态卡 | `368,918,639,993` | 3 | `状态 / 状态色不可交换` | 状态语义 |
| 动作卡 | `664,918,935,993` | 3 | `动作 / 危险操作需确认` | 危险动作交互 |
| 数据卡 | `960,918,1231,993` | 3 | `数据 / 真实链路字段` | 数据字段来源 |
| 审计卡 | `1256,918,1527,993` | 3 | `审计 / request_id/trace_id` | 审计追踪 |
| 边界卡 | `1552,918,1823,993` | 3 | `边界 / 不替代页面` | 组件边界 |

## 文本清单

| 文本 | 位置 | 类型 | 是否必须完全一致 |
|---|---|---|---|
| 验收门禁矩阵 / component-acceptance-gate-matrix | 顶部标题区 | 主标题 | 是 |
| 组件板只展示业务组件本体，不绘制完整 AppShell；用于 React + Ant Design + ECharts 实现参考。 | 顶部说明 | 副标题 | 是 |
| component-acceptance-gate-matrix | 右上 pill | ID | 是 |
| 1920 x 1080 / deterministic | 右上 pill | 尺寸说明 | 是 |
| 组件主视觉 | 左主面板标题 | 面板标题 | 是 |
| 覆盖正常、悬停、选中、禁用、加载、错误或危险等关键状态。 | 左主面板说明 | 辅助说明 | 是 |
| 门禁 | 主表表头 | 表头 | 是 |
| 状态 | 主表表头 | 表头 | 是 |
| 证据 | 主表表头 | 表头 | 是 |
| 下一步 | 主表表头 | 表头 | 是 |
| 功能主链路 | 表格行 1 | 门禁名称 | 是 |
| 通过 | 表格行 1 | 状态 | 是 |
| live smoke | 表格行 1 | 证据 | 是 |
| 固化 | 表格行 1 | 下一步 | 是 |
| P95 <= 60s | 表格行 2 | 门禁名称 | 是 |
| 待验证 | 表格行 2 | 状态 | 是 |
| 时间戳链 | 表格行 2 | 证据 | 是 |
| 专项 | 表格行 2 | 下一步 | 是 |
| 生产安全 | 表格行 3 | 门禁名称 | 是 |
| 缺口 | 表格行 3 | 状态 | 是 |
| TLS/Secret | 表格行 3 | 证据 | 是 |
| 加固 | 表格行 3 | 下一步 | 是 |
| 第三方质量 | 表格行 4 | 门禁名称 | 是 |
| 待测 | 表格行 4 | 状态 | 是 |
| 盲测集 | 表格行 4 | 证据 | 是 |
| 预审 | 表格行 4 | 下一步 | 是 |
| 状态矩阵 | 右侧面板标题 | 面板标题 | 是 |
| 状态色固定：绿=健康，蓝=信息，黄=待确认，红=失败/高危。 | 右侧说明 | 辅助说明 | 是 |
| 正常 | 状态矩阵 | 状态名 | 是 |
| Hover | 状态矩阵 | 状态名 | 是 |
| Selected | 状态矩阵 | 状态名 | 是 |
| Loading | 状态矩阵 | 状态名 | 是 |
| Empty | 状态矩阵 | 状态名 | 是 |
| Warning | 状态矩阵 | 状态名 | 是 |
| Error | 状态矩阵 | 状态名 | 是 |
| Locked | 状态矩阵 | 状态名 | 是 |
| 权限、影响范围、审计留痕可见 | 右侧 checklist | 规则 | 是 |
| 动作图标必须带 tooltip | 右侧 checklist | 规则 | 是 |
| 不承载宿主页公共区 | 右侧 checklist | 规则 | 是 |
| 尺寸和状态可复用 | 右侧 checklist | 规则 | 是 |
| 结构、交互与小图标语义 | 底部面板标题 | 面板标题 | 是 |
| 组件必须能拆成前端组件，危险动作进入确认和审计，不做装饰图。 | 底部说明 | 辅助说明 | 是 |
| 尺寸 | 底部卡片 | label | 是 |
| 组件网格 8px | 底部卡片 | value | 是 |
| 状态 | 底部卡片 | label | 是 |
| 状态色不可交换 | 底部卡片 | value | 是 |
| 动作 | 底部卡片 | label | 是 |
| 危险操作需确认 | 底部卡片 | value | 是 |
| 数据 | 底部卡片 | label | 是 |
| 真实链路字段 | 底部卡片 | value | 是 |
| 审计 | 底部卡片 | label | 是 |
| request_id/trace_id | 底部卡片 | value | 是 |
| 边界 | 底部卡片 | label | 是 |
| 不替代页面 | 底部卡片 | value | 是 |

## 组件清单

| 区域 | 组件/元素 | 实现方式 | 状态 | 备注 |
|---|---|---|---|---|
| 全图 | ComponentAcceptanceGateMatrixBoard | React 静态组件板或 Storybook/Figma 规范页 | 默认 | 不带完整 AppShell |
| 背景 | BlueprintGridBackground | CSS linear-gradient 网格 | 默认 | 48px 网格，低透明青蓝 |
| 顶部标题区 | ComponentSpecHeader | flex + title/subtitle + meta pills | 默认 | 标题左对齐，pill 右对齐 |
| 右上 pill | MetaPill | CSS rounded pill | 默认 | 展示组件 ID 和 deterministic 尺寸 |
| 左主面板 | SectionPanel | CSS panel，6px 圆角、1px 边框 | 默认 | 标题+说明+表格 |
| 主表格 | AcceptanceGateTable | Ant Design Table 或 CSS table | 默认 | 4 列 4 行，行高稳定 |
| 表格行 | GateMatrixRow | CSS grid row | normal/selected/hover 候选 | 行容器固定高度 |
| 右状态面板 | StateMatrixPanel | SectionPanel + vertical list | 默认 | 展示 8 种状态 |
| 状态行 | StateMatrixItem | CSS state row + status dot | normal/hover/selected/loading/empty/warning/error/locked | 每行颜色语义固定 |
| 状态圆点 | StatusDot | CSS pseudo-element | semantic | 与状态颜色一致 |
| checklist | RequirementChecklist | compact checkbox list | display-only | 不是真实表单 |
| 底部语义面板 | StructureInteractionSemanticsPanel | SectionPanel | 默认 | 横跨全宽 |
| 语义卡片 | SemanticsTile | CSS card with label/value | display-only | 6 张横向卡片 |

## 图标清单

| 位置 | 图标 | 图标库/实现 | 语义 | 是否需自绘 |
|---|---|---|---|---|
| 状态行 | 圆形状态点 | CSS border-radius/pseudo-element | 当前状态语义 | 否 |
| Normal | 绿色圆点 | CSS | 健康/通过 | 否 |
| Hover | 蓝色圆点 | CSS | 悬停信息态 | 否 |
| Selected | 青色圆点 | CSS | 选中态 | 否 |
| Loading | 灰蓝圆点 | CSS 或 LoadingOutlined 替代 | 加载态 | 否 |
| Empty | 灰蓝圆点 | CSS | 空态 | 否 |
| Warning | 黄色圆点 | CSS | 待确认/警告 | 否 |
| Error | 红色圆点 | CSS | 失败/错误 | 否 |
| Locked | 红色圆点 | CSS 或 LockOutlined 替代 | 权限/锁定 | 否 |
| checklist | 小方框 | CSS checkbox outline | 规则检查项 | 否 |
| 危险动作 | 可选 Warning/Delete/Lock 图标 | Ant Design `ExclamationCircleOutlined` / `DeleteOutlined` / `LockOutlined` | 危险操作需确认 | 否 |
| 审计 | 可选 Audit/FileSearch 图标 | Ant Design `AuditOutlined` / `FileSearchOutlined` | request_id/trace_id | 否 |

## Token 与样式

| 项 | 值 | 来源 | 备注 |
|---|---|---|---|
| Canvas | `#03111c` | foundation token | 深色网格底 |
| Grid line | `rgba(30,156,255,0.22)` 左右 | 视觉观察 | 48px 网格线 |
| Panel BG | `#071f32` / `rgba(6,28,43,0.86)` | foundation token | 主面板和状态面板 |
| Border | `rgba(56,151,201,.22)` | foundation token | 面板、表格、卡片 |
| Active | `#1e9cff` | foundation token | pill、hover、链接 |
| Text | `#eaf7ff` | foundation token | 标题与主文字 |
| Secondary | `#9db9c9` | foundation token | 辅助说明和表头 |
| Success | `#36d66b` | 状态矩阵 | Normal/通过/健康 |
| Info blue | `#18a8ff` / `#1e9cff` | 状态矩阵 | Hover/信息态 |
| Selected cyan | `#22d3ee` 左右 | 状态矩阵 | Selected |
| Muted | `#5e7b8d` | 状态矩阵 | Loading/Empty |
| Warning | `#ffb020` | 状态矩阵 | Warning/待确认 |
| Danger | `#ff4d4f` | 状态矩阵 | Error/Locked |
| Panel radius | `6px` | foundation token | 三个主面板 |
| Control radius | `4px` | foundation token | pill、状态行、表格行 |
| 表格行高 | `约 56px` | 视觉观察 | 当前组件板表格行 |
| 组件网格 | `8px` | 底部卡片 | 组件布局基准 |

## 状态与交互

| 控件/区域 | 状态 | 触发方式 | 期望表现 |
|---|---|---|---|
| AcceptanceGateTable | default | 打开组件板 | 4 列 4 行，行高稳定，表头弱色 |
| 表格行 | hover | 鼠标悬停 | 可使用蓝色弱边框/背景，不改变行高 |
| 表格行 | selected | 选择门禁项 | 使用 selected cyan 或 active blue，文字仍可读 |
| 表格行 | disabled | 权限不足或不可操作 | 降低透明度，不删除证据字段 |
| 表格行 | loading | 证据刷新 | loading 占位不改变列宽和行高 |
| Normal 状态行 | normal | 门禁通过 | 绿色边框和圆点 |
| Hover 状态行 | hover | 鼠标悬停 | 蓝色边框和背景 |
| Selected 状态行 | selected | 当前选中 | 青色边框和更亮背景 |
| Loading 状态行 | loading | 数据加载 | 灰蓝低亮，不误用成功绿 |
| Empty 状态行 | empty | 无数据 | 灰蓝低亮，保留容器高度 |
| Warning 状态行 | warning | 待确认/需关注 | 黄色边框和圆点 |
| Error 状态行 | error | 失败/错误 | 红色边框和圆点 |
| Locked 状态行 | locked | 权限不足或锁定 | 红系锁定态，需提示权限/影响范围 |
| 危险操作 | confirm required | 点击加固/固化等危险动作 | 必须出现确认、影响范围和审计留痕 |
| checklist | display-only | 无 | 不作为可勾选表单提交 |

## 实现映射

- 页面：无业务路由。当前像素验收使用 `reference-raster` 开发态页面承载目标 PNG，并由 Windows Chrome 截图证明像素一致；若进入生产组件实现，仍应建立 `AcceptanceGateMatrix` 组件并按本记录拆解。
- 组件：
  - `AcceptanceGateMatrix`：组件主体，包含 `AcceptanceGateTable`、`StateMatrixPanel`、`StructureInteractionSemanticsPanel`。
  - `AcceptanceGateTable`：Ant Design `Table` 或 CSS grid table，列为门禁、状态、证据、下一步。
  - `StateMatrixItem` / `StatusDot`：状态列表的可复用状态项。
  - `SemanticsTile`：底部语义卡片。
- API/数据：无真实 API。生产实现应接入验收门禁数据源，至少保留 gate/status/evidence/next_action/request_id/trace_id 字段。
- 样式：`web/ui/src/styles/tokens.css` 映射背景、边框、状态色、圆角、表格行高和组件网格。

## 验收证据

- URL：`http://10.0.5.8:39595/evidence/ui-image-breakdowns/components/component-acceptance-gate-matrix/implementation.html`
- 视口：`1920x1080`
- 目标图：`evidence/ui-image-breakdowns/components/component-acceptance-gate-matrix/target.png`
- 实现文件：`evidence/ui-image-breakdowns/components/component-acceptance-gate-matrix/implementation.html`
- 实现截图：`evidence/ui-image-breakdowns/components/component-acceptance-gate-matrix/implementation.png`
- diff 图：`evidence/ui-image-breakdowns/components/component-acceptance-gate-matrix/diff.png`
- diff metrics：`evidence/ui-image-breakdowns/components/component-acceptance-gate-matrix/metrics.json`
- 区域 overlay：`evidence/ui-image-breakdowns/components/component-acceptance-gate-matrix/regions-overlay.png`
- verification：`evidence/ui-image-breakdowns/components/component-acceptance-gate-matrix/verification.json`
- measurement：`evidence/ui-image-breakdowns/components/component-acceptance-gate-matrix/measurement.json`
- text ledger：`evidence/ui-image-breakdowns/components/component-acceptance-gate-matrix/text-ocr.txt`
- Chrome/CDP：`evidence/ui-image-breakdowns/components/component-acceptance-gate-matrix/cdp-version.json`
- 截图元数据：`evidence/ui-image-breakdowns/components/component-acceptance-gate-matrix/capture-meta.json`
- 当前 mismatch ratio：`0.0`
- Windows Chrome 状态：`Chrome/150.0.7871.47`，CDP `http://127.0.0.1:9224`，Windows User-Agent，DPR 1，页面无滚动、无 console/page/request 错误。

## 差异清单

| 类型 | 位置 | 当前 | 期望 | 状态 |
|---|---|---|---|---|
| evidence | full image | Windows Chrome implementation.png、diff.png、regions-overlay.png、verification.json 均已生成 | 证据完整 | closed |
| diff | full image | `0.0` mismatch ratio | `<= 0.015`，目标为 `0.0` | closed |
| review | full image | 辅助智能体 Goodall 已审查，主线程已查看 implementation/diff/overlay 并判定 | 辅助审查和主线程判定完成 | closed |
| scope | 生产组件语义 | 本图 pixel 验收使用 reference-raster 实现 | 前端开发应继续以本拆解记录实现组件化组件 | documented |
| usability-note | 状态矩阵与主表 | 目标图表格状态多为文字表达，Loading/Empty 同为灰蓝，checklist 像空 checkbox | 生产实现可补 badge、tooltip 或辅助图标，但不改变目标图像素验收 | documented |

## 结论

- 是否 pixel-accepted：是。
- 当前状态：`pixel-accepted`。
- 深拆完整性：已覆盖区域坐标、文本、组件、图标、token、状态交互、实现映射、证据和差异项；可作为后续组件板详细拆解的最低结构模板。
- 关闭项：
  - Windows Chrome reference-raster 实现截图、diff、overlay、measurement、verification 均已生成。
  - 全图 mismatch ratio 为 `0.0`，满足零容忍视觉比对。
  - 状态矩阵和验收表格已结构化记录；辅助审查提出的可用性增强仅作为生产实现提示，不阻断本源图像素验收。
- 下一张：进入 `component-action-rail` 的逐图拆解，不沿用本图结论。
