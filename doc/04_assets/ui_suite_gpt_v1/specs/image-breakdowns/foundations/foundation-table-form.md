# foundation-table-form.png 逐图精拆记录

## 基本信息

- 分类：foundations
- 图片 ID：foundation-table-form
- 源图：`doc/04_assets/ui_suite_gpt_v1/screens/foundations/foundation-table-form.png`
- 源图尺寸：1920 x 1080
- 对应 prompt：`doc/04_assets/ui_suite_gpt_v1/prompts/foundation-table-form.prompt.txt`
- 对应 layer：`doc/04_assets/ui_suite_gpt_v1/specs/layers/foundation-table-form.json`
- 对应 manifest：`doc/04_assets/ui_suite_gpt_v1/manifest.json`
- 对应路由：无
- 宿主路由：无
- 页面性质：设计系统 foundation 规范板
- 当前阶段：`pixel-accepted`
- 拆解目的：锁定高密表格、筛选表单、按钮层级、危险动作和状态容器几何规则
- 坐标系统：所有 bbox 均基于 1920 x 1080 目标 PNG
- OCR 方式：人工视觉读取并用表格/表单/规则裁剪校正
- 辅助审查：Raman 已完成只读视觉拆解
- 证据目录：`evidence/ui-image-breakdowns/foundations/foundation-table-form/`
- 目标图证据：`evidence/ui-image-breakdowns/foundations/foundation-table-form/target.png`
- 实现截图证据：`evidence/ui-image-breakdowns/foundations/foundation-table-form/implementation.png`
- 视觉差异证据：`evidence/ui-image-breakdowns/foundations/foundation-table-form/diff.png`
- 区域覆盖证据：`evidence/ui-image-breakdowns/foundations/foundation-table-form/regions-overlay.png`
- 验收证据：`evidence/ui-image-breakdowns/foundations/foundation-table-form/verification.json`
- 生产边界：本图是 table/form density token 和交互语义来源，不是独立业务页面

## 目标图观察

- 整体是深色安全运营台风格的表格与表单密度规范板。
- 顶部标题区高度约 73px。
- 主标题为 `Foundation Table And Form Density`。
- 副标题为 `用 screen.png token 构造高密表格、筛选和表单状态规范`。
- 右上角显示 `第一基准：screen.png`。
- 主体上方分为左右两块面板。
- 左侧是 `01 高密表格模式`。
- 右侧是 `02 筛选与表单控件`。
- 底部是整宽 `03 密度规则` 面板。
- 左侧表格面板宽约 1237px，高约 549px。
- 表格不是卡片列表，而是一个高密度数据表。
- 表格包含 9 列：ID、对象、风险、来源、状态、证据、时间窗、动作、审计。
- 表格包含 4 行数据。
- 表格表头背景略深，列之间有细竖线。
- 表格行之间用低透明青蓝线分隔。
- 风险字段具有明确颜色语义。
- 高危为红色。
- 中危为黄色。
- 低危和健康为绿色。
- 状态字段也使用绿色表达正常和通过。
- 证据字段中的 `Hash OK` 使用绿色，表达校验通过。
- 审计字段中的 `已记录` 使用绿色，表达闭环。
- 审计字段中的 `待补` 使用默认浅色，表达未闭环。
- 审计字段中的 `授权` 使用浅色，表达有权限授权。
- 表格动作列包含阻断、复核、查看、下载。
- 这些是行内动作，不应让行高扩张。
- 右侧表单面板是紧凑工具区，不是营销卡片。
- 表单包含 5 个字段：时间窗、资产/IP、风险等级、证据类型、审批原因。
- 每个字段左侧是标签，右侧是长输入框/选择框。
- 输入框高度约 40px，边框为青蓝线。
- 审批原因与提交审批、危险执行绑定。
- 按钮行包含重置、保存视图、提交审批、危险执行。
- 按钮层级从低到高依次为重置、保存视图、提交审批、危险执行。
- 重置按钮为弱灰蓝。
- 保存视图按钮为蓝色主操作。
- 提交审批按钮为黄色警示操作。
- 危险执行按钮为红色危险操作。
- 危险执行不能孤立出现，必须有审批原因、权限、影响范围和审计闭环。
- 底部密度规则面板包含 5 条绿色 bullet。
- 第一条锁定表格行高以 32px 为目标，hover 不导致扩张。
- 第二条要求筛选区横向紧凑，不做营销卡片。
- 第三条要求危险动作具备权限、影响范围和审计原因。
- 第四条要求空/加载/错误状态保持同样容器几何。
- 第五条禁止卡片套卡片，重复项才使用卡片承载。
- 本图重点不是业务数据本身，而是运营系统中的密度和动作语义。

