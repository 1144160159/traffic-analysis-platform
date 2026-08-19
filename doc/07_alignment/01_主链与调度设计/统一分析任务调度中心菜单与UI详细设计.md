# 统一分析任务调度中心菜单与UI详细设计

更新时间：2026-08-16

状态：`DRAFT_UI_CONTRACT / UI_D1_EIGHT_PAGE_VISUAL_CANDIDATE / NOT_IMPLEMENTED / NON_EXECUTION_AUTHORIZATION`

适用链路：采集 → 特征处理 → 加密流量特征识别 → 恶意流量检测 → 机器摘要 → 可选人读报告

## 1. 裁决与设计目标

当前裁决（2026-08-16）：UI详细设计恢复推进，但仅冻结菜单、页面职责、交互、状态、权限、前端函数和视觉基线，不进入生产代码实现。已有三张概念图不再被当作互斥的整站方案，而是按页面职责分别吸收：队列优先骨架用于运行监控，阶段骨架用于任务编排，任务叙事骨架用于运行详情。本轮已新增任务管理、调度管理、任务编排、运行监控、即时分析向导、运行详情、调度资源和报告中心八张核心页面视觉候选；它们仍是设计输入，不是浏览器或运行验收证据。

本设计替换“按自动/人工拆成两套页面”和“把所有技术分析结果堆在首页”的旧思路。产品只保留一条任务主线：

1. `默认方案/自定义方案`只说明计划参数的准备方式；
2. `持续窗口/定时/事件/按需`是另一组独立触发设置；
3. 所有组合进入同一个任务物化、调度、编排、执行、回执和机器摘要链；
4. Run在机器摘要及证据清单持久化后终态；HTML/PDF人读报告独立异步生成；
5. 普通用户围绕“建任务、看进度、读结论、处理异常”操作，Topic、consumer group、checkpoint、Flink UID、lease epoch等进入管理员或技术详情；
6. 复用现有深色公共Shell、可折叠分组导航、Ant Design组件、权限、React Query、表格、Steps、Drawer、Tabs、Descriptions、Alert和Result，不整体重写App；
7. “任务调度”作为一个完整一级业务域，任务管理、调度管理、任务编排、运行监控和调度资源是其二级模块，不再把同一业务域拆散成三个一级菜单。

设计关键词：`一条主线、一个主按钮、少层级、结果渐进披露、状态不猜测、操作可恢复`。

## 2. 用户、场景与成功标准

| 用户 | 首要任务 | 默认可见信息 | 高级信息 |
|---|---|---|---|
| 安全运营人员 | 新建/查看任务，判断是否可信，进入告警或取证 | 阶段、结论、完整性、关键发现、下一动作 | 技术详情按需展开 |
| 任务规划人员 | 维护模板、触发和批准版本 | 方案来源、触发方式、窗口、计划版本 | exact-set、兼容性报告 |
| 平台管理员 | 容量、配额、队列、租约和失败恢复 | 全局健康、积压、异常队列 | lease/fence/worker细节 |
| 审计/管理人员 | 查看机器摘要、人读报告和操作轨迹 | 范围、结论、证据完整性、报告状态 | 全量审计和下载记录 |

核心任务成功标准：运营人员从运行监控进入“发起即时分析”到提交默认方案不超过3个步骤；规划人员能在一个调度业务域内完成定义、调度和编排治理；运行列表一屏可回答“是什么、到哪一步、结论是否可信、下一步做什么”；任一未知、未运行或不完整状态都不能显示成“安全/未发现威胁”。

### 2.1 产品词汇冻结

| 产品词汇 | 唯一含义 | 禁止混称 |
|---|---|---|
| 任务定义 | 可复用的业务目标、范围和治理引用 | 一次实际执行 |
| 调度计划 | 何时、以何种TriggerKind物化任务，并绑定哪个已批准方案 | 分析算法计划 |
| 编排版本 | 固定五段分别使用哪些exact-set组件版本 | 自由DAG、调度规则 |
| 任务实例 | 一次TriggerInstance物化出的AnalysisTask | 任务定义、运行尝试 |
| 运行 | 一个AnalysisTask的一次AnalysisRun执行尝试；列表一行一个Run | 调度计划、报告任务 |
| 机器摘要 | 随Run终局冻结的机器事实和evidence manifest | 可选PDF/HTML |
| 人读报告 | 从机器摘要异步派生的独立制品 | Run终态或第五阶段 |

页面标题继续使用用户已确认的“任务管理 / 调度管理 / 任务编排”，但正文、字段和API必须使用上表对象名。菜单显示名不得反向改变领域对象基数。

## 3. 信息架构与菜单

### 3.1 一级菜单与完整业务域

~~~text
综合态势                    保留现有仪表盘、态势大屏和专题面板
任务调度                    新增完整调度业务域
├─ 任务管理                  管理可复用任务定义及其批准方案
├─ 调度管理                  管理持续窗口、定时、事件触发和启停
├─ 任务编排                  管理固定五段编排、版本和兼容性
├─ 运行监控                  查看Task/Run、阶段、异常、取消和重试
└─ 调度资源                  管理配额、队列、租约和执行器，仅管理员
研判取证                    告警、战役、攻击链、加密流量、取证
数据资产                    探针、数据质量、资产、图谱、融合、基准
检测能力                    特征、规则、模型、MLOps、SOAR、白名单
分析报告                    机器摘要、人读报告
审计配置                    审计、通知、用户权限、系统配置
~~~

“任务调度”是调度中心唯一一级入口；“分析报告”是跨任务的结果消费入口，不与Run终态耦合。现有专业域保留并通过深链或Drawer承接专业操作。菜单迁移不得删除已有路由、权限、操作、字段、导出和收藏深链；“数据资产/检测能力”等名称只是未来分组候选，未完成兼容清单和浏览器回归前不改现有分组。

二级模块与领域对象固定映射：

| 二级模块 | 管理对象 | 不承担的职责 |
|---|---|---|
| 任务管理 | `AnalysisTaskDefinition`、active plan/report policy引用 | 不展示实时lease，不直接推进Run |
| 调度管理 | `AnalysisScheduleRevision`、TriggerInstance查询投影 | 不编辑feature/model exact-set |
| 任务编排 | `AnalysisPlanRevision`、固定StageGraph、CompatibilityReport | 不创建Task，不做任意拖拽DAG |
| 运行监控 | `AnalysisTask`、`AnalysisRun`、BusinessPhase投影、ExecutionNodeAttempt、MachineSummary入口 | 不修改历史plan或schedule |
| 调度资源 | quota、queue、lease、executor capability和健康 | 不替业务用户选择方案 |
| 分析报告 | MachineSummary、HumanReadableReport | 不反向修改RunState |

### 3.2 路由建议

