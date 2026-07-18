# UI ImageGen Method

更新日期：2026-07-13

本文档记录“园区网络全流量采集与分析系统”UI 图继续生成的方法，供 Codex 上下文压缩、重连或后续接力时直接恢复执行。

## UI 参考图与项目逻辑的优先级

UI 图只作为视觉方向、组件语言和信息密度参考，不是页面业务逻辑或前端实现的绝对真源。后续生成、审核和开发必须遵循以下优先级：

1. 项目真实业务链路、API/数据契约、权限边界和页面状态模型。
2. `agent.md`、`doc/01_design/`、页面 contract、现有前端路由与可验证交互。
3. foundations 提供的色彩、字体、间距、组件和 AppShell 视觉语言。
4. 单张 UI 参考图的具体排版与内容。

当参考图与前三级依据冲突时，必须先判定参考图不合理，再自主修正信息架构、状态流、内容模块和布局；禁止为了复刻图片而实现错误业务逻辑。foundations 的“硬门禁”仅约束统一视觉语言和公共壳责任边界，不冻结页面内容结构。

页面设计与开发还必须满足：

- 先审业务对象、用户任务、状态流、权限、异常态和闭环动作，再决定页面模块。
- 每个模块都要能回答“数据从哪里来、用户为什么需要、可执行什么动作、动作后状态如何变化”。
- 主业务视口应充分利用，形成摘要、查询/筛选、核心数据、分析视图、上下文证据和处置动作的完整闭环。
- 不允许用无业务价值的装饰填充空白，也不允许因内容拆解不足出现大面积无内容区域；合理留白只用于层级、分组和可读性。
- 参考图出现对象错配、Tab 层级混淆、重复导航、错误权限入口、错误动作或空洞布局时，生成和开发阶段均可自主重构。
- 最终验收对象是运行中的真实页面；UI 图与实现不一致时，只要实现更符合项目逻辑、视觉规范和业务可用性，应以实现为准并同步更新 contract/参考图。

## 输入依据

每轮生成前必须读取并遵守：

- `agent.md`：项目主链路、前端技术栈、协作规则、Git 工作区保护规则。
- `doc/README.md` 与 `doc/01_design/`：产品范围、菜单信息架构、二级菜单矩阵、UI 前端规范。
- `doc/04_assets/ui_suite_gpt_v1/manifest.json`：181 张交付基线、目标路径、prompt 路径。
- `doc/04_assets/ui_suite_gpt_v1/CHAT_IMAGEGEN_INVENTORY.md`：总清单与目录约定。
- `doc/04_assets/ui_suite_gpt_v1/GENERATION_STATE.md`：最新可执行断点。
- `doc/04_assets/ui_suite_gpt_v1/CONTEXT_HANDOFF.md`：上下文压缩后的接力口径。
- `doc/04_assets/ui_suite_gpt_v1/PAGE_OVERLAY_COMPONENT_GAP_INVENTORY.md`：缺口统计与补图顺序。
- `doc/04_assets/ui_suite_gpt_v1/OVERLAY_COMPONENT_IMAGEGEN_PROGRESS.md`：浮层、组件、状态、响应式进度。

## 当前范围

manifest 交付基线当前为 181 张：

- foundations：8 张，已完成。
- pages：27 张，已完成。
- overlays：70 张，已完成 70 张，剩余 0 张。
- components：48 张，已完成 48 张，剩余 0 张。
- states：16 张，已完成 16 张，剩余 0 张。
- responsive：12 张，已完成 12 张，剩余 0 张。

当前总进度为已落盘 181 张、剩余 0 张。manifest 交付基线已完成；P7 扩容 18 张已写入 `manifest.json` 和 prompt 队列，并全部完成。

## 业务合理性复核方法

manifest 收口或批量返工后，必须基于 `screens/` 图片内容再做一次业务合理性复核：

