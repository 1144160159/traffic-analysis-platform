# component-batch-action-bar.png 逐图精拆记录

## 基本信息

- 分类：components
- 图片 ID：component-batch-action-bar
- 中文名称：批量操作栏
- 源图：`doc/04_assets/ui_suite_gpt_v1/screens/components/component-batch-action-bar.png`
- 源图尺寸：1920 x 1080
- 对应 prompt：`doc/04_assets/ui_suite_gpt_v1/prompts/component-batch-action-bar.prompt.txt`
- 对应 manifest：`doc/04_assets/ui_suite_gpt_v1/manifest.json`
- 对应 layer：`doc/04_assets/ui_suite_gpt_v1/specs/layers/component-batch-action-bar.json`
- 对应路由/宿主路由：无。该图是批量操作栏基础组件板，不是完整业务页面。
- 当前状态：`pixel-accepted`
- 复刻范围：只复刻目标 PNG 中的组件板视觉，不声明生产 React 组件已经完成。
- 证据目录：`evidence/ui-image-breakdowns/components/component-batch-action-bar/`

## 目标图观察

- 画布为 1920 x 1080 深色蓝图网格背景。
- 顶部左侧标题为 `component-batch-action-bar / 批量操作栏组件规范`。
- 顶部副标题说明该图是基础组件板，不绘制完整 AppShell。
- 顶部右侧有一枚系统标签 `batch action bar system`。
- 主体不是普通页面，而是 2 列 x 3 行的组件规范板。
- 左上区块是 `01 结构与触发边界`。
- 右上区块是 `02 动作分组与危险门禁`。
- 左中区块是 `03 选择范围与跨页全选`。
- 右中区块是 `04 权限、影响范围与审计`。
- 左下区块是 `05 状态矩阵`。
- 右下区块是 `06 React 映射与验收`。
- 六个区块均使用深色面板、青蓝描边和更深的区块标题条。
- 每个区块右上角有小型英文标签：anatomy、actions、selection、guardrail、states、react。
- 画面底部有一条分隔线和黄色说明文字。
- 画面没有完整 AppShell。
- 画面没有弹窗展开态。
- 画面没有浏览器地址栏。
- 画面没有滚动条。

## 业务语义

- BatchActionBar 只在存在选中项时出现。
- BatchActionBar 不承载筛选、搜索、分页、列设置或表格主体。
- 选中计数 `已选择 38 项` 是批量操作的核心上下文。
- `当前页` 表示当前操作范围只覆盖当前页已加载行。
- `含 3 项受限` 表示当前选中项里存在权限、状态或约束限制。
- 安全动作包括 `确认告警`、`转 SOAR`、`导出证据` 等。
- `更多` 是二级动作入口，但目标图不展开菜单。
- `清空` 是清除选择上下文，不是删除数据。
- 动作分组覆盖告警处置、资产治理、规则模型和高危操作。
- 高危操作包含 `删除`、`回滚`、`吊销令牌`，必须进入确认或审批流程。
- 跨页全选必须展示筛选条件快照和排除项。
- 全部告警规模为 `128,420 项`，目标图提示应进入后台任务和审计任务。
- 权限审计区强调影响对象、受限对象、审批角色、回滚方式、审计载荷和执行状态。
- React 映射区给出组件 API 和验收条件。
- 生产实现必须具备 selectedRowKeys、selectionScope、actions、preserveSelectedRowKeys 和 confirmAction 语义。

## 坐标系说明

- 坐标基于目标 PNG 直接视觉读取。
- 坐标单位为 px。
- bbox 格式为 `x,y,w,h`。
- 所有坐标都以左上角为原点。
- 画布宽度为 1920。
- 画布高度为 1080。
- 顶部标题区从 x=40、y=34 附近开始。
- 主体左列 x=40，右列 x=961。
- 主体顶行 y=92。
- 主体中行 y=386。
- 主体底行 y=738。
- 左列面板宽约 891px。
- 右列面板宽约 920px。
- 顶行面板高约 266px。
- 中行面板高约 321px。
- 底行面板高约 208px。
- 标题条高约 38px。
- 按钮高度约 31px。
- 选择范围行高约 37px。
- 状态卡高度约 44px。
- React 代码行高度约 27px。