| 页面 | 建议路由 | 菜单可见 | 兼容策略 |
|---|---|---:|---|
| 任务管理 | `/analysis/task-definitions` | 是 | 调度域默认入口；定义与active引用 |
| 新建任务定义 | `/analysis/task-definitions/new` | 否 | 创建第一个DRAFT revision，不用大Modal |
| 任务定义详情 | `/analysis/task-definitions/:definitionId` | 否 | 永久只读聚合详情 |
| 新建定义修订 | `/analysis/task-definitions/:definitionId/revisions/new` | 否 | 从已批准版本复制DRAFT |
| 编辑定义修订 | `/analysis/task-definitions/:definitionId/revisions/:revisionId/edit` | 否 | 仅DRAFT可编辑，历史revision只读 |
| 调度管理 | `/analysis/schedules` | 是 | Schedule revision、触发历史和启停 |
| 调度详情 | `/analysis/schedules/:scheduleId` | 否 | 当前引用、触发历史和审计只读 |
| 新建调度修订 | `/analysis/schedules/:scheduleId/revisions/new` | 否 | 复制为DRAFT后编辑 |
| 任务编排 | `/analysis/orchestrations` | 是 | Plan revision、固定五段和兼容性 |
| 编排修订详情 | `/analysis/orchestrations/:planId/revisions/:revisionId` | 否 | 历史只读；DRAFT通过显式编辑动作进入工作区 |
| 运行监控 | `/analysis/runs` | 是 | 一行一个AnalysisRun及异常处置 |
| 发起即时分析 | `/analysis/runs/new` | 否 | 从运行监控唯一主按钮进入 |
| 运行详情 | `/analysis/runs/:runId` | 否 | 任务行、告警、报告均可深链 |
| 调度资源 | `/analysis/resources` | 是，管理员 | 配额、队列、lease和执行器 |
| 分析报告 | `/analysis/reports` | 是 | 机器摘要/人读报告双页签 |
| 第一版候选别名 | `/analysis/tasks`、`/analysis/policies` | 否 | 分别重定向到`/analysis/runs`和`/analysis/task-definitions`并保留query/hash |
| 现有专业路径 | 现有路径 | 保留 | 由任务/结果深链进入，绝不先删除 |

### 3.3 全量菜单顺序与兼容迁移矩阵

目标顺序冻结为：<code>综合态势 → 任务调度 → 研判取证 → 数据资产 → 检测能力 → 分析报告 → 审计配置</code>。首轮只新增<code>任务调度</code>和<code>分析报告</code>，现有六组及其route ID/path保持原样；第二轮通过route manifest alias重组显示分组，路由本身不迁移。这样菜单名可以按业务调整，同时收藏、外链、权限、导出和浏览器历史不失效。

| 当前分组/route ID | 目标分组 | 目标显示名 | Path策略 | 首轮状态 |
|---|---|---|---|---|
| 综合态势：dashboard/screen/topics | 综合态势 | 仪表盘/态势大屏/专题面板 | 原path不变 | 保留 |
| 新增：analysis-task-definitions | 任务调度 | 任务管理 | `/analysis/task-definitions` | 新增、feature flag |
| 新增：analysis-schedules | 任务调度 | 调度管理 | `/analysis/schedules` | 新增、feature flag |
| 新增：analysis-orchestrations | 任务调度 | 任务编排 | `/analysis/orchestrations` | 新增、feature flag |
| 新增：analysis-runs | 任务调度 | 运行监控 | `/analysis/runs` | 新增、feature flag |
| 新增：analysis-resources | 任务调度 | 调度资源 | `/analysis/resources` | admin-only、feature flag |
| 威胁分析：alerts/campaigns/attack-chains/encrypted-traffic/forensics | 研判取证 | 告警中心/战役列表/攻击链分析/加密流量/取证分析 | 原path不变，只改group title | 第二轮候选 |
| 采集监测：probes/data-quality | 数据资产 | 探针管理/数据质量 | 原path不变，只改group | 第二轮候选 |
| 资产图谱：assets/graph/fusion/baselines | 数据资产 | 资产台账/实体图谱/数据融合/行为基准 | 原path不变，只改group | 第二轮候选 |
| 检测运营：rules/deployments/models/mlops/playbooks/whitelist | 检测能力 | 规则管理/部署管理/模型管理/MLOps编排/SOAR剧本/白名单 | 原path不变，只改group title | 第二轮候选 |
| 新增：analysis-reports | 分析报告 | 分析报告 | `/analysis/reports?tab=machine|human` | 新增、feature flag |
| 审计配置：compliance/audit-log/notifications/settings | 审计配置 | 合规审计/审计日志/通知配置/系统设置 | 原path不变 | 保留 |

菜单分组只是显示投影，不是授权边界。第二轮把<code>collection-monitoring + asset-graph</code>合并显示为“数据资产”时，原domain ID继续作为兼容alias，权限仍读取每条route的<code>requiredScopes</code>；不得以新group scope替代原route scope。<code>activeNavId</code>必须由动态详情路由回映到二级菜单，且折叠/展开状态按用户保存，不由当前页面强制覆盖。

## 4. 运行监控

### 4.1 页面层级

~~~text
全局Shell
  └─ 任务调度分组展开 + 运行监控选中
      └─ 页面标题 + 一句话说明 + 发起即时分析（唯一主按钮）
      ├─ 三项轻摘要：运行中 / 需关注 / 今日完成
      ├─ 搜索与四个常用筛选：状态、方案、触发、时间
      └─ Run表格
          ├─ 行点击进入运行详情
          ├─ 默认不常驻右侧详情栏
          └─ 取消、重试、申请报告进入行菜单或详情页
~~~

禁止在首屏堆叠模型准确率、风险分布、协议占比、流量趋势等技术图；禁止默认打开占据三分之一屏幕的详情栏。这些内容只在选中Run的分析结果或分析报告中出现。表格保持主区完整，窄Drawer只用于快速查看错误原因或确认操作。

### 4.2 列表字段

最多八列：

| 列 | 内容 | 交互 |
|---|---|---|
| 任务定义/目标 | 定义名、目标范围、负责人、Task/Run短ID | 名称是真实Link；整行点击仅作增强 |
| 方案 | `默认方案`或`自定义方案` + revision | 可筛选，不表达执行模式 |
| 触发 | 持续窗口/定时/事件/按需 | 与方案独立显示 |
| 运行状态/阶段 | RunState文字 + 唯一一条五段紧凑进度 | 失败显示原因摘要；不靠颜色表达 |
| 机器结论 | FindingConclusion业务文案；仅THREAT_FOUND时可附独立RiskSeverity | 未终态显示“待形成”；风险不冒充结论 |
| 证据状态 | Completeness + IntegrityState分别显示 | 未验证、部分、失败均不能显示成功绿 |
| 更新时间 | 最近权威状态时间 | 创建/开始时间可聚焦查看，不能只依赖hover |
| 操作 | 查看、取消/重试、申请报告 | 只展示当前`allowedActions`允许项；危险操作二次确认 |

五段业务步骤固定为：

~~~text
数据采集 → 特征处理 → 加密特征识别 → 恶意流量检测 → 机器摘要
~~~

人读报告不出现在该步骤条中，单独显示：`未申请 / 排队中 / 生成中 / 对象校验中 / 可下载 / 失败`。只有<code>AVAILABLE</code>显示下载动作；<code>VERIFYING</code>不得因对象路径已返回而提前可下载。

### 4.3 状态与空态

| 条件 | 文案 | 视觉/动作 |
|---|---|---|
| 没有可用批准定义 | 暂无可发起的任务定义 | 有写权限者“前往任务管理”；无权限者提示联系任务规划人员 |
| 有批准定义但没有Run | 尚无运行记录 | 主按钮“发起即时分析” |
| 筛选无结果 | 没有符合当前条件的运行 | “清除筛选”并保留未应用的输入 |
| 阶段等待 | 等待上游回执 | 中性色，不使用成功绿 |
| 输入为空 | 本窗口没有有效输入 | `INCONCLUSIVE`，可查看范围 |
| 依赖不可达 | 数据源或执行器不可用 | `ERROR`，展示重试/诊断 |
| Run成功且明确阴性 | 未检出已选检测器定义的威胁 | 必须同时显示范围与完整性 |
| 未知enum/revision倒退 | 状态无法确认 | fail closed，禁止关键操作 |

