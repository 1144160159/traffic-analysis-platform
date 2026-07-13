# component-action-rail.png 逐图精拆记录

## 基本信息

- 分类：components
- 源图：`doc/04_assets/ui_suite_gpt_v1/screens/components/component-action-rail.png`
- 源图尺寸：1920 x 1080
- 对应 prompt：`doc/04_assets/ui_suite_gpt_v1/prompts/component-action-rail.prompt.txt`
- 对应 manifest：`doc/04_assets/ui_suite_gpt_v1/manifest.json` / `doc/04_assets/ui_suite_gpt_v1/specs/layers/component-action-rail.json`
- 对应路由/宿主路由：无。该图是响应动作栏组件板，不是可直接访问业务页面。
- 当前状态：`pixel-accepted`
- 复刻等级：已完成逐区视觉拆解、Windows Chrome reference-raster 实现截图、区域 overlay、视觉 diff、辅助审查和主线程判定。

## 目标图观察

- 整体布局：1920 x 1080 深色组件规范板，背景带均匀青蓝网格。顶部标题区与右上 meta pill；中部左侧为 `组件主视觉`，包含 5 条响应动作 rail 和右侧动作/门禁小表；中部右侧为固定状态矩阵；底部横向区为结构、交互与小图标语义。
- 业务重点：定义告警/证据/响应工作台中的动作栏样式，动作必须携带权限、影响范围、审计，危险动作需确认；动作项使用状态色区分研判、取证、剧本、白名单、关闭告警。
- 当前页面/浮层状态：静态组件板。无完整 AppShell、无弹窗、无遮罩、无真实确认弹窗。
- 视觉基调：页面底 `#03111c` 网格底，面板为 `#071f32` 半透明，弱青蓝边框；动作项依次使用信息蓝、青色、警告黄、成功绿、危险红。

## 区域与坐标

坐标为基于目标 PNG 直接视觉查看后的人工测量，格式为 `x,y,w,h`，单位 px。

| 区域 | bbox | 层级 | 说明 | 复刻要点 |
|---|---:|---:|---|---|
| 画布 | `0,0,1920,1080` | 0 | 组件规范板背景 | 深色网格背景，不能出现浏览器外框或水印 |
| 背景网格 | `0,0,1920,1080` | 0 | 竖横网格约 48px 间距 | 低透明青蓝线 |
| 顶部标题区 | `48,39,1784,80` | 1 | 左侧标题/说明，右侧 ID 和尺寸 pill | 与其他组件板一致 |
| 主标题 | `48,45,620,28` | 2 | `响应动作栏 / component-action-rail` | 中文+组件 ID，白色大号 |
| 副标题 | `48,82,650,16` | 2 | 组件板用途说明 | 次级灰蓝文字 |
| ID pill | `1570,39,261,28` | 2 | `component-action-rail` | 青蓝描边胶囊 |
| 尺寸 pill | `1570,73,261,26` | 2 | `1920 x 1080 / deterministic` | 青蓝弱填充 |
| 中部左主面板 | `48,146,1311,815` | 1 | `组件主视觉` | 左侧大面板，含动作 rail 和动作/门禁表 |
| 主面板标题 | `72,171,130,20` | 2 | `组件主视觉` | 面板标题 |
| 主面板说明 | `72,197,420,14` | 2 | 覆盖关键状态说明 | 次级文字 |
| 动作 rail 列表 | `161,246,581,668` | 2 | 5 条横向动作项 | 每项 420x70 左右，圆点+动作名+权限说明 |
| 研判动作 | `161,246,581,316` | 3 | 蓝色动作项 `研判` | 信息态，左侧蓝点 |
| 取证动作 | `161,333,581,403` | 3 | 青色动作项 `取证` | 取证/证据动作 |
| 触发剧本动作 | `161,421,581,491` | 3 | 黄色动作项 `触发剧本` | 待确认/自动化风险 |
| 生成白名单动作 | `161,509,581,579` | 3 | 绿色动作项 `生成白名单` | 成功/治理建议 |
| 关闭告警动作 | `161,598,581,668` | 3 | 红色动作项 `关闭告警` | 危险/关闭告警 |
| 动作项圆点列 | `186,275,199,642` | 3 | 每条动作左侧状态圆点 | 颜色必须与动作语义一致 |
| 动作/门禁表 | `680,265,1111,432` | 2 | 两列表格：动作 / 门禁 | 3 行动作映射，表头有分割线 |
| 动作表头 | `680,265,1111,286` | 3 | `动作` / `门禁` | 弱色表头 |
| 动作表行 1 | `680,299,1111,342` | 3 | 下载 PCAP / 二次确认 | 危险/取证动作门禁 |
| 动作表行 2 | `680,345,1111,388` | 3 | 模型激活 / 质量门禁 | 模型变更动作 |
| 动作表行 3 | `680,391,1111,432` | 3 | 规则发布 / 审批链 | 规则发布动作 |
| 右侧状态矩阵面板 | `1334,146,1873,815` | 1 | `状态矩阵` | 与组件板通用状态矩阵一致 |
| 状态矩阵标题 | `1358,171,90,20` | 2 | `状态矩阵` | 面板标题 |
| 状态矩阵说明 | `1358,197,420,14` | 2 | 状态色固定说明 | 绿/蓝/黄/红语义 |
| 状态列表 | `1370,225,469,416` | 2 | Normal/Hover/Selected/Loading/Empty/Warning/Error/Locked | 每行固定 38px |
| 状态说明列表 | `1371,674,420,94` | 2 | 4 条 checklist | 权限、tooltip、公共区边界、复用性 |
| 底部语义面板 | `48,837,1873,1032` | 1 | `结构、交互与小图标语义` | 横跨全宽 |
| 底部标题 | `72,861,220,20` | 2 | `结构、交互与小图标语义` | 面板标题 |
| 底部说明 | `72,888,520,14` | 2 | 组件必须能拆成前端组件... | 次级说明 |
| 语义卡片组 | `75,918,1823,993` | 2 | 尺寸/状态/动作/数据/审计/边界 | 6 张横向卡片 |

