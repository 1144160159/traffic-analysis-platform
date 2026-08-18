# 统一分析任务执行主链:调度中心实现设计 卷A(调度内核·服务执行链·数据库交互链)

更新时间:2026-08-17
状态:<code>IMPLEMENTATION_DESIGN_CANDIDATE / NOT_EXECUTED</code>
上游真源:`课题一主营业务流程闭环与中期详细设计对齐方案.md` §6-§10、§76 各节(步骤编号 FS/ER/DT/MF/HR 沿用);`核心主链代码级设计-v1.md`(函数卡)。
本卷覆盖:调度内核算法、Go 服务执行链(权威事务)、数据库交互链(DDL+SQL 模板)。API/前端/数据端见卷B。

---

## 1. 调度内核设计

### 1.1 生命周期总览

```text
Definition(DRAFT→VALIDATED→ACTIVE→SUSPENDED→RETIRED)
  ├─ PlanRevision(不可变;GovernanceHead CAS: DRAFT→VALIDATED→APPROVED→ACTIVE→RETIRED)
  └─ ScheduleRevision(不可变;ActivationHead: DRAFT→ACTIVE⇄PAUSED→RETIRED;精确绑定一个已批准 plan revision)
TriggerInstance(PENDING_MATERIALIZATION→MATERIALIZED|SUPPRESSED|QUARANTINED)
  → AnalysisTask(1:1 MATERIALIZED)
  → AnalysisRun(ACCEPTED→PREPARING→QUEUED→RUNNING→FINALIZING→SUCCEEDED|PARTIALLY_SUCCEEDED|FAILED;可 CANCEL_REQUESTED→CANCELLED)
  → StageAttempt(PENDING→DISPATCHED→RUNNING→SUCCEEDED|PARTIAL|FAILED / SKIPPED(reason)/CANCELLED)
  → Receipt → Result → MachineSummary → 异步 HumanReport
```

### 1.2 触发与物化

**物化身份判别联合**(数据库唯一键 `tenant+identity_kind+canonical_identity_hash`):

| TriggerKind | identity |
|---|---|
| ON_DEMAND | tenant + actor + client_idempotency_key |
| CRON_WINDOW | tenant + schedule_revision + window_id |
| CONTINUOUS_WINDOW | tenant + schedule_revision + window_id |
| EVENT_DRIVEN | tenant + schedule_revision + debounce_bucket_id |

**Scheduler.Tick 伪代码**(每 5s,单实例经 PG advisory lock 抢租约):

```go
func (s *Scheduler) Tick(ctx context.Context) error {
    lock := s.acquireSchedulerLease(ctx)          // pg_advisory_lock(scheduler_tick, 10s TTL)
    if !lock { return nil }
    defer lock.Release()

    for _, sch := range s.repo.ListActiveSchedules(ctx) {
        next, ok := sch.NextWindow(s.now(), s.lastTick)   // 时区+窗口对齐+prepare_lead_time
        if !ok { continue }
        switch sch.MisfirePolicy {
        case MISFIRE_FAIL:     s.repo.RecordMisfire(ctx, sch, next)
        case MISFIRE_DELAY:    next = next.ShiftedTo(s.now())
        case MISFIRE_BOUNDED_REPLAY: s.routeToBoundedReplay(ctx, sch, next) // 只走 Kafka bounded replay/PCAP,禁止假装 LIVE 回填
        }
        trigger := TriggerInstance{
            Identity:   TriggerIdentity{ScheduleRevision: sch.Revision, WindowID: next.WindowID()},
            RequestSHA: sha256(canonical(triggerPayload(sch, next))),
            State:      PENDING_MATERIALIZATION,
        }
        if err := s.repo.InsertTriggerInstance(ctx, trigger); err != nil { /* dup → skip */ continue }
        s.materializer.Enqueue(ctx, trigger)   // 同进程 outbox → MaterializeAnalysisTaskAtomic
    }
    return nil
}
```