## 5. 发起即时分析向导

### 5.1 三步流程

| 步骤 | 用户回答的问题 | 核心字段 |
|---|---|---|
| 1 选择任务与范围 | 分析哪个批准任务定义、哪个时间窗或对象 | task definition、source summary、target、window、硬上限 |
| 2 选择方案 | 使用批准默认方案，还是提交允许的定制覆盖 | plan source、active/base plan、override diff、报告策略 |
| 3 校验并提交 | 当前计划是否可执行、是否需要审批、是否立即运行 | preflight、兼容性、资源影响、审批状态、幂等提交 |

默认方案在第2步只显示可读摘要和“查看完整方案”Drawer，不要求运营人员逐项选择feature/model/rule。只有选择自定义方案时，才展开四组允许覆盖项：数据源限制、特征集、加密识别、恶意检测；未授权字段只读并明确“继承自模板”。

### 5.2 两组正交控制

方案准备方式：

- `使用默认方案`：从批准模板解析exact版本；
- `自定义方案`：仅开放有权限的字段，未覆盖字段明确继承模板。

即时向导的TriggerKind固定为`ON_DEMAND`。持续窗口、定时和事件触发在“调度管理”创建，避免用户在即时向导中同时理解计划准备与长期调度。正交性仍保留：默认方案和自定义方案都可以按需运行，也都可以在调度管理中绑定持续/定时/事件触发。

UI不得把方案来源与触发方式合并成“自动任务/人工任务”。所有组合都必须经过权限、preflight、Compiler和统一物化事务；前端不能通过直接调用Probe、Flink、模型或报告接口拼装运行。

### 5.3 操作规则

- 顶部Steps只表示表单进度，不假装任务已经执行；
- 每步只回答一个业务问题，高级参数放Drawer；
- 返回上一步保留输入；catalog revision变化时明确提示重新校验；
- 提交前展示可读摘要、影响范围和兼容性结果；
- 客户端提交前生成idempotency key；超时显示“提交结果待确认”并用原key恢复；
- 默认方案且已有批准计划时，最后主按钮为“提交并开始分析”；
- 自定义方案若要求maker/checker，最后主按钮为“提交审批”，审批通过后才显示“开始分析”；
- 只有最后一步出现提交类主按钮，其它步骤使用“下一步”。

## 6. 任务调度中心五个模块

五个模块在同一“任务调度”分组中展开，不再散落为三个一级菜单。每个页面只承担一个聚合的管理职责。

| 模块 | 面向对象 | 首屏核心内容 | 唯一主动作 |
|---|---|---|---|
| 任务管理 | 规划人员 | 定义名、目标摘要、active plan、active schedules、状态、owner | 新建任务定义 |
| 调度管理 | 规划/管理员 | 任务、已批准方案、触发类型、窗口/cron/event、状态、下次触发、最近结果 | 新建调度计划 |
| 任务编排 | 规划人员 | plan revision、五段编排、兼容性、审批状态、使用定义数 | 新建编排版本 |
| 运行监控 | 运营人员 | 一行一个Run、五段进度、机器结论、证据状态、异常和下一动作 | 发起即时分析 |
| 调度资源 | 管理员 | 配额、队列积压、租约健康、执行器能力和阻塞原因 | 调整资源策略 |

### 6.1 任务管理

列表最多七列：任务定义、目标、当前方案、调度数、最近运行、状态、操作。点击进入定义工作区，使用“基本信息 / 方案版本 / 调度计划 / 报告策略 / 审计记录”五个Tab；Tab只切换同一定义的治理信息，不混入实时运行图。激活、挂起、退休必须使用确认面板并提交expected revision、reason和idempotency key，不能用本地Switch直接变色。

### 6.2 调度管理

列表默认按“下次触发时间”排序，展示TriggerKind的业务文案，不显示cron解析串以外的底层技术参数。新建调度使用四步短表单：

1. 选择任务定义，以来源开关筛选默认/自定义方案，并选择一个已批准plan revision/hash；
2. 选择TriggerKind并配置持续窗口、Cron或事件条件；
3. 配置时区、窗口、并发、重叠策略、misfire策略和调度类别；
4. 查看影响、兼容性、preflight有效期和审批要求后提交。

ScheduleRevision必须保存方案绑定，但不重复保存plan source；UI从被绑定的不可变plan读取来源。激活后每个TriggerInstance仍冻结exact plan revision/hash，不能在触发时偷偷跟随新的active plan；要升级方案必须新建调度revision并重新审批。暂停只影响未来TriggerInstance；页面固定提示“仅停止未来触发；已物化任务和正在运行的分析不受影响”。最近触发记录进入下方Tab或Drawer，不在每行塞入时间线。

### 6.3 任务编排

中期只允许复制批准模板并形成新revision；不提供任意拖拽DAG。主画布固定显示“数据采集→特征处理→加密特征识别→恶意流量检测→机器摘要”，PlanReady与Reconcile以细小技术闸门标记，不允许删除。点击某阶段只打开右侧配置Drawer，展示批准组件、exact version/hash、依赖、兼容性和变更diff。新增阶段必须走合同、状态、回执、兼容、恢复和证据评审后再进入模板。

### 6.4 运行监控

运行监控采用队列优先骨架，见第4章。默认只展示运行列表；选择行后进入独立详情路由。批量操作只允许“导出当前筛选”和管理员有权执行的安全动作，不提供批量取消或批量重试默认入口。

### 6.5 调度资源

仅管理员可见，分“容量配额 / 队列 / 租约 / 执行器”四个Tab。首屏只显示是否健康、哪里阻塞、影响多少Run和推荐动作；高基数ID、fencing token、worker心跳进入详情Drawer。资源修改必须展示影响预览、受影响tenant/class、回滚值和审批要求。

## 7. 运行详情

### 7.1 概览页

顶部只保留：任务名、权威状态、五段进度、取消/重试、更多操作。内容区回答六个问题：

1. 分析范围是什么；
2. 当前执行到哪里；
3. 最终机器结论是什么；
4. 数据和证据是否完整；
5. 关键发现与限制是什么；
6. 人读报告处于什么状态。

### 7.2 四个页签

| 页签 | 默认内容 | 不放在这里的内容 |
|---|---|---|
| 概览 | 范围、阶段、结论、完整性、关键发现、报告状态 | 全量向量/receipt |
| 分析结果 | detection outcome、协议/家族、风险分布、时间线 | 调度与lease细节 |
| 证据 | EvidenceManifest、对象、hash、来源、下载审计 | 原始系统配置 |
| 技术详情 | plan exact-set、模型/规则版本、fence/count/watermark、receipt | 面向管理者的长篇报告 |

复杂结果优先用表格、分组和渐进展开；单页默认最多一张风险分布图、一张时间线和一张证据完整性图。

## 8. 分析报告

### 8.1 机器摘要

随Run终态冻结，默认可直接打开，字段为：范围、终态、总体结论、完整性、结果分布、关键发现、限制、summary hash、evidence manifest hash。机器摘要是API/UI和后续人读报告的唯一冻结输入。

### 8.2 人读报告

按需或策略异步生成。列表展示Run、机器摘要hash、模板/语言、状态、请求人、更新时间和下载操作。报告失败提供“重试生成”，但不显示Run失败。