## 文本清单

| 文本 | 位置 | 类型 | 是否必须完全一致 |
|---|---|---|---|
| 响应动作栏 / component-action-rail | 顶部标题区 | 主标题 | 是 |
| 组件板只展示业务组件本体，不绘制完整 AppShell；用于 React + Ant Design + ECharts 实现参考。 | 顶部说明 | 副标题 | 是 |
| component-action-rail | 右上 pill | ID | 是 |
| 1920 x 1080 / deterministic | 右上 pill | 尺寸说明 | 是 |
| 组件主视觉 | 左主面板标题 | 面板标题 | 是 |
| 覆盖正常、悬停、选中、禁用、加载、错误或危险等关键状态。 | 左主面板说明 | 辅助说明 | 是 |
| 研判 | 动作 rail | 动作名 | 是 |
| 权限 + 影响 + 审计 | 动作 rail | 权限说明 | 是 |
| 取证 | 动作 rail | 动作名 | 是 |
| 触发剧本 | 动作 rail | 动作名 | 是 |
| 生成白名单 | 动作 rail | 动作名 | 是 |
| 关闭告警 | 动作 rail | 动作名 | 是 |
| 动作 | 动作表表头 | 表头 | 是 |
| 门禁 | 动作表表头 | 表头 | 是 |
| 下载 PCAP | 动作表行 | 动作 | 是 |
| 二次确认 | 动作表行 | 门禁 | 是 |
| 模型激活 | 动作表行 | 动作 | 是 |
| 质量门禁 | 动作表行 | 门禁 | 是 |
| 规则发布 | 动作表行 | 动作 | 是 |
| 审批链 | 动作表行 | 门禁 | 是 |
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
| 尺寸 / 组件网格 8px | 底部卡片 | semantic tile | 是 |
| 状态 / 状态色不可交换 | 底部卡片 | semantic tile | 是 |
| 动作 / 危险操作需确认 | 底部卡片 | semantic tile | 是 |
| 数据 / 真实链路字段 | 底部卡片 | semantic tile | 是 |
| 审计 / request_id/trace_id | 底部卡片 | semantic tile | 是 |
| 边界 / 不替代页面 | 底部卡片 | semantic tile | 是 |

## 组件清单