**OnDemandTrigger.Submit**:RBAC(analysis:run scope)→ 服务端 preflight(权限/范围/容量/兼容性)→ `plan_source=default` 走 DefaultPlanResolver;**`plan_source=custom` 走 CustomPlanResolver(主业务链执行环节:人工选择探针/特征/识别模型/检测模型/阈值,覆盖项→NormalizedAnalysisIntent+selection_origins,审批字段 maker/checker)→ 与默认计划经同一 PlanCompiler 冻结** → 生成 idempotency key(客户端提供则复用)→ 提交 TriggerInstance → 物化 → **HTTP 202 仅在物化事务提交后返回**,body 含 task_id/run_id/status_url。

**EventTrigger.AcceptAtomic**:按 `tenant+schedule_revision+debounce_bucket_id` 去重窗口;同 bucket 重复事件只登记不重复物化。

### 1.3 队列与准入

- `effective_class = authorized_trigger_override ?? schedule.class ?? definition.default_class`(class 变化必须单独 RBAC 授权)。
- `hard_cap[dim] = min(tenant_policy[dim], definition_budget[dim], plan_envelope[dim], schedule_restriction[dim], trigger_limit[dim])`;`requested[dim] = trigger ?? schedule ?? plan preferred`,满足 `plan.min ≤ allocation ≤ hard_cap`。
- AdmissionReservation:`RESERVED→CONSUMED→RELEASED` 或 `RESERVED→EXPIRED`;冻结 run/resource pool/vector/policy_sha256/epoch/expires_at/authority_revision;过期必须重新准入;Run 终态、取消、lease 回收均释放配额。
- LIVE 窗口:`prepare_at = window_start - prepare_lead_time` 前完成 Trigger 冻结+PlanReady+Admission;`window_start` 后才激活采集订阅;未按时 ready 只能 misfire 策略处理。

### 1.4 Lease & Fencing

```go
type StageLease struct {
    TenantID, TaskID, RunID, StageID string
    ExecutionNodeID string; Attempt int
    FencingToken string   // 单调;旧 token 的 receipt 一律 QUARANTINED
    ExpiresAt time.Time   // heartbeat 续租;过期回收为 PENDING/attempt+1
}
```

租约仅由 Queue&Allocator 事务发放;执行器 receipt 必须回带 fencing_token;`ValidateStageAttemptTransition` 拒绝 terminal 回退、attempt gap、旧 fencing token。

### 1.5 Durable Orchestrator.Advance

```go
func (o *Orchestrator) Advance(ctx, runID string) (AdvanceDecision, error) {
    run := o.repo.LockRun(ctx, runID)            // SELECT ... FOR UPDATE
    if run.Terminal() { return TerminalReplay(run) }

    // A. PlanReady 屏障(阶段1前,不可删除)
    if run.Phase == PREPARING && !o.planReady(ctx, run) { return Wait(WAIT_PLAN_READY) }
    if run.Phase == PREPARING && o.admissionValid(ctx, run) {
        o.repo.TransitionRun(ctx, run, PREPARING→QUEUED, EvPlanReady)
    }
    if run.Phase == QUEUED && o.hasNodeLease(ctx, run) {
        o.repo.TransitionRun(ctx, run, QUEUED→RUNNING, EvLeaseAcquired)
    }

    // B. 选择 0..N 个可派发节点(RuleDetection 与 BehaviorDetection 可同次选中)
    dispatchable := []ExecutionNode{}
    for _, node := range run.Plan.StageDAG.Ordered() {
        latest := o.repo.LatestAttempt(ctx, run, node)
        if latest.TerminalOrSkipped() { continue }
        if latest.Active() { continue }                    // 已有活跃 attempt
        switch node.ActivationMode {
        case PIPELINED_STREAM:                             // 数据到来即流水,不等上游终态
            if o.subscriptionActive(ctx, run) && o.reservationConsumed(ctx, run) { dispatchable = append(dispatchable, node) }
        case AFTER_UPSTREAM_CLOSE:                          // 必须消费冻结 manifest(离线节点)
            if o.upstreamManifestFrozen(ctx, run, node) { dispatchable = append(dispatchable, node) }
        case AUTHORITY_LOCAL:                               // Reconcile/Finalizer,谓词驱动
            if node.IsReconcile && o.allBusinessNodesTerminal(ctx, run) { dispatchable = append(dispatchable, node) }
        }
    }
    for _, node := range dispatchable {
        o.repo.CreateStageAttempt(ctx, run, node)          // + 发 dispatch outbox(SHARED_STREAM 只写逻辑准入/subscription/PlanReady ACK,不建新 Job)
    }

    // C. 0 候选 → 明确 wait reason
    if len(dispatchable) == 0 {
        return Wait(firstWaitReason(ctx, run))   // WAIT_PLAN_READY/WAIT_WINDOW_START/WAIT_WATERMARK/WAIT_PROVIDER_ACK/WAIT_CAPACITY/READY_TO_RECONCILE/READY_TO_FINALIZE/UNRECOVERABLE_FAILURE
    }
    return Dispatched(dispatchable)
}
```

