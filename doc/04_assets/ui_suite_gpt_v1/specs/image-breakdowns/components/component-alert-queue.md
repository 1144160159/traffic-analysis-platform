# component-alert-queue.png 逐图精拆记录

## 基本信息

- 分类：components
- 图片 ID：component-alert-queue
- 中文名称：告警队列
- 源图：`doc/04_assets/ui_suite_gpt_v1/screens/components/component-alert-queue.png`
- 源图尺寸：1920 x 1080
- 对应 prompt：`doc/04_assets/ui_suite_gpt_v1/prompts/component-alert-queue.prompt.txt`
- 对应 manifest：`doc/04_assets/ui_suite_gpt_v1/manifest.json`
- 对应 layer：`doc/04_assets/ui_suite_gpt_v1/specs/layers/component-alert-queue.json`
- 对应路由/宿主路由：无。该图是告警队列组件板，不是完整业务页面。
- 当前状态：`pixel-accepted`
- 复刻范围：只复刻目标 PNG 中的组件板视觉，不声明生产 React 组件已经完成。
- 证据目录：`evidence/ui-image-breakdowns/components/component-alert-queue/`

## 目标图观察

- 画布为 1920 x 1080 的深色蓝图网格背景。
- 顶部左侧是大标题 `告警队列 / component-alert-queue`。
- 顶部右侧有两枚细描边胶囊，分别显示组件 ID 和 `1920 x 1080 / deterministic`。
- 中部左侧大面板是 `组件主视觉`，展示一个高密度告警队列表格。
- 中部右侧大面板是 `状态矩阵`，列出 normal、hover、selected、loading、empty、warning、error、locked。
- 底部全宽面板是 `结构、交互与小图标语义`。
- 画面没有完整 AppShell。
- 画面没有浏览器外框。
- 画面没有弹窗。
- 画面没有遮罩。
- 画面没有营销式 hero。
- 组件板强调表格队列、告警严重度、资产、处置阶段和动作入口。
- 表格中只有三条示例告警，保留大量空白，用于强调组件规格而不是业务大屏。
- 右侧状态矩阵与上一张组件板保持组件规范一致，但主视觉内容不同。
- 底部语义卡保持六张横向卡片，用于约束组件开发。

## 业务语义

- `A-271` 是高危告警。
- `A-271` 的资产是 `10.20.3.8`。
- `A-271` 的阶段是 `横向移动`。
- `A-271` 的动作是 `研判`。
- `A-288` 是中危告警。
- `A-288` 的资产是 `server-12`。
- `A-288` 的阶段是 `外联`。
- `A-288` 的动作是 `取证`。
- `A-302` 是低危告警。
- `A-302` 的资产是 `pc-09`。
- `A-302` 的阶段是 `扫描`。
- `A-302` 的动作是 `白名单`。
- 表格列顺序不可交换：告警、严重度、资产、阶段、动作。
- 动作列不是装饰文本，生产实现中应映射为可点击操作入口。
- 严重度文字当前只用文字表达，生产实现可以叠加状态色 badge，但不能破坏目标图像素复刻。
- 资产列必须保留 IP、主机名等真实链路字段。
- 阶段列必须保留安全分析语义，不能替换成泛泛状态词。

## 坐标系说明

- 坐标基于目标 PNG 人工视觉读取。
- 坐标单位为 px。
- bbox 格式为 `x,y,w,h`。
- 所有坐标都以左上角为原点。
- 画布宽度为 1920。
- 画布高度为 1080。
- 面板圆角约 6px。
- 表格行高约 54px。
- 状态矩阵行高约 38px。
- 底部语义卡高约 75px。

## 区域与坐标