## 区域与坐标

| 区域 | bbox | 层级 | 说明 | 复刻要点 |
|---|---:|---:|---|---|
| 画布 | `0,0,1920,1080` | 0 | 深色蓝图网格背景 | 不出现浏览器外框、水印、滚动条 |
| 顶部标题区 | `40,34,1800,58` | 1 | 标题、副标题、右侧系统标签 | 左标题右标签 |
| 标题文字 | `40,43,760,36` | 2 | `component-batch-action-bar / 批量操作栏组件规范` | 大号亮色 |
| 副标题 | `40,80,820,18` | 2 | 基础组件板说明 | 灰蓝小字 |
| 系统标签 | `1580,33,260,27` | 2 | batch action bar system | 青蓝描边 |
| 01 区块 | `40,92,891,266` | 1 | 结构与触发边界 | 左上 |
| 01 标题条 | `40,92,891,38` | 2 | 01 标题和 anatomy 标签 | 深青标题条 |
| 01 主操作栏 | `72,155,820,53` | 3 | BatchActionBar 本体 | checkbox、计数、pill、按钮 |
| 选择计数组 | `88,172,106,18` | 4 | 勾选框和已选择 38 项 | 绿色勾选 |
| 当前页 pill | `208,170,67,23` | 4 | 当前页 | 蓝色 pill |
| 受限 pill | `286,170,94,23` | 4 | 含 3 项受限 | 黄色 pill |
| 确认告警按钮 | `402,167,84,31` | 4 | 安全动作 | 绿色按钮 |
| 转 SOAR 按钮 | `494,167,76,31` | 4 | SOAR 动作 | 蓝色按钮 |
| 导出证据按钮 | `578,167,85,31` | 4 | 导出动作 | 蓝色按钮 |
| 更多按钮 | `670,167,59,31` | 4 | 二级菜单入口 | 蓝色按钮 |
| 清空按钮 | `736,167,59,31` | 4 | 清除选择 | 灰色按钮 |
| 01 分隔线 | `72,230,820,1` | 3 | 工具栏与说明分隔 | 细线 |
| 触发边界说明 | `72,252,640,18` | 3 | BatchActionBar 出现条件 | 绿色文字 |
| 浮动底栏选项 | `72,296,231,39` | 3 | 浮动底栏/表格底部吸附 | 蓝色描边 |
| 表头内联选项 | `342,296,231,39` | 3 | 表头内联/小数据列表 | 绿色描边 |
| 抽屉内栏选项 | `612,296,231,39` | 3 | 抽屉内栏/局部对象选择 | 黄色描边 |
| 02 区块 | `961,92,920,266` | 1 | 动作分组与危险门禁 | 右上 |
| 02 标题条 | `961,92,920,38` | 2 | 02 标题和 actions 标签 | 深青标题条 |
| 告警处置组 | `1000,150,330,31` | 3 | 批量确认/关闭/转 SOAR | 绿色按钮组 |
| 资产治理组 | `1000,197,330,31` | 3 | 批量打标/导出资产/加入观察 | 蓝色按钮组 |
| 规则模型组 | `1000,244,330,31` | 3 | 发布/停用/归档 | 黄色按钮组 |
| 高危操作组 | `1000,291,330,31` | 3 | 删除/回滚/吊销令牌 | 红色按钮组 |
| 危险门禁提示 | `1000,324,849,23` | 3 | Popconfirm/Modal/审批流提示 | 红色描边提示条 |
| 03 区块 | `40,386,891,321` | 1 | 选择范围与跨页全选 | 左中 |
| 03 标题条 | `40,386,891,38` | 2 | 03 标题和 selection 标签 | 深青标题条 |
| 选择范围矩阵 | `72,456,827,214` | 3 | 四行选择范围 | 外框青蓝 |
| 当前页行 | `76,478,797,37` | 4 | 当前页 38 项 | 绿色安全默认 |
| 筛选结果行 | `76,522,797,37` | 4 | 筛选结果 2,486 项 | 黄色需确认范围 |
| 全部告警行 | `76,566,797,37` | 4 | 全部告警 128,420 项 | 红色高成本 |
| 手动排除行 | `76,610,797,37` | 4 | 手动排除 5 项 | 青蓝可撤销 |
| 跨页说明 | `72,654,700,18` | 4 | 筛选条件快照和 task_id | 绿色说明 |
| 04 区块 | `961,386,920,321` | 1 | 权限、影响范围与审计 | 右中 |
| 04 标题条 | `961,386,920,38` | 2 | 04 标题和 guardrail 标签 | 深青标题条 |
| 影响对象卡 | `1000,456,381,47` | 3 | 2,486 告警/138 资产 | 黄色描边 |
| 受限对象卡 | `1420,456,381,47` | 3 | 12 项无权限 | 红色描边 |
| 审批角色卡 | `1000,522,381,47` | 3 | sec_lead 或值班主管 | 蓝色描边 |
| 回滚方式卡 | `1420,522,381,47` | 3 | 生成反向批处理任务 | 绿色描边 |
| 审计载荷卡 | `1000,588,381,47` | 3 | tenant/trace_id/selectedKeys | 蓝色描边 |
| 执行状态卡 | `1420,588,381,47` | 3 | 排队/运行/完成/失败 | 绿色描边 |
| 高影响提示 | `1000,659,849,23` | 3 | 审批、操作者、影响范围、回滚、trace_id | 黄色描边 |
| 05 区块 | `40,738,891,208` | 1 | 状态矩阵 | 左下 |
| 05 标题条 | `40,738,891,38` | 2 | 05 标题和 states 标签 | 深青标题条 |
| 默认状态卡 | `72,804,242,44` | 3 | 默认/38 项已选/执行 | 蓝色状态 |
| 悬停状态卡 | `344,804,242,44` | 3 | 悬停/按钮描边增强/执行 | 蓝色状态 |
| 加载状态卡 | `616,804,242,44` | 3 | 加载/提交中防重复/执行 | 蓝色 + spinner |
| 禁用状态卡 | `72,866,242,44` | 3 | 禁用/无可操作项/执行 | 灰色弱化 |
| 错误状态卡 | `344,866,242,44` | 3 | 错误/部分失败可重试/执行 | 红色 |
| 危险状态卡 | `616,866,242,44` | 3 | 危险/需要二次确认/执行 | 黄色边框 + 红按钮 |
| 06 区块 | `961,738,920,208` | 1 | React 映射与验收 | 右下 |
| 06 标题条 | `961,738,920,38` | 2 | 06 标题和 react 标签 | 深青标题条 |
| 代码行 1 | `1000,796,841,27` | 3 | BatchActionBar JSX | code strip |
| 代码行 2 | `1000,828,841,27` | 3 | Table rowSelection JSX | code strip |
| 代码行 3 | `1000,859,841,27` | 3 | confirmAction payload | code strip |
| 验收项 1 | `1000,900,370,25` | 3 | 不替代表格筛选/分页 | 绿色勾选 |
| 验收项 2 | `1420,900,372,25` | 3 | 危险动作进入确认流 | 绿色勾选 |
| 验收项 3 | `1000,932,370,25` | 3 | 跨页全选有范围快照 | 绿色勾选 |
| 验收项 4 | `1420,932,372,25` | 3 | 执行结果可追踪 | 绿色勾选 |
| 底部说明 | `40,990,1841,55` | 1 | 分隔线和黄色脚注 | 不作为 AppShell |