### 1.6 取消与重试

- 取消拆三步:`RequestCancelRunAtomic`(冻结 CancelTargetManifest:全部 active attempt/READY 队列/未确认 dispatch/outbox/订阅 revision)→ 逐 attempt 取消回执 → `EvaluateCancelClosureAtomic`(exact-set 全 terminal/drained/fenced → Reconcile+Finalizer 冻结取消型三件套 → CAS CANCELLED → 释放配额)。HTTP 断开、单个 executor ACK、仅写 CANCEL_REQUESTED 都不等于取消完成。
- Stage retry 前置:`input_replay_manifest_sha256` 与 provider replay capability 校验;无重放输入的 SHARED_STREAM 返回 `STAGE_RETRY_UNSUPPORTED` → 整 Run retry(新 run,不复写旧终态)。

---

## 2. 后端服务执行链(Go analysis-service)

### 2.1 包结构与职责

```text
go/control-plane/cmd/analysis-service/main.go      装配:wiring、outbox dispatcher、scheduler loop、orchestrator loop、lease reaper、report coordinator
go/control-plane/internal/analysis/
  api/        薄 handler:decode/auth/幂等头/状态码;零业务逻辑
  service/    plan_compiler.go materializer.go scheduler.go trigger.go allocator.go
              orchestrator.go reconciler.go finalizer.go human_report.go report_policy.go
  repository/ task_def.go plan.go schedule.go trigger.go run.go stage.go receipt.go
              result.go summary.go report.go admission.go inbox.go outbox.go
  state/      run_state.go stage_state.go run_closure.go(纯函数)
  contract/   errors.go(稳定错误码表) topics.go envelope.go
  config/     loader.go(fail-closed:无 JWT key 拒启)
```

### 2.2 权威事务逐一(SQL 模板随 §3)

| 事务 | 语义 |
|---|---|
| SaveTaskDefinitionRevisionAtomic | 幂等 identity+request_sha256 → 插不可变 revision → CAS definition head + history/audit/receipt → COMMIT |
| SavePlanDraftAtomic | 校验 catalog/权限 revision → Compile → 插不可变 plan spec+双哈希 → 插 governance head(DRAFT)+history → COMMIT |
| ApproveOrActivatePlanAtomic | maker/checker + expected governance revision → 验 plan/hash/兼容 → CAS PlanGovernanceHead + approval/history/audit/receipt → COMMIT |
| SaveOrActivateScheduleRevisionAtomic | 验精确 approved plan 绑定 + restriction-only 合并 → 插不可变 schedule spec → CAS ScheduleActivationHead + history/audit/receipt → COMMIT |
| MaterializeAnalysisTaskAtomic | 见 §3.2 全量 SQL;202 只在提交后 |
| ApplyStageReceiptAtomic | 见 §3.3;inbox outcome ∈ RECEIVED/APPLIED/REPLAYED/QUARANTINED_HASH_CONFLICT/STALE_FENCE/LATE_TERMINAL;只有 DB/依赖临时不可用才 rollback 重试;确定性非法消息同事务提交 quarantine 后即提交 offset |
| Reconciler.RunAtomic / FinalizeRunAtomic | 见代码级设计卷 §2.4/2.5 |
| RequestHumanReportAtomic | 验 Run 终态+摘要 hash+模板权限 → tenant+run+summary hash+template+locale 身份 → 插 HumanReadableReport(QUEUED)+outbox → COMMIT |
| ApplyHumanReportReceiptAtomic | inbox 去重 → 身份/hash/size 校验 → PG metadata + ReportState 推进;事务内不碰 MinIO;不改 RunState |
| RequestCancelRunAtomic / EvaluateCancelClosureAtomic | 见 §1.6 |