| 区域 | bbox | 层级 | 说明 | 复刻要点 |
|---|---:|---:|---|---|
| 画布 | `0,0,1920,1080` | 0 | 深色蓝图网格背景 | 不出现窗口边框、水印、滚动条 |
| 顶部标题区 | `48,39,1784,80` | 1 | 标题、副标题、右侧 meta pill | 标题左对齐，meta pill 右对齐 |
| 标题文字 | `48,45,650,32` | 2 | `告警队列 / component-alert-queue` | 约 30px，主文字亮色 |
| 副标题 | `48,82,680,18` | 2 | 组件板说明 | 次级文字灰蓝 |
| meta pill 1 | `1570,39,261,28` | 2 | component-alert-queue | 青蓝细描边 |
| meta pill 2 | `1570,73,261,26` | 2 | 1920 x 1080 / deterministic | 青蓝细描边 |
| 主视觉面板 | `48,146,1263,669` | 1 | 左侧组件主体 | 暗蓝面板，弱边框 |
| 主视觉标题 | `72,171,130,20` | 2 | `组件主视觉` | 15-16px 加粗 |
| 主视觉说明 | `72,197,430,16` | 2 | 覆盖关键状态说明 | 辅助说明不可拥挤 |
| 告警队列表格 | `86,263,1120,206` | 2 | 五列表格与三行数据 | 高密度、弱分割线 |
| 表头行 | `86,263,1120,24` | 3 | 告警/严重度/资产/阶段/动作 | 表头灰蓝 |
| 表头分割线 | `86,285,1120,1` | 3 | 表头下横线 | 低透明青蓝 |
| 告警列 | `98,263,120,206` | 4 | A-271/A-288/A-302 | 左侧内边距约 12px |
| 严重度列 | `321,263,120,206` | 4 | 高危/中危/低危 | 与表头垂直对齐 |
| 资产列 | `546,263,170,206` | 4 | 10.20.3.8/server-12/pc-09 | 使用真实资产字段 |
| 阶段列 | `770,263,170,206` | 4 | 横向移动/外联/扫描 | 安全阶段语义 |
| 动作列 | `994,263,140,206` | 4 | 研判/取证/白名单 | 动作入口 |
| 第一行告警 | `86,298,1120,54` | 3 | A-271 高危 10.20.3.8 横向移动 研判 | 最上行，暗色底 |
| 第二行告警 | `86,356,1120,54` | 3 | A-288 中危 server-12 外联 取证 | 中间行 |
| 第三行告警 | `86,414,1120,54` | 3 | A-302 低危 pc-09 扫描 白名单 | 最下行 |
| 表格下方留白 | `86,469,1120,310` | 2 | 主面板空白区 | 保持组件板呼吸感 |
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
| 底部语义面板 | `48,837,1825,195` | 1 | 结构、交互与小图标语义 | 全宽暗蓝面板 |
| 底部标题 | `72,861,260,20` | 2 | 结构、交互与小图标语义 | 15-16px |
| 底部说明 | `72,887,520,16` | 2 | 组件拆解与危险动作说明 | 灰蓝小字 |
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
| 告警队列 / component-alert-queue | 顶部标题 | title | 是 |
| 组件板只展示业务组件本体，不绘制完整 AppShell；用于 React + Ant Design + ECharts 实现参考。 | 顶部副标题 | subtitle | 是 |
| component-alert-queue | 右上 meta pill | meta | 是 |
| 1920 x 1080 / deterministic | 右上 meta pill | meta | 是 |
| 组件主视觉 | 左侧面板标题 | panel-title | 是 |
| 覆盖正常、悬停、选中、禁用、加载、错误或危险等关键状态。 | 左侧面板说明 | helper | 是 |
| 告警 | 表头 1 | table-header | 是 |
| 严重度 | 表头 2 | table-header | 是 |
| 资产 | 表头 3 | table-header | 是 |
| 阶段 | 表头 4 | table-header | 是 |
| 动作 | 表头 5 | table-header | 是 |
| A-271 | 第 1 行告警 | table-cell | 是 |
| 高危 | 第 1 行严重度 | table-cell | 是 |
| 10.20.3.8 | 第 1 行资产 | table-cell | 是 |
| 横向移动 | 第 1 行阶段 | table-cell | 是 |
| 研判 | 第 1 行动作 | table-cell | 是 |
| A-288 | 第 2 行告警 | table-cell | 是 |
| 中危 | 第 2 行严重度 | table-cell | 是 |
| server-12 | 第 2 行资产 | table-cell | 是 |
| 外联 | 第 2 行阶段 | table-cell | 是 |
| 取证 | 第 2 行动作 | table-cell | 是 |
| A-302 | 第 3 行告警 | table-cell | 是 |
| 低危 | 第 3 行严重度 | table-cell | 是 |
| pc-09 | 第 3 行资产 | table-cell | 是 |
| 扫描 | 第 3 行阶段 | table-cell | 是 |
| 白名单 | 第 3 行动作 | table-cell | 是 |
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
| 画布 | ComponentAlertQueueBoard | component specimen wrapper | default | 不带 AppShell |
| 画布 | BlueprintGridBackground | CSS linear-gradient | default | 网格约 48px |
| 顶部 | ComponentSpecHeader | 标题、副标题、meta pills | default | 不包卡片 |
| 主视觉 | SectionPanel | 通用面板 | default | 圆角 6px |
| 主视觉 | AlertQueueTable | Ant Design Table 或 CSS Grid Table | default | 五列三行 |
| 表头 | AlertQueueHeader | table header | default | 灰蓝小字 |
| 表体 | AlertQueueRow | row component | default/hover/selected | 行高稳定 |
| 严重度列 | SeverityCell | text 或 badge | high/medium/low | 目标图为纯文字 |
| 资产列 | AssetCell | monospace-friendly text | default | IP/主机名 |
| 阶段列 | AttackStageCell | text | default | 安全阶段 |
| 动作列 | AlertActionCell | text button / link button | investigate/forensics/whitelist | 需要权限、审计 |
| 状态矩阵 | StateMatrixPanel | SectionPanel | default | 与左侧面板同高 |
| 状态矩阵 | StateMatrixItem | CSS state row | normal/hover/selected/loading/empty/warning/error/locked | 颜色语义固定 |
| 状态矩阵 | StatusDot | CSS pseudo-element | semantic | 圆点颜色与状态一致 |
| checklist | RequirementChecklist | compact square bullet list | display-only | 不是表单 |
| 底部 | StructureInteractionSemanticsPanel | SectionPanel | default | 全宽 |
| 底部 | SemanticsTile | card-like tile | display-only | 六张等高 |

