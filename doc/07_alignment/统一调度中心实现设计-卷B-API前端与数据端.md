# 统一分析任务执行主链:调度中心实现设计 卷B(API 接口·前端执行链·后端数据端)

更新时间:2026-08-17
状态:<code>IMPLEMENTATION_DESIGN_CANDIDATE / NOT_EXECUTED</code>
配套:卷A(调度内核/服务执行链/数据库交互链)。

---

## 1. API 接口设计(analysis-service,前缀 `/api/v1/analysis`)

### 1.1 通用契约

```jsonc
// 成功
{ "success": true, "data": { ... }, "meta": { "revision": 12, "as_of": "..." } }
// 失败
{ "success": false, "error": { "code": "ANALYSIS_INVALID_TRANSITION", "message": "安全文案", "trace_id": "...", "detail": {} } }
```

- 幂等:写接口带 `Idempotency-Key`(客户端生成;缺失由服务端返回 `MISSING_IDEMPOTENCY_KEY` 400);`If-Match: <revision>` 用于治理/状态 CAS。
- 鉴权 scope:`analysis:read` / `analysis:run`(触发、取消、重试)/ `analysis:admin`(定义、计划审批、调度、资源)。
- 分页:`limit`(1-100,默认 20)+ `cursor`(HMAC 绑定 tenant+过滤参数+TTL,参照 alert 游标模式)。
- 状态正交(ENG-DOM-004):run 详情同时返回 `state/completeness/integrity_state/finding_conclusion/risk_severity/report_state`,**绝不合并成一个 status 字段**。

### 1.2 资源端点表

| # | 方法与路径 | 请求体要点 | 响应 | 错误码 |
|---|---|---|---|---|
| 1 | `POST /task-definitions` | name, default_class, owner | 201 definition(revision=1) | MISSING_IDEMPOTENCY_KEY;INVALID_TRANSITION |
| 2 | `GET /task-definitions` `GET /task-definitions/{id}` | — | 列表/详情(active plan/schedule/report policy 引用) | NOT_FOUND |
| 3 | `PATCH /task-definitions/{id}` | status 迁移(DRAFT→VALIDATED→ACTIVE→SUSPENDED→RETIRED), If-Match | 200 新 revision+receipt | ANALYSIS_INVALID_TRANSITION;REVISION_CONFLICT |
| 4 | `POST /plans` | task_definition_id, plan_source, source_spec(SourceKind/window/limits), selected_feature_ids, recognition_model_ref, detector/rule refs, thresholds, completion_policy, resource_budget | 201 plan_revision + 双哈希(execution_spec_sha256/plan_revision_sha256) | ANALYSIS_PLAN_* |
| 5 | `POST /plans/{id}:approve` `POST /plans/{id}:activate` | If-Match(governance revision);maker/checker | 200 governance head 新 revision+receipt | maker==checker 拒绝;REVISION_CONFLICT |
| 6 | `POST /schedules` | task_definition_id, approved_plan_revision(精确绑定), trigger_kind, timezone, cron/window 字段, prepare_lead_time, misfire_policy, concurrency_policy, class, restrictions | 201 schedule_revision+schedule_sha256 | ANALYSIS_PLAN_NOT_APPROVED |
| 7 | `POST /schedules/{id}:activate` `:pause` | If-Match | 200 ActivationHead revision | 同上 |
| 8 | `POST /triggers`(即时分析) | task_definition_id, plan_source=default|custom, custom_overrides?, window(source/start/end), Idempotency-Key | **202**(物化提交后) task_id+run_id+status_url `/runs/{run_id}` | FEATURE_NOT_RELEASED;CAPACITY_DENIED;IDEMPOTENCY_PAYLOAD_MISMATCH |
| 9 | `GET /tasks` `GET /tasks/{id}` | — | 列表(最多 7 列)/详情(current run 投影) | — |
| 10 | `GET /runs` | 筛选:definition/task/state/class/时间窗 | 一行一个 Run(最多 8 列) | — |
| 11 | `GET /runs/{id}` | — | 五轴状态+五阶段投影+结论+证据状态+报告状态+receipt refs | NOT_FOUND |
| 12 | `GET /runs/{id}/stages` | — | 每 ExecutionNode:attempt/state/fencing/counts/watermark | — |
| 13 | `GET /runs/{id}/results` | cursor;disposition 过滤 | 逐 input×detector 明细(DetectorDisposition) | — |
| 14 | `GET /runs/{id}/receipts` | — | StageReceipt 明细 | — |
| 15 | `POST /runs/{id}:cancel` | Idempotency-Key | 202 cancel receipt(非即完成;终态已提交→replay 该终态) | ANALYSIS_RUN_NOT_CANCELABLE |
| 16 | `POST /runs/{id}:retry` | Idempotency-Key | 202 新 run_id(同 task 下) | — |
| 17 | `GET /runs/{id}/summary` | — | MachineAnalysisSummary(终态前 404) | RUN_NOT_FINAL |
| 18 | `POST /runs/{id}/report` | template?, locale | 202 report_id(ReportState=QUEUED) | — |
| 19 | `GET /reports/{id}` `GET /reports/{id}/download` | — | ReportState/hash/size;受控下载(签名 URL+审计) | NOT_AVAILABLE |
| 20 | `GET /queue` `GET /leases` `GET /reservations`(admin) | — | 积压/容量/租约/预留 | FORBIDDEN |

