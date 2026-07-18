# 强化学习驱动的产品与全栈交付闭环

- 生效时间：`2026-07-12`
- 适用范围：业务页面、跨页面业务链、API、数据库仿真、测试、K8s 部署和生产性能优化。
- 基础流程：`FULL_STACK_PAGE_DELIVERY_WORKFLOW.md`。
- 控制原则：强化学习只负责在满足硬约束的候选方案中学习和排序，不得绕过安全、业务、测试、审计、视觉和生产门禁。

## 1. 目标与边界

目标不是让智能体反复改页面，而是建立可审计的多目标优化闭环：

```text
产品业务建模
  -> 功能与验收矩阵
  -> UI/API/数据/部署候选方案
  -> 全栈实现和测试
  -> Windows Chrome + K8s 真实验证
  -> 多维奖励计算
  -> 独立 Critic 审查
  -> 主线程裁决
  -> 经验入库
  -> 离线策略更新
  -> 回归重放或下一轮优化
```

需要同时优化五类目标：

1. **产品业务合理性**：页面是否支持真实角色做出正确决策，跨页面链路是否闭环。
2. **功能完备性**：可见功能是否有真实 API、数据、权限、审计、状态和异常处理。
3. **视觉与交互质量**：层级、密度、可读性、一致性、响应性和业务 ROI 是否合格。
4. **测试全面性**：单元、契约、集成、浏览器、性能、安全、韧性和回滚是否有证据。
5. **生产性能效率**：前端加载、API/DB、Kafka/Flink、K8s 资源、发布速度和稳定性是否改善。

以下内容永远不交给 reward 决定：权限边界、tenant 隔离、凭证、审计真实性、数据清理、生产破坏性操作、第三方验收结论和人工发布授权。

## 2. 强化学习模型

采用“约束型离线强化学习 + staging contextual bandit + 人工主线程裁决”的渐进方案。初期不训练可直接修改生产的在线策略。

### 2.1 Episode

一个 episode 是一个页面、Tab、浮层、跨页业务链或性能专项的一次完整交付尝试。

Episode 必须包含：

- `run_id`、任务 ID、页面/链路 ID、代码版本、镜像版本和 evaluator 版本。
- 修改前 observation、候选 actions、被选择 action 和选择原因。
- UI/API/数据/部署影响面。
- 所有硬门禁结果、reward breakdown、Critic 结论和主线程裁决。
- 失败原因、回修动作、回滚点和最终状态。

缺少生产证据、数据库证据或主线程裁决的 episode 可以用于问题分析，但不能进入正向训练集。

### 2.2 State

状态向量按五个域组织，不直接把源码全文塞入策略：

| 状态域 | 主要特征 |
|---|---|
| 产品 | 用户角色、任务目标、决策问题、业务阶段、上游/下游对象、风险级别 |
| 功能 | 控件数量、真实 endpoint 比例、状态覆盖、RBAC、审计、分页、跨页上下文 |
| 视觉 | 业务 ROI、布局层级、信息密度、溢出、对齐、颜色语义、可访问性、视口 |
| 质量 | 测试类型覆盖、失败分类、变更风险、历史回归、mutation/property test 结果 |
| 生产 | Web Vitals、bundle、API/DB latency、错误率、资源、checkpoint、rollout、回滚 |

Observation 必须带数据来源和时间戳；历史值、seeded 值和 live 值不能混写。

### 2.3 Action

策略只从允许的动作集合中选择：

- 产品动作：调整信息架构、任务顺序、默认筛选、下钻路径、风险确认和反馈闭环。
- UI 动作：调整业务区栅格、密度、图表类型、表格列、滚动、分页、Drawer/Modal。
- 功能动作：补 endpoint、错误态、RBAC、审计、幂等、异步任务、上下文透传。
- 数据动作：补 schema/index、seed 场景、聚合、分页排序和清理策略。
- 测试动作：增加风险对应的测试、扩展边界样本、回放历史缺陷。
- 性能动作：缓存、请求合并、查询优化、批处理、资源配置、镜像和 rollout 策略。