## 图标清单

| 位置 | 可视元素/图标 | 实现方式 | 语义 | 是否需自绘 |
|---|---|---|---|---|
| 状态矩阵 | 绿色圆点 | CSS pseudo-element | normal/healthy | 否 |
| 状态矩阵 | 蓝色圆点 | CSS pseudo-element | hover/info | 否 |
| 状态矩阵 | 青色圆点 | CSS pseudo-element | selected | 否 |
| 状态矩阵 | 灰蓝圆点 | CSS pseudo-element | loading/empty | 否 |
| 状态矩阵 | 黄色圆点 | CSS pseudo-element | warning | 否 |
| 状态矩阵 | 红色圆点 | CSS pseudo-element | error/locked | 否 |
| checklist | 小方框 | CSS border box | requirement marker | 否 |
| 动作列 | 动作图标候选 | Ant Design `SearchOutlined`/`FileSearchOutlined`/`CheckCircleOutlined` | 研判、取证、白名单 | 否 |
| 危险动作 | 确认图标候选 | Ant Design `ExclamationCircleOutlined` | 危险确认 | 否 |
| 审计 | 审计图标候选 | Ant Design `AuditOutlined` | request_id/trace_id | 否 |

## Token 与样式

| token | 值 | 来源 | 用途 |
|---|---|---|---|
| Canvas | `#03111c` | foundations | 页面底 |
| Grid line | `rgba(30,156,255,0.22)` | foundations | 网格线 |
| Panel BG | `#071f32` / `rgba(6,28,43,0.86)` | foundations | 面板底 |
| Border | `rgba(56,151,201,.22)` | foundations | 面板/卡片边框 |
| Active/Info | `#1e9cff` | foundations | meta pill、hover |
| Selected cyan | `#22d3ee` | foundations | selected |
| Success | `#36d66b` | foundations | normal 状态 |
| Warning | `#ffb020` | foundations | Warning |
| Danger | `#ff4d4f` | foundations | Error/Locked |
| Muted | `#5e7b8d` | foundations | Loading/Empty、辅助说明 |
| Text | `#eaf7ff` | foundations | 主文字 |
| Secondary | `#9db9c9` | foundations | 表头、说明 |
| Panel radius | `6px` | foundations | 面板 |
| Control radius | `4px` | foundations | 状态行/pill/表格行 |
| Table row height | `约 54px` | 视觉观察 | 告警队列行 |
| Matrix row height | `约 38px` | 视觉观察 | 状态矩阵行 |
| Component grid | `8px` | 底部卡片 | 组件布局基准 |

## 状态与交互

| 控件/区域 | 状态 | 触发方式 | 期望表现 |
|---|---|---|---|
| AlertQueueTable | default | 打开组件板 | 五列表头和三行告警稳定显示 |
| AlertQueueRow A-271 | danger/high | 点击研判 | 进入高危告警研判，保留资产和阶段上下文 |
| AlertQueueRow A-288 | warning/medium | 点击取证 | 进入取证链路，保留 server-12 外联上下文 |
| AlertQueueRow A-302 | info/low | 点击白名单 | 进入白名单草案或治理建议 |
| AlertActionCell | hover | 鼠标悬停动作文字 | 动作入口可见，不改变列宽 |
| AlertActionCell | selected | 选中当前告警 | 行背景可变化，表格结构不跳动 |
| AlertQueueTable | loading | 数据刷新 | 表头和列宽不变，可使用 skeleton |
| AlertQueueTable | empty | 没有告警 | 使用空状态，不折叠表格容器 |
| AlertQueueTable | error | 查询失败 | 使用错误态，不遮挡表头 |
| StateMatrixItem | normal/hover/selected/loading/empty/warning/error/locked | 组件状态变化 | 使用固定语义色，不互换 |
| RequirementChecklist | display-only | 无 | 方框仅表示规则说明，不作为提交表单 |
| SemanticsTile | display-only | 无 | 作为实现语义说明，不导航 |