### 1.3 关键请求/响应示例

```jsonc
// POST /triggers(即时分析,三步向导最终提交)
{
  "task_definition_id": "…", "plan_source": "default",           // 或 "custom"+custom_overrides
  "window": { "kind": "PCAP_REPLAY", "object_ref": "s3://flink-checkpoints/eval/…", "start": "…", "end": "…" },
  "client_context": { "purpose": "acceptance", "actor_note": "..." }
}
// 202 →
{ "success": true, "data": { "task_id": "…", "run_id": "…",
    "status_url": "/api/v1/analysis/runs/<run_id>",
    "execution_spec_sha256": "…" } }

// GET /runs/{id} →
{ "success": true, "data": {
    "run_id": "…", "task_id": "…", "state": "RUNNING",
    "completeness": "PARTIAL", "integrity_state": "UNVERIFIED",
    "finding_conclusion": "NOT_EVALUATED", "risk_severity": "UNKNOWN",
    "phases": [ { "phase": "S1", "state": "SUCCEEDED", "input_count": 1024, "fence": {...} }, … ],
    "report": { "state": "NOT_REQUESTED" },
    "receipts": { "reconcile": null, "closure": null } } }
```

---

## 2. 前端执行链(Web UI)

### 2.1 路由与页面(任务调度业务域,RouteManifest 新增 group `analysis`)

| 路由 | 页面 | 要点 |
|---|---|---|
| `/analysis/tasks` | 任务管理 | 七列列表;详情五 Tab(基本信息/方案版本/调度计划/报告策略/审计记录);不堆实时图表 |
| `/analysis/schedules` | 调度管理 | 四步创建(任务+批准方案→TriggerKind→窗口/并发/misfire/class→影响与审批);保存=DRAFT 不触发,激活才生效;暂停只影响未来 |
| `/analysis/orchestration` | 任务编排 | 只读五段(数据源→特征处理→加密识别→恶意检测→机器摘要);PlanReady/Reconcile 低调展示;无拖拽 DAG |
| `/analysis/runs` | 运行监控 | 八列;默认/自定义方案仅 Tag;即时分析三步向导(选任务与范围→选方案→校验提交)固定 ON_DEMAND |
| `/analysis/runs/:id` | 运行详情 | 四 Tab(概览/分析结果/证据/技术详情);概览=目标/阶段/结论/完整性/关键发现/报告入口 |
| `/analysis/reports` | 分析报告 | 机器摘要(随 Run 终态冻结,可查)/人读报告(异步,失败不回退 Run) |
| `/analysis/resources` | 调度资源(admin) | 容量配额/队列/租约/执行器四 Tab;首屏只答"哪里阻塞、影响多少 Run、下一动作" |

### 2.2 三步即时分析向导状态机(前端)

```ts
type InstantAnalysisFlow =
  | { step: 'scope'; taskDefinitionId?: string; window?: AnalysisWindow }
  | { step: 'plan'; planSource: 'default' | 'custom'; customOverrides?: PlanOverrides }
  | { step: 'review'; preflight?: PreflightReceipt }
  | { step: 'submitting'; idempotencyKey: string }
  | { step: 'accepted'; runId: string }
  | { step: 'failed'; code: AnalysisErrorCode };
```

- `planSource='custom'` 才展开可覆盖字段(**主业务链执行环节**:探针/采集源、特征 exact-set、加密识别模型、检测模型/规则、阈值);默认方案不展示逐项 feature/model/rule;自定义覆盖项进入 `custom_overrides`,审批字段先走 maker/checker。
- 提交:生成一次 idempotencyKey 并随重试复用;断线重试同 key 查 receipt(transport_unknown 语义);202 后跳转运行详情。

### 2.3 React Query 设计

```ts
// query keys 全参数化(ENG-UI-003):
['analysis','run', tenantId, sessionEpoch, runId]
['analysis','runs', tenantId, sessionEpoch, {state, class, cursor}]
['analysis','run-results', tenantId, sessionEpoch, runId, {cursor, disposition}]

// 运行详情轮询:先查 receipt,不盲目 refetch
useQuery({ queryKey, queryFn: fetchAnalysisRun, refetchInterval: isTerminal ? false : 5000,
           refetchIntervalInBackground: false, retry: 0 });
// mutation 不盲重试;transport timeout → 复用原 key 查询 receipt
useAnalysisRunMutation(runId, { onTransportUnknown: (key) => refetchReceipt(key) });
```

### 2.4 运行详情渲染规则(ENG-UI-004)