单页动作不得修改公共 AppShell。高风险生产动作必须转人工门禁，不能作为自动 exploration。

### 2.4 Hard Constraints

Hard constraints 先于 reward 执行，任一失败即 `infeasible`：

- 业务对象、状态机和跨页 ID 语义正确。
- 所有可见按钮真实可用，不存在假按钮或前端伪成功。
- API、tenant、RBAC、审计和数据库链路通过。
- 正常生产路由无 4xx/5xx、requestfailed、pageerror 和非 warning console error。
- 业务 ROI 严格 `<0.125`，无重叠、遮挡、不可达内容和不可解释截断。
- 关键测试、schema/seed 幂等、回滚和安全扫描通过。
- rollout Ready，关键 SLO 无回退，凭证没有进入代码、日志或训练数据。

Hard constraint 失败的 episode 奖励固定为不可接受，不允许通过其他高分抵消。

### 2.5 Reward

仅对通过 hard constraints 的候选计算奖励。初始权重如下，页面可在任务开始前声明更严格配置，但不能事后改权重让结果通过。

| Reward 域 | 权重 | 核心指标 |
|---|---:|---|
| 产品业务价值 `R_business` | 0.25 | 任务闭环、决策有效性、上下文连续性、风险控制、专家偏好 |
| 功能完备 `R_function` | 0.20 | 功能/API/状态/RBAC/审计覆盖、真实分页、异常恢复 |
| 测试质量 `R_quality` | 0.20 | 风险覆盖、缺陷检出、回归重放、稳定性、无效测试惩罚 |
| 视觉交互 `R_visual` | 0.15 | ROI、层级、密度、可读性、一致性、可访问性、响应式 |
| 生产性能 `R_performance` | 0.15 | 延迟、吞吐、错误率、资源、rollout、checkpoint、回滚 |
| 可维护性 `R_maintainability` | 0.05 | 复用、复杂度、契约一致、文档同步、变更范围 |

```text
R_total = 0.25R_business
        + 0.20R_function
        + 0.20R_quality
        + 0.15R_visual
        + 0.15R_performance
        + 0.05R_maintainability
        - P_regression
        - P_cost
        - P_uncertainty
```

奖励使用 `0..100` 标准化分数。以下惩罚不能被正向指标稀释：

- 新增 P0/P1 回归、数据错误或权限漏洞：episode 直接失败。
- 测试只改断言以放过缺陷、视觉模式冒充业务模式、mock 冒充 live：直接失败。
- 性能改善来自减少功能、隐藏数据或降低测试强度：直接失败。
- 超出任务修改范围、公共壳层漂移、文档与部署漂移：扣分并要求主线程判断。

### 2.6 Preference Feedback

“更美观”和“业务更合理”不能只依赖单一像素指标，采用成对偏好数据：

1. 同一 observation 生成最多 2 到 3 个受约束候选。
2. 设计 Critic、业务 Critic、测试 Critic 和性能 Critic 独立排序并说明理由。
3. 主线程选择 `A better / B better / tie / both reject`。
4. 记录偏好理由标签，如层级、密度、决策效率、业务缺口、性能回退。
5. 偏好只更新离线排序策略；未经 holdout 验证不得改变生产默认策略。

## 3. 分阶段交付流程

### 阶段 0：目标、基线与风险分级

- 定义用户、场景、业务目标、成功指标和非目标。
- 读取产品设计、页面拆解、历史缺陷、生产 SLO 和现有 evidence。
- 记录 baseline：截图、API、数据库、测试、资源和性能。
- 按 `low/medium/high/critical` 评估变更风险和人工门禁。

输出：`objective.md`、`baseline.json`、`risk-profile.json`。

门禁：没有明确用户任务、baseline 或风险等级时不得生成方案。

### 阶段 1：产品业务设计

对每个页面回答：

1. 谁在什么场景进入页面？
2. 他需要判断什么、处置什么、留下什么证据？
3. 页面输入来自哪里，结果流向哪里？
4. 默认值、筛选、排序和风险提示是否符合业务优先级？
5. 空态、异常、权限不足和部分数据缺失时应该如何继续工作？