### 2.3 消费者/执行器矩阵(服务执行链)

| 组件 | 消费 | 产出 |
|---|---|---|
| RunScopeRouter(Flink,共享分支) | base flow.events.v1 + RunSubscription | analysis.envelopes.v1(run-scoped,0..N) |
| Session/Feature/Recognition/Detection(Flink 常驻) | envelopes + plan-global spec | 结果写 CH(带 run_id)+ StageReceipt |
| ProbeCaptureWindowExecutor(Rust) | typed capture command | source receipt(packet/byte/drop)+ envelope |
| PcapReplayAdapter(analysis-service worker) | replay command | 同上 |
| Orchestrator/Reconciler/Finalizer(analysis-service) | receipts(PG inbox) | 状态推进、三件套、终态 |
| HumanReport worker(独立对象) | report outbox | 对象+ReportArtifactReceipt |
| AutoReportCoordinator | run 终态事件 | 调 RequestHumanReportAtomic(DISABLED/ON_DEMAND/AUTO_ASYNC) |

### 2.4 错误码表(analysis 域)

| code | HTTP | 语义 |
|---|---|---|
| ANALYSIS_PLAN_NOT_APPROVED | 409 | schedule/trigger 绑定未批准 plan |
| ANALYSIS_IDEMPOTENCY_PAYLOAD_MISMATCH | 409 | 同 identity 异 request_sha256 |
| ANALYSIS_WINDOW_MISFIRED | 409 | LIVE 窗口已错过,misfire 策略裁决 |
| ANALYSIS_CAPACITY_DENIED | 429 | hard_cap 拒绝(带 retry-after/缺口维度) |
| ANALYSIS_STALE_FENCE | 409 | 旧 fencing token receipt |
| ANALYSIS_RUN_NOT_CANCELABLE | 409 | 终态已提交 |
| ANALYSIS_FEATURE_NOT_RELEASED | 404 | MANUAL_CUSTOM 未发布 |
| ANALYSIS_STAGE_RETRY_UNSUPPORTED | 409 | 无重放输入 |
| ANALYSIS_INVALID_TRANSITION | 409 | 状态机非法越迁 |

---

## 3. 数据库交互链

### 3.1 PG DDL(common/sql/pg/25-analysis-scheduler-v1.sql 大纲,全部幂等)

