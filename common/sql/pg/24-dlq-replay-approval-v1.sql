-- =============================================================================
-- DLQ Replay Approval Authority (T1-M10)
--
-- 约束依据(ENG-ARCH-002/ENG-CMD-001):
--   审批命令受理必须以 PostgreSQL 为唯一权威:同一事务提交 state + history +
--   receipt;Redis 只允许幂等辅助,不得接受命令。
-- 幂等:全部 IF NOT EXISTS,可重复执行;旧 approval_id 永不复用(唯一约束)。
-- =============================================================================

BEGIN;

CREATE TABLE IF NOT EXISTS dlq_replay_approvals (
    id BIGSERIAL PRIMARY KEY,
    tenant_id TEXT NOT NULL,
    approval_id TEXT NOT NULL,
    requested_by TEXT NOT NULL,
    approved_by TEXT NOT NULL,
    status TEXT NOT NULL DEFAULT 'approved',
    reason TEXT NOT NULL DEFAULT '',
    request_hash TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT uq_dlq_replay_approvals_approval UNIQUE (tenant_id, approval_id)
);

CREATE INDEX IF NOT EXISTS idx_dlq_replay_approvals_tenant
    ON dlq_replay_approvals (tenant_id);

-- 追加式审批历史(不可变审计轨迹)。
CREATE TABLE IF NOT EXISTS dlq_replay_approval_history (
    id BIGSERIAL PRIMARY KEY,
    tenant_id TEXT NOT NULL,
    approval_id TEXT NOT NULL,
    action TEXT NOT NULL,
    actor TEXT NOT NULL,
    detail JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_dlq_replay_approval_history_lookup
    ON dlq_replay_approval_history (tenant_id, approval_id, created_at);

-- 审批命令受理回执(ENG-CMD-001:accepted 仅在同事务提交后成立)。
CREATE TABLE IF NOT EXISTS dlq_replay_approval_receipts (
    id BIGSERIAL PRIMARY KEY,
    tenant_id TEXT NOT NULL,
    approval_id TEXT NOT NULL,
    status TEXT NOT NULL,
    accepted_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT uq_dlq_replay_approval_receipts_approval UNIQUE (tenant_id, approval_id)
);

INSERT INTO alignment_schema_migrations(version, description)
VALUES ('202608170001', 'M10 DLQ replay approval authority (state+history+receipt)')
ON CONFLICT (version) DO NOTHING;

COMMIT;