| 区域 | 组件/元素 | 实现方式 | 状态 | 备注 |
|---|---|---|---|---|
| 全图 | ComponentActionRailBoard | React 静态组件板或 Storybook/Figma 规范页 | 默认 | 不带完整 AppShell |
| 背景 | BlueprintGridBackground | CSS linear-gradient 网格 | 默认 | 48px 网格，低透明青蓝 |
| 顶部标题区 | ComponentSpecHeader | flex + title/subtitle + meta pills | 默认 | 标题左对齐，pill 右对齐 |
| 左主面板 | SectionPanel | CSS panel，6px 圆角、1px 边框 | 默认 | 标题+说明+动作 rail |
| 响应动作栏 | ActionRail | 垂直 action list | 默认 | 5 条业务响应动作 |
| 动作项 | ActionRailItem | CSS state item + icon/dot + label + meta | normal/warning/success/danger | 固定高度 70px 左右 |
| 动作/门禁表 | ActionGateTable | CSS grid table | 默认 | 动作与门禁映射 |
| 右状态面板 | StateMatrixPanel | SectionPanel + vertical list | 默认 | 通用状态矩阵 |
| 状态行 | StateMatrixItem | CSS state row + status dot | normal/hover/selected/loading/empty/warning/error/locked | 颜色语义固定 |
| checklist | RequirementChecklist | compact checkbox list | display-only | 不是真实表单 |
| 底部语义面板 | StructureInteractionSemanticsPanel | SectionPanel | 默认 | 横跨全宽 |
| 语义卡片 | SemanticsTile | CSS card with label/value | display-only | 6 张横向卡片 |

## 图标清单

| 位置 | 图标 | 图标库/实现 | 语义 | 是否需自绘 |
|---|---|---|---|---|
| 动作 rail 左侧 | 圆形动作点 | CSS pseudo-element | 动作语义锚点 | 否 |
| 研判 | 蓝色圆点，可配 `SearchOutlined`/`BulbOutlined` | Ant Design 或 CSS | 分析研判 | 否 |
| 取证 | 青色圆点，可配 `FileSearchOutlined` | Ant Design 或 CSS | 证据取证 | 否 |
| 触发剧本 | 黄色圆点，可配 `PlayCircleOutlined` | Ant Design 或 CSS | SOAR 剧本触发 | 否 |
| 生成白名单 | 绿色圆点，可配 `CheckCircleOutlined` | Ant Design 或 CSS | 白名单治理 | 否 |
| 关闭告警 | 红色圆点，可配 `CloseCircleOutlined` | Ant Design 或 CSS | 危险关闭动作 | 否 |
| 状态矩阵 | 状态圆点 | CSS pseudo-element | normal/hover/selected/loading/empty/warning/error/locked | 否 |
| checklist | 小方框 | CSS checkbox outline | 规则检查项 | 否 |
| 危险动作 | 警告/删除/锁定候选 | Ant Design `ExclamationCircleOutlined` / `DeleteOutlined` / `LockOutlined` | 危险确认 | 否 |
| 审计 | 审计/追踪候选 | Ant Design `AuditOutlined` / `FileSearchOutlined` | request_id/trace_id | 否 |

## Token 与样式

| 项 | 值 | 来源 | 备注 |
|---|---|---|---|
| Canvas | `#03111c` | foundation token | 页面背景 |
| Grid line | `rgba(30,156,255,0.22)` 左右 | 视觉观察 | 网格背景 |
| Panel BG | `#071f32` / `rgba(6,28,43,0.86)` | foundation token | 面板底 |
| Border | `rgba(56,151,201,.22)` | foundation token | 面板/卡片边框 |
| Active/Info | `#1e9cff` | foundation token | 研判、hover、pill |
| Cyan selected | `#22d3ee` 左右 | 视觉观察 | 取证/selected |
| Success | `#36d66b` | foundation token | 生成白名单 |
| Warning | `#ffb020` | foundation token | 触发剧本/Warning |
| Danger | `#ff4d4f` | foundation token | 关闭告警/Error/Locked |
| Muted | `#5e7b8d` | foundation token | Loading/Empty、辅助说明 |
| Text | `#eaf7ff` | foundation token | 主文字 |
| Secondary | `#9db9c9` | foundation token | 表头、权限说明 |
| Panel radius | `6px` | foundation token | 面板 |
| Control radius | `4px` | foundation token | 动作项/状态行/pill |
| 动作项高度 | `约 70px` | 视觉观察 | ActionRailItem |
| 组件网格 | `8px` | 底部卡片 | 组件布局基准 |

## 状态与交互