## 文本清单

| 文本 | 位置 | 类型 | 必须一致 |
|---|---|---|---|
| component-batch-action-bar / 批量操作栏组件规范 | 顶部标题 | title | 是 |
| 基础组件板：不绘制完整 AppShell；覆盖选中计数、跨页全选、危险动作、权限和审计门禁 | 顶部副标题 | subtitle | 是 |
| batch action bar system | 右上标签 | meta | 是 |
| 01 结构与触发边界 | 01 标题条 | section-title | 是 |
| anatomy | 01 标签 | section-tag | 是 |
| 已选择 38 项 | 01 操作栏 | toolbar-text | 是 |
| 当前页 | 01 操作栏 pill | pill | 是 |
| 含 3 项受限 | 01 操作栏 pill | pill | 是 |
| 确认告警 | 01 操作栏按钮 | button | 是 |
| 转 SOAR | 01 操作栏按钮 | button | 是 |
| 导出证据 | 01 操作栏按钮 | button | 是 |
| 更多 | 01 操作栏按钮 | button | 是 |
| 清空 | 01 操作栏按钮 | button | 是 |
| BatchActionBar 只在存在选中项时出现，不承载筛选、搜索、分页、列设置或表格主体。 | 01 说明 | rule | 是 |
| 浮动底栏 | 01 承载位置 | placement | 是 |
| 表格底部吸附 | 01 承载位置 | placement | 是 |
| 表头内联 | 01 承载位置 | placement | 是 |
| 小数据列表 | 01 承载位置 | placement | 是 |
| 抽屉内栏 | 01 承载位置 | placement | 是 |
| 局部对象选择 | 01 承载位置 | placement | 是 |
| 02 动作分组与危险门禁 | 02 标题条 | section-title | 是 |
| actions | 02 标签 | section-tag | 是 |
| 告警处置 | 02 分组 | group-label | 是 |
| 批量确认 | 02 按钮 | action-button | 是 |
| 批量关闭 | 02 按钮 | action-button | 是 |
| 转 SOAR | 02 按钮 | action-button | 是 |
| 资产治理 | 02 分组 | group-label | 是 |
| 批量打标 | 02 按钮 | action-button | 是 |
| 导出资产 | 02 按钮 | action-button | 是 |
| 加入观察 | 02 按钮 | action-button | 是 |
| 规则模型 | 02 分组 | group-label | 是 |
| 发布 | 02 按钮 | action-button | 是 |
| 停用 | 02 按钮 | action-button | 是 |
| 归档 | 02 按钮 | action-button | 是 |
| 高危操作 | 02 分组 | group-label | 是 |
| 删除 | 02 高危按钮 | danger-button | 是 |
| 回滚 | 02 高危按钮 | danger-button | 是 |
| 吊销令牌 | 02 高危按钮 | danger-button | 是 |
| 危险批量动作必须进入 Popconfirm / Modal / 审批流，不允许直接执行。 | 02 门禁提示 | danger-rule | 是 |
| 03 选择范围与跨页全选 | 03 标题条 | section-title | 是 |
| selection | 03 标签 | section-tag | 是 |
| 当前页 38 项 | 03 当前页行 | selection-scope | 是 |
| 只影响本页已加载行 | 03 当前页行 | selection-desc | 是 |
| 安全默认 | 03 当前页行 | selection-status | 是 |
| 筛选结果 2,486 项 | 03 筛选结果行 | selection-scope | 是 |
| 跨页全选，排除 12 项锁定 | 03 筛选结果行 | selection-desc | 是 |
| 需确认范围 | 03 筛选结果行 | selection-status | 是 |
| 全部告警 128,420 项 | 03 全部告警行 | selection-scope | 是 |
| 后台任务处理，生成审计任务 | 03 全部告警行 | selection-desc | 是 |
| 高成本 | 03 全部告警行 | selection-status | 是 |
| 手动排除 5 项 | 03 手动排除行 | selection-scope | 是 |
| 排除已归档和权限不足对象 | 03 手动排除行 | selection-desc | 是 |
| 可撤销 | 03 手动排除行 | selection-status | 是 |
| 跨页全选必须展示 “筛选条件快照” 和排除项；后台任务需要可追踪 task_id。 | 03 底部说明 | footnote | 是 |
| 04 权限、影响范围与审计 | 04 标题条 | section-title | 是 |
| guardrail | 04 标签 | section-tag | 是 |
| 影响对象 | 04 卡片 | guard-label | 是 |
| 2,486 告警 / 138 资产 | 04 卡片 | guard-value | 是 |
| 受限对象 | 04 卡片 | guard-label | 是 |
| 12 项无权限，已自动排除 | 04 卡片 | guard-value | 是 |
| 审批角色 | 04 卡片 | guard-label | 是 |
| sec_lead 或值班主管 | 04 卡片 | guard-value | 是 |
| 回滚方式 | 04 卡片 | guard-label | 是 |
| 生成反向批处理任务 | 04 卡片 | guard-value | 是 |
| 审计载荷 | 04 卡片 | guard-label | 是 |
| tenant / trace_id / selectedKeys | 04 卡片 | guard-value | 是 |
| 执行状态 | 04 卡片 | guard-label | 是 |
| 排队中 / 运行中 / 已完成 / 失败 | 04 卡片 | guard-value | 是 |
| 高影响批处理必须写入审批、操作者、影响范围、回滚和 trace_id。 | 04 底部提示 | warning | 是 |
| 05 状态矩阵 | 05 标题条 | section-title | 是 |
| states | 05 标签 | section-tag | 是 |
| 默认 | 05 状态卡 | state-label | 是 |
| 38 项已选 | 05 状态卡 | state-desc | 是 |
| 悬停 | 05 状态卡 | state-label | 是 |
| 按钮描边增强 | 05 状态卡 | state-desc | 是 |
| 加载 | 05 状态卡 | state-label | 是 |
| 提交中防重复 | 05 状态卡 | state-desc | 是 |
| 禁用 | 05 状态卡 | state-label | 是 |
| 无可操作项 | 05 状态卡 | state-desc | 是 |
| 错误 | 05 状态卡 | state-label | 是 |
| 部分失败可重试 | 05 状态卡 | state-desc | 是 |
| 危险 | 05 状态卡 | state-label | 是 |
| 需要二次确认 | 05 状态卡 | state-desc | 是 |
| 执行 | 05 状态按钮 | state-button | 是 |
| 06 React 映射与验收 | 06 标题条 | section-title | 是 |
| react | 06 标签 | section-tag | 是 |
| `<BatchActionBar selectedRowKeys={keys} scope={selectionScope} actions={actions} />` | 06 代码行 | code | 是 |
| `<Table rowSelection={{ selectedRowKeys, preserveSelectedRowKeys: true }} />` | 06 代码行 | code | 是 |
| `confirmAction({ impact, permission, auditPayload, rollbackPlan })` | 06 代码行 | code | 是 |
| 不替代表格筛选/分页 | 06 验收项 | acceptance-check | 是 |
| 危险动作进入确认流 | 06 验收项 | acceptance-check | 是 |
| 跨页全选有范围快照 | 06 验收项 | acceptance-check | 是 |
| 执行结果可追踪 | 06 验收项 | acceptance-check | 是 |
| 本图为批量操作栏基础组件板，公共 AppShell 仅作为 token 参考，不作为画面结构。 | 底部脚注 | footer | 是 |