正文只保留执行摘要、范围、总体结论、完整性、关键发现、关键证据和限制；全量特征、模型输出、规则版本和receipt进入附录。

## 9. 关键前端合同（PLANNED）

~~~ts
type PlanSource = 'AUTO_DEFAULT' | 'MANUAL_CUSTOM'
type TriggerKind = 'CONTINUOUS_WINDOW' | 'CRON_WINDOW' | 'EVENT_DRIVEN' | 'ON_DEMAND'
type RunState =
  | 'ACCEPTED' | 'PREPARING' | 'QUEUED' | 'RUNNING' | 'FINALIZING'
  | 'SUCCEEDED' | 'PARTIALLY_SUCCEEDED' | 'FAILED'
  | 'CANCEL_REQUESTED' | 'CANCELLED'
type DetectorDisposition =
  | 'POSITIVE' | 'NEGATIVE' | 'INCONCLUSIVE'
  | 'INCOMPATIBLE' | 'ERROR' | 'NOT_RUN'
type FindingConclusion =
  | 'THREAT_FOUND' | 'NO_THREAT_OBSERVED' | 'INCONCLUSIVE'
  | 'NO_DATA' | 'NOT_EVALUATED'
type RiskSeverity = 'CRITICAL' | 'HIGH' | 'MEDIUM' | 'LOW' | 'NONE' | 'UNKNOWN'
type Completeness = 'UNKNOWN' | 'COMPLETE' | 'PARTIAL' | 'INCOMPLETE'
type IntegrityState = 'UNVERIFIED' | 'VERIFIED' | 'FAILED'
type ReportState =
  | 'NOT_REQUESTED' | 'QUEUED' | 'GENERATING' | 'VERIFYING'
  | 'AVAILABLE' | 'FAILED' | 'CANCELLED'
type AnalysisProductStage =
  | 'ACQUISITION' | 'FEATURE' | 'ENCRYPTED_RECOGNITION'
  | 'THREAT_DETECTION' | 'MACHINE_SUMMARY'
type AnalysisRunAction =
  | 'VIEW' | 'CANCEL' | 'RETRY_STAGE' | 'RETRY_TASK'
  | 'RELAUNCH_WITH_NEW_PLAN' | 'REQUEST_REPORT' | 'RETRY_REPORT'
  | 'DOWNLOAD_REPORT' | 'OPEN_ALERTS' | 'OPEN_FORENSICS'

interface AnalysisRunRowViewModel {
  taskId: string
  runId: string
  title: string
  targetSummary: string
  planSource: PlanSource
  planRevision: string
  triggerKind: TriggerKind
  currentStage: AnalysisProductStage | null
  runState: RunState
  findingConclusion: FindingConclusion | 'PENDING'
  highestRiskSeverity: RiskSeverity
  completeness: Completeness
  integrityState: IntegrityState
  reportState: ReportState
  authorityRevision: string
  snapshotHash: string
  allowedActions: AnalysisRunAction[]
}
~~~

<code>PENDING</code>只属于前端展示层，不得写回后端FindingConclusion；DetectorDisposition只用于输入×detector明细，不得直接充当Run总体结论。<code>FAILED</code>属于Run、Integrity或Report状态，不是Completeness值。decoder必须分别验证RunState、FindingConclusion、RiskSeverity、Completeness、IntegrityState和ReportState，不能通过一组字段推导或覆盖另一组字段。<code>currentStage=null</code>表示尚未进入产品阶段或已经终态，页面应结合RunState显示“准备中/已结束”，不得补一个虚构阶段。

函数边界：

~~~text
analysisSchemas.decodeTaskDefinitionPage/decodeSchedulePage/decodeRunPage/decodeRunDetail/decodeReportPage
analysisQueryKeys.taskDefinitions/schedules/orchestrations/runs/run/reports
buildOnDemandAnalysisIntent
preflightOnDemandAnalysis
submitOnDemandAnalysis
recoverAnalysisOperation
acceptAuthoritySnapshot
useAnalysisTaskDefinitions
useAnalysisSchedules
useAnalysisOrchestrations
useAnalysisRunList
useAnalysisRun
deriveAnalysisRunRowViewModel
deriveAnalysisRunOverviewViewModel
deriveHumanReportViewModel
AnalysisTaskDefinitionsPage
AnalysisScheduleManagementPage
AnalysisOrchestrationPage
AnalysisRunMonitorPage
AnalysisOnDemandWizardPage
AnalysisRunDetailPage
AnalysisResourceManagementPage
AnalysisReportCenterPage
~~~

query key必须含tenant、session epoch和run/filters；revision用十进制字符串或BigInt比较；低revision拒绝，同revision异hash视为integrity failure；unknown enum不得映射到默认成功态。

## 10. 视觉与组件规范

### 10.1 延续现有系统

- 目标视口：1920×1080，DPR=1，Windows Chrome；同步校验1440宽和移动断点；
- 保留深海军蓝全局Shell、左侧导航、青色主强调和现有状态色；
- 使用现有Ant Design Typography、Button、Table、Steps、Tag、Tabs、Drawer、Alert、Result、Descriptions、Skeleton和Empty；
- 8px基础间距；Shell外层gutter保持现有实现，任务页内容面板padding 16–24px、面板间gutter 16–24px；正文14/22、表格13px、页面标题24/32；
- 不新增另一套图标、字体、圆角或卡片体系。

### 10.2 降低复杂度

- 页面只有一个主按钮；
- 首屏摘要最多3项，不做整排KPI卡；
- 一个容器只表达一个层级，避免卡片套卡片；
- 任务进度用一条五段Steps，不重复出现多个流程图；
- 专业结果在详情页按需展开，不在任务列表展示完整算法输出；
- 技术ID默认缩写，完整值支持复制；
- 颜色必须配合文字/图标，不仅依靠颜色传达状态。

## 11. 响应式与可访问性

| 宽度 | 布局 |
|---|---|
| ≥1600 | 完整侧栏、八列表格、行内五段进度 |
| 1200–1599 | 压缩列宽，负责人并入任务信息，次要操作进菜单 |
| 901–1199 | 复用现有156px窄侧栏，表格转主从列表，筛选进入Drawer |
| ≤900 | 复用现有移动导航Drawer，页面单列；五段编排改为纵向 |
| ≤640 | 单列运行卡，只保留状态、阶段、机器结论、证据状态和主动作 |

键盘焦点可见；所有Drawer/Modal支持焦点陷阱、Esc关闭和触发点焦点返回；表格排序/筛选有可读标签；状态更新仅在事实变化时使用非打断式live region；危险操作写明对象与影响；点击目标桌面不小于36px，移动端按44px设计。运行名必须是真实`<Link>`，五段Steps同时输出阶段名和状态文本，并给当前阶段`aria-current="step"`。

## 12. 既有UI图的页面级裁决与资产位置

三套图不再要求整站“三选一”。它们分别验证了三种信息骨架，本轮按页面职责组合使用：

1. 运行队列优先：作为`运行监控`桌面基线，但默认关闭常驻右栏，保持列表为绝对主区；
2. 阶段泳道优先：只吸收到`任务编排`和运行详情中的五段流程，不作为运行监控首页；
3. 任务叙事优先：作为`运行详情/概览`基线，不作为高密度任务列表。

上一轮骨架资产已经保存：