```sql
CREATE TABLE IF NOT EXISTS analysis_task_definitions(
  id UUID PRIMARY KEY, tenant_id TEXT NOT NULL, name TEXT NOT NULL,
  status TEXT NOT NULL DEFAULT 'DRAFT',               -- DRAFT|VALIDATED|ACTIVE|SUSPENDED|RETIRED
  active_plan_revision BIGINT, active_schedule_revision BIGINT,
  default_scheduling_class TEXT NOT NULL DEFAULT 'BASELINE',
  report_policy_revision BIGINT,
  revision BIGINT NOT NULL, created_by TEXT NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(), updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE(tenant_id, name), CHECK(revision>0));

CREATE TABLE IF NOT EXISTS analysis_plan_revisions(
  id UUID PRIMARY KEY, tenant_id TEXT NOT NULL, task_definition_id UUID NOT NULL REFERENCES analysis_task_definitions(id),
  plan_revision BIGINT NOT NULL, plan_source TEXT NOT NULL,  -- AUTO_DEFAULT|MANUAL_CUSTOM
  source_spec JSONB NOT NULL, selected_feature_ids JSONB NOT NULL, feature_set_id TEXT NOT NULL,
  encrypted_recognition_model_ref JSONB NOT NULL, threat_detector_refs JSONB NOT NULL, rule_refs JSONB NOT NULL,
  machine_summary_schema_ref TEXT NOT NULL, stage_dag JSONB NOT NULL, completion_policy JSONB NOT NULL,
  resource_budget JSONB NOT NULL, catalog_revision BIGINT NOT NULL, selection_origins JSONB NOT NULL,
  canonicalization_version TEXT NOT NULL,
  execution_spec_sha256 TEXT NOT NULL, plan_revision_sha256 TEXT NOT NULL,
  created_by TEXT NOT NULL, created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE(tenant_id, task_definition_id, plan_revision));

CREATE TABLE IF NOT EXISTS analysis_schedule_revisions(
  id UUID PRIMARY KEY, tenant_id TEXT NOT NULL, task_definition_id UUID NOT NULL,
  approved_plan_revision BIGINT NOT NULL, execution_spec_sha256 TEXT NOT NULL,
  trigger_kind TEXT NOT NULL, timezone TEXT NOT NULL DEFAULT 'UTC',
  cron_or_window JSONB NOT NULL, prepare_lead_time INTERVAL NOT NULL DEFAULT '5 minutes',
  misfire_policy TEXT NOT NULL DEFAULT 'MISFIRE_FAIL', concurrency_policy TEXT NOT NULL DEFAULT 'FORBID_OVERLAP',
  scheduling_class TEXT NOT NULL, resource_restrictions JSONB NOT NULL,
  revision BIGINT NOT NULL, schedule_sha256 TEXT NOT NULL,
  UNIQUE(tenant_id, task_definition_id, revision));

CREATE TABLE IF NOT EXISTS analysis_trigger_instances(
  id UUID PRIMARY KEY, tenant_id TEXT NOT NULL,
  identity_kind TEXT NOT NULL, canonical_identity_hash TEXT NOT NULL, request_sha256 TEXT NOT NULL,
  state TEXT NOT NULL DEFAULT 'PENDING_MATERIALIZATION',  -- |MATERIALIZED|SUPPRESSED|QUARANTINED
  materialized_task_id UUID, trigger_kind TEXT NOT NULL, window_id TEXT, created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE(tenant_id, identity_kind, canonical_identity_hash));

CREATE TABLE IF NOT EXISTS analysis_tasks(
  id UUID PRIMARY KEY, tenant_id TEXT NOT NULL, task_definition_id UUID NOT NULL,
  plan_revision BIGINT NOT NULL, execution_spec_sha256 TEXT NOT NULL,
  schedule_revision BIGINT, trigger_instance_id UUID NOT NULL REFERENCES analysis_trigger_instances(id),
  effective_class TEXT NOT NULL, effective_policy_sha256 TEXT NOT NULL,
  current_run_id UUID, created_at TIMESTAMPTZ NOT NULL DEFAULT now());

CREATE TABLE IF NOT EXISTS analysis_runs(
  id UUID PRIMARY KEY, tenant_id TEXT NOT NULL, task_id UUID NOT NULL REFERENCES analysis_tasks(id),
  state TEXT NOT NULL DEFAULT 'ACCEPTED',       -- ACCEPTED|PREPARING|QUEUED|RUNNING|FINALIZING|SUCCEEDED|PARTIALLY_SUCCEEDED|FAILED|CANCEL_REQUESTED|CANCELLED
  completeness TEXT NOT NULL DEFAULT 'UNKNOWN', integrity_state TEXT NOT NULL DEFAULT 'UNVERIFIED',
  finding_conclusion TEXT NOT NULL DEFAULT 'NOT_EVALUATED', risk_severity TEXT NOT NULL DEFAULT 'UNKNOWN',
  window_start TIMESTAMPTZ, window_end TIMESTAMPTZ, revision BIGINT NOT NULL DEFAULT 1,
  started_at TIMESTAMPTZ, finalized_at TIMESTAMPTZ, created_at TIMESTAMPTZ NOT NULL DEFAULT now(), CHECK(revision>0));

CREATE TABLE IF NOT EXISTS analysis_stage_attempts(
  id UUID PRIMARY KEY, run_id UUID NOT NULL REFERENCES analysis_runs(id),
  business_phase_id TEXT NOT NULL,              -- S1..S5
  execution_node_id TEXT NOT NULL,              -- SESSIONIZATION|FEATURE_EXTRACTION|ENCRYPTED_RECOGNIZER|RULE_DETECTION|BEHAVIOR_DETECTION|DETECTION_AGGREGATE|RECONCILE|MACHINE_FINALIZATION|...
  attempt INT NOT NULL, state TEXT NOT NULL DEFAULT 'PENDING',
  provider_mode TEXT NOT NULL, activation_mode TEXT NOT NULL,
  fencing_token TEXT, lease_expires_at TIMESTAMPTZ,
  started_at TIMESTAMPTZ, finished_at TIMESTAMPTZ, created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE(run_id, execution_node_id, attempt));

CREATE TABLE IF NOT EXISTS analysis_stage_receipts(
  id UUID PRIMARY KEY, run_id UUID NOT NULL, execution_node_id TEXT NOT NULL, attempt INT NOT NULL,
  fencing_token TEXT NOT NULL, provider TEXT NOT NULL,
  input_count BIGINT NOT NULL DEFAULT 0, output_count BIGINT NOT NULL DEFAULT 0,
  error_count BIGINT NOT NULL DEFAULT 0, reject_count BIGINT NOT NULL DEFAULT 0,
  watermark TIMESTAMPTZ, fence JSONB NOT NULL, payload_hash TEXT NOT NULL,
  received_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE(run_id, execution_node_id, attempt, payload_hash));

CREATE TABLE IF NOT EXISTS analysis_results(
  id UUID PRIMARY KEY, run_id UUID NOT NULL, input_identity TEXT NOT NULL,
  detector_id TEXT NOT NULL, disposition TEXT NOT NULL,      -- POSITIVE|NEGATIVE|INCONCLUSIVE|INCOMPATIBLE|ERROR|NOT_RUN
  score DOUBLE PRECISION, labels JSONB, evidence_refs JSONB,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE(run_id, input_identity, detector_id));

CREATE TABLE IF NOT EXISTS analysis_machine_summaries(
  run_id UUID PRIMARY KEY REFERENCES analysis_runs(id),
  finding_conclusion TEXT NOT NULL, risk_severity TEXT NOT NULL,
  completeness TEXT NOT NULL, integrity_state TEXT NOT NULL,
  scope JSONB NOT NULL, key_findings JSONB NOT NULL, limitations JSONB NOT NULL,
  evidence_manifest_hash TEXT NOT NULL, closure_manifest_hash TEXT NOT NULL,
  canonical_sha256 TEXT NOT NULL, created_at TIMESTAMPTZ NOT NULL DEFAULT now());

CREATE TABLE IF NOT EXISTS analysis_evidence_manifests(
  run_id UUID PRIMARY KEY, entries JSONB NOT NULL, sha256 TEXT NOT NULL, created_at TIMESTAMPTZ NOT NULL DEFAULT now());

CREATE TABLE IF NOT EXISTS analysis_run_closure_manifests(
  run_id UUID PRIMARY KEY, decision_inputs JSONB NOT NULL, priority INTEGER NOT NULL,
  node_exact_set JSONB NOT NULL, differences JSONB NOT NULL, sha256 TEXT NOT NULL, created_at TIMESTAMPTZ NOT NULL DEFAULT now());

CREATE TABLE IF NOT EXISTS analysis_human_reports(
  id UUID PRIMARY KEY, run_id UUID NOT NULL, summary_sha256 TEXT NOT NULL,
  template_revision TEXT NOT NULL, locale TEXT NOT NULL,
  state TEXT NOT NULL DEFAULT 'QUEUED',          -- QUEUED|GENERATING|VERIFYING|AVAILABLE|FAILED|CANCELLED
  object_key TEXT, object_sha256 TEXT, object_size BIGINT,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(), updated_at TIMESTAMPTZ NOT NULL DEFAULT now());

CREATE TABLE IF NOT EXISTS analysis_admission_reservations(
  id UUID PRIMARY KEY, run_id UUID NOT NULL, resource_pool TEXT NOT NULL,
  resource_vector JSONB NOT NULL, policy_sha256 TEXT NOT NULL,
  state TEXT NOT NULL DEFAULT 'RESERVED',        -- RESERVED|CONSUMED|RELEASED|EXPIRED
  epoch BIGINT NOT NULL, expires_at TIMESTAMPTZ NOT NULL, authority_revision BIGINT NOT NULL,
  UNIQUE(run_id, resource_pool));

CREATE TABLE IF NOT EXISTS analysis_inbox(
  event_id TEXT PRIMARY KEY, tuple_hash TEXT NOT NULL, outcome TEXT NOT NULL,
  received_at TIMESTAMPTZ NOT NULL DEFAULT now());

CREATE TABLE IF NOT EXISTS analysis_outbox(
  id BIGSERIAL PRIMARY KEY, event_id TEXT NOT NULL UNIQUE, topic TEXT NOT NULL,
  key TEXT NOT NULL, payload JSONB NOT NULL, state TEXT NOT NULL DEFAULT 'PENDING',
  attempts INT NOT NULL DEFAULT 0, next_attempt_at TIMESTAMPTZ NOT NULL DEFAULT now());

CREATE TABLE IF NOT EXISTS analysis_history(
  id BIGSERIAL PRIMARY KEY, tenant_id TEXT NOT NULL, entity TEXT NOT NULL, entity_id UUID NOT NULL,
  action TEXT NOT NULL, actor TEXT NOT NULL, detail JSONB NOT NULL DEFAULT '{}'::jsonb,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now());
CREATE INDEX IF NOT EXISTS idx_analysis_history_lookup ON analysis_history(tenant_id, entity, entity_id, created_at);
```