## 组件清单

| 位置 | 组件/元素 | 前端实现建议 | 状态 | 备注 |
|---|---|---|---|---|
| 画布 | BatchActionBarSpecBoard | component specimen wrapper | default | 不带 AppShell |
| 画布 | BlueprintGridBackground | CSS linear-gradient | default | 网格背景 |
| 顶部 | ComponentSpecHeader | 标题、副标题、系统标签 | default | 右侧单标签 |
| 六个区块 | SpecSectionPanel | reusable panel | default | 标题条 + 内容区 |
| 01 | BatchActionBar | React component | selected | 选中时出现 |
| 01 | SelectionSummary | Checkbox + count | checked | 已选择 38 项 |
| 01 | ScopePill | pill | current-page | 当前页 |
| 01 | RestrictedPill | warning pill | warning | 含 3 项受限 |
| 01 | BatchActionButton | Ant Design Button | success/info/muted | 确认、SOAR、导出、更多、清空 |
| 01 | PlacementOption | option chip | info/success/warning | 三种承载位置 |
| 02 | ActionGroup | grouped buttons | success/info/warning/danger | 四组动作 |
| 02 | DangerActionGroup | danger buttons | danger | 删除、回滚、吊销令牌 |
| 02 | DangerGateMessage | Alert inline bar | danger | 强制确认/审批 |
| 03 | SelectionScopeMatrix | list panel | mixed | 当前页/筛选/全部/排除 |
| 03 | SelectionScopeRow | scope row | safe/warning/danger/info | 四行范围 |
| 04 | GuardrailCard | key-value card | warning/danger/info/success | 权限审计字段 |
| 04 | AuditPayloadCard | key-value card | info | tenant/trace_id/selectedKeys |
| 04 | GuardrailWarning | Alert inline bar | warning | 审批和 trace_id |
| 05 | BatchStateMatrix | state card grid | six states | 状态矩阵 |
| 05 | StateCard | compact card | default/hover/loading/disabled/error/danger | 带执行按钮 |
| 05 | LoadingSpinner | icon | loading | 加载态按钮内 |
| 06 | ReactMappingPanel | code reference panel | default | JSX + confirmAction |
| 06 | CodeStrip | pre/code | default | 三行代码 |
| 06 | AcceptanceChecklist | checkbox-like checks | success | 四条验收条件 |