- 五轴独立渲染:`state`(RunState)、`completeness`、`integrity_state`、`finding_conclusion`、`risk_severity` 各自组件,未知枚举 fail-closed 显示"结果待确认"。
- `DetectorDisposition` 只在"分析结果"Tab 明细展示;`FindingConclusion` 是机器总体结论;`THREAT_FOUND` 时才附 `RiskSeverity`。
- 报告:`report.state=FAILED` 时文案"分析完成,报告生成失败",**不**把 Run 显示为失败,不伪装 AVAILABLE。
- 空态分级:no-data/not-evaluated/unknown/partial/stale 分别文案(复用 PageStateBoundary);只有 complete+fresh+verified 且领域判空为真才显示标准 Empty。

### 2.5 权限与可访问性

- 路由 `requiredScopes: analysis:read`;触发/取消/重试按钮 `analysis:run`;资源页 `analysis:admin`(服务端强制,前端仅隐藏)。
- 表格行键盘选中、Drawer 焦点陷阱与归还、状态用文本+图标双编码(不单靠颜色)、目标 ≥24×24(ENG-UI-002)。

---

## 3. 后端数据端(Kafka 合同)

### 3.1 Topic 目录(全部新 topic,首建在 init-jobs/01-kafka-topics.yaml 登记)

| topic | key | 语义 | 生产者 → 消费者 |
|---|---|---|---|
| `analysis.plan.events.v1` | `tenant+execution_spec_sha256`(compact) | plan-global canonical spec(无 task/run 上下文) | PlanCompiler → Flink 常驻作业 PlanReady |
| `analysis.run.events.v1` | `tenant+run_id` | RunSubscription(PREPARE/ACTIVE/CANCELLED revision)、run 状态事件 | Orchestrator → RunScopeRouter/执行器 |
| `analysis.envelopes.v1` | `tenant+run_id+community_id` | run-scoped 分析信封(AnalysisFlowEnvelope/AnalysisRecognitionEnvelope/DetectionOutcome) | RunScopeRouter → S2-S4 |
| `analysis.receipts.v1` | `tenant+run_id+execution_node_id+attempt` | StageReceipt / PlanReadyReceipt / source receipt | 执行器 → Orchestrator(PG inbox) |
| `analysis.report.requests.v1` | `tenant+report_id` | 人读报告请求 outbox 事件 | ReportCoordinator → Report worker |

### 3.2 信封 Schema(JSON,加法式)

```jsonc
// AnalysisFlowEnvelope(run-scoped,唯一承载执行上下文;base flow 不归属 Run)
{ "schema_version": "1", "tenant_id": "…", "task_id": "…", "run_id": "…",
  "execution_spec_sha256": "…", "window_id": "…", "stage_id": "S1", "attempt": 1,
  "fencing_token": "…", "event": { "flow": { … } } }

// RunSubscription(compact by tenant+run_id)
{ "schema_version": "1", "tenant_id": "…", "run_id": "…", "revision": 3,
  "state": "ACTIVE", "window": {…}, "execution_spec_sha256": "…", "fence": {…} }

// StageReceipt
{ "schema_version": "1", "run_id": "…", "execution_node_id": "RULE_DETECTION",
  "attempt": 1, "fencing_token": "…", "provider": "flink-rule-job",
  "input_count": 1024, "output_count": 1024, "error_count": 0, "reject_count": 0,
  "watermark": "…", "fence": {…}, "payload_hash": "…" }
```

### 3.3 兼容与迁移

- 既有 v1 topic 的 key 合同(`tenant_id+community_id`)不变;run 身份只出现在新 envelope;需要物理分区按 run 隔离时才新建 v2 topic(双写→影子消费→diff→切换,ENG-EVOL-001)。
- 派生分支 Kafka key、Flink keyed state、PG 唯一键、CH 结果键、MinIO 对象路径、报告查询都至少绑定 `tenant+task+run+execution_spec_sha256`。

---

## 4. 四链首尾衔接清单(实现自检)

| 链 | 起点 | 终点 | 必须闭环的证据 |
|---|---|---|---|
| 前端执行链 | 三步向导提交(Idempotency-Key) | 运行详情五轴+报告页 | 202 物化 receipt;轮询收敛;报告 FAILED 不回退 Run |
| 后端数据端 | TriggerInstance | analysis.* topic + receipt | identity 判重;compact key;offset 仅在 PG 提交后 |
| 后端服务执行链 | Scheduler.Tick/Orchestrator.Advance | Finalizer 三件套+终态 | 每事务 SQL 模板;状态机穷举;Reconcile always-run |
| 数据库交互链 | 25-analysis-scheduler-v1.sql(expand) | PG 权威 + CH 结果(带 run_id) | 唯一约束/幂等/CAS;FINAL 有性能依据才用 |

实现顺序仍按卷A §4 列车顺序;每列车验收以本卷对应小节为准,缺一节不进入下一列车。