建立业务链：`入口 -> 观察 -> 判断 -> 下钻 -> 动作 -> 确认 -> 审计 -> 反馈/回放`。

产品评审维度：任务完成率、决策时间、信息充分性、错误操作风险、跨页上下文保留和审计可追溯性。

输出：`business-journey.md`、`decision-model.json`、`business-rules.json`。

### 阶段 2：功能完备性建模

为全部可见能力建立功能矩阵：

| 能力 | UI 入口 | API | DB/Topic | RBAC | 审计 | 状态 | 错误/恢复 | 测试 |
|---|---|---|---|---|---|---|---|---|

必须覆盖：查询、筛选、排序、分页、刷新、图表联动、详情、批量动作、危险动作、导入导出、异步任务、反馈和跨页跳转。

功能完备率不能只按按钮计数。一个功能只有 UI、服务端、数据、权限、审计、错误态和测试全部存在时才计为完成。

输出：`capability-matrix.json`、`gap-list.md`、API/数据契约。

### 阶段 3：候选方案与安全探索

- 低风险页面可生成 2 到 3 个业务区候选；高风险状态机只允许保守改进。
- 候选必须复用同一 AppShell、业务规则和数据契约。
- 先静态评估信息层级和功能映射，再实现最有希望的候选。
- exploration 预算受代码范围、时间、资源和生产风险限制。

输出：`action-candidates.json`、`candidate-comparison.md`、`selected-action.json`。

### 阶段 4：三类契约与数据设计

沿用全栈流程的 UI、API、数据三类契约，并增加：

- 产品规则到 API/数据库字段的 traceability。
- 每个 reward 指标的数据采集方式。
- seeded/live/visual/fallback 模式隔离。
- 性能预算、容量假设、索引、缓存和批处理策略。
- 失败注入、回滚和兼容性策略。

门禁：契约无法支撑业务规则、测试或性能指标时退回阶段 1/2。

### 阶段 5：全栈实现

执行顺序：

```text
schema/index/seed
  -> repository/service/handler
  -> API contract and RBAC/audit
  -> frontend service/query/state
  -> page/chart/table/action/overlay
  -> deployment/config/observability
```

实现必须保持普通生产路由使用 live/seeded 数据，视觉模式只服务截图。所有优化动作记录 change intent，便于 reward 归因。

### 阶段 6：全面测试

按风险建立测试金字塔和测试矩阵：

| 层级 | 必测内容 |
|---|---|
| 静态 | TypeScript/Go/Java lint、契约、路由、DDL/YAML、依赖和 Secret 扫描 |
| 单元 | reducer、格式化、图表 option、状态机、权限判定、分页和错误映射 |
| Property/Mutation | 分页边界、过滤组合、tenant/RBAC、关键状态机和计算逻辑 |
| Repository 集成 | 真实 PG/CH/Redis/MinIO、索引、事务、幂等和 tenant 隔离 |
| API 契约 | request/response、错误码、分页 total、并发、幂等、审计失败 |
| 数据链路 | seed 两次一致、Kafka/Flink/DB 对账、清理和异常数据 |
| 前端组件 | loading/error/empty/403、交互、图表变化、滚动和分页 |
| 浏览器 E2E | Windows Chrome 正常路由、跨页上下文、动作、刷新后持久化 |
| 视觉/可访问性 | ROI、视口、键盘、focus、对比度、文本溢出和截图 diff |
| 性能 | Web Vitals、bundle、API/DB load、资源、长稳和容量边界 |
| 韧性/安全 | 依赖故障、重试、超时、Pod 滚动、checkpoint、越权和输入攻击 |
| 升级/回滚 | schema 兼容、镜像回滚、savepoint/数据恢复和版本降级 |

测试奖励关注“能否发现真实缺陷”，不奖励重复或无断言测试。历史 P0/P1 缺陷必须进入 regression replay 集。

### 阶段 7：Staging 与生产性能优化

