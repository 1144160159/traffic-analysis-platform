-- M10 统一分析任务调度中心 v3:Outbox broker ACK 台账 + DRR 公平队列状态(加法式迁移)。
-- 变更:
--  analysis_outbox 增加 published_at / broker_ack(RequiredAcks=all 的 nil 返回 = broker ACK 事实);
--  新增 analysis_drr_state(tenant, scheduling_class) 持久化 deficit/quantum/last_served_at/scheduler_epoch。
BEGIN;

ALTER TABLE analysis_outbox
    ADD COLUMN IF NOT EXISTS published_at TIMESTAMPTZ,
    ADD COLUMN IF NOT EXISTS broker_ack BOOLEAN NOT NULL DEFAULT false;

CREATE TABLE IF NOT EXISTS analysis_drr_state (
    tenant_id TEXT NOT NULL,
    scheduling_class TEXT NOT NULL,
    deficit BIGINT NOT NULL DEFAULT 0,
    quantum BIGINT NOT NULL DEFAULT 1,
    last_served_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    scheduler_epoch BIGINT NOT NULL DEFAULT 0,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (tenant_id, scheduling_class)
);

INSERT INTO alignment_schema_migrations(version, description)
VALUES ('202608170300', 'M10 analysis outbox ack ledger + DRR state (additive)')
ON CONFLICT (version) DO NOTHING;

COMMIT;