## 区域与坐标

坐标为基于目标 PNG 直接视觉读取和局部裁剪校正后的人工测量，格式为 `x,y,w,h`。

| 区域 | bbox | 层级 | 说明 | 复刻要点 |
|---|---:|---:|---|---|
| 画布 | `0,0,1920,1080` | 0 | 16:9 foundation 规范板 | 保持 1920x1080 |
| 顶部标题区 | `0,0,1920,73` | 1 | 标题、副标题、基准说明 | y=72 分割线 |
| 表格面板 | `24,92,1237,549` | 1 | `01 高密表格模式` | 不套卡片 |
| 表格面板标题栏 | `25,93,1235,43` | 2 | section title | 43px 高度 |
| 高密表格 | `54,155,1176,455` | 2 | 9 列 4 行表格 | 稳定列宽 |
| 表头 | `54,155,1176,39` | 3 | 表格列名 | 深色 header |
| AL-0001 行 | `54,194,1176,58` | 3 | 高危阻断行 | 风险红、审计绿 |
| AL-0002 行 | `54,252,1176,58` | 3 | 中危复核行 | 风险黄、审计待补 |
| AS-1038 行 | `54,310,1176,58` | 3 | 低危查看行 | 低危/正常/已记录为绿 |
| EV-2891 行 | `54,368,1176,58` | 3 | 健康下载行 | 健康/通过/Hash OK 为绿 |
| 表单面板 | `1290,92,607,549` | 1 | `02 筛选与表单控件` | 紧凑工具面板 |
| 表单面板标题栏 | `1291,93,605,43` | 2 | section title | 与表格标题栏一致 |
| 表单字段组 | `1326,152,533,321` | 2 | 5 个 label/control 行 | 标签左、控件右 |
| 时间窗字段 | `1326,152,533,41` | 3 | 时间窗/近 24 小时 | 输入框高约 40px |
| 资产 IP 字段 | `1326,222,533,41` | 3 | 资产/IP 范围 | 值为 CIDR |
| 风险等级字段 | `1326,292,533,41` | 3 | 风险等级 | 高 / 中 / 低 |
| 证据类型字段 | `1326,362,533,41` | 3 | 证据类型 | PCAP / Session / 日志 |
| 审批原因字段 | `1326,432,533,41` | 3 | 危险操作上下文 | 审计原因必填语义 |
| 按钮组 | `1326,550,507,42` | 2 | 四个动作按钮 | 层级色固定 |
| 密度规则面板 | `24,670,1873,369` | 1 | `03 密度规则` | 底部整宽 |
| 密度规则标题栏 | `25,671,1871,43` | 2 | section title | 分割线 |
| 密度规则列表 | `59,738,620,221` | 2 | 五条 green bullet | 行距约 52px |

## 文本清单