先在 staging/隔离数据范围测量，再进入生产 canary。初始性能预算：

| 范围 | 默认门槛 |
|---|---|
| Web | LCP `<=2.5s`、INP `<=200ms`、CLS `<=0.1`，且不比 baseline 回退 10% |
| API | 常用读接口 P95 `<=500ms`，动作受理 P95 `<=1s`，5xx `<0.1%` |
| DB | 无新增全表扫描/N+1；目标查询计划、rows scanned 和 P95 留证 |
| Kafka/Flink | 当前作业 RUNNING；checkpoint age 不超过 2 个周期；稳定窗口无新失败 |
| K8s | rollout 全部 Ready；无 CrashLoop/OOM；CPU/内存不比 baseline 无理由增加 15% |
| 发布 | migration 可重入、镜像可追踪、readiness/rollback 验证通过 |

页面或专项可定义更严格 SLO。绝对门槛和相对回退门槛同时执行，不能用低负载掩盖退化。

优化顺序：先测量瓶颈，再优化请求/查询/批处理/缓存/资源，最后重新跑功能和视觉回归。禁止先调大资源掩盖代码或查询问题。

### 阶段 8：Windows Chrome 双轨验收

沿用全栈流程：

- 业务轨使用正常生产路由验证真实 API、数据、分页、RBAC、审计和跨页链路。
- 视觉轨使用确定性数据验证 target、implementation、diff、metrics 和 ROI。
- 增加用户任务计时、关键操作步数、键盘路径、视觉稳定性和 Web Vitals 采集。

### 阶段 9：Reward、Critic 与主线程裁决

1. Gate Engine 先输出 hard constraints；失败项直接进入 repair。
2. Reward Evaluator 按冻结版本计算各域分数和置信度。
3. 页面逻辑 Critic 与页面布局 Critic 是每个页面的必选独立子代理门禁；此外业务、测试和性能 Critic 按风险执行。
4. 所有 Critic 独立审查，不读取其他 Critic 的结论后再作答。
5. 主线程逐条核对证据并记录 `accepted/rejected/deferred`，处理逻辑与布局建议冲突后，选择接受、回修、回退或人工门禁。
6. 只有两项必选页面审核均有报告、主线程裁决完整、成立的 P0/P1 已回修且其他证据完整，episode 才可进入正向经验库。

Critic 找到问题不等于问题成立；主线程必须逐条记录 `accepted/rejected/deferred` 和理由。

### 阶段 10：部署、Canary 与回滚

- 顺序：schema/seed -> backend -> frontend -> route -> observability。
- 双节点镜像导入和 digest/tag 对齐后再 rollout。
- canary 先验证内部 tenant/测试范围，再扩大流量。
- 观察错误率、P95、资源、DB、Kafka/Flink 和用户任务指标。
- 任一硬门禁回退立即停止扩量并执行已验证 rollback。

生产变更成功不自动等于学习成功；只有稳定窗口结束且证据完整才结算最终 reward。

### 阶段 11：经验回放与策略更新

- 正向样本：主线程接受、无回归、稳定窗口通过的 episode。
- 负向样本：真实缺陷、回滚、Critic 成立问题和奖励投机案例。
- 中性样本：证据不足、外部阻塞或无法归因，不进入正负训练。
- 每次策略更新使用冻结训练集、holdout 页面和历史缺陷回放。
- 新策略先 shadow 评分，再 staging A/B；通过后才可成为默认排序策略。

策略版本必须可回退。不得删除失败经验，不得只保留高 reward 样本。

## 4. 经验与证据协议

机器可读契约：

- `reinforcement-learning-policy.json`：冻结的初始 hard gates、reward 权重、性能预算和学习策略。
- `learning-episode.schema.json`：episode、Critic、主线程裁决、生产稳定性和 evidence 的 JSON Schema。

建议目录：

```text
evidence/learning/<task-id>/<run-id>/
  observation.json
  action-candidates.json
  selected-action.json
  gates.json
  reward.json
  critic-page-logic.json
  critic-page-layout.json
  critic-design.json
  critic-business.json
  critic-quality.json
  critic-performance.json
  main-thread-decision.json
  regression-replay.json
  production-window.json
  episode.json
```