| 方向 | 文件 |
|---|---|
| 运行队列优先 | `doc/04_assets/ui_suite_gpt_v1/screens/concepts/task-center/task-center-run-queue-priority-20260816.png` |
| 阶段泳道优先 | `doc/04_assets/ui_suite_gpt_v1/screens/concepts/task-center/task-center-stage-lane-priority-20260816.png` |
| 任务叙事优先 | `doc/04_assets/ui_suite_gpt_v1/screens/concepts/task-center/task-center-run-narrative-priority-20260816.png` |

这些图的左侧菜单仍是上一轮信息架构，只保留为设计沿革和骨架参考，不再作为实现坐标。

### 12.1 本轮八页核心视觉候选

统一目录：`doc/04_assets/ui_suite_gpt_v1/screens/pages/analysis-scheduling/`。

| 页面 | 视觉候选 |
|---|---|
| 任务管理 | `analysis-task-management-20260816.png` |
| 调度管理 | `analysis-schedule-management-20260816.png` |
| 任务编排 | `analysis-orchestration-20260816.png` |
| 运行监控 | `analysis-run-monitor-20260816.png` |
| 即时分析向导 | `analysis-on-demand-wizard-20260816.png` |
| 运行详情 | `analysis-run-detail-20260816.png` |
| 调度资源 | `analysis-resource-management-20260816.png` |
| 报告中心 | `analysis-report-center-20260816.png` |

八张图按1920×1080产品方向构图，生成器实际输出均为1672×941的同宽高比预览。它们已经采用新的任务调度菜单和简化信息层级，但仍可能存在字体渲染、字段合并、Shell尺寸近似和底部状态栏省略；字段与状态以本文为真源。只有在真实1920×1080 CSS viewport完成参考图与浏览器截图同视口Design QA，才可晋级实现视觉基线。目录内`README.md`和`PROMPT_MANIFEST.md`记录资产边界和提示词合同。

### 12.2 三图逐项吸收与拒绝矩阵

| 视觉事实 | 吸收 | 拒绝/修正 |
|---|---|---|
| 队列优先图的完整表格、筛选和单一主按钮 | 用于运行监控 | 默认常驻右侧详情栏删除；行点击进入独立详情路由 |
| 队列优先图把“低风险62%”放在同一单元格 | 仅保留一行摘要的紧凑性 | FindingConclusion、RiskSeverity、Completeness和IntegrityState必须有独立标签与字段语义 |
| 阶段优先图的五段大卡 | 用于任务编排版本预览 | 不放运行监控首屏；不把人读报告混成第五阶段以外的运行状态 |
| 阶段优先图的数字步骤 | 保留业务顺序 | 产品文案固定为数据采集/特征处理/加密特征识别/恶意流量检测/机器摘要；S0/S5只作细闸门 |
| 叙事优先图的单Run摘要 | 用于运行详情概览 | 左侧128条任务列表不与详情永久同屏；详情通过路由进入并支持返回恢复 |
| 三图现有“任务中心/策略与模板/综合报告”导航 | 不吸收 | 改为任务调度一级域及五个二级模块；分析报告独立一级入口 |
| 三图顶部全局Shell和深色视觉语言 | 复用 | 页面区不再重复一整排KPI卡；任务页轻摘要最多3项 |
| 三图的“人读报告生成中/可下载” | 保留独立报告区 | 增加“对象校验中”；VERIFYING期间没有下载按钮 |

### 12.3 UI-D1最终套图的统一画面合同

- Canvas为1920×1080 CSS像素、DPR=1；页面内容不得依赖生成器缩放后的1672×941坐标；
- 顶部Shell、全局状态条、用户区和现有图标语言保持一致；任务页面只设计Shell以下区域；
- 左侧导航在所有八张图中使用同一顺序和相同展开状态，当前页面对应二级项用青色左边线与浅色底高亮；
- 页面标题24px、说明14px；唯一主按钮位于标题区右侧；筛选区不超过一行，低频筛选进入Drawer；
- 表格首屏至少显示8条真实业务行；不使用Lorem ipsum、纯占位卡或无语义随机数字；
- 方案来源与触发方式使用不同Tag列；不得出现“自动任务/人工计划”分类；
- 五段进度只出现一次；报告状态不进入五段进度；机器结论、风险严重度、完整性、完整性校验和报告状态使用不同字段；
- loading、empty、read error、403、transport unknown、integrity failure至少各形成一张状态注释，不要求全部塞进主视觉图；
- 图中文字、表格列、按钮和状态必须能映射到第20章API及第21章函数合同，无法映射的装饰信息删除。

## 13. 迁移顺序

1. 新增runtime decoder、query keys、authority snapshot guard和只读view model；
2. 在`routeManifest`新增`任务调度`分组及五个受权限控制的路由，先使用只读占位页，旧页面不删除；
3. 依次接入任务管理、运行监控和运行详情真实API；
4. 接入调度管理和固定五段任务编排，所有写操作先走preflight和revision guard；
5. 接入即时分析默认方案，再接自定义方案maker/checker流程；
6. 接入机器摘要/人读报告双状态和调度资源管理员页；
7. 为第一版候选`/analysis/tasks`、`/analysis/policies`保留重定向；
8. 路由兼容、权限、收藏、导出和浏览器回归通过后，逐个隐藏重复菜单；
9. 观察期结束并确认无回退需求后，才提议删除重复入口；删除仍需单独授权。

## 14. 验收清单

- [ ] 默认/自定义方案与四种触发方式可正交组合；
- [ ] 所有触发经同一物化API和状态机；
- [ ] 列表八列以内且仅一个主按钮；
- [ ] 五阶段步骤以机器摘要结束；
- [ ] 人读报告状态独立，失败不回退Run；
- [ ] 空输入、不可达、未运行和unknown不显示为安全；
- [ ] 任务管理、调度管理、任务编排、运行监控、调度资源、即时向导、运行详情、分析报告均有loading/empty/error/permission/transport unknown状态；
- [ ] 旧路由、深链、权限、字段、动作和导出未减少；
- [ ] 1920×1080、1440宽和移动布局通过视觉回归；
- [ ] 键盘、焦点、对比度、状态非颜色依赖和危险操作确认通过；
- [ ] 新页面套图已按页面职责吸收三种骨架，并修正“任务调度”菜单；
- [ ] 真实API、浏览器、回滚和观察证据产生前保持`NOT_IMPLEMENTED`。

## 15. 当前非声明

本文件已经形成完整调度业务域、菜单、页面职责、状态、组件、前端合同和迁移候选，但不证明前端已实现、真实API已联调、浏览器已验收或项目已完成。三套UI图只作为页面骨架参考；任何一张图都不能作为运行证据。

## 16. 页面蓝图与信息密度合同

### 16.1 任务管理页

~~~text
页面标题：任务管理                         [新建任务定义]
说明：管理可复用的分析目标、批准方案和未来调度绑定

搜索 + 状态 + Owner + 最近运行时间                         [重置]
----------------------------------------------------------------
任务定义 | 目标摘要 | 当前方案 | 调度数 | 最近运行 | 状态 | 操作
----------------------------------------------------------------
                                              分页 + 每页条数
~~~

首屏不显示Run五段进度和模型结果。点击行进入定义详情；行菜单只放查看、复制、激活/挂起和审计，不放“立即删除”。退休是带影响预览的权威命令，删除不进入首版。

### 16.2 调度管理页

~~~text
页面标题：调度管理                         [新建调度计划]
说明：决定批准任务在何时被触发，不改变任务的分析方案