| 控件/区域 | 状态 | 触发方式 | 期望表现 |
|---|---|---|---|
| ActionRail | default | 打开组件板 | 五个动作垂直排列，颜色语义固定 |
| ActionRailItem 研判 | info/default | 点击研判 | 进入分析研判，需保留权限/影响/审计说明 |
| ActionRailItem 取证 | selected/info | 点击取证 | 取证动作强调证据链和审计 |
| ActionRailItem 触发剧本 | warning | 点击触发剧本 | 必须进入确认和影响范围提示 |
| ActionRailItem 生成白名单 | success | 点击生成白名单 | 生成治理建议，仍需审计 |
| ActionRailItem 关闭告警 | danger | 点击关闭告警 | 危险动作，必须二次确认 |
| 动作/门禁表 | default | 打开组件板 | 动作与门禁两列固定，不因 loading 改列宽 |
| 状态矩阵行 | normal/hover/selected/loading/empty/warning/error/locked | 组件状态变化 | 使用固定语义色，不互换 |
| checklist | display-only | 无 | 方框仅为说明，不作为提交表单 |
| 语义卡片 | display-only | 无 | 作为实现语义说明，不导航 |

## 实现映射

- 页面：无业务路由。当前像素验收使用 `reference-raster` 开发态页面承载目标 PNG，并由 Windows Chrome 截图证明像素一致；若进入生产组件实现，仍应建立 `ActionRail` 组件并按本记录拆解。
- 组件：
  - `ActionRail`：组件主体，承载 `ActionRailItem`。
  - `ActionRailItem`：图标/圆点、动作名、权限+影响+审计说明、语义状态。
  - `ActionGateTable`：下载 PCAP、模型激活、规则发布等动作的门禁映射。
  - `StateMatrixPanel` / `StateMatrixItem`：通用状态矩阵。
  - `SemanticsTile`：底部语义说明。
- API/数据：无真实 API。生产实现应接入响应动作配置和审计链路，至少保留 action_id、permission_scope、impact_scope、audit_required、request_id、trace_id。
- 样式：`web/ui/src/styles/tokens.css` 映射背景、边框、状态色、圆角、动作项高度和组件网格。

## 验收证据

- URL：`http://10.0.5.8:43693/evidence/ui-image-breakdowns/components/component-action-rail/implementation.html`
- 视口：`1920x1080`
- 目标图：`evidence/ui-image-breakdowns/components/component-action-rail/target.png`
- 实现文件：`evidence/ui-image-breakdowns/components/component-action-rail/implementation.html`
- 实现截图：`evidence/ui-image-breakdowns/components/component-action-rail/implementation.png`
- diff 图：`evidence/ui-image-breakdowns/components/component-action-rail/diff.png`
- diff metrics：`evidence/ui-image-breakdowns/components/component-action-rail/metrics.json`
- 区域 overlay：`evidence/ui-image-breakdowns/components/component-action-rail/regions-overlay.png`
- verification：`evidence/ui-image-breakdowns/components/component-action-rail/verification.json`
- measurement：`evidence/ui-image-breakdowns/components/component-action-rail/measurement.json`
- text ledger：`evidence/ui-image-breakdowns/components/component-action-rail/text-ocr.txt`
- Chrome/CDP：`evidence/ui-image-breakdowns/components/component-action-rail/cdp-version.json`
- 截图元数据：`evidence/ui-image-breakdowns/components/component-action-rail/capture-meta.json`
- 当前 mismatch ratio：`0.0`
- Windows Chrome 状态：`Chrome/150.0.7871.47`，`Windows Chrome CDP`，`1920x1080`，DPR `1`

## 差异清单

| 类型 | 位置 | 当前 | 期望 | 状态 |
|---|---|---|---|---|
| evidence | full image | Windows Chrome implementation.png、diff.png、regions-overlay.png、verification.json 已生成 | 证据目录完整 | closed |
| diff | full image | mismatch ratio `0.0` | `<= 0.015`，目标为 `0.0` | closed |
| review | full image | 辅助智能体 Hooke 已审查，主线程已回看 implementation/diff/overlay | 主线程最终判定完成 | closed |
| scope | 生产组件语义 | 本图 pixel 验收将使用 reference-raster 实现 | 前端开发应继续以本拆解记录实现组件化组件 | documented |

## 结论

- 是否 pixel-accepted：是。
- 当前状态：`pixel-accepted`。
- 深拆完整性：已覆盖区域坐标、文本、组件、图标、token、状态交互、实现映射和完整证据。
- 主线程判定：Windows Chrome 截图与目标图一致，diff mismatch ratio 为 `0.0`，overlay 区域覆盖与记录一致。

## 主线程补充核对

- 顶部标题、动作 rail、门禁表、状态矩阵和底部语义卡片均已按 1920x1080 坐标系单独定位。
- 已逐项回看 implementation、diff 和 overlay 三张证据图，主线程确认没有文字遮挡、状态误配和区域偏移。