## 图标清单

| 位置 | 可视元素/图标 | 实现方式 | 语义 | 是否需自绘 |
|---|---|---|---|---|
| 01 已选择 | 绿色勾选框 | Ant Design Checkbox | 已有选中项 | 否 |
| 01 当前页 | 蓝色 pill | CSS pill | 当前页范围 | 否 |
| 01 含 3 项受限 | 黄色 pill | CSS pill | 受限选择提醒 | 否 |
| 01 确认告警 | 绿色按钮 | Ant Design Button | 安全批量动作 | 否 |
| 01 转 SOAR | 蓝色按钮 | Ant Design Button | SOAR 流转 | 否 |
| 01 清空 | 灰色按钮 | Ant Design Button | 清除选择 | 否 |
| 02 危险提示 | 红色提示条 | Alert inline | 危险门禁 | 否 |
| 03 当前页/筛选/排除 | checkbox | Ant Design Checkbox | 选择范围 | 否 |
| 03 全部告警 | 空 checkbox | Ant Design Checkbox | 未选全量范围 | 否 |
| 04 高影响提示 | 黄色提示条 | Alert inline | 审批审计要求 | 否 |
| 05 加载 | 旋转加载符号 | Ant Design LoadingOutlined | 提交中防重复 | 否 |
| 05 禁用按钮 | 灰色按钮 | disabled Button | 不可执行 | 否 |
| 05 错误按钮 | 红色按钮 | danger Button | 失败重试 | 否 |
| 05 危险按钮 | 红色按钮 | danger Button | 二次确认 | 否 |
| 06 验收项 | 绿色勾选框 | Checkbox-like marker | 验收条件满足 | 否 |