1. 按 pages、overlays、components、states、responsive 生成或查看 contact sheet，先看整体是否存在模板化、职责重复或业务动作错配。
2. 状态图优先检查加载、空态、API 错误、网络错误、认证、授权、降级、运行背压和任务状态是否被混成同一种“错误/重试”语义。
3. 401 与 403 必须分开：401 主动作是重新认证，403 主动作是申请权限、查看审计或返回上一页，不得把权限不足画成普通重试。
4. 响应式图必须绑定具体页面和断点，说明核心业务保留、次要区域折叠、危险动作位置和上下文传递方式，不能只是空白布局线框。
5. 业务复核发现的问题需要直接修复最终 PNG，并保留 `*.before-business-fix.png` 与新的 `*.raw-deterministic.png`。
6. 不把 UI 图当作逻辑真源；逐页核对路由状态、选中对象、Tab 层级、数据来源、权限、动作结果和异常/空态。
7. 检查 1920x1080 主业务视口是否被有效内容充分利用。若存在大面积空白，先判断是内容缺失、栅格失衡还是状态设计错误，再补充真实业务模块或重排布局，禁止只加装饰。
8. UI 图逻辑错误时，允许依据项目 contract 和真实业务自主调整页面；调整结果需要回写 breakdown/contract，保持图、代码和验收口径一致。

2026-06-28 已完成一次全量业务合理性复核，记录见 `doc/04_assets/ui_suite_gpt_v1/BUSINESS_REASONABILITY_AUDIT.md`。

## 生成批次

每次生成 2 张后，必须立即做一次上下文压缩式文档同步：

1. 默认使用 GPT 内置 `image_gen.imagegen`，每张图单独调用一次。
2. 对 AppShell 精确组件、公共区基准板或需要严格复刻 `screen.png` 尺寸/职责边界的图，允许使用 `screen.png` 确定性裁切重组；这种情况保留 `*.raw-deterministic.png`，并在批次记录中注明未使用 imagegen。
3. 将最新 imagegen 结果提取为目标文件，并同时保留 `*.raw-imagegen.png`；确定性生成则直接写入目标文件和 `*.raw-deterministic.png`。
4. 统一最终 PNG 尺寸为 `1920x1080`。
5. 校验目标文件存在、尺寸正确、raw 追溯文件存在。
6. 更新 `GENERATION_STATE.md`、`CONTEXT_HANDOFF.md`、`OVERLAY_COMPONENT_IMAGEGEN_PROGRESS.md`、`PAGE_OVERLAY_COMPONENT_GAP_INVENTORY.md`。
7. 在本文件或进度文档中记录批次 ID、完成项、下一项、剩余数量和关键口径变化。

提取命令：

```bash
python3 doc/04_assets/ui_suite_gpt_v1/extract_latest_imagegen.py <targetFile>
```

如果连续生成两张，必须在每次 `image_gen.imagegen` 返回后立刻执行一次提取命令，避免最新结果被后一张覆盖。

## 浮层新口径

从 2026-06-27 起，后续弹窗、抽屉、下拉、确认框可不携带公共区域，只带业务区域：

- 不绘制完整 AppShell。
- 不绘制顶部栏、左侧菜单、底部栏。
- 不要求宿主页面背景可识别。
- 只展示当前 Modal、Drawer、Dropdown、Popconfirm 或业务浮层容器本体。
- 公共 AppShell 只作为色彩、字号、密度、边框、圆角、图标和状态语义参考。
- 浮层必须体现权限、影响范围、状态解释、下一步动作、危险提示和审计留痕。

## 质量门禁

每张最终图必须满足：

- 16:9，最终文件 `1920x1080`。
- 深海军蓝 SOC 指挥台风格，延续 foundations。
- 中文为主，只保留必要英文技术词和单位。
- 不出现浏览器地址栏、营销页、海报化构图、水印。
- 不出现大面积渐变球、装饰光斑或浅色主题。
- 状态语义稳定：绿色健康/通过，蓝色信息/低危，黄色中危/待确认，红色高危/失败。
- 面板圆角约 6px，按钮圆角约 4px，表格行高约 32px，字体密度与既有页面一致。
- 组件、小图标和局部状态图也必须能作为前端开发和 Figma 设计参考，而不是装饰图。
- 页面业务逻辑通过审核：对象、Tab、路由、权限、数据、动作及状态转换与项目一致。
- 主内容区布局饱满且有层级，不得出现因模块缺失、固定高度或栅格失衡造成的大面积空白。
- 内容填充必须具有业务价值；不得使用无意义图表、重复 KPI、虚构入口或装饰面板凑满页面。
- 真实前端可以纠正参考图，但纠正后必须同步页面 contract 和 UI 参考资产，避免三者继续漂移。

## 批次记录

