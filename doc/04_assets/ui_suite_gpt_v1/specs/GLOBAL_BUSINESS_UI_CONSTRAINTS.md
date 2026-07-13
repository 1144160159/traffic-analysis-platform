# 全系统业务页面交互与动态图约束

- 生效时间：`2026-07-11`
- 范围：所有已登录业务页面和业务详情页；登录、OIDC 回调、404 与公共 topbar/sidebar/bottombar 不计入业务像素区域。
- 全栈交付：所有页面同时遵循 `FULL_STACK_PAGE_DELIVERY_WORKFLOW.md`；本文件的 UI 通过不等于 API、数据库和审计通过。

## 强制规则

1. 业务趋势、分布、热力、关系、地图和仪表类图示必须用 ECharts 动态渲染；数据来自页面 API 契约，API 不可用时使用同结构的 typed fallback 数据。
2. 每个可见业务按钮必须能产生可观察的结果：路由跳转、筛选/分页状态变化、右侧详情 Drawer、受控 Modal，或由服务端受理并持久化审计的业务动作。纯展示按钮和仅前端模拟成功均不允许保留。
3. 可能超出面板可视高度的业务内容必须放入有稳定尺寸的滚动容器，并使用 `overflow-y: auto`、最小高度约束和稳定滚动条槽位，避免撑破业务区。
4. 记录型业务表在结果超过可视容量时必须采用受控分页；页码、上一页/下一页、条数与当前页均可操作，切换页不会改变公共壳层位置。
5. 正常生产路由的图表、分页和按钮必须走真实 API 与数据库；typed fallback 只用于降级，视觉仿真只用于截图，二者均不能作为业务链路完成证据。
6. 动作成功必须由服务端完成 RBAC、tenant 校验和审计写入；前端延时、会话缓存或静态成功提示不算完成。
7. 数据库仿真必须是可重复的 seeded 数据，覆盖分页、图表、边界状态和动作对象，并保留 seed 报告与清理方式。

## 验收口径

- 静态：运行 `node tests/e2e/business_ui_constraint_inventory.mjs` 生成全系统缺口清单，并逐页消除报告中的候选项或记录为非业务图标/非表格豁免。
- 运行时：每个菜单状态至少验证一个图表、一个操作按钮、一个需要分页的表格和一个溢出承载区域；无 4xx/5xx、`requestfailed`、非 warning console error 或 page error。
- 全栈：正常生产路由验证真实分页参数、服务端 RBAC、动作响应、审计持久化和数据库记录；视觉仿真模式不得参与该结论。
- 视觉：逐页只对业务 ROI 评分；公共 topbar/sidebar/bottombar 保留为整图诊断，不作为业务区回修理由。

## 当前执行顺序

1. 先完成数据质量字段质量 r235 的先行实现与生产验证。
2. 使用清单按“可见业务按钮、静态 SVG 图、伪分页、内容溢出”四类排序整改全系统业务页。
3. 每页完成后更新路由截图、交互证据、ROI 指标和审查记录。

## 2026-07-11 基线审计

以下历史批次中的 ROI、截图和前端交互通过只代表当时的 UI 阶段证据。凡仍使用 typed 补位、会话审计或前端仿真动作的页面，均须按 `FULL_STACK_PAGE_DELIVERY_WORKFLOW.md` 补齐后端和 seeded 数据后重新取得全栈结论。

- 证据：`evidence/business-ui-constraints/inventory-latest.json`，生成器：`tests/e2e/business_ui_constraint_inventory.mjs`。
- 范围：28 个非登录、非回调、非 404 的业务页面。
- 当前清单：526 个已接入交互的控件声明，另有 212 个需逐页确认或接入处理器的候选；页面内 SVG 图表候选已降为 `0`；4 个页面没有分页标记。数据质量页、告警研判、专题工作台、探针管理、态势大屏、白名单治理、规则管理、审计日志、取证工作台和通知配置的图表 SVG 候选均为 `0`，该统计仍是整改清单，不代表全系统已经通过。
- 第一批：数据质量其余 7 个 Tab、告警详情、专题工作台、白名单治理、审计日志、发布管理、模型管理和通知配置。
- 审计使用 TypeScript JSX AST 识别没有 `onClick`、`href`、`disabled`、submit 语义或传播属性的候选控件；候选仍需逐页复核，不能直接当作最终违规数。

