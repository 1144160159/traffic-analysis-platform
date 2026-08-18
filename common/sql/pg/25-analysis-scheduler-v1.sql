-- =============================================================================
-- 统一分析任务调度中心权威 schema(T1-M10 核心主链,P0 合同冻结)
-- 权威约束:ENG-ARCH-002/ENG-CMD-001 —— 命令受理=PG 同事务 state+history+audit+
-- outbox+receipt;Redis 只承担幂等辅助。全部 IF NOT EXISTS,幂等可重复执行。
-- =============================================================================

BEGIN;

-- 任务定义
CREATE TABLE IF NOT EXISTS analysis_task_definitions (
    id UUID PRIMARY KEY,
    tenant_id TEXT NOT NULL,
    name TEXT NOT NULL,
    state TEXT NOT NULL DEFAULT 'DRAFT',          -- DRAFT|VALIDATED|ACTIVE|SUSPENDED|RETIRED
    owner TEXT NOT NULL DEFAULT '',
    active_plan_revision BIGINT,
    active_schedule_revision BIGINT,
    default_scheduling_class TEXT NOT NULL DEFAULT 'BASELINE',
    report_policy_revision BIGINT,
    revision BIGINT NOT NULL DEFAULT 1,
    created_by TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT uq_analysis_task_definitions_name UNIQUE (tenant_id, name),
    CONSTRAINT chk_analysis_task_definitions_rev CHECK (revision > 0)
);

-- 不可变计划修订(spec 只 INSERT;治理状态由 governance head CAS 推进)
CREATE TABLE IF NOT EXISTS analysis_plan_revisions (
    id UUID PRIMARY KEY,
    tenant_id TEXT NOT NULL,
    task_definition_id UUID NOT NULL REFERENCES analysis_task_definitions(id),
    plan_revision BIGINT NOT NULL,
    plan_source TEXT NOT NULL,                    -- AUTO_DEFAULT|MANUAL_CUSTOM
    source_kind TEXT NOT NULL,
    source_spec JSONB NOT NULL,
    selected_feature_ids JSONB NOT NULL,
    feature_set_id TEXT NOT NULL,
    encrypted_recognition_model_ref TEXT NOT NULL DEFAULT '',
    threat_detector_refs JSONB NOT NULL,
    rule_refs JSONB NOT NULL,
    machine_summary_schema_ref TEXT NOT NULL DEFAULT '',
    stage_dag JSONB NOT NULL,
    completion_policy JSONB NOT NULL,
    resource_budget JSONB NOT NULL,
    catalog_revision BIGINT NOT NULL,
    selection_origins JSONB NOT NULL DEFAULT '[]'::jsonb,
    canonicalization_version TEXT NOT NULL DEFAULT 'v1',
    execution_spec_sha256 TEXT NOT NULL,
    plan_revision_sha256 TEXT NOT NULL,
    created_by TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT uq_analysis_plan_revisions UNIQUE (tenant_id, task_definition_id, plan_revision)
);
CREATE INDEX IF NOT EXISTS idx_analysis_plan_revisions_spec
    ON analysis_plan_revisions (tenant_id, execution_spec_sha256);