搜索 + 方案来源 + 触发方式 + 状态 + 下次触发时间             [重置]
----------------------------------------------------------------
调度/rev | 任务定义 | 方案来源/rev | 触发/规则 | 下次触发 | 最近物化 | 生命周期 | 操作
----------------------------------------------------------------
~~~

启停不使用无确认Switch。点击“暂停”打开窄确认Drawer，明确列出：未来Trigger停止、PENDING Trigger仍会物化、已运行Run不受影响、expected revision和恢复动作。调度创建第一步必须同时选择任务定义和一个已批准plan revision/hash；plan source只是该方案的可读来源。激活后的ScheduleRevision不得跟随definition的active plan漂移。

### 16.3 任务编排页

~~~text
左：方案版本列表        中：固定五段编排画布          右：阶段配置Drawer（按需）

rev / 状态 / owner      数据采集 -> 特征处理 -> 加密识别 -> 恶意检测 -> 机器摘要
审批记录                技术闸门：PlanReady、Reconcile只读且不可删除
使用任务数              兼容性：通过 / 有条件 / 不兼容
~~~

画布不允许自由连线、删除required stage或把人读报告拖入DAG。主动作“新建编排版本”本质是复制已批准版本后生成DRAFT；编辑只发生在新revision，历史版本永远只读。

### 16.4 运行监控页

~~~text
页面标题：运行监控                         [发起即时分析]
运行中 18     需关注 7     今日完成 26

搜索 + 状态 + 方案来源 + 触发方式 + 时间                   [重置]
--------------------------------------------------------------------------------
任务/目标 | 方案 | 触发 | 状态/阶段 | 机器结论 | 证据状态 | 更新时间 | 操作
--------------------------------------------------------------------------------
~~~

桌面默认没有常驻右栏。机器结论使用FindingConclusion，THREAT_FOUND时才附独立RiskSeverity；证据状态分别显示Completeness和IntegrityState。轻量错误Drawer只显示失败摘要、最近权威时间、建议动作和“进入详情”；关闭后不丢列表筛选。行点击进入运行详情，返回时恢复原cursor、筛选和滚动位置。

### 16.5 调度资源页

顶部最多三项摘要：队列是否积压、租约是否健康、执行器是否就绪。下方四个Tab各用一张表，不做资源拓扑大屏。修改配额、权重或保底容量必须先显示before/after、受影响范围、审批要求和回滚值。

### 16.6 分析报告页

页签固定为“机器摘要 / 人读报告”。机器摘要页按Run终态和完整性筛选；人读报告页按ReportState筛选。两者都深链到同一Run，但状态、重试和下载动作完全独立。报告列表不重复五段运行进度，只显示Run终态、summary hash、报告状态和操作。

## 17. 用户旅程与操作闭环

### 17.1 默认方案即时分析

~~~mermaid
flowchart LR
  A[运行监控] --> B[发起即时分析]
  B --> C[选择任务定义与范围]
  C --> D[使用默认方案]
  D --> E[Preflight]
  E -->|通过| F[提交并开始分析]
  E -->|不通过| G[显示具体阻断和修复入口]
  F --> H{HTTP结果}
  H -->|202| I[进入运行详情]
  H -->|503 outcome unknown| J[保留原key并恢复查询]
  J --> I
~~~

默认路径不展示feature/model/rule逐项编辑。用户需要知道的是“使用哪个批准方案、覆盖范围、资源影响和是否可执行”。

### 17.2 自定义方案即时分析

~~~mermaid
flowchart LR
  A[选择任务与范围] --> B[切换自定义方案]
  B --> C[仅展开有权覆盖字段]
  C --> D[Preflight + 兼容性diff]
  D --> E{是否需审批}
  E -->|是| F[提交审批]
  F --> G[审批通过]
  G --> H[开始分析]
  E -->|否| H
  H --> I[同一OnDemand Trigger与Materializer]
~~~

自定义方案不产生另一套运行页面。审批等待状态留在任务定义/编排工作区；未批准时不得出现伪“运行中”。

### 17.3 定时任务治理

规划人员先在任务管理选择定义，再在调度管理创建ScheduleRevision。保存DRAFT不触发；激活后才产生未来TriggerInstance。暂停、恢复和退休都在调度管理完成；取消当前Run必须跳转运行详情并调用CancelRun，两个动作不能合并成一个按钮。

### 17.4 失败恢复

运行失败时默认推荐动作由服务端`allowedActions`决定：阶段可重试则显示“重试当前阶段”；只允许整任务重试则显示“重试任务”；需要换方案时显示“以新方案重新发起”，后者创建新Task而不是Retry。UI不能仅凭RunState自行推断动作。

## 18. 权威状态、页面状态与文案矩阵

### 18.1 页面通用状态

| 状态 | 页面表现 | 允许动作 |
|---|---|---|
| `LOADING` | 保留表头/页面骨架的Skeleton | 仅允许离开页面 |
| `READY` | 展示权威revision和generated_at | 依`allowedActions` |
| `EMPTY` | 解释为何为空；有权限时给唯一CTA | 新建或清除筛选 |
| `READ_ERROR` | Alert说明读取失败，保留筛选 | 重试读取 |
| `UNAUTHENTICATED` | 401会话失效，不渲染受保护对象 | 重新登录后恢复安全route state |
| `FORBIDDEN` | 403页，列出所需scope但不泄露对象 | 返回有权页面 |
| `NOT_FOUND` | 404对象不存在、已退休或无权区分时使用统一文案 | 返回列表；不猜测对象内容 |
| `REVISION_CONFLICT` | 409显示当前revision和用户变更未提交状态 | 刷新比较或复制为新revision |
| `STALE_PREFLIGHT` | 黄色阻断条，说明哪些revision漂移 | 重新校验 |
| `VALIDATION_ERROR` | 422逐字段说明错误并聚焦首个错误 | 修正后重新预检 |
| `QUOTA_LIMITED` | 429展示限制类别、可重试时间和申请入口 | 不本地绕过限额 |
| `TRANSPORT_OUTCOME_UNKNOWN` | 持久结果面板，禁止重复新key提交 | 用原key恢复 |
| `INTEGRITY_FAILURE` | 红色阻断页/Drawer，锁定关键动作 | 复制trace ID、联系管理员 |
| `UNKNOWN_ENUM` | “状态无法确认”，不用默认图标和成功色 | 刷新或上报 |
| `STALE_SNAPSHOT` | 保留旧数据并标明生成时间，不冒充当前事实 | 刷新；写操作锁定到新snapshot |

### 18.2 结论文案

| 后端事实 | 面向用户文案 | 禁止文案 |
|---|---|---|
| `THREAT_FOUND + COMPLETE` | 检出威胁，证据完整 | 系统异常 |
| `NO_THREAT_OBSERVED + COMPLETE` | 在所选检测范围内未检出威胁 | 网络安全、绝对安全 |
| `INCONCLUSIVE` | 当前证据不足，无法形成确定结论 | 未发现威胁 |
| `NO_DATA` | 本窗口没有有效输入，无法形成威胁结论 | 未发现威胁、检测通过 |
| `NOT_EVALUATED` | 分析未完成，不能形成总体结论 | 正常、通过 |
| detector `INCOMPATIBLE` | 相关检测器与输入不兼容，查看未评估范围 | 正常 |
| detector `ERROR/NOT_RUN` | 相关检测失败或未执行，查看技术详情 | 无告警、通过 |
| Run终态且Report校验中 | 分析已完成，人读报告正在校验 | 可下载 |
| Run终态且Report失败 | 分析已完成，人读报告生成或校验失败 | 分析失败 |
| source fence证明有效输入为0 | 由zero-input policy决定RunState；机器结论为NO_DATA | 未发现威胁、检测通过 |