治理头(PlanGovernanceHead/ScheduleActivationHead)与 revision head(definition/plan/schedule)用单行表 + CAS UPDATE(`UPDATE ... SET revision=$new WHERE revision=$expected`),行数!=1 即冲突。

### 3.2 MaterializeAnalysisTaskAtomic 事务 SQL 模板

```sql
BEGIN;
SELECT pg_advisory_xact_lock(hashtextextended($canonical_identity_hash::text, 0));
INSERT INTO analysis_materialization_ledger(identity_hash, request_sha256, created_at)
  VALUES($1,$2,now()) ON CONFLICT (identity_hash) DO NOTHING;
-- 影响 0 行 → SELECT request_sha256:同值=精确重放(返回原 receipt),异值=完整性冲突 409
SELECT ... FROM analysis_trigger_instances WHERE id=$t FOR UPDATE;      -- state=PENDING_MATERIALIZATION 否则拒绝
SELECT ... FROM analysis_task_definitions WHERE id=$d FOR UPDATE;
SELECT ... FROM analysis_plan_revisions WHERE id=$p AND execution_spec_sha256=$h FOR UPDATE; -- 不匹配→-002
-- 窗口/capacity 校验(读 effective policy + admission 表)→ 不通过→SUPPRESSED/容量拒绝
INSERT INTO analysis_tasks(...) VALUES(...) RETURNING id;
INSERT INTO analysis_runs(...) VALUES(..., 'ACCEPTED', ...) RETURNING id;
INSERT INTO analysis_stage_attempts(run_id, business_phase_id, execution_node_id, attempt, state, provider_mode, activation_mode)
  SELECT $run, p.* FROM jsonb_to_recordset($node_exact_set::jsonb) AS p(...);  -- required ExecutionNode exact-set 种子
INSERT INTO analysis_business_phase_projections(run_id, phase, state) VALUES(...); -- 五段种子
UPDATE analysis_trigger_instances SET materialized_task_id=$task, state='MATERIALIZED'
  WHERE id=$t AND state='PENDING_MATERIALIZATION'; -- rows=1 否则冲突
UPDATE analysis_tasks SET current_run_id=$run WHERE id=$task;
INSERT INTO analysis_history(...) VALUES(...);
INSERT INTO analysis_outbox(event_id, topic, key, payload) VALUES($ev,'analysis.run.events.v1',$run, ...);
INSERT INTO analysis_analysis_receipts(...) VALUES(...);
COMMIT;
```