| 序号 | 文本 | 位置 | 类型 | 复刻要求 |
|---:|---|---|---|---|
| 1 | Foundation Table And Form Density | 顶部标题区 | 主标题 | 必须完全一致 |
| 2 | 用 screen.png token 构造高密表格、筛选和表单状态规范 | 顶部标题区 | 副标题 | 必须完全一致 |
| 3 | 第一基准：screen.png | 顶部右侧 | 基准说明 | 必须完全一致 |
| 4 | 01 高密表格模式 | 表格面板标题 | 区块标题 | 必须完全一致 |
| 5 | ID | 表头 | 列名 | 必须完全一致 |
| 6 | 对象 | 表头 | 列名 | 必须完全一致 |
| 7 | 风险 | 表头 | 列名 | 必须完全一致 |
| 8 | 来源 | 表头 | 列名 | 必须完全一致 |
| 9 | 状态 | 表头 | 列名 | 必须完全一致 |
| 10 | 证据 | 表头 | 列名 | 必须完全一致 |
| 11 | 时间窗 | 表头 | 列名 | 必须完全一致 |
| 12 | 动作 | 表头 | 列名 | 必须完全一致 |
| 13 | 审计 | 表头 | 列名 | 必须完全一致 |
| 14 | AL-0001 / 实验区-核心区 / 高危 / 规则+模型 / 处理中 / PCAP 1 / 24h / 阻断 / 已记录 | 表格行 1 | 行数据 | 必须完全一致 |
| 15 | AL-0002 / 宿舍区-汇聚B / 中危 / 行为基准 / 待确认 / Session 2 / 24h / 复核 / 待补 | 表格行 2 | 行数据 | 必须完全一致 |
| 16 | AS-1038 / 核心交换机 / 低危 / 资产台账 / 正常 / 日志 4 / 7d / 查看 / 已记录 | 表格行 3 | 行数据 | 必须完全一致 |
| 17 | EV-2891 / 对象存储 / 健康 / 取证分析 / 通过 / Hash OK / 30d / 下载 / 授权 | 表格行 4 | 行数据 | 必须完全一致 |
| 18 | 02 筛选与表单控件 | 表单面板标题 | 区块标题 | 必须完全一致 |
| 19 | 时间窗 | 表单字段 | label | 必须完全一致 |
| 20 | 近 24 小时 | 表单字段 | value | 必须完全一致 |
| 21 | 资产/IP | 表单字段 | label | 必须完全一致 |
| 22 | 10.12.0.0/16 | 表单字段 | value | 必须完全一致 |
| 23 | 风险等级 | 表单字段 | label | 必须完全一致 |
| 24 | 高 / 中 / 低 | 表单字段 | value | 必须完全一致 |
| 25 | 证据类型 | 表单字段 | label | 必须完全一致 |
| 26 | PCAP / Session / 日志 | 表单字段 | value | 必须完全一致 |
| 27 | 审批原因 | 表单字段 | label | 必须完全一致 |
| 28 | 用于取证闭环复核 | 表单字段 | value | 必须完全一致 |
| 29 | 重置 | 按钮组 | button | 必须完全一致 |
| 30 | 保存视图 | 按钮组 | button | 必须完全一致 |
| 31 | 提交审批 | 按钮组 | button | 必须完全一致 |
| 32 | 危险执行 | 按钮组 | button | 必须完全一致 |
| 33 | 03 密度规则 | 底部面板标题 | 区块标题 | 必须完全一致 |
| 34 | 表格行高以 32px 为目标，hover 不导致扩张 | 规则 1 | density rule | 必须完全一致 |
| 35 | 筛选区尽量横向紧凑，不做营销卡片 | 规则 2 | density rule | 必须完全一致 |
| 36 | 危险动作必须具备权限、影响范围和审计原因 | 规则 3 | density rule | 必须完全一致 |
| 37 | 空/加载/错误状态保持同样容器几何 | 规则 4 | density rule | 必须完全一致 |
| 38 | 禁止卡片套卡片，重复项才使用卡片承载 | 规则 5 | density rule | 必须完全一致 |

## 组件清单

| 组件 | bbox | 类型 | 说明 | 前端映射 |
|---|---:|---|---|---|
| FoundationBoardHeader | `0,0,1920,73` | layout header | 表格表单密度规范标题 | Static header |
| DenseTablePanel | `24,92,1237,549` | foundation panel | 高密表格模式 | FoundationPanel |
| DenseDataTable | `54,155,1176,455` | data table | 9 列 4 行表格 | Ant Design Table compact |
| DenseTableHeader | `54,155,1176,39` | table header | 固定表头 | Table header |
| DenseTableRows | `54,194,1176,232` | table body | 4 行数据 | Table body |
| FilterFormPanel | `1290,92,607,549` | foundation panel | 筛选表单控件 | FoundationPanel |
| FilterForm | `1326,152,533,321` | form | 5 个字段 | Ant Design Form |
| FilterInputControls | `1430,152,429,321` | input group | 5 个长控件 | Input/Select |
| ActionButtonRow | `1326,550,507,42` | button row | 四级按钮层级 | Button group |
| DensityRulesPanel | `24,670,1873,369` | foundation panel | 密度规则 | FoundationPanel |
| DensityRuleList | `59,738,620,221` | bullet list | 五条规则 | RuleList |
| RiskSemanticText | `344,214,45,190` | status text set | 风险状态色 | StatusText |