## Token 与样式

| token | 值 | 用途 | 复刻要求 |
|---|---|---|---|
| canvas.background | `#03111c` | 页面底色 | 必须一致 |
| grid.line | `rgba(30,156,255,0.24)` | 蓝图网格线 | 低透明 |
| panel.background | `rgba(6,28,43,0.86)` | 六个区块面板 | 不做亮卡片 |
| panel.header.background | `#0b3c52` | 区块标题条 | 统一深青 |
| panel.border | `rgba(32,200,232,0.55)` | 面板和控件描边 | 细线 |
| text.primary | `#eaf7ff` | 标题和主标签 | 明亮 |
| text.secondary | `#9db9c9` | 说明文字 | 灰蓝 |
| accent.info | `#1e9cff` | 普通动作、标签、蓝色描边 | 信息态 |
| accent.success | `#00ff80` | 勾选、安全、确认动作 | 健康/通过 |
| accent.warning | `#ffe600` | 受限、确认范围、高影响 | 中危/确认 |
| accent.danger | `#ff3030` | 高危、错误、删除 | 危险/失败 |
| control.background | `#08253a` | 工具栏、状态卡、按钮底 | 稳定深色 |
| code.background | `#051928` | React code strip | 深色代码块 |
| radius.panel | `6px` | 区块面板 | 与 foundations 一致 |
| radius.button | `4px` | 按钮和 pill | 小圆角 |
| density.button.height | `31px` | 操作栏按钮 | 紧凑 |
| density.selection.row | `37px` | 选择范围行 | 稳定 |
| density.state.card | `44px` | 状态卡 | 稳定 |
| spacing.grid | `8px` | 组件间距 | 8px 基线 |