### 18.3 Authority snapshot接收规则

~~~ts
function acceptAuthoritySnapshot<T extends AuthoritySnapshot>(
  current: T | undefined,
  incoming: T,
): { state: 'ACCEPTED' | 'IGNORED_STALE' | 'INTEGRITY_FAILURE'; value?: T }
~~~

内部顺序固定：校验tenant/session epoch和aggregate identity；用BigInt比较十进制revision；低revision忽略并记录受控诊断；同revision同hash精确重放；同revision异hash进入完整性失败并冻结写操作；高revision验证状态不倒退后接收。query cache不能以到达顺序覆盖authority顺序。

## 19. 权限、动作与审计合同

建议scope保持领域清晰：

~~~text
analysis:definition:read / write / approve
analysis:schedule:read / write / activate
analysis:plan:read / write / approve
analysis:run:read / trigger / cancel / retry
analysis:report:read / request / download
analysis:resource:read / write
analysis:audit:read
~~~

| 动作 | 页面 | 服务端命令 | UI硬约束 |
|---|---|---|---|
| 新建任务定义 | 任务管理 | SaveTaskDefinitionRevision | `If-Match: 0`，不产生Run |
| 激活/挂起定义 | 定义详情 | Activate/SuspendTaskDefinition | reason、expected revision、影响说明 |
| 保存/激活计划 | 任务编排 | Save/Approve/ActivatePlanRevision | maker/checker；旧revision只读 |
| 保存/启停调度 | 调度管理 | Save/Activate/PauseScheduleRevision | 明确只影响未来trigger |
| 发起即时分析 | 即时向导 | Preflight + SubmitOnDemandTrigger | 原key恢复；不能直连执行器 |
| 取消Run | 运行详情 | RequestCancelRun | 明确HTTP断开不等于取消完成 |
| 重试阶段/任务 | 运行详情 | RetryStage / RetryTask | 仅显示server allowed action |
| 申请/重试报告 | 运行详情/报告 | Request/RetryHumanReport | 不修改RunState |
| 下载报告 | 报告 | IssueReportDownloadTicket | ticket短期有效且写下载审计 |
| 调整资源 | 调度资源 | ResourcePolicy command | before/after、审批、回滚值 |

前端可用scope控制菜单可见性，但最终动作授权必须由API重新判定。被拒绝动作不通过隐藏按钮掩盖审计；服务端403/409/412要转换成可读原因并保留trace ID。

## 20. 页面到API的唯一映射

| 页面/动作 | 读API | 写API |
|---|---|---|
| 任务管理 | `GET /analysis/task-definitions`、`/{id}`、`/{id}/plans`、`/{id}/report-policies` | definition create/activate/suspend；plan/report policy独立命令 |
| 调度管理 | `GET /analysis/schedules`、trigger history projection | schedule save/activate/pause |
| 任务编排 | task definition plans、catalog、compatibility report | plans preflight/save/approve/activate |
| 运行监控 | `GET /analysis/tasks`、`GET /analysis/runs` | on-demand preflight/submit |
| 运行详情 | run、results、machine-summary、stage receipts projection | cancel、retry task/stage、request report |
| 调度资源 | resource/queue/lease/executor health queries | resource policy commands；不直接改数据库 |
| 分析报告 | machine-summary、human-reports | request/retry/download-ticket |

前端通过`web/ui/src/services/analysisSchedulingApi.ts`的typed函数访问本域API，并由`services/api.ts`兼容重导出。页面、hooks和组件不得直接`fetch`，不得拼接APISIX地址，也不得以mock成功替代真实响应；不要继续把实现堆入现有聚合文件。

## 21. 前端目录、函数与维护性设计（PLANNED）

### 21.1 建议文件边界

~~~text
web/ui/src/
├─ pages/
│  ├─ AnalysisTaskDefinitionsPage.tsx
│  ├─ AnalysisScheduleManagementPage.tsx
│  ├─ AnalysisOrchestrationPage.tsx
│  ├─ AnalysisRunMonitorPage.tsx
│  ├─ AnalysisOnDemandWizardPage.tsx
│  ├─ AnalysisRunDetailPage.tsx
│  ├─ AnalysisResourceManagementPage.tsx
│  └─ AnalysisReportCenterPage.tsx
├─ features/analysis-scheduling/
│  ├─ contracts.ts
│  ├─ queryKeys.ts
│  ├─ authoritySnapshot.ts
│  ├─ viewModels.ts
│  ├─ allowedActions.ts
│  ├─ operationRecovery.ts
│  ├─ components/
│  │  ├─ AnalysisStageSteps.tsx
│  │  ├─ PlanSourceTag.tsx
│  │  ├─ RunStateTag.tsx
│  │  ├─ FindingConclusionView.tsx
│  │  ├─ RiskSeverityTag.tsx
│  │  ├─ CompletenessView.tsx
│  │  ├─ IntegrityStateView.tsx
│  │  ├─ ReportStateTag.tsx
│  │  ├─ AuthorityStateAlert.tsx
│  │  └─ OperationRecoveryPanel.tsx
│  └─ forms/
│     ├─ OnDemandAnalysisWizard.tsx
│     ├─ ScheduleRevisionForm.tsx
│     └─ PlanRevisionEditor.tsx
├─ routes/
│  ├─ routeManifest.tsx
│  ├─ routePageRegistry.tsx
│  └─ pageDesignContracts.v1.json
└─ services/
   ├─ analysisSchedulingApi.ts
   └─ api.ts                 # 兼容重导出
~~~

`routeManifest`仍是菜单、权限、标题和验收真源；`routePageRegistry`只把route ID映射到lazy page，替换`App.tsx`继续增长的长条件链，但不得改变现有route行为。公共AppShell、tokens和样式继续复用。

分析域禁止复用当前通过中文正则猜色、unknown默认绿色的通用`StatusTag`。上述类型化组件必须对每个已知enum穷尽映射；编译期遗漏失败，运行期unknown统一显示灰色“状态无法确认”并锁定关键动作。RiskSeverity只描述已发现风险的严重度，DetectorDisposition只属于检测明细，二者都不能替代FindingConclusion。

### 21.2 核心函数合同

| 函数 | 内部步骤 | 失败边界 |
|---|---|---|
| `decodeAnalysisRunPage` | 校验schema→unknown enum拒绝→revision字符串保留→allowedActions exact-set→冻结对象 | 不返回部分成功对象 |
| `buildOnDemandAnalysisIntent` | 读取选择→分离plan source与trigger→规范化window→只收集允许override→生成canonical request | 不读取页面展示文案反推合同值 |
| `preflightOnDemandAnalysis` | 调typed API→绑定request hash→保存receipt/expiry/revisions→显示confirmations | 过期或漂移必须重做 |
| `submitOnDemandAnalysis` | 客户端先持有key→校验preflight绑定→提交→202进入详情→503进入恢复 | 不自动换key重试 |
| `recoverAnalysisOperation` | 读取session内恢复句柄→原key/URL查询→exact receipt恢复→清理句柄 | 不在日志或URL明文泄露key |
| `deriveAnalysisRunRowViewModel` | 组合run/task/summary/report→保持六个正交轴→生成文案→裁剪allowed actions | 无输出不映射NO_THREAT_OBSERVED |
| `acceptAuthoritySnapshot` | identity→session epoch→BigInt revision→hash→transition | 同revision异hash锁定写操作 |
| `deriveAnalysisRunOverviewViewModel` | 范围→五段→结论→完整性→关键发现→报告状态 | 报告失败不回退Run |