## 告警详情 API 降级约束

- 告警详情的 API 契约已在 `web/ui/src/services/pageApiPlans.ts` 注册。当前生产网关未部署 `/v1/alerts/{id}`、`/evidence` 和 `/feedback`，所以 `ALERT_DETAIL_API_ENABLED=false` 时由 `alertDetailApi.ts` 直接返回 typed 仿真快照，避免页面产生 404 或 console error。
- 后端接口部署并完成路由验收后，将运行时变量设为 `ALERT_DETAIL_API_ENABLED=true`。在对应服务端写入与审计端点部署前，现有“确认后提交”模拟任务只算前端阶段能力，告警详情不能标记 `full-stack-accepted`。

## r245 专题工作台整页批次

- 生产镜像：`traffic/web-ui:ui-topic-graphs-20260711-r245`。加密隧道局部影响面、APT 战役攻击关系、APT 事件趋势和处置动作分布均为基于页面 API 契约/typed fallback 的 ECharts 动态图。
- 所有三个专题分支的可见业务动作均通过 `TopicActionButton` 提供确认 Drawer 与模拟任务反馈；隧道和 APT 证据表均具有受控分页及稳定的纵向滚动承载。
- Windows Chrome 交互证据：`evidence/ui-image-breakdowns/pages/topics-apt-campaign/interaction-r245.json`，覆盖隧道、数据外传、APT 三个路由状态，验证 ECharts canvas、标签切换、第 2 页、动作确认和零运行时错误。
- r245 APT 业务 ROI：`0.10059123258314683 <= 0.125`，整图诊断：`0.09548707561728395 <= 0.125`；证据为 `metrics-business-r245.json` 与 `metrics.json`。

## r246 探针管理批次

- 生产镜像：`traffic/web-ui:ui-probes-echarts-20260711-r246`。部署拓扑、采集带宽趋势和批量发送带宽均替换为基于 `/v1/probes` 页面快照及 typed fallback 的 ECharts canvas。
- 探针状态矩阵接入 7 条/页的受控分页、稳定纵向滚动；2D/3D、刷新、时间范围、全屏矩阵、批量运维、行级操作和心跳日志均产生状态变化或确认 Drawer 的可观察结果。
- Windows Chrome 交互证据：`evidence/ui-image-breakdowns/pages/probes/interaction-r246.json`，验证 3 张 ECharts canvas、拓扑切换、24 小时筛选、第 2 页、滚动、批量/行级操作和全屏矩阵，且没有 4xx/5xx、请求失败、console error 或 page error。
- r246 探针业务 ROI：`0.06313953620919602 <= 0.125`，整图诊断：`0.06781780478395062 <= 0.125`；证据为 `metrics-business-r246.json` 与 `metrics.json`。

## r247 态势大屏批次

- 生产镜像：`traffic/web-ui:ui-screen-echarts-20260711-r247`。园区覆盖地图和园区数字孪生拓扑已从静态 SVG 迁移到由 `screenVisuals.probeMapNodes/probeMapLinks` 与 `screenVisuals.topologyNodes/topologyEdges` 驱动的 ECharts 图层。
- 原有 2D/3D 切换和 DOM 拓扑节点保持可用；ECharts 拓扑边层使用 `pointer-events: none`，因此不会阻挡业务节点的选择、详情联动或路由下钻。
- Windows Chrome 交互证据：`evidence/ui-image-breakdowns/pages/screen/interaction-r247.json`，验证两个新增 ECharts canvas、2D 状态、节点选择和零运行时错误。
- r247 态势大屏业务 ROI：`0.09511460514200094 <= 0.125`，整图诊断：`0.09130738811728395 <= 0.125`；证据为 `metrics-business-r247.json` 与 `metrics.json`。