## 图标清单

| 图标 | bbox | 状态 | 语义 | 复刻要点 |
|---|---:|---|---|---|
| 密度规则绿点 1 | `60,740,12,12` | success | rule-bullet | 规则列表 |
| 密度规则绿点 2 | `60,792,12,12` | success | rule-bullet | 规则列表 |
| 密度规则绿点 3 | `60,844,12,12` | success | rule-bullet | 规则列表 |
| 密度规则绿点 4 | `60,896,12,12` | success | rule-bullet | 规则列表 |
| 密度规则绿点 5 | `60,948,12,12` | success | rule-bullet | 规则列表 |
| 高危风险文字标记 | `344,214,30,16` | danger | high-risk | 红色状态文字 |

## Token 与样式

| Token | 值 | 用途 | 约束 |
|---|---|---|---|
| table-row-target-height | `32px` | 表格目标行高 | hover 不扩张 |
| table-hover-height | `no expansion` | hover 状态 | 不改变容器几何 |
| form-control-height | `40px` | 表单控件高度 | 紧凑 |
| button-height | `40px` | 按钮高度 | 层级一致 |
| panel-radius | `6px` | 面板圆角 | foundation 标准 |
| button-radius | `4px` | 按钮圆角 | 紧凑按钮 |
| danger-red | `#ff4d4f` | 高危/危险执行 | 危险语义 |
| warning-yellow | `#ffb020` | 中危/提交审批 | 警示语义 |
| success-green | `#36d66b` | 低危/健康/通过/已记录 | 成功语义 |
| active-blue | `#1e9cff` | 保存视图/强调 | 信息或主操作 |
| panel-bg | `#071f32` | 面板底色 | 深色低饱和 |
| border-weak | `rgba(56,151,201,.22)` | 边框/分割线 | 细线 |
| main-text | `#eaf7ff` | 主文字 | 标题/正文 |
| secondary-text | `#9db9c9` | 标签/弱按钮 | 次级文字 |

## 状态与交互

- 表格行高以 32px 为目标。
- 表格 hover 不得导致行高扩张。
- 表格列宽应稳定，不因状态文字变化导致左右抖动。
- 风险颜色语义必须固定。
- 高危使用红色。
- 中危使用黄色。
- 低危、健康、正常、通过使用绿色。
- 证据字段可以出现 `PCAP 1`、`Session 2`、`日志 4`、`Hash OK` 等值。
- `Hash OK` 是证据完整性通过语义，应使用绿色。
- 表格动作列为单行行内动作。
- `阻断` 是危险类动作，需要确认和审计。
- `复核` 是审核类动作。
- `查看` 是只读动作。
- `下载` 是证据导出动作，需按权限控制。
- 审计列是闭环状态，不是普通备注。
- `已记录` 表示审计闭环完成。
- `待补` 表示审计材料不完整。
- `授权` 表示权限授权存在。
- 筛选表单用于限定表格数据范围。
- 时间窗控制时间范围。
- 资产/IP 控制资产范围。
- 风险等级控制状态过滤。
- 证据类型控制证据来源。
- 审批原因支撑危险动作和审批链。
- 按钮层级从弱到强为重置、保存视图、提交审批、危险执行。
- 危险执行必须具备权限、影响范围和审计原因。
- 空状态、加载状态、错误状态必须保持同样容器几何。
- 禁止卡片套卡片。
- 重复项才使用卡片承载。

## 实现映射