## 状态与交互

| 组件 | 状态 | 触发 | 期望 |
|---|---|---|---|
| BatchActionBar | hidden | selectedRowKeys 为空 | 工具栏不出现，不替代表格筛选、搜索、分页、列设置 |
| BatchActionBar | selected | 选中 38 行 | 展示计数、范围、受限项和动作 |
| ScopePill | current-page | 当前页范围 | 只影响本页已加载行 |
| RestrictedPill | warning | 存在 3 项受限 | 提醒但不直接执行受限对象 |
| BatchActionButton | safe | 点击确认告警 | 进入安全批量确认流程 |
| BatchSecondaryButton | workflow | 点击转 SOAR | 进入 SOAR 流转并保留审计上下文 |
| MoreButton | menu | 点击更多 | 生产实现展开二级动作菜单，目标图不展示展开态 |
| ClearButton | clear | 点击清空 | 清除选择上下文，不删除数据 |
| DangerActionGroup | danger | 点击删除/回滚/吊销令牌 | 必须进入 Popconfirm、Modal 或审批流 |
| SelectionScopeMatrix | current-page | 当前页行勾选 | 安全默认范围 |
| SelectionScopeMatrix | cross-page | 筛选结果范围 | 展示筛选条件快照和排除项 |
| SelectionScopeMatrix | full-corpus | 全部告警范围 | 使用后台任务并生成审计任务 |
| SelectionScopeMatrix | manual-exclude | 手动排除对象 | 支持撤销排除 |
| GuardrailCard | permission-filtered | 有无权限对象 | 自动排除并显示受限对象数量 |
| AuditPayloadCard | audit-required | 高影响批处理 | 写入 tenant、trace_id、selectedKeys、操作者、影响范围和回滚方案 |
| StateCard | default | 38 项已选 | 执行按钮可用 |
| StateCard | hover | 悬停按钮 | 按钮描边增强 |
| StateCard | loading | 提交中 | 防重复提交并显示 spinner |
| StateCard | disabled | 无可操作项 | 按钮弱化，生产实现应给出原因提示 |
| StateCard | error | 部分失败 | 支持重试并保留上下文 |
| StateCard | danger | 危险动作 | 需要二次确认 |
| ReactMappingPanel | developer-reference | 前端实现 | 映射 selectedRowKeys、selectionScope、actions、confirmAction |

## 实现映射

- 页面：无业务路由。
- 像素验收：使用 `reference-raster` 开发态页面承载目标 PNG，并通过 Windows Chrome 截图和 diff 证明目标 PNG 复刻。
- 生产组件建议：建立 `BatchActionBar`、`BatchActionButton`、`SelectionScopePanel`、`SelectionScopeRow`、`BatchGuardrailPanel`、`BatchActionStateMatrix`、`ReactMappingPanel`。
- Ant Design 映射：`Checkbox`、`Button`、`Space`、`Dropdown`、`Popconfirm`、`Modal`、`Alert`、`Tag`、`Tooltip`。
- 数据字段建议：`selectedRowKeys`、`selectionScope`、`filterSnapshot`、`excludedKeys`、`restrictedKeys`、`actions`、`impact`、`permission`、`auditPayload`、`rollbackPlan`、`task_id`。
- API/数据：目标图未绑定 API；生产实现应从告警、资产、规则、证据、SOAR 和审计服务取数。
- 权限：执行前要二次校验租户、角色、对象权限和当前对象状态。
- 审计：高影响批处理必须记录操作者、审批人、影响范围、回滚方式、trace_id 和结果。
- 跨页全选：必须保存筛选条件快照，避免数据刷新后选择语义漂移。
- 危险动作：删除、回滚、吊销令牌不能直接执行。
- 更多菜单：目标图只给入口，生产实现需补齐菜单项、禁用原因、tooltip 和键盘焦点。
- 状态矩阵：作为组件态说明，不参与业务过滤。