## r249 白名单治理批次

- 生产镜像：`traffic/web-ui:ui-whitelist-interactions-20260711-r249`。白名单命中趋势与规则/模型覆盖维度均为来自页面快照或 typed fallback 的 ECharts canvas；静态 SVG 图表候选为 `0`。
- 白名单列表以 5 条/页提供受控分页和水平滚动承载；筛选、分页、到期 Tab、审批展开、行级详情/编辑以及顶部业务动作都具有可观察状态或确认 Drawer，且不改动公共 AppShell 位置。
- Windows Chrome 交互证据：`evidence/ui-image-breakdowns/pages/whitelist/interaction-r249.json`，验证 2 张 ECharts canvas、第 2 页、水平滚动、创建与行级 Drawer、到期 Tab、审批展开，且无 4xx/5xx、请求失败、console error 或 page error。
- r249 白名单业务 ROI：`0.08112792687359807 <= 0.125`，整图诊断：`0.08160831404320988 <= 0.125`；证据为 `metrics-business-r249.json` 与 `metrics.json`。

## r250 规则管理批次

- 生产镜像：`traffic/web-ui:ui-rules-interactions-20260711-r250`。平均延时、P95 延时、CPU 占用和内存占用均由 typed fallback 驱动的 ECharts sparkline 渲染，静态 SVG 图表候选为 `0`。
- 规则列表以 7 条/页提供受控分页和纵横滚动承载；筛选、编辑 Tab、样本 Tab、样本回放、命中矩阵、误报/白名单建议、版本与发布动作均产生状态或确认 Drawer，且公共 AppShell 未被修改。
- Windows Chrome 交互证据：`evidence/ui-image-breakdowns/pages/rules/interaction-r250.json`，验证 4 张 ECharts canvas、第 2 页、列表滚动、新建规则确认、编辑 Tab、样本 Tab 与全量发布 Drawer，且无 4xx/5xx、请求失败、console error 或 page error。
- r250 规则管理业务 ROI：`0.07719207586218252 <= 0.125`，整图诊断：`0.07834587191358025 <= 0.125`；证据为 `metrics-business-r250.json` 与 `metrics.json`。

## r251 审计日志批次

- 生产镜像：`traffic/web-ui:ui-auditlog-interactions-20260711-r251`。日志保留周期、归档位置、完整性校验和脱敏状态均由 typed fallback 驱动的 ECharts sparkline 渲染，静态 SVG 图表候选为 `0`。
- 日志列表以 10 条/页提供受控分页和纵横滚动承载；检索筛选、详情 Tab、Diff/时间线、导出格式和顶部取证操作均产生状态或确认 Drawer，且公共 AppShell 未被修改。
- Windows Chrome 交互证据：`evidence/ui-image-breakdowns/pages/audit-log/interaction-r251.json`，验证 4 张 ECharts canvas、第 2 页、列表滚动、导出确认、详情 Tab 与 CSV 格式切换，且无 4xx/5xx、请求失败、console error 或 page error。
- r251 审计日志业务 ROI：`0.07454938780576464 <= 0.125`，整图诊断：`0.07539255401234568 <= 0.125`；证据为 `metrics-business-r251.json` 与 `metrics.json`。

## r252 取证工作台批次

- 生产镜像：`traffic/web-ui:ui-forensics-interactions-20260711-r252`。会话数据包趋势由页面 API 契约/typed fallback 驱动的 ECharts canvas 渲染，静态 SVG 图表候选为 `0`。
- 取证任务列表以 5 条/页提供受控分页和纵横滚动承载；来源上下文、筛选、会话复放、PCAP 索引、证据包、hash、签名 URL 与取证审计均产生状态或确认 Drawer，且公共 AppShell 未被修改。
- Windows Chrome 交互证据：`evidence/ui-image-breakdowns/pages/forensics/interaction-r252.json`，验证 ECharts canvas、第 2 页、列表滚动、新建任务确认与会话复放 Drawer，且无 4xx/5xx、请求失败、console error 或 page error。
- r252 取证工作台业务 ROI：`0.08618595455311151 <= 0.125`，整图诊断：`0.0854663387345679 <= 0.125`；证据为 `metrics-business-r252.json` 与 `metrics.json`。

