-- M10 统一分析任务调度中心 v2:触发实例承载有效调度策略输入(加法式迁移)。
-- 变更:analysis_trigger_instances 增加 effective_class / resource_restrictions,
-- 使调度触发(schedule)与按需触发(on-demand)都经 ResolveEffectiveSchedulingPolicy
-- 冻结有效类别与资源上限,不再由物化硬编码。
BEGIN;

ALTER TABLE analysis_trigger_instances
    ADD COLUMN IF NOT EXISTS effective_class TEXT NOT NULL DEFAULT 'BASELINE',
    ADD COLUMN IF NOT EXISTS resource_restrictions JSONB NOT NULL DEFAULT '{}'::jsonb,
    ADD COLUMN IF NOT EXISTS schedule_revision BIGINT;

INSERT INTO alignment_schema_migrations(version, description)
VALUES ('202608170200', 'M10 analysis trigger effective policy inputs (additive)')
ON CONFLICT (version) DO NOTHING;

COMMIT;
