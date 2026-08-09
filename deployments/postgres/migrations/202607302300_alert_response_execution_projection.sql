-- F-COMMON-003 / F-PLAYBOOK-001 / T-KAFKA-001 / WP-07
-- Expand: add a stable response-event envelope, explicit approval/result state
-- and immutable execution receipts. Real effects remain blocked until a
-- provider adapter and independent approval workflow exist.
-- Backfill: derive deterministic event IDs for existing action/outbox rows.
-- Previously published non-dry-run requests are not represented as success;
-- the new consumer records blocked_external_executor.
-- Rollback: set ALERT_RESPONSE_EXECUTION_V1_ENABLED=false, stop the new
-- consumer group and retain all additive receipt/evidence columns.

BEGIN;

ALTER TABLE alert_response_actions
  ADD COLUMN IF NOT EXISTS event_id UUID,
  ADD COLUMN IF NOT EXISTS action_id TEXT NOT NULL DEFAULT '',
  ADD COLUMN IF NOT EXISTS revision BIGINT NOT NULL DEFAULT 1,
  ADD COLUMN IF NOT EXISTS trace_id TEXT NOT NULL DEFAULT '',
  ADD COLUMN IF NOT EXISTS idempotency_key TEXT NOT NULL DEFAULT '',
  ADD COLUMN IF NOT EXISTS approval_status TEXT NOT NULL DEFAULT 'not_required',
  ADD COLUMN IF NOT EXISTS approved_by TEXT NOT NULL DEFAULT '',
  ADD COLUMN IF NOT EXISTS approved_at TIMESTAMPTZ,
  ADD COLUMN IF NOT EXISTS result JSONB NOT NULL DEFAULT '{}'::jsonb,
  ADD COLUMN IF NOT EXISTS error TEXT NOT NULL DEFAULT '';

UPDATE alert_response_actions
SET event_id=COALESCE(event_id,uuid_generate_v5(uuid_ns_oid(),'alert.response.requested.v1:'||job_id)),
    action_id=COALESCE(NULLIF(action_id,''),NULLIF(detail->>'action_id',''),'legacy.alert-action'),
    approval_status=CASE
      WHEN dry_run THEN 'not_required'
      ELSE 'pending'
    END
WHERE event_id IS NULL OR action_id='' OR approval_status='not_required';

ALTER TABLE alert_response_actions
  ALTER COLUMN event_id SET NOT NULL;
CREATE UNIQUE INDEX IF NOT EXISTS uq_alert_response_actions_event
  ON alert_response_actions(event_id);
CREATE UNIQUE INDEX IF NOT EXISTS uq_alert_response_actions_tenant_idempotency
  ON alert_response_actions(tenant_id,idempotency_key)
  WHERE idempotency_key<>'';

ALTER TABLE alert_response_outbox
  ADD COLUMN IF NOT EXISTS event_id UUID,
  ADD COLUMN IF NOT EXISTS schema_version INTEGER NOT NULL DEFAULT 1,
  ADD COLUMN IF NOT EXISTS aggregate_version BIGINT NOT NULL DEFAULT 1,
  ADD COLUMN IF NOT EXISTS partition_key TEXT NOT NULL DEFAULT '';

UPDATE alert_response_outbox outbox
SET event_id=COALESCE(outbox.event_id,action.event_id),
    partition_key=COALESCE(NULLIF(outbox.partition_key,''),outbox.tenant_id||':'||outbox.job_id),
    payload=outbox.payload || jsonb_build_object(
      'event_id',action.event_id::text,
      'event_type','alert.response.requested.v1',
      'schema_version',1,
      'aggregate_version',1,
      'reason',action.reason,
      'requested_by',action.requested_by
    )
FROM alert_response_actions action
WHERE action.job_id=outbox.job_id;

ALTER TABLE alert_response_outbox
  ALTER COLUMN event_id SET NOT NULL;
CREATE UNIQUE INDEX IF NOT EXISTS uq_alert_response_outbox_event
  ON alert_response_outbox(event_id);

CREATE TABLE IF NOT EXISTS alert_response_execution_receipts (
  event_id           UUID PRIMARY KEY,
  job_id             TEXT NOT NULL UNIQUE REFERENCES alert_response_actions(job_id) ON DELETE RESTRICT,
  tenant_id          TEXT NOT NULL,
  alert_id           TEXT NOT NULL,
  action_id          TEXT NOT NULL,
  state              TEXT NOT NULL
                     CHECK (state IN ('simulated_completed','blocked_external_executor','failed')),
  simulated          BOOLEAN NOT NULL,
  external_effect    BOOLEAN NOT NULL DEFAULT false,
  result             JSONB NOT NULL DEFAULT '{}'::jsonb,
  error              TEXT NOT NULL DEFAULT '',
  kafka_partition    INTEGER NOT NULL,
  kafka_offset       BIGINT NOT NULL,
  created_at         TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE (kafka_partition,kafka_offset)
);
CREATE INDEX IF NOT EXISTS idx_alert_response_receipts_tenant_job
  ON alert_response_execution_receipts(tenant_id,job_id,created_at);

CREATE TABLE IF NOT EXISTS alignment_schema_migrations (
  version     TEXT PRIMARY KEY,
  description TEXT NOT NULL,
  applied_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
  applied_by  TEXT NOT NULL DEFAULT current_user
);
INSERT INTO alignment_schema_migrations(version,description)
VALUES ('202607302300','safe alert response simulation and execution receipts')
ON CONFLICT (version) DO NOTHING;

COMMIT;