## r254 通知配置批次

- 生产镜像：`traffic/web-ui:ui-notifications-interactions-20260711-r254`。邮件、短信、Webhook、企业微信、钉钉与工单渠道的送达趋势均由 typed fallback 仿真数据驱动的 ECharts canvas 渲染；动作接口沿用 `pageApiPlans.notifications` 的设置、测试发送和静默规则端点预留，静态 SVG 图表候选为 `0`。
- 订阅规则以 6 条/页提供受控分页和纵横滚动承载；渠道开关、条件构造、升级策略、历史筛选、模板/静默窗口分页及顶部动作均产生可观察状态或确认 Drawer，且未修改公共 AppShell 几何。
- Windows Chrome 交互证据：`evidence/ui-image-breakdowns/pages/notifications/interaction-r254.json`，验证 6 张 ECharts canvas、第 2 页、表格纵向滚动、渠道开关状态、新增渠道确认、模板编辑 Drawer 和静默规则端点映射，且无 4xx/5xx、请求失败、console error 或 page error。
- r254 通知配置业务 ROI：`0.0748470306014791 <= 0.125`，整图诊断：`0.07531684027777778 <= 0.125`；证据为 `metrics-business-r254.json` 与 `metrics-business.json`。

## r255 告警研判批次

- 生产镜像：`traffic/web-ui:ui-alert-triage-interactions-20260711-r255`。选中告警风险评分由 `RingChart` ECharts gauge 基于 `__riskScore` API/typed fallback 数据动态渲染，静态 CSS 表盘已移除，静态 SVG 图表候选为 `0`。
- 当前部署为 r255；其中保留 r254 通知配置的已验证业务实现，通知页源码在本批次未发生变化。
- 告警列表以 10 条/页提供受控分页与纵横滚动；视图保存、筛选/重置、批量指派/导出、行级操作、关联告警簇、处置动作与反馈均产生可观察状态或带 endpoint/audit 内容的确认 Drawer，公共 AppShell 未改动。
- Windows Chrome 交互证据：`evidence/ui-image-breakdowns/pages/alerts/interaction-r255.json`，验证风险仪表 ECharts canvas、第 2 页、表格滚动、视图保存、行级详情和关联告警动作，且无 4xx/5xx、请求失败、console error 或 page error。
- r255 告警研判业务 ROI：`0.07573173518815957 <= 0.125`；证据为 `metrics-business-r255.json` 与 `metrics-business.json`。

## r259 拓扑 SVG 与态势大屏回退批次

- 当前生产镜像：`traffic/web-ui:ui-screen-original-svg-20260711-r259`。拓扑类可视化不再使用 ECharts graph canvas；探针部署拓扑、加密隧道关系图和 APT 战役关系图统一由页面 API 契约或 typed fallback 数据驱动的 SVG 图层渲染。
- 态势大屏的 `ProbeCoverageMap` 与 `TopologyTwinLayer` 已回退到原始 API 驱动 SVG 代码，恢复园区轮廓、分区、道路、建筑、链路、探针节点和 2D/3D 业务状态；非拓扑趋势、分布和统计图仍保留 ECharts。
- Windows Chrome 真实路由证据：`screen/interaction-r259-original-svg.json`、`probes/interaction-r259-topology-svg.json`、`topics-apt-campaign/interaction-r259-topology-svg.json`。三页均无 4xx/5xx、请求失败、console error 或 page error，且节点选择、分页、滚动和业务动作均有可观察结果。
- 业务 ROI：探针 `0.06007381541333719`、态势大屏 `0.10485385738730421`、APT 专题 `0.09128387903290155`，均低于全局阈值 `0.125`；评分区域统一为 `content-root:198,80,1722,917`，公共 AppShell 不参与本批次像素回修。
- 同一 r259 镜像包含部署管理页修复：6 张发布健康 ECharts 均完整显示，发布列表支持第 2 页和稳定纵向滚动，新建发布、行级操作和证据动作均打开可观察 Drawer；`deployments/interaction-r259.json` 运行时错误数组为空，业务 ROI 为 `0.07075855849694188 <= 0.125`。