- 2026-06-27 batch-overlay-01：完成 `modal-fusion-rule-edit`、`modal-baseline-threshold`。两张图均使用内置 `image_gen.imagegen` 逐张生成，并通过 `extract_latest_imagegen.py` 提取到目标目录；最终 PNG 均为 `1920x1080`，raw 原图均已保留。当前 manifest 已落盘 70/163，剩余 93；overlay 已完成 35/52，剩余 17。下一批从 `modal-forensics-task` 和 `drawer-campaign-detail` 开始，按 manifest 当前缺口顺序回补。
- 2026-06-27 batch-overlay-02：完成 `modal-forensics-task`、`drawer-campaign-detail`。两张图均只展示业务容器本体，不绘制公共 AppShell 或宿主页面；最终 PNG 均为 `1920x1080`，raw 原图均已保留。当前 manifest 已落盘 72/163，剩余 91；overlay 已完成 37/52，剩余 15。下一批从 `drawer-attack-chain-detail` 和 `drawer-encrypted-fingerprint` 开始。
- 2026-06-27 batch-overlay-03：完成 `drawer-attack-chain-detail`、`drawer-encrypted-fingerprint`。两张图均只展示业务 Drawer 本体；最终 PNG 均为 `1920x1080`，raw 原图均已保留。当前 manifest 已落盘 74/163，剩余 89；overlay 已完成 39/52，剩余 13。下一批从 `drawer-certificate-detail` 和 `modal-rule-edit` 开始。
- 2026-06-27 batch-overlay-04：完成 `drawer-certificate-detail`、`modal-rule-edit`。两张图均只展示业务容器本体；最终 PNG 均为 `1920x1080`，raw 原图均已保留。当前 manifest 已落盘 76/163，剩余 87；overlay 已完成 41/52，剩余 11。下一批从 `drawer-rule-detail` 和 `popconfirm-delete` 开始。
- 2026-06-28 batch-overlay-05：完成 `drawer-rule-detail`、`popconfirm-delete`。两张图均只展示业务容器本体；最终 PNG 均为 `1920x1080`，raw 原图均已保留。当前 manifest 已落盘 78/163，剩余 85；overlay 已完成 43/52，剩余 9。下一批从 `modal-rule-publish` 和 `modal-deployment-create` 开始。
- 2026-06-28 batch-overlay-06：完成 `modal-rule-publish`、`modal-deployment-create`。两张图均只展示业务容器本体；最终 PNG 均为 `1920x1080`，raw 原图均已保留。当前 manifest 已落盘 80/163，剩余 83；overlay 已完成 45/52，剩余 7。下一批从 `modal-deployment-rollback` 和 `drawer-model-detail` 开始。
- 2026-06-28 batch-overlay-07：完成 `modal-deployment-rollback`、`drawer-model-detail`。两张图均只展示业务容器本体；最终 PNG 均为 `1920x1080`，raw 原图均已保留。当前 manifest 已落盘 82/163，剩余 81；overlay 已完成 47/52，剩余 5。下一批从 `drawer-mlops-task-detail` 和 `modal-playbook-edit` 开始。
- 2026-06-28 batch-overlay-08：完成 `drawer-mlops-task-detail`、`modal-playbook-edit`。两张图均只展示业务容器本体；最终 PNG 均为 `1920x1080`，raw 原图均已保留。当前 manifest 已落盘 84/163，剩余 79；overlay 已完成 49/52，剩余 3。下一批从 `modal-whitelist-add` 和 `drawer-whitelist-approval` 开始。
- 2026-06-28 batch-overlay-09：完成 `modal-whitelist-add`、`drawer-whitelist-approval`。两张图均只展示业务容器本体；最终 PNG 均为 `1920x1080`，raw 原图均已保留。当前 manifest 已落盘 86/163，剩余 77；overlay 已完成 51/52，剩余 1。下一张为 overlay 队列最后一张 `modal-settings-token`，完成后进入 component。
- 2026-06-28 batch-overlay-10：完成 overlay 队列最后一张 `modal-settings-token`。最终 PNG 为 `1920x1080`，raw 原图已保留。当前 manifest 已落盘 87/163，剩余 76；overlay 已完成 52/52，剩余 0。下一阶段从 component 队列 `component-app-header` 开始。
- 2026-06-28 batch-component-baseline-refresh-01：返工 `component-app-header`、`component-primary-sidebar`。先确认顶部通知/用户动作组会与左侧底部用户区和底部右侧全局动作区重复，再更新 prompt、AppShell 规范和全部 prompt 公共约束；最终 PNG 不再采用漂移的 imagegen 结果，而是基于 `screens/pages/screen.png` 直接裁切重组成确定性规格板。验收口径：顶部只到六个快捷入口；用户身份只在左侧底部；通知角标、设置、全局配置、电源只在底部右侧。原 imagegen 终稿备份为 `*.before-duplication-fix.png`，raw 原图保留。
- 2026-06-28 batch-component-02：完成 `component-secondary-menu`、`component-bottom-status-bar`。两张图均为严格 AppShell 组件，使用 `screens/pages/screen.png` 确定性裁切重组而非 imagegen；最终 PNG 均为 `1920x1080`，并保留 `*.raw-deterministic.png`。当前 manifest 已落盘 91/163，剩余 72；component 已完成 4/48，剩余 44。下一批从 `component-breadcrumb-context` 和 `component-site-time-selector` 开始。
- 2026-06-28 batch-component-03：完成 `component-breadcrumb-context`、`component-site-time-selector`。两张图均使用 `screens/pages/screen.png` 与 foundations token 确定性绘制而非 imagegen；最终 PNG 均为 `1920x1080`，并保留 `*.raw-deterministic.png`。当前 manifest 已落盘 93/163，剩余 70；component 已完成 6/48，剩余 42。下一批从 `component-quick-entry` 和 `component-user-menu` 开始。
- 2026-06-28 batch-component-04：完成 `component-quick-entry`、`component-user-menu`。两张图均使用 `screens/pages/screen.png` 与 foundations token 确定性绘制而非 imagegen；生成前再次确认顶部通知/用户动作组会与左侧底部用户区和底部右侧全局动作区重复。验收口径：quick-entry 只覆盖顶部六个业务快捷入口，user-menu 只从左侧底部用户卡触发；最终 PNG 均为 `1920x1080`，并保留 `*.raw-deterministic.png`。当前 manifest 已落盘 95/163，剩余 68；component 已完成 8/48，剩余 40。下一批从 `component-button` 和 `component-icon-button` 开始。
- 2026-06-28 batch-component-05：完成 `component-button`、`component-icon-button`。两张均为基础控件组件板，使用 foundations token 确定性绘制而非 imagegen；最终 PNG 均为 `1920x1080`，并保留 `*.raw-deterministic.png`。当前 manifest 已落盘 97/163，剩余 66；component 已完成 10/48，剩余 38。下一批从 `component-status-chip` 和 `component-tooltip` 开始。
- 2026-06-28 batch-component-06：完成 `component-status-chip`、`component-tooltip`。两张均为基础控件组件板，使用 foundations token 和 Noto Sans CJK 字体确定性绘制而非 imagegen；最终 PNG 均为 `1920x1080`，并保留 `*.raw-deterministic.png`。`status-chip` 明确标签/Badge 只表达状态、计数和筛选上下文，不承载危险提交；`tooltip` 明确 Tooltip 只解释上下文、权限和风险，危险确认必须进入 Popconfirm / Modal。当前 manifest 已落盘 99/163，剩余 64；component 已完成 12/48，剩余 36。下一批从 `component-tabs` 和 `component-segmented` 开始。
- 2026-06-28 batch-component-07：完成 `component-tabs`、`component-segmented`。两张均为基础控件组件板，使用 foundations token 和 Noto Sans CJK 字体确定性绘制而非 imagegen；最终 PNG 均为 `1920x1080`，并保留 `*.raw-deterministic.png`。`tabs` 明确只做同一路由内复杂内容分组，不替代左侧菜单或新增路由；`segmented` 明确只做同一区域轻量互斥切换，不替代 Tabs、菜单或路由。当前 manifest 已落盘 101/163，剩余 62；component 已完成 14/48，剩余 34。下一批从 `component-dropdown` 和 `component-pagination` 开始。
- 2026-06-28 batch-component-08：完成 `component-dropdown`、`component-pagination`。两张均为基础控件组件板，使用 foundations token 和 Noto Sans CJK 字体确定性绘制而非 imagegen；最终 PNG 均为 `1920x1080`，并保留 `*.raw-deterministic.png`。`dropdown` 明确只做局部动作/选择集合，不替代 AppShell 左侧菜单、用户常驻区或全局导航，危险项只进入确认流程；`pagination` 明确只做列表位置反馈和服务端/游标分页，不承载筛选、导出或危险动作。当前 manifest 已落盘 103/163，剩余 60；component 已完成 16/48，剩余 32。下一批从 `component-input` 和 `component-search` 开始。
- 2026-06-28 batch-component-09：完成 `component-input`、`component-search`。两张均为基础控件组件板，使用 foundations token 和 Noto Sans CJK 字体确定性绘制而非 imagegen；最终 PNG 均为 `1920x1080`，并保留 `*.raw-deterministic.png`。`input` 明确只做字段录入、校验、脱敏、单位和危险变更审计，不承载搜索建议层、通知/用户入口或完整 AppShell；`search` 明确只做业务查询、建议层、筛选 chip 和查询状态，不承载用户头像、通知铃铛、设置、电源或全局导航动作。该批之后继续进入 `component-select` 和 `component-date-range`，并已由 batch-component-10 接续完成。
- 2026-06-28 batch-component-10：完成 `component-select`、`component-date-range`。两张均为基础控件组件板，使用 foundations token 和 Noto Sans CJK 字体确定性绘制而非 imagegen；最终 PNG 均为 `1920x1080`，并保留 `*.raw-deterministic.png`。`select` 明确只做有限集合选择、远程搜索、分组选项、受限选项和高影响选择审计，不承载日期范围、搜索建议层或全局动作；`date-range` 明确只做业务查询时间窗、快捷项、时区/延迟、最大跨度和高成本查询门禁，不替代顶部站点/时间模块。该批之后继续进入 `component-switch-checkbox-radio` 和 `component-condition-builder`，并已由 batch-component-11 接续完成。
- 2026-06-28 batch-component-11：完成 `component-switch-checkbox-radio`、`component-condition-builder`。两张均为基础表单组件板，使用 foundations token 和 Noto Sans CJK 字体确定性绘制而非 imagegen；最终 PNG 均为 `1920x1080`，并保留 `*.raw-deterministic.png`。`switch-checkbox-radio` 明确只做布尔开关、多选范围、半选树、互斥策略和高影响开关审计，不承载导航、通知、用户菜单或顶部快捷入口；`condition-builder` 明确只做条件组、字段 schema、操作符、值输入、嵌套、拖拽、命中预估和保存前门禁，不替代 Select/Search/DateRange。该批之后继续进入 `component-batch-action-bar` 和 `component-data-table`，并已由 batch-component-12 接续完成。
- 2026-06-28 batch-component-12：完成 `component-batch-action-bar`、`component-data-table`。两张均为基础数据操作组件板，使用 foundations token 和 Noto Sans CJK 字体确定性绘制而非 imagegen；最终 PNG 均为 `1920x1080`，并保留 `*.raw-deterministic.png`。`batch-action-bar` 明确只处理已选对象、跨页范围、动作集合、危险确认、权限影响和审计追踪，不承载筛选、搜索、分页、列配置或表格主体；`data-table` 明确只处理数据密度、列结构、行选择、排序筛选、分页、行状态和虚拟滚动，批量动作交给独立 BatchActionBar。当前 manifest 已落盘 111/163，剩余 52；component 已完成 24/48，剩余 24。下一批从 `component-description-list` 和 `component-kpi-tile` 开始。
- 2026-06-28 batch-component-13：完成 `component-description-list`、`component-kpi-tile`。两张均为基础数据展示组件板，使用 foundations token 和 Noto Sans CJK 字体确定性绘制而非 imagegen；最终 PNG 均为 `1920x1080`，并保留 `*.raw-deterministic.png`。`description-list` 明确只处理详情键值、分组、脱敏、复制、权限锁定、字段状态和审计 trace_id，不替代 Table/Form/Drawer；`kpi-tile` 明确只处理单指标摘要、趋势、阈值、状态矩阵、数据新鲜度和下钻审计，不复刻完整 dashboard 或顶部状态栏 KPI。当前 manifest 已落盘 113/163，剩余 50；component 已完成 26/48，剩余 22。下一批从 `component-health-card` 和 `component-ranking-list` 开始。
- 2026-06-28 batch-component-14：完成 `component-health-card`、`component-ranking-list`。两张均为基础数据展示组件板，使用 foundations token 和 Noto Sans CJK 字体确定性绘制而非 imagegen；最终 PNG 均为 `1920x1080`，并保留 `*.raw-deterministic.png`。`health-card` 明确只处理对象健康、健康分、子检查、依赖徽标、趋势、修复建议和审计门禁，不替代 KPI Tile、Alert Card 或完整监控大屏；`ranking-list` 明确只处理 TopN 行列表、排序阈值、指标趋势、行状态和轻量下钻，不替代 DataTable、Bar Chart 或完整 dashboard。当前 manifest 已落盘 115/163，剩余 48；component 已完成 28/48，剩余 20。下一批从 `component-log-list` 和 `component-evidence-file-card` 开始。
- 2026-06-28 batch-component-15：完成 `component-log-list`、`component-evidence-file-card`。两张均为基础数据展示组件板，使用 foundations token、DroidSansFallback 中文字体和 DejaVu Sans 英文字体确定性绘制而非 imagegen；最终 PNG 均为 `1920x1080`，并保留 `*.raw-deterministic.png`。`log-list` 明确只处理实时日志列表、级别语义、trace 高亮、展开详情、定位来源、复制、脱敏和权限状态，不替代 DataTable、Audit Log 页面或 Timeline；`evidence-file-card` 明确只处理单个证据对象卡片、文件类型图标、hash、签名 URL、保留期、权限、下载/预览/复制 hash/关联告警和审计门禁，不替代证据详情 Modal、文件表格或上传组件。该批结束时 manifest 为 117/163，component 为 30/48，随后由收口批次继续补齐。
- 2026-06-28 batch-component-16 至 batch-responsive-06：按两张一组连续补齐最后 46 张 manifest 缺口，均使用 foundations token、DroidSansFallback 中文字体和 DejaVu Sans 英文字体确定性绘制而非 imagegen；最终 PNG 均为 `1920x1080`，并保留 `*.raw-deterministic.png`。完成顺序为：`component-empty-card` + `component-permission-card`；`component-line-area-chart` + `component-donut-chart`；`component-bar-ranking-chart` + `component-sankey-flow`；`component-radar-quality` + `component-heatmap`；`component-topology-graph` + `component-timeline-state-machine`；`component-alert-queue` + `component-risk-score`；`component-alert-timeline` + `component-evidence-drawer`；`component-asset-context` + `component-action-rail`；`component-feedback-block` + `component-acceptance-gate-matrix`；`state-page-loading` + `state-table-loading`；`state-chart-loading` + `state-empty-page`；`state-empty-table` + `state-empty-chart`；`state-api-error` + `state-network-error`；`state-unauthorized` + `state-forbidden`；`state-partial-degraded` + `state-offline-probe`；`state-stream-backpressure` + `state-task-running`；`state-task-failed` + `state-success-accepted`；`responsive-dashboard-1440` + `responsive-dashboard-1920`；`responsive-screen-4k` + `responsive-alerts-1440`；`responsive-alerts-1920` + `responsive-forensics-1440`；`responsive-graph-1440` + `responsive-compliance-1440`；`responsive-tablet-dashboard` + `responsive-tablet-alert-detail`；`responsive-mobile-navigation` + `responsive-mobile-alert-list`。当前 manifest 已落盘 163/163，剩余 0；component、state、responsive 均完成。
- 2026-06-28 business-audit-fix-01：基于 `screens/` contact sheet 做全量业务合理性复核后，返工 `screens/states/` 16 张和 `screens/responsive/` 12 张，共 28 张。主要修复点：状态图不再共用泛化错误模板；加载、空态、API/网络错误、401、403、降级、探针离线、流处理背压、任务运行/失败/受理成功均具备独立业务原因和动作；响应式图从空白断点线框改为 dashboard、告警、取证、图谱、合规、平板和移动端的页面级折叠策略。被返工图片均保留 `*.before-business-fix.png`，新的确定性 raw 均保留为 `*.raw-deterministic.png`。
- 2026-06-28 batch-p7-overlay-01 至 batch-p7-overlay-09：用户确认“全部处理生成 UI 图”后，把 P7 18 张 overlay 扩容写入 `manifest.json`、prompt 队列和状态文档，并按两张一批使用内置 `image_gen.imagegen` 逐张生成、逐张提取。完成顺序为：`modal-topic-save-view` + `drawer-topic-scope-edit`；`modal-topic-report-export` + `modal-topic-evidence-package-export`；`drawer-topic-subscription` + `dropdown-topic-share-favorite`；`modal-campaign-report-export` + `modal-forensics-evidence-export`；`drawer-compliance-gate-detail` + `modal-compliance-evidence-package-export`；`modal-compliance-report-export` + `drawer-audit-operation-detail`；`modal-audit-export` + `modal-notification-channel-edit`；`modal-notification-template-preview-test` + `drawer-notification-silence-rule`；`popconfirm-settings-token-revoke` + `drawer-settings-rbac-edit`。全部最终 PNG 均为 `1920x1080`，并保留 `*.raw-imagegen.png`；当前 manifest 已落盘 181/181，剩余 0。
