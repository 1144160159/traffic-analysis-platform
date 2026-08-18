-- M11 统一分析任务调度中心 v1:阶段 READY 队列 + DRR 计量单位统一(§76.45.3)——加法式迁移。
-- 变更:
--   analysis_stage_queue:阶段候选队列(稳定排序 deadline NULLS LAST, ready_at, run_id,
--     execution_node_id, attempt;选中/更新 DRR/消费预留/lease/CAS DISPATCHED/outbox 同事务);
--   analysis_drr_state.quantum:既有 <=1 的行升为 1000(milli 单位,与向量折算 cost 一致)。
BEGIN;

CREATE TABLE IF NOT EXISTS analysis_stage_queue (
    id UUID PRIMARY KEY,
    tenant_id TEXT NOT NULL,
    run_id UUID NOT NULL REFERENCES analysis_runs(id),
    execution_node_id TEXT NOT NULL,
    attempt INT NOT NULL,
    state TEXT NOT NULL DEFAULT 'READY',           -- READY|CLAIMED|DONE|EXPIRED
    cost_milli BIGINT NOT NULL DEFAULT 1000,       -- 冻结权重折算后的 DRR cost(milli)
    ready_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    deadline TIMESTAMPTZ,
    claimed_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT uq_analysis_stage_queue UNIQUE (run_id, execution_node_id, attempt)
);
CREATE INDEX IF NOT EXISTS idx_analysis_stage_queue_claim
    ON analysis_stage_queue (state, deadline NULLS LAST, ready_at, run_id, execution_node_id, attempt);

-- DRR 计量统一:quantum 与 cost 同单位(milli);存量行(旧单位 1)一次性升级。
UPDATE analysis_drr_state SET quantum = 1000 WHERE quantum <= 1;

INSERT INTO alignment_schema_migrations(version, description)
VALUES ('202608180100', 'M11 analysis stage ready queue + DRR milli units (additive)')
ON CONFLICT (version) DO NOTHING;

COMMIT;