`reward.json` 至少包含 evaluator 版本、冻结权重、原始指标、标准化方法、置信度、惩罚和数据来源。截图、日志、API、数据库和性能原始证据继续放在原 evidence/acceptance 目录，通过路径和 checksum 引用，避免重复复制。

训练数据禁止包含 Secret、token、PCAP payload、真实个人信息和未脱敏审计详情。

## 5. 角色与职责

| 角色 | 职责 |
|---|---|
| Product Agent | 用户任务、业务规则、决策链和功能缺口 |
| Design Agent | 信息层级、视觉、交互、响应式和可访问性 |
| Page Logic Critic | 独立审核页面对象、状态机、数据来源、权限、动作结果和业务闭环；只报告，不实施、不裁决 |
| Page Layout Critic | 独立审核空间利用、层级、栅格、密度、滚动、响应式、空白和可访问性；只报告，不实施、不裁决 |
| Full-stack Agent | UI/API/数据/部署实现和最小测试 |
| Test Agent | 风险模型、测试矩阵、负例、回归和证据质量 |
| Performance Agent | baseline、负载、资源、查询、Flink/K8s 和 SLO |
| Critic Agents | 独立审查并给出可证伪 findings |
| Gate Engine | 执行硬门禁，不参与审美偏好 |
| Reward Evaluator | 使用冻结规则计算 reward，不修改证据 |
| Main Thread | 最终裁决、人工门禁、发布和经验标签 |

实现者不能同时充当唯一 Critic，Reward Evaluator 不能修改实现或测试。

## 6. 防止奖励投机

- 不以更低 ROI 代替业务正确性，不以截图相似代替真实交互。
- 不通过隐藏按钮、减少数据、关闭动画或删除功能提高视觉/性能分。
- 不通过提高资源 limit、缩小测试数据或跳过测试制造性能通过。
- 不允许 evaluator 读取目标结论后动态改权重、阈值或归一化。
- 不以测试数量、代码行数、控件数量作为独立正奖励。
- 不把 Critic 的措辞强度作为缺陷严重度，必须回到证据和业务影响。
- 同一页面重复回修设置预算；超过预算进入设计复盘或人工 triage，避免无效震荡。

## 7. 状态机与完成定义

```text
discovered
  -> business-designed
  -> capability-mapped
  -> contracts-ready
  -> implemented
  -> test-passed
  -> staging-qualified
  -> business-accepted
  -> visual-accepted
  -> performance-accepted
  -> critic-reviewed
  -> main-thread-accepted
  -> production-stable
  -> learning-eligible
```

任一阶段可进入 `repair-required`、`human-gate`、`rolled-back` 或 `blocked`。只有 `production-stable` 才表示交付完成；只有证据和 reward 协议完整的完成项才进入 `learning-eligible`。

## 8. 分阶段落地优先级

### P0：先形成可靠闭环

- 固化 hard constraints、功能矩阵、测试矩阵，并使用 `learning-episode.schema.json` 校验 episode。
- 将 Campaign 作为首个全栈 episode，补真实后端、seed、审计和双轨验收。
- 采集 baseline/reward，但先不自动更新策略。

### P1：建立偏好和经验回放

- 对 5 到 10 个代表页面建立成对 UI/业务偏好。
- 建立历史 P0/P1 缺陷 regression replay。
- 实现冻结 reward evaluator 和独立 Critic 输出格式。

### P2：离线策略与 staging shadow

- 累积至少 20 个主线程已裁决 episode 后训练/调整候选排序策略。
- 在 holdout 页面验证，禁止训练集页面自证。
- 新策略只做 shadow recommendation，不自动改代码或部署。

### P3：受控自动优化

- 仅对低风险业务区和性能参数启用 bounded exploration。
- 自动实现仍经过现有 Codex Loop policy、workspace、测试、Reviewer 和 evidence gate。
- 生产发布、扩量、回滚和高风险动作继续由主线程控制。