### 21.3 Query key与缓存

~~~text
['analysis', tenantId, sessionEpoch, 'task-definitions', filterHash, cursor]
['analysis', tenantId, sessionEpoch, 'schedules', filterHash, cursor]
['analysis', tenantId, sessionEpoch, 'orchestrations', definitionId]
['analysis', tenantId, sessionEpoch, 'runs', filterHash, cursor]
['analysis', tenantId, sessionEpoch, 'run', runId]
['analysis', tenantId, sessionEpoch, 'reports', reportType, filterHash, cursor]
~~~

退出登录、tenant或session epoch变化时清空analysis namespace。列表与详情共享不可变snapshot时可以局部更新，但不得用乐观成功覆盖权威命令状态；激活、取消、重试和报告请求都等待服务端receipt后再显示已接受。

## 22. UI页面套图任务书

UI-D1高保真套图不是再画一个“全功能大屏”，而是按以下八张核心生产页面逐张形成：

| 图 | 必须表达 | 必须避免 |
|---|---|---|
| 任务管理 | 定义列表、active plan、调度数、状态、唯一新建按钮 | Run进度和模型图表 |
| 调度管理 | 已批准方案绑定、触发类型、窗口/规则、下次触发、暂停影响 | 把默认/自定义当触发类型；激活后跟随active plan漂移 |
| 任务编排 | 固定五段、技术闸门、版本、兼容性Drawer | 任意拖拽和卡片堆叠 |
| 运行监控 | 完整Run列表、五段紧凑进度、机器结论、证据状态 | 默认常驻右栏、整排KPI、把风险当结论 |
| 即时分析向导 | 三步、ON_DEMAND固定、默认/自定义方案、preflight和原key恢复 | 把方案来源做成触发方式；直接调用执行器 |
| 运行详情 | 单Run叙事、五段、机器结论、报告独立状态 | 全量技术输出首屏出现 |
| 调度资源 | 配额/队列/租约/执行器、阻塞范围和下一动作 | 资源拓扑大屏；无数据显示健康 |
| 分析报告 | 机器摘要/人读报告双页签及独立状态 | 报告失败显示Run失败 |

所有图必须复用当前AppShell的顶部栏、166px可折叠左侧分组导航、深海军蓝token、青色主强调、6px圆角和Ant Design组件语言；左侧“任务调度”展开，当前二级项高亮。UI文案以2026-08-16为时间锚。图中不得使用“自动任务/人工计划”作为运行分类，只使用“默认方案/自定义方案”和独立TriggerKind。

## 23. UI专项分阶段验收

| UI Phase | 交付 | 退出门 |
|---|---|---|
| UI-D0 | 菜单/对象/页面/状态/权限合同 | 本文评审；无职责重叠 |
| UI-D1 | 八张核心页面视觉候选、prompt manifest及各自交互注释 | 同一Shell；对象、状态、信息密度和文案审查；仍非运行证据 |
| UI-C0 | decoder/query key/authority guard/route骨架 | unit tests；unknown和hash冲突负例 |
| UI-C1 | 任务管理、运行监控、运行详情只读 | 真实API loading/empty/error/403 |
| UI-C2 | 默认方案即时分析和operation recovery | 202/409/412/503恢复矩阵 |
| UI-C3 | 调度管理、任务编排和maker/checker | revision、审批、暂停语义 |
| UI-C4 | 自定义方案、人读报告、调度资源 | 权限、正交状态、管理员边界 |
| UI-QA | 1920/1440/移动、键盘、浏览器、兼容深链 | 同视口截图+DOM/API/console证据 |

在UI-D1选定并冻结每个页面目标前不得进入页面实现；在analysis API、状态和错误合同冻结前，UI-C1以后保持`BLOCKED_BY_CONTRACT`。静态图通过不等于真实页面通过，真实页面通过也不等于统一调度业务闭环通过。

## 24. 角色入口、跨页一致性与刷新合同

### 24.1 角色默认入口

| 角色 | 默认入口 | 默认能力边界 |
|---|---|---|
| 安全运营 | 运行监控 | 读取批准定义目录、发起有权即时分析、查看Run/证据/报告；不能改全局编排 |
| 任务规划 | 任务管理 | 定义、调度、编排治理和审批；运行监控以治理视角只读 |
| 平台管理员 | 运行监控 | 全域可见；调度资源写操作仍受审批和服务端allowedActions约束 |
| 审计人员 | 报告中心 | Run、机器摘要、人读报告和审计只读；不能通过隐藏入口获得写能力 |

菜单顺序不随角色变化，只隐藏无read scope的项；动态详情始终回映同一二级菜单。安全运营若能发起即时分析，至少必须拥有批准任务定义目录的只读能力，否则向导不得用空白自由输入代替权威定义。

### 24.2 跨页fixture与身份一致性

UI-D1及后续测试固定一组跨页fixture：

~~~text
Definition  DEF-OUTBOUND-001 rev 12  外联流量深度分析
Plan        PLAN-OUTBOUND-007 rev 7  execution_spec_sha256=sha256:7b...
Schedule    SCH-OUTBOUND-003 rev 4  CRON_WINDOW
Trigger     TRG-20260816-0900
Task        TASK-20260816-001
Run         RUN-20260816-001 attempt 1
Summary     SUM-RUN-001 sha256:4f...
Report      HREP-RUN-001 revision 1
~~~

任务管理、调度管理、编排、运行、摘要和报告页面引用同一组ID、revision、hash、时间窗和状态迁移。任何视觉图、Storybook fixture或browser seed若改变其中一个引用，必须同时更新其余消费者；禁止每页使用互相矛盾的随机数据。

### 24.3 查询与刷新频率

| 页面/状态 | 刷新策略 | 停止条件 |
|---|---|---|
| 任务/调度/编排列表 | 用户刷新或窗口重新聚焦后条件刷新 | immutable revision详情不轮询 |
| 运行列表 | 存在active Run时10秒；全终态时30秒或手动 | 页面不可见、离线或authority冲突时暂停 |
| 单Run详情 | active时3秒，连续退避到10秒；支持服务端事件后失效 | terminal snapshot已接收且hash稳定 |
| 调度资源 | 管理员可见时10秒；异常可缩短到5秒 | 页面不可见或资源API不可确认 |
| 人读报告 | QUEUED/GENERATING/VERIFYING时5秒 | AVAILABLE/FAILED/CANCELLED |

轮询只刷新权威snapshot，不制造本地中间状态；仅事实发生变化时更新live region。浏览器离线、401、同revision异hash或连续错误进入明确页面状态，不继续静默高频请求。

## 25. UI-D1之后仍需补齐的设计帧

八张核心图只覆盖主页面目标，不能替代以下开发前设计帧：任务定义五Tab详情、调度创建四步、调度详情/触发历史、即时分析审批等待与transport unknown、机器摘要/人读报告双状态、通用异常状态板，以及运行监控1440/1024/390、即时向导390、运行详情390。每帧都必须复用第24.2节fixture，并标注路由、scope、API、页面状态、主动作、返回路径和焦点恢复。

这些补充帧完成前，UI-D1状态只能保持`VISUAL_CANDIDATE`；真实页面完成同视口参考对比、键盘、响应式、API/console和深链验证后，才可申请UI-QA，不得用生成图跳过实现验证。