## r260 仪表盘交互闭环批次

- 当前生产镜像：`traffic/web-ui:ui-dashboard-interactions-20260711-r260`。仪表盘 20 张 KPI、阶段、质量环和 Top Talkers ECharts 继续由 dashboard API/typed fallback 数据驱动；拓扑 SVG 回退和 r259 其他已验收页面保持不变。
- “创建闭环任务”和右侧 5 个缺口动作均接入确认 Drawer；`pageApiPlans.dashboard.actions` 预留任务接口、权限范围、审计事件、默认请求体和防护规则，确认后显示仿真任务队列反馈。
- `dashboard/interaction-r260.json` 验证 20 张 canvas、证据补齐接口与审计事件、确认反馈、8 行第 2 页，且无 4xx/5xx、请求失败、console error 或 page error；静态清单中仪表盘被动按钮数为 `0`。
- r260 仪表盘业务 ROI：`0.11076111695841993 <= 0.125`；证据为 `metrics-business-r260.json` 和 `diff-business-r260.png`，评分区域为 `content-root:198,80,1722,917`。

## r271 数据质量全 Tab 闭环与固定几何批次

- 当前生产镜像：`traffic/web-ui:ui-data-quality-tabs-query-20260711-r271`。数据质量 8 个 Tab 的图表均为页面 API/typed fallback 驱动的动态 ECharts；拓扑类页面继续遵循原始 API 驱动 SVG 的全局例外。
- 页面业务区统一校准到 `content-root:198,80,1722,917`；Topic/Flink 保留 274px 右侧业务栏，其余页面保留 198px 业务栏，通过独立标题栏宽度校准使 8 个 Tab 在所有页面保持同一几何。公共 AppShell 源码与几何未修改。
- `pageApiPlans` 预留 `POST /v1/data-quality/actions`、权限、审计事件 `DATA_QUALITY_ACTION_REQUESTED`、默认请求体与防护规则；8 个 Tab 的业务按钮均可打开可观察 Drawer，不再存在被动按钮。
- Tab 点击仅更新 `tab` 查询参数并保留现有 URL 条件；Windows Chrome 从总览真实点击字段质量后，前三个 Tab `x/y/width/height` 最大差为 `0`。
- Windows Chrome 证据：`evidence/ui-image-breakdowns/pages/data-quality/interaction-r271-all-tabs.json`。8 页 canvas 数依次为 `22/7/11/7/9/7/5/6`，动作、endpoint、audit 均通过，4 类运行时错误数组均为空；逐路由和真实点击两类几何差均为 `0`。
- r271 业务 ROI：总览 `0.12151615440441677`、Topic `0.11704137994799484`、Flink `0.12448878266629683`、字段 `0.11759296904388268`、存储 `0.0939867289310064`、重放 `0.08187963325341308`、报告 `0.10563152835142621`、设置 `0.08981529681319558`，全部严格低于 `0.125`。

## r283 模型管理闭环批次