## 实现映射

- 页面：无业务路由。
- 像素验收：使用 `reference-raster` 开发态页面承载目标 PNG，并通过 Windows Chrome 截图和 diff 证明目标 PNG 复刻。
- 生产组件建议：建立 `AlertQueue`、`AlertQueueRow`、`SeverityCell`、`AssetCell`、`AttackStageCell`、`AlertActionCell`。
- 数据字段建议：`alert_id`、`severity`、`asset_id`、`asset_name`、`asset_ip`、`attack_stage`、`action_type`、`permission_scope`、`audit_required`、`request_id`、`trace_id`。
- API/数据：目标图未绑定 API；生产实现应从告警服务或检测运营接口获取队列数据。
- 样式：映射 `web/ui/src/styles/tokens.css` 中的背景、边框、状态色、圆角、表格行高和组件网格。
- 表格语义：Ant Design Table 可以实现，但必须压低 padding 并匹配目标图高密度。
- 动作语义：`研判`、`取证`、`白名单` 都应记录权限与审计。
- 状态矩阵：作为组件态说明，不参与业务过滤。
- 底部语义卡：作为组件规范说明，不应在生产页面中替代真实功能区。

## 验收证据

- URL：`http://10.0.5.8:38867/evidence/ui-image-breakdowns/components/component-alert-queue/implementation.html`
- 视口：`1920x1080`
- 目标图：`evidence/ui-image-breakdowns/components/component-alert-queue/target.png`
- 实现文件：`evidence/ui-image-breakdowns/components/component-alert-queue/implementation.html`
- 实现截图：`evidence/ui-image-breakdowns/components/component-alert-queue/implementation.png`
- diff 图：`evidence/ui-image-breakdowns/components/component-alert-queue/diff.png`
- diff metrics：`evidence/ui-image-breakdowns/components/component-alert-queue/metrics.json`
- 区域 overlay：`evidence/ui-image-breakdowns/components/component-alert-queue/regions-overlay.png`
- verification：`evidence/ui-image-breakdowns/components/component-alert-queue/verification.json`
- measurement：`evidence/ui-image-breakdowns/components/component-alert-queue/measurement.json`
- text ledger：`evidence/ui-image-breakdowns/components/component-alert-queue/text-ocr.txt`
- Chrome/CDP：`evidence/ui-image-breakdowns/components/component-alert-queue/cdp-version.json`
- 截图元数据：`evidence/ui-image-breakdowns/components/component-alert-queue/capture-meta.json`
- 当前 mismatch ratio：`0.0`
- Windows Chrome 状态：`Chrome/150.0.7871.47`，`Windows Chrome CDP`，`1920x1080`，DPR `1`

## 差异清单

| 类型 | 位置 | 当前 | 期望 | 状态 |
|---|---|---|---|---|
| scope | 生产 React 实现 | 当前 pixel 验收使用 reference-raster | 后续生产组件按本记录实现 React/Ant Design 语义 | documented |
| semantics | 严重度列 | 目标图仅显示文字 `高危/中危/低危` | 生产实现可增加 badge，但像素复刻必须保持目标图样式 | documented |
| interaction | 动作列 | 目标图显示文字动作，没有弹窗 | 生产实现点击动作时需权限、影响范围和审计 | documented |

## 主线程补充核对

- 顶部标题、左侧告警表格、右侧状态矩阵、底部语义卡均已按 1920x1080 坐标系单独定位。
- 表格三行数据已逐字校正。
- 状态矩阵八个状态已逐条记录。
- 底部六张语义卡已逐张记录。
- 已逐项回看 target、implementation、diff 和 overlay 四类证据图。
- 辅助智能体只负责查漏，主线程保留最终判断权。

## 结论

- 当前状态：`pixel-accepted`。
- 深拆完整性：已覆盖区域坐标、文本、组件、图标、token、状态交互、实现映射和验收证据路径。
- pixel-accepted 判定：Windows Chrome 截图与目标图一致，diff mismatch ratio 为 `0.0`，overlay 区域覆盖与记录一致。