- `DenseDataTable` 映射 Ant Design Table 的 compact density。
- `DenseTableHeader` 使用固定表头、细分割线和稳定列宽。
- `DenseTableRows` 使用稳定行高和固定 hover 几何。
- `RiskSemanticText` 映射到统一 StatusText/Tag 语义。
- `FilterForm` 映射到 Ant Design Form，采用 label/control 横向结构。
- `FilterInputControls` 映射到 Input、Select、DateRange、CidrInput 等控件。
- `ActionButtonRow` 映射到按钮组，保持层级颜色。
- `DangerActionButton` 必须走确认、权限、影响范围、审计链。
- `DensityRuleList` 可作为文档/规范组件。
- 表格和表单在面板内，但表格行不能再包装成卡片。
- 表单控件不能放大成营销卡片。
- 空/加载/错误状态应使用同一容器宽高。
- Loading skeleton 不应改变表格列宽。
- Error block 不应撑开表单面板。
- Empty table 不应移除表头和容器。

## 验收证据

- 源图：`doc/04_assets/ui_suite_gpt_v1/screens/foundations/foundation-table-form.png`
- target：`evidence/ui-image-breakdowns/foundations/foundation-table-form/target.png`
- implementation：`evidence/ui-image-breakdowns/foundations/foundation-table-form/implementation.png`
- diff：`evidence/ui-image-breakdowns/foundations/foundation-table-form/diff.png`
- regions overlay：`evidence/ui-image-breakdowns/foundations/foundation-table-form/regions-overlay.png`
- measurement：`evidence/ui-image-breakdowns/foundations/foundation-table-form/measurement.json`
- text ledger：`evidence/ui-image-breakdowns/foundations/foundation-table-form/text-ocr.txt`
- metrics：`evidence/ui-image-breakdowns/foundations/foundation-table-form/metrics.json`
- capture metadata：`evidence/ui-image-breakdowns/foundations/foundation-table-form/capture-meta.json`
- verification：`evidence/ui-image-breakdowns/foundations/foundation-table-form/verification.json`
- 浏览器要求：Windows Chrome CDP `http://127.0.0.1:9224`
- 截图要求：1920 x 1080，DPR 1
- diff 要求：对 target 与 implementation 生成 `diff.png`
- 审查要求：辅助智能体检查证据，主线程最终判断
- 证据边界：reference-raster 证明目标 PNG 复刻，生产表格/表单语义仍需按本记录落地

## 差异清单

| 类型 | 位置 | 当前记录 | 验收要求 | 状态 |
|---|---|---|---|---|
| 视觉截图 | 全图 | Windows Chrome 截图由像素门禁生成 | 必须有 implementation.png | closed |
| 视觉差异 | 全图 | diff 由像素门禁生成 | mismatch ratio 达到门禁 | closed |
| 区域覆盖 | 全图 | JSON 已记录 22 个区域 | overlay 覆盖表格、表单、按钮、规则 | closed |
| 文本校正 | 全图 | 已人工校正 38 条文本 | text ledger 同步记录 | closed |
| 表格语义 | 表格面板 | 9 列 4 行和状态色已记录 | 高密稳定列宽 | closed |
| 表单语义 | 表单面板 | 5 字段和 4 按钮已记录 | 危险动作绑定审批原因 | closed |
| 生产边界 | React/Ant 实现 | reference-raster 只证明像素复刻 | 语义实现按本记录落地 | documented |

## 结论

- `foundation-table-form.png` 是表格与表单密度规范板。
- 它锁定高密度表格模式、紧凑筛选表单和按钮动作层级。
- 它明确表格行高以 32px 为目标，hover 不导致扩张。
- 它明确筛选区保持横向紧凑，不做营销卡片。
- 它明确危险动作必须具备权限、影响范围和审计原因。
- 它明确空、加载、错误状态保持同样容器几何。
- 它明确禁止卡片套卡片，重复项才使用卡片承载。
- 本记录已完成区域、文本、组件、图标、token、交互和实现映射拆解。
- Windows Chrome 截图、视觉 diff、辅助审查和主线程判定已经完成。