### 3.3 ApplyStageReceiptAtomic 事务 SQL 模板

```sql
BEGIN;
INSERT INTO analysis_inbox(event_id, tuple_hash, outcome) VALUES($1,$2,'RECEIVED')
  ON CONFLICT (event_id) DO NOTHING;          -- 冲突→outcome=REPLAYED(精确重放)
SELECT * FROM analysis_stage_attempts
  WHERE run_id=$run AND execution_node_id=$node AND attempt=$attempt FOR UPDATE;  -- 缺行→attempt gap 隔离
-- fencing_token 与 attempt.fencing_token 不匹配→STALE_FENCE→quarantine fact+outcome=QUARANTINED,提交后 ACK
-- attempt 已 terminal→LATE_TERMINAL→quarantine fact+提交后 ACK
INSERT INTO analysis_stage_receipts(run_id, execution_node_id, attempt, fencing_token, provider,
  input_count, output_count, error_count, reject_count, watermark, fence, payload_hash)
  VALUES(...) ON CONFLICT (run_id,execution_node_id,attempt,payload_hash) DO NOTHING;  -- 同 tuple 异 hash→integrity failure
UPDATE analysis_stage_attempts SET state=$new, finished_at=now()
  WHERE id=$a AND state=$expected;             -- rows=1;=0 且非重放→LATE_TERMINAL
-- 重算 business_phase_projection(节点 exact-set 确定性投影)
INSERT INTO analysis_history(...) VALUES(...);
INSERT INTO analysis_outbox(...) VALUES(...,'analysis.run.events.v1',...);  -- 下一阶段 ready 时
COMMIT;
-- Kafka offset 提交仅在 COMMIT 后;确定性非法消息在 quarantine 提交后即提交 offset(不无限重投)
```

