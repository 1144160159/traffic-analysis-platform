-- M04 per-subtask rule application receipts and exact aggregation state.
BEGIN;

ALTER TABLE rule_outbox
    ADD COLUMN IF NOT EXISTS runtime_status TEXT NOT NULL DEFAULT 'pending',
    ADD COLUMN IF NOT EXISTS runtime_applied_at TIMESTAMPTZ,
    ADD COLUMN IF NOT EXISTS runtime_last_error TEXT NOT NULL DEFAULT '';

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint WHERE conname = 'rule_outbox_runtime_status_check'
    ) THEN
        ALTER TABLE rule_outbox ADD CONSTRAINT rule_outbox_runtime_status_check
            CHECK (runtime_status IN ('pending', 'partial', 'applied', 'failed'));
    END IF;
END $$;

CREATE INDEX IF NOT EXISTS idx_rule_outbox_event_id
    ON rule_outbox ((payload->>'event_id'))
    WHERE payload ? 'event_id';

CREATE TABLE IF NOT EXISTS rule_update_applied_acks (
    event_id TEXT NOT NULL,
    tenant_id TEXT NOT NULL,
    rule_id TEXT NOT NULL,
    rule_version BIGINT NOT NULL CHECK (rule_version > 0),
    action TEXT NOT NULL CHECK (action IN ('create','update','delete','enable','disable','sync')),
    checksum TEXT NOT NULL CHECK (checksum ~ '^[0-9a-f]{32}$'),
    subtask_index INT NOT NULL CHECK (subtask_index >= 0),
    parallelism INT NOT NULL CHECK (parallelism > 0 AND subtask_index < parallelism),
    status TEXT NOT NULL CHECK (status IN ('applied','duplicate','stale','conflict')),
    current_version BIGINT NOT NULL CHECK (current_version >= 0),
    error TEXT NOT NULL DEFAULT '',
    payload JSONB NOT NULL,
    acknowledged_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (event_id, subtask_index)
);

CREATE INDEX IF NOT EXISTS idx_rule_update_applied_acks_status
    ON rule_update_applied_acks (event_id, status, subtask_index);
CREATE INDEX IF NOT EXISTS idx_rule_update_applied_acks_rule
    ON rule_update_applied_acks (tenant_id, rule_id, rule_version, acknowledged_at DESC);

CREATE TABLE IF NOT EXISTS alignment_schema_migrations (
    version TEXT PRIMARY KEY,
    description TEXT NOT NULL,
    applied_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    applied_by TEXT NOT NULL DEFAULT current_user
);
INSERT INTO alignment_schema_migrations(version, description)
VALUES ('202608141500', 'M04 per-subtask rule update application receipts and exact parallelism aggregation')
ON CONFLICT (version) DO NOTHING;

COMMIT;
