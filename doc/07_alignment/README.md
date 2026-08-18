# 前后端逻辑对齐整改报告包

更新时间：2026-08-16

本目录用于承载《前后端逻辑对齐分析》和《架构设计》所提出问题的仓库级整改设计。报告坚持“功能只增不减、外部契约向后兼容、真实证据优先、逐功能纵向切片、控制面与数据面解耦”的原则。

## 文件

- `课题一PR里程碑代码级详细设计.md`：课题一系统工程M00–M13的PR演进主控卷，包含212个父执行任务、每任务唯一terminal TASK-IDX、30个R00–R29关闭切片、当前1289个单类型原子PR、代码/数据/发布流程图、102项canonical逐ID关闭列车、CNAS外部活动节点、候选身份、证据、回滚和任务卡；第38—76章继续补充运行时总图、跨语言合同、原子PR执行包、GoF 23模式研判、developer claim catalog、逐任务/逐REQ closure、候选外部制品provenance、M06-N004代码黄金剖面，以及统一分析任务调度中心和采集→特征→加密识别→恶意检测→机器摘要→可选人读报告的函数级设计。第76.28—76.48节进一步冻结独立analysis-service、Task/Run/StageAttempt基数、BusinessPhase/ExecutionNode分层、FindingConclusion与RunClosure、调度exact-plan绑定、前置准入、RunScopeRouter重叠隔离、确定性调度与公平队列、Proto/Topic、存储锁序、恢复矩阵、RBAC/配额/可观测性、HTTP合同、34张控制面函数级执行卡候选及14张独立UI执行卡候选。领取层1289/1289已有说明不等于ATC新增卡已注册或任何生产代码可修改。工程仍为`NOT_EXECUTED / EXECUTION_NO_GO`，受信验签器和多语言精确symbol resolver尚未完成，全部父任务仍DRAFT、DoR/候选/晋级均BLOCKED。
- `课题一主营业务流程闭环与中期详细设计对齐方案.md`：以统一任务调度中心重构业务架构，冻结六条活动业务流和旧BF06 tombstone，明确`AUTO_DEFAULT/MANUAL_CUSTOM`只作为plan source开关且与trigger正交；给出Definition/Plan/Schedule/Trigger/Task/Run/StageAttempt/Receipt/RunClosure/MachineSummary/HumanReport聚合、BusinessPhase/ExecutionNode分层、双plan hash、exact schedule binding、RunScopeRouter、中期硬主链、M00—M13接入、设计模式、ATC PR列车和恶意负例。主详细设计第76章是其函数级执行投影；两份文件均不增加执行授权。
- `统一分析任务调度中心菜单与UI详细设计.md`：状态为`DRAFT_UI_CONTRACT / UI_D1_EIGHT_PAGE_VISUAL_CANDIDATE / NOT_IMPLEMENTED`；已将调度能力收敛为一个“任务调度”一级业务域及任务管理、调度管理、任务编排、运行监控、调度资源五个二级模块，补齐三步即时分析、页面蓝图、FindingConclusion与证据状态、权限/API/函数合同、UI执行卡和八张核心页面视觉候选；仍未实现、联调、浏览器验证或验收。
- `三份设计文档与当前实现差异清单-2026-08-17.md`：把本目录《课题一PR里程碑代码级详细设计》(76.43—76.48 + v1 Topic/RunScope/Flink 连续性)、《课题一主营业务流程闭环与中期详细设计对齐方案》、《统一分析任务调度中心菜单与UI详细设计》三份合同与当前实现(Go v32 / Rust wire-replay / Flink run 系列 / Web UI / live PG)逐条对照的差异登记；只登记差异与建议顺序，不含执行授权，状态 `DIFF_REGISTERED / NOT_EXECUTED`。
- `generated/M02代码直达Registry切换就绪账本.md`：由REG-03 pre-switch ledger生成的阅读视图，冻结旧34卡、421张replacement、1676候选exact-set、逐父completion替换和tombstone，并列出当前阻断；它不表示四个现役catalog已切换。
- `generated/M02代码直达Locator覆盖清单.md`：C03的逐locator resolver覆盖视图，区分已有resolver但缺candidate、缺trusted resolver、planned文件不存在和有序共享locator；任何行都不等于locator已解析。
- `generated/M02函数评审覆盖清单.md`：C04的421叶函数/非函数分区、静态合同输入与正式签署缺口视图；5个helper合同通过v3 append-only owner rebinding独立归属，静态合同不等于`FUNCTION_DESIGN_REVIEW_RECEIPT`，声明式binding也不等于非函数豁免。
- `generated/M02写范围Supersession账本.md`：不改P408冻结记录的有效写范围窄化设计；当前仅`DESIGNED_NOT_REGISTERED`，四份active catalog及未来candidate package均未启用。
- `generated/M01早期受信验证两阶段候选冻结工作单.md`：把 84 个 M01 v2 候选叶精确分成 55 个 `PLANNED` 源叶（81 个唯一 Git blob）与 29 个 `PLANNED_OUTPUT` 证据叶；提供仅能在隔离 clean worktree 使用的不可覆盖 design-candidate 冻结命令，当前 76 个源缺失且不会创建候选。
- `generated/M01早期受信验证设计评审工作单目录.md`：把 N015 的 25 个函数、3 个类型、8 个非函数设计面展开为 36 条可领取评审工作单；真实候选、实名独立评审、裁决和签名均缺失，因此全部阻断。
- `generated/M01早期受信验证四目录原子切换阻断预检.md`：只读核算现役 1289/56 与候选 `1289-9+37=1317` 的 exact-set、N010/N015 职责边界和八项切换前置条件；不生成未来目录，也不授权切换。
- `generated/M01受信验证器镜像构建签名发布工作单.md`：将 N015 首次 trust bootstrap 的外部镜像活动收紧为 6 个候选 Git 输入、7 个仓外制品、供应链/安全负责人 2-of-2 签署及专用 receipt 校验；当前源码、身份、构建、发布、签名和 receipt 均未产生。
- `前后端逻辑对齐整改大纲.md`：2.0版整改思路、步骤、方法论、54项功能清单、48项技术清单和分卷目录。
- `前后端逻辑对齐整改报告-5万字完整版.md`：第0卷至第7卷10万字扩展完整版；为兼容既有引用沿用历史文件名。
- `编制基线清单-2026-07-30-10万字扩展版.md`：当前2.0版输入附件、仓库静态锚点、严格汉字统计、ID覆盖、交付文件哈希和未执行live边界。
- `编制基线清单-2026-07-30.md`：首版5万字报告的历史基线，只用于追溯，不代表当前文件状态。