### 3.4 ClickHouse 结果表(additive,run-scoped)

```sql
CREATE TABLE IF NOT EXISTS analysis_detections_local ON CLUSTER traffic_cluster (
  tenant_id String, run_id String, execution_spec_sha256 String,
  input_identity String, detector_id String, disposition LowCardinality(String),
  score Float64, labels String, evidence_refs String,
  ts DateTime64(3) DEFAULT now64(3)
) ENGINE = ReplicatedMergeTree('/clickhouse/tables/{shard}/analysis_detections_local','{replica}')
PARTITION BY toYYYYMM(ts) ORDER BY (tenant_id, run_id, input_identity, detector_id);
-- Distributed 视图 analysis_detections
-- 另有 analysis_run_results_local(阶段计数/fence/watermark,ORDER BY (tenant_id,run_id,execution_node_id,attempt))
```

### 3.5 迁移顺序(ENG-EVOL-002 expand)

1) 建表(全部 additive)→ 2) 既有 flow/session/alert 表 `ALTER TABLE ADD COLUMN IF NOT EXISTS run_id String DEFAULT ''`(legacy-unattributed)→ 3) 双读影子对账 → 4) 切读 → 5) contract。禁止同一 PR 内 producer 发送消费者不认识的消息。

---

## 4. 落地顺序(PR 列车→包)

| 列车 | 产出包/文件 | 依赖 |
|---|---|---|
| ATC-CONTRACT | contract/errors.go topics.go envelope.go;proto analysis.v1 | — |
| ATC-DATA | common/sql/pg/25-analysis-scheduler-v1.sql + init-jobs 镜像 + CH additive | ATC-CONTRACT |
| ATC-AUTH | repository/{task_def,plan,schedule}.go + 四个定义/计划事务 | ATC-DATA |
| ATC-SCHED | scheduler.go trigger.go allocator.go lease.go | ATC-AUTH |
| ATC-ORCH | orchestrator.go reconciler.go inbox/outbox | ATC-SCHED |
| **ATC-CUSTOM(核心,与 AUTO 同拓扑)** | plan_resolver.go(CustomPlanResolver)、preflight、审批链、前端 custom 向导分支 | ATC-AUTH |
| ATC-PROBE/FEATURE/RECOGNITION/DETECTION | Rust capture_window / Java 数据面改造 | ATC-ORCH |
| ATC-SUMMARY | finalizer.go + CH 结果表 | ATC-DETECTION |
| ATC-HREPORT | human_report.go + worker | ATC-SUMMARY |
| ATC-UI | 卷B 前端执行链 | ATC-SUMMARY |

每列车的实现必须携带:函数卡(代码级设计卷 §2-§4)、模式 ADR 引用、§7 oracle 测试。禁止跳列(如无 ATC-DATA 先写 ATC-AUTH 事务)。