- 当前生产镜像：`traffic/web-ui:ui-models-page-contract-20260711-r283`。模型指标 7 张趋势图读取 API hidden metrics，特征贡献和样本分布由选中模型 ID 驱动 typed fallback，共 `9` 张动态 ECharts；切换模型的 canvas 变化与同 ID metrics 变化均有测试证据。
- 模型动作已完成前端交互和 API 契约预留；异步仿真 service 与会话审计不能证明服务端动作完成，版本注册、反馈、重训、评估、激活、停用和回滚仍需补齐后端 RBAC、持久化审计与任务查询。
- 模型列表保留真实 API 行和 `model_id` lineage；历史截图使用明确标记的 `SIM-MODEL-*` typed catalog 补足目标容量。该补位只支持视觉验收，真实第二页必须改由 seeded 数据和后端分页返回后才能通过全栈验收。
- `models/interaction-r283.json` 验证 9 张 ECharts、动态切换、第二页 API 参数与空页仿真补位、搜索、解释 Tab、顶部/行级动作、三个底部动作提交、Slider 状态和零运行时错误；业务 ROI `0.07570132875343398 < 0.125`，证据为 `metrics-business-r283.json`、`diff-business-r283.png`。

## r289 MLOps 编排闭环批次

- 当前生产镜像：`traffic/web-ui:ui-mlops-task-detail-20260712-r289`。6 条门禁 sparkline 和 1 张误报率/漂移趋势均为 typed simulation ECharts，7 个 canvas 均完整位于业务视口内，静态 SVG 图表候选为 `0`；该结果只证明视觉阶段通过。
- MLOps 可见业务按钮均绑定具体导航或前端 endpoint 契约，但会话审计和异步回执不能替代服务端 RBAC、持久化任务与数据库审计，当前不构成全栈通过。
- 32 条任务均显示 API 派生/条件派生/仿真模式，6 条/页受控分页；状态筛选、刷新、反馈详情、模型版本详情和行级动作均可用。
- 任务详情使用独立 GET 契约并展示当前任务完整字段；`mlops/interaction-r289.json` 验证 7 张 canvas、6 页、12 类动作、截图有效性、筛选/刷新/导航及零运行时错误；业务 ROI `0.0748254989949806 < 0.125`。

## r302 Campaign 首个全栈 episode

- 生产镜像：Web `traffic/web-ui:ui-campaign-fullstack-20260712-r302`、Alert `traffic/alert-service:campaign-fullstack-20260712-r214`、Ingest `traffic/ingest-gateway:kafka-security-20260712-r3`；Kubernetes 工作负载使用对应 `image@sha256`，双节点本地镜像和 `image-digests.lock.json` 已同步。
- Campaign 列表读取真实 `/api/v1/campaigns` 和 ClickHouse 数据，支持服务端风险、状态、阶段、关键字筛选以及 5/8/10/20 条每页选择。集合动作固定绑定 `campaign-collection`，不接受任意 `metadata.campaign_id`；资源动作以 URL 战役 ID 为准并校验租户内存在性。
- 所有动作均明确为 `simulation=true`、`dry_run=true`，按钮使用“模拟”或“导出当前页”语义，不修改真实战役状态。作业写入 `campaign_action_jobs`，审计写入 `audit_logs`，两次写入使用同一 PostgreSQL 事务；失败路径具有 rollback 单测。
- 风险分布和证据完整度继续使用 ECharts。风险图输入随每页 8 条切换到 5 条而从 `0,8,0` 更新为 `0,5,0`；生产 API 未提供显式证据完整度时显示“待接入”，不再根据不等价的行项目比例伪造 0% 或综合百分比。
- Windows Chrome CDP 证据：`evidence/ui-image-breakdowns/pages/campaigns/interaction-r302-r214-r3.json`。验证 2 张非空可见 canvas、第二页、末页、每页 5 条、4 类服务端筛选、16 个动作、16 个持久化作业、16 条数据库审计、viewer 写操作 403、5 个导航目标和零 4xx/5xx、requestfailed、console/page error。
- 业务区 ROI：视觉目标态 `0.07992848973512325`，生产交互态 `0.07875628374604357`，均严格低于 `0.125`；证据为 `metrics-business-r302-r214-r3.json` 与 `metrics-business-production-r302-r214-r3.json`。
- 本批次通过不代表全系统完成。正式 Windows Codex Desktop Chrome extension 逐页面双门禁仍须由当前会话具备可信扩展工具面后执行；CDP 证据只记录本轮真实 Windows Chrome 路由结果，不替代该全局门禁。