## 证据等级

| 标记 | 含义 | 使用规则 |
|---|---|---|
| `E1-S-静态已证实` | 已由本次读取的当前仓库源码或清单直接证明 | 可以进入整改任务；不等于live已复测 |
| `E1-H-历史已证实` | 历史验收文件曾证明，但本次没有在当前发布重跑 | 可用于设计复测，不能声明当前通过 |
| `E2-高概率` | 静态代码显示高概率风险，但需要真实环境数据确认影响程度 | 必须先采集基线，不能直接宣称根因 |
| `E3-待核验` | 来自设计分析或历史证据，当前环境尚未复测 | 只能形成验证任务，不能写成完成事实 |
| `D-目标设计` | 本报告提出的目标状态 | 实施后必须由测试与证据闭环 |

## 使用方式

1. 先从`课题一主营业务流程闭环与中期详细设计对齐方案.md`确定统一Task/Plan/Run主链、release profile和要关闭的BF/stage；产品/UI工作再读`统一分析任务调度中心菜单与UI详细设计.md`；实现工作从`课题一PR里程碑代码级详细设计.md`选择对应里程碑父任务，并从完整整改报告确认对应`feature_id/canonical_id`。
2. 从机器task registry读取父任务的单类型`pr_sequence[]`；R00–R29还必须读取各自的叶子PR序列，外部CUSTODY/EXECUTE/ATTEST/APPROVAL读取为非PR依赖节点。再按主文49章生成原子PR执行包，解析精确locator/signature、字段合同、owner、候选身份、事务、测试oracle、依赖和回滚；未达到READY不得建实现PR。
3. 按“Feature Contract → additive Migration → consumer/projector ready → authority/outbox/producer → canary → reconcile → Typed UI → Tests → IDX → PROM”顺序实施。
4. 每个里程碑PROM前输出清单，复跑适用的兼容性、真实API/依赖、数据一致性、浏览器、性能、回滚和观察门；IDX和PROM不得同PR。
5. 未取得真实`EXPLAIN/PROFILE/query_log/trace/lag`及同候选不可变证据时，不得宣称数据库、链路、质量、发布或项目已经完成。