-- 计划治理头(CAS 单行)
CREATE TABLE IF NOT EXISTS analysis_plan_governance_heads (
    tenant_id TEXT NOT NULL,
    plan_id UUID PRIMARY KEY REFERENCES analysis_plan_revisions(id),
    state TEXT NOT NULL DEFAULT 'DRAFT',          -- DRAFT|VALIDATED|APPROVED|ACTIVE|RETIRED
    authority_revision BIGINT NOT NULL DEFAULT 0,
    approved_by TEXT,
    approved_at TIMESTAMPTZ,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- 不可变调度修订
CREATE TABLE IF NOT EXISTS analysis_schedule_revisions (
    id UUID PRIMARY KEY,
    tenant_id TEXT NOT NULL,
    task_definition_id UUID NOT NULL REFERENCES analysis_task_definitions(id),
    revision BIGINT NOT NULL,
    approved_plan_revision BIGINT NOT NULL,
    execution_spec_sha256 TEXT NOT NULL,
    trigger_kind TEXT NOT NULL,
    timezone TEXT NOT NULL DEFAULT 'UTC',
    window_or_cron JSONB NOT NULL,
    prepare_lead_time_ms BIGINT NOT NULL DEFAULT 300000,
    misfire_policy TEXT NOT NULL DEFAULT 'MISFIRE_FAIL',
    concurrency_policy TEXT NOT NULL DEFAULT 'FORBID_OVERLAP',
    scheduling_class TEXT NOT NULL DEFAULT 'BASELINE',
    resource_restrictions JSONB NOT NULL DEFAULT '{}'::jsonb,
    schedule_sha256 TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT uq_analysis_schedule_revisions UNIQUE (tenant_id, task_definition_id, revision)
);

-- 调度激活头(CAS 单行)
CREATE TABLE IF NOT EXISTS analysis_schedule_activation_heads (
    tenant_id TEXT NOT NULL,
    schedule_id UUID PRIMARY KEY REFERENCES analysis_schedule_revisions(id),
    state TEXT NOT NULL DEFAULT 'DRAFT',          -- DRAFT|ACTIVE|PAUSED|RETIRED
    authority_revision BIGINT NOT NULL DEFAULT 0,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- 触发实例(物化后与 Task 一一对应)
CREATE TABLE IF NOT EXISTS analysis_trigger_instances (
    id UUID PRIMARY KEY,
    tenant_id TEXT NOT NULL,
    identity_kind TEXT NOT NULL,                  -- actor|schedule|event
    canonical_identity_hash TEXT NOT NULL,
    request_sha256 TEXT NOT NULL,
    state TEXT NOT NULL DEFAULT 'PENDING_MATERIALIZATION',
    materialized_task_id UUID,
    trigger_kind TEXT NOT NULL,
    window_id TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT uq_analysis_trigger_identity UNIQUE (tenant_id, identity_kind, canonical_identity_hash)
);

-- 物化幂等台账
CREATE TABLE IF NOT EXISTS analysis_materialization_ledger (
    identity_hash TEXT PRIMARY KEY,
    request_sha256 TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- 业务请求
CREATE TABLE IF NOT EXISTS analysis_tasks (
    id UUID PRIMARY KEY,
    tenant_id TEXT NOT NULL,
    task_definition_id UUID NOT NULL REFERENCES analysis_task_definitions(id),
    plan_revision BIGINT NOT NULL,
    execution_spec_sha256 TEXT NOT NULL,
    schedule_revision BIGINT,
    trigger_instance_id UUID NOT NULL REFERENCES analysis_trigger_instances(id),
    effective_class TEXT NOT NULL,
    effective_policy_sha256 TEXT NOT NULL,
    current_run_id UUID,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- 有界执行尝试
CREATE TABLE IF NOT EXISTS analysis_runs (
    id UUID PRIMARY KEY,
    tenant_id TEXT NOT NULL,
    task_id UUID NOT NULL REFERENCES analysis_tasks(id),
    execution_spec_sha256 TEXT NOT NULL,
    state TEXT NOT NULL DEFAULT 'ACCEPTED',
    completeness TEXT NOT NULL DEFAULT 'UNKNOWN',
    integrity_state TEXT NOT NULL DEFAULT 'UNVERIFIED',
    finding_conclusion TEXT NOT NULL DEFAULT 'NOT_EVALUATED',
    risk_severity TEXT NOT NULL DEFAULT 'UNKNOWN',
    window_start TIMESTAMPTZ,
    window_end TIMESTAMPTZ,
    revision BIGINT NOT NULL DEFAULT 1,
    cancel_manifest_sha256 TEXT,
    started_at TIMESTAMPTZ,
    finalized_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT chk_analysis_runs_rev CHECK (revision > 0)
);
CREATE INDEX IF NOT EXISTS idx_analysis_runs_task ON analysis_runs (task_id, created_at);

-- 执行节点 attempt
CREATE TABLE IF NOT EXISTS analysis_stage_attempts (
    id UUID PRIMARY KEY,
    tenant_id TEXT NOT NULL,
    run_id UUID NOT NULL REFERENCES analysis_runs(id),
    business_phase_id TEXT NOT NULL,              -- S1..S5
    execution_node_id TEXT NOT NULL,
    attempt INT NOT NULL,
    state TEXT NOT NULL DEFAULT 'PENDING',
    provider_mode TEXT NOT NULL,
    activation_mode TEXT NOT NULL,
    fencing_token TEXT,
    lease_expires_at TIMESTAMPTZ,
    skip_reason TEXT,
    started_at TIMESTAMPTZ,
    finished_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT uq_analysis_stage_attempts UNIQUE (run_id, execution_node_id, attempt)
);

-- 业务阶段投影(UI 固定五段;真实权威仍为 StageAttempt)
CREATE TABLE IF NOT EXISTS analysis_business_phase_projections (
    run_id UUID NOT NULL REFERENCES analysis_runs(id),
    phase_id TEXT NOT NULL,
    state TEXT NOT NULL DEFAULT 'PENDING',
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (run_id, phase_id)
);

-- 执行器回执(不可变)
CREATE TABLE IF NOT EXISTS analysis_stage_receipts (
    id UUID PRIMARY KEY,
    tenant_id TEXT NOT NULL,
    run_id UUID NOT NULL REFERENCES analysis_runs(id),
    execution_node_id TEXT NOT NULL,
    attempt INT NOT NULL,
    fencing_token TEXT NOT NULL,
    provider TEXT NOT NULL,
    input_count BIGINT NOT NULL DEFAULT 0,
    output_count BIGINT NOT NULL DEFAULT 0,
    error_count BIGINT NOT NULL DEFAULT 0,
    reject_count BIGINT NOT NULL DEFAULT 0,
    watermark TIMESTAMPTZ,
    fence JSONB NOT NULL,
    payload_hash TEXT NOT NULL,
    received_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT uq_analysis_stage_receipts UNIQUE (run_id, execution_node_id, attempt, payload_hash)
);

-- 每输入×detector 结果
CREATE TABLE IF NOT EXISTS analysis_results (
    id UUID PRIMARY KEY,
    tenant_id TEXT NOT NULL,
    run_id UUID NOT NULL REFERENCES analysis_runs(id),
    input_identity TEXT NOT NULL,
    detector_id TEXT NOT NULL,
    disposition TEXT NOT NULL,
    score DOUBLE PRECISION,
    labels JSONB NOT NULL DEFAULT '[]'::jsonb,
    evidence_refs JSONB NOT NULL DEFAULT '[]'::jsonb,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT uq_analysis_results UNIQUE (run_id, input_identity, detector_id)
);

-- 机器摘要/证据 manifest/闭包 manifest(与终态同事务)
CREATE TABLE IF NOT EXISTS analysis_machine_summaries (
    tenant_id TEXT NOT NULL,
    run_id UUID PRIMARY KEY REFERENCES analysis_runs(id),
    finding_conclusion TEXT NOT NULL,
    risk_severity TEXT NOT NULL,
    completeness TEXT NOT NULL,
    integrity_state TEXT NOT NULL,
    scope JSONB NOT NULL,
    key_findings JSONB NOT NULL,
    limitations JSONB NOT NULL,
    evidence_manifest_hash TEXT NOT NULL,
    closure_manifest_hash TEXT NOT NULL,
    canonical_sha256 TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS analysis_evidence_manifests (
    tenant_id TEXT NOT NULL,
    run_id UUID PRIMARY KEY REFERENCES analysis_runs(id),
    entries JSONB NOT NULL,
    sha256 TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS analysis_run_closure_manifests (
    tenant_id TEXT NOT NULL,
    run_id UUID PRIMARY KEY REFERENCES analysis_runs(id),
    decision_inputs JSONB NOT NULL,
    priority INT NOT NULL,
    node_exact_set JSONB NOT NULL,
    differences JSONB NOT NULL,
    sha256 TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- 人读报告(独立状态,失败不回退 Run)
CREATE TABLE IF NOT EXISTS analysis_human_reports (
    id UUID PRIMARY KEY,
    tenant_id TEXT NOT NULL,
    run_id UUID NOT NULL REFERENCES analysis_runs(id),
    summary_sha256 TEXT NOT NULL,
    template_revision TEXT NOT NULL,
    locale TEXT NOT NULL DEFAULT 'zh-CN',
    state TEXT NOT NULL DEFAULT 'QUEUED',
    object_key TEXT,
    object_sha256 TEXT,
    object_size BIGINT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT uq_analysis_human_reports UNIQUE (run_id, summary_sha256, template_revision, locale)
);

-- 容量准入预留
CREATE TABLE IF NOT EXISTS analysis_admission_reservations (
    id UUID PRIMARY KEY,
    tenant_id TEXT NOT NULL,
    run_id UUID NOT NULL REFERENCES analysis_runs(id),
    resource_pool TEXT NOT NULL,
    resource_vector JSONB NOT NULL,
    policy_sha256 TEXT NOT NULL,
    state TEXT NOT NULL DEFAULT 'RESERVED',       -- RESERVED|CONSUMED|RELEASED|EXPIRED
    epoch BIGINT NOT NULL,
    expires_at TIMESTAMPTZ NOT NULL,
    authority_revision BIGINT NOT NULL DEFAULT 0,
    CONSTRAINT uq_analysis_admission UNIQUE (run_id, resource_pool)
);

-- 审批台账(maker/checker;人工选择覆盖审批字段的主业务链执行环节)
CREATE TABLE IF NOT EXISTS analysis_plan_approvals (
    id UUID PRIMARY KEY,
    tenant_id TEXT NOT NULL,
    plan_id UUID NOT NULL REFERENCES analysis_plan_revisions(id),
    requested_by TEXT NOT NULL,
    approved_by TEXT NOT NULL,
    state TEXT NOT NULL DEFAULT 'PENDING',        -- PENDING|APPROVED|REJECTED
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    decided_at TIMESTAMPTZ,
    CONSTRAINT uq_analysis_plan_approvals UNIQUE (plan_id, requested_by)
);

-- inbox/outbox/history
CREATE TABLE IF NOT EXISTS analysis_inbox (
    event_id TEXT PRIMARY KEY,
    tuple_hash TEXT NOT NULL,
    outcome TEXT NOT NULL,
    received_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS analysis_outbox (
    id BIGSERIAL PRIMARY KEY,
    event_id TEXT NOT NULL UNIQUE,
    topic TEXT NOT NULL,
    key TEXT NOT NULL,
    payload JSONB NOT NULL,
    state TEXT NOT NULL DEFAULT 'PENDING',        -- PENDING|PUBLISHED|DEAD
    attempts INT NOT NULL DEFAULT 0,
    next_attempt_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS analysis_history (
    id BIGSERIAL PRIMARY KEY,
    tenant_id TEXT NOT NULL,
    entity TEXT NOT NULL,
    entity_id UUID NOT NULL,
    action TEXT NOT NULL,
    actor TEXT NOT NULL,
    detail JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_analysis_history_lookup
    ON analysis_history (tenant_id, entity, entity_id, created_at);

-- 命令回执
CREATE TABLE IF NOT EXISTS analysis_receipts (
    id UUID PRIMARY KEY,
    tenant_id TEXT NOT NULL,
    operation_id TEXT NOT NULL,
    operation TEXT NOT NULL,
    idempotency_key TEXT NOT NULL,
    request_hash TEXT NOT NULL,
    state TEXT NOT NULL,                          -- accepted|running|succeeded|failed|cancelled
    revision BIGINT NOT NULL DEFAULT 1,
    result JSONB,
    error_code TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT uq_analysis_receipts UNIQUE (tenant_id, operation_id)
);

-- 触发实例携带定义/计划引用(调度物化输入;加法式)
ALTER TABLE analysis_trigger_instances ADD COLUMN IF NOT EXISTS task_definition_id TEXT;
ALTER TABLE analysis_trigger_instances ADD COLUMN IF NOT EXISTS plan_revision BIGINT;
ALTER TABLE analysis_trigger_instances ADD COLUMN IF NOT EXISTS actor TEXT;

-- 探针命令 revision 计数器:探针要求命令 revision 严格单调递增
-- (stale_command_revision 拒绝),调度中心按 (tenant, probe) 原子递增。
CREATE TABLE IF NOT EXISTS analysis_probe_command_revisions (
    tenant_id TEXT NOT NULL,
    probe_id   TEXT NOT NULL,
    command_revision BIGINT NOT NULL DEFAULT 0 CHECK (command_revision >= 0),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (tenant_id, probe_id)
);

INSERT INTO alignment_schema_migrations(version, description)
VALUES ('202608170100', 'M10 unified analysis scheduler authority schema')
ON CONFLICT (version) DO NOTHING;

COMMIT;