## 验收证据

- URL：`http://10.0.5.8:36527/evidence/ui-image-breakdowns/components/component-batch-action-bar/implementation.html`
- 视口：`1920x1080`
- 目标图：`evidence/ui-image-breakdowns/components/component-batch-action-bar/target.png`
- 实现文件：`evidence/ui-image-breakdowns/components/component-batch-action-bar/implementation.html`
- 实现截图：`evidence/ui-image-breakdowns/components/component-batch-action-bar/implementation.png`
- diff 图：`evidence/ui-image-breakdowns/components/component-batch-action-bar/diff.png`
- diff metrics：`evidence/ui-image-breakdowns/components/component-batch-action-bar/metrics.json`
- 区域 overlay：`evidence/ui-image-breakdowns/components/component-batch-action-bar/regions-overlay.png`
- verification：`evidence/ui-image-breakdowns/components/component-batch-action-bar/verification.json`
- measurement：`evidence/ui-image-breakdowns/components/component-batch-action-bar/measurement.json`
- text ledger：`evidence/ui-image-breakdowns/components/component-batch-action-bar/text-ocr.txt`
- Chrome/CDP：`evidence/ui-image-breakdowns/components/component-batch-action-bar/cdp-version.json`
- 截图元数据：`evidence/ui-image-breakdowns/components/component-batch-action-bar/capture-meta.json`
- 当前 mismatch ratio：`0.0`
- Windows Chrome 状态：`Chrome/150.0.7871.47`，`Windows Chrome CDP`，devicePixelRatio `1`，无滚动条，无 console/page/request 错误

## 差异清单

| 类型 | 位置 | 当前 | 期望 | 状态 |
|---|---|---|---|---|
| scope | 生产 React 实现 | 当前 pixel 验收使用 reference-raster | 后续生产组件按本记录实现 BatchActionBar、选择范围、危险门禁、权限审计语义 | documented |
| production-detail | 更多菜单和确认流程 | 目标图只展示入口和规范，不展示展开菜单、确认弹窗、审批详情和任务进度 | 生产实现补齐菜单内容、确认流程、审批细节和任务追踪 | documented |
| accessibility | 批量操作控件 | 目标图只展示视觉状态 | 生产实现补齐键盘焦点、ARIA、半选态、禁用原因和加载提示 | documented |

## 主线程补充核对

- 顶部标题、副标题和右上系统标签已定位。
- 六个区块已逐区记录，不按其它组件板套模板。
- 01 区块的勾选框、选中计数、当前页、受限项和五个操作按钮已记录。
- 01 区块三种承载位置已逐项记录。
- 02 区块四个动作分组已逐组记录。
- 02 区块高危动作和红色门禁提示已记录。
- 03 区块四种选择范围已逐行记录。
- 03 区块跨页说明、筛选条件快照和 task_id 已记录。
- 04 区块六张 guardrail 卡片已逐张记录。
- 04 区块高影响批处理提示已记录。
- 05 区块六个状态卡已逐卡记录。
- 05 区块加载 spinner、禁用态、错误态和危险态已列入图标与交互。
- 06 区块三行 React 代码已逐行记录。
- 06 区块四条验收 checklist 已逐条记录。
- 底部黄色脚注已记录，明确公共 AppShell 只作为 token 参考。
- token 已覆盖背景、网格、面板、标题条、按钮、状态色、代码块、圆角、行高和间距。
- 交互状态已覆盖隐藏、选中、跨页全选、危险确认、权限过滤、审计写入、加载、防重复、禁用、错误、重试和 React 映射。
- 主线程已逐项回看 target、implementation、diff 和 overlay 四类证据图。
- 辅助智能体只负责查漏，主线程保留最终判断权。

## 结论

- 当前状态：`pixel-accepted`。
- 深拆完整性：已覆盖区域坐标、文本、组件、图标、token、状态交互、实现映射和验收证据路径。
- pixel-accepted 判定：Windows Chrome reference-raster 截图完成，diff mismatch ratio 为 `0.0`，辅助智能体审查结论已纳入，主线程判定通过。
