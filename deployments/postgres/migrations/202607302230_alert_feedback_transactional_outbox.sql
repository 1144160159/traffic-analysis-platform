-- F-ALERT-001 / F-MODEL-001 / T-PG-001 / T-KAFKA-001 / WP-07
-- Expand: make PostgreSQL the authoritative feedback command store and commit
-- business state, minimum audit and Kafka outbox in one transaction.
-- Backfill: event_id is the historical feedback_id. Existing rows receive a
-- conservative payload and remain queryable; no historical Kafka event is
-- emitted automatically.
-- Cutover: deploy the alert-service outbox writer/worker only after this
-- migration. ClickHouse becomes an asynchronous projection.
-- Rollback: set ALERT_FEEDBACK_TRANSACTIONAL_OUTBOX_V1_ENABLED=false, stop the
-- outbox worker and retain all rows. Do not restore direct request-path Kafka
-- publication without an explicit data reconciliation decision.

BEGIN;

CREATE TABLE IF NOT EXISTS alert_feedback (
  feedback_id       UUID PRIMARY KEY,
  event_id          UUID,
  tenant_id         TEXT NOT NULL,
  alert_id          TEXT NOT NULL,
  user_id           UUID,
  label             TEXT NOT NULL,
  reason_code       TEXT,
  comment           TEXT,
  add_to_whitelist  BOOLEAN NOT NULL DEFAULT false,
  alert_type        TEXT NOT NULL DEFAULT '',
  severity          TEXT NOT NULL DEFAULT '',
  model_version     TEXT NOT NULL DEFAULT '',
  rule_version      TEXT NOT NULL DEFAULT '',
  idempotency_key   TEXT NOT NULL DEFAULT '',
  trace_id          TEXT NOT NULL DEFAULT '',
  payload           JSONB NOT NULL DEFAULT '{}'::jsonb,
  status            TEXT NOT NULL DEFAULT 'accepted',
  created_at        TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at        TIMESTAMPTZ NOT NULL DEFAULT now()
);
ALTER TABLE alert_feedback ADD COLUMN IF NOT EXISTS event_id UUID;
ALTER TABLE alert_feedback ADD COLUMN IF NOT EXISTS alert_type TEXT NOT NULL DEFAULT '';
ALTER TABLE alert_feedback ADD COLUMN IF NOT EXISTS severity TEXT NOT NULL DEFAULT '';
ALTER TABLE alert_feedback ADD COLUMN IF NOT EXISTS model_version TEXT NOT NULL DEFAULT '';
ALTER TABLE alert_feedback ADD COLUMN IF NOT EXISTS rule_version TEXT NOT NULL DEFAULT '';
ALTER TABLE alert_feedback ADD COLUMN IF NOT EXISTS idempotency_key TEXT NOT NULL DEFAULT '';
ALTER TABLE alert_feedback ADD COLUMN IF NOT EXISTS trace_id TEXT NOT NULL DEFAULT '';
ALTER TABLE alert_feedback ADD COLUMN IF NOT EXISTS payload JSONB NOT NULL DEFAULT '{}'::jsonb;
ALTER TABLE alert_feedback ADD COLUMN IF NOT EXISTS status TEXT NOT NULL DEFAULT 'accepted';
ALTER TABLE alert_feedback ADD COLUMN IF NOT EXISTS updated_at TIMESTAMPTZ NOT NULL DEFAULT now();

UPDATE alert_feedback
SET event_id=feedback_id
WHERE event_id IS NULL;
UPDATE alert_feedback
SET payload=jsonb_build_object(
  'event_id',event_id,
  'event_type','alert.feedback.v1',
  'schema_version',1,
  'aggregate_version',1,
  'feedback_id',feedback_id,
  'tenant_id',tenant_id,
  'alert_id',alert_id,
  'user_id',COALESCE(user_id::text,''),
  'label',label,
  'reason_code',COALESCE(reason_code,''),
  'comment',COALESCE(comment,''),
  'add_to_whitelist',add_to_whitelist,
  'alert_type',alert_type,
  'severity',severity,
  'model_version',model_version,
  'rule_version',rule_version,
  'timestamp',(extract(epoch FROM created_at)*1000)::bigint
)
WHERE payload='{}'::jsonb;
ALTER TABLE alert_feedback ALTER COLUMN event_id SET NOT NULL;
CREATE UNIQUE INDEX IF NOT EXISTS uq_alert_feedback_event_id
  ON alert_feedback (event_id);
CREATE UNIQUE INDEX IF NOT EXISTS uq_alert_feedback_tenant_idempotency
  ON alert_feedback (tenant_id,idempotency_key)
  WHERE idempotency_key <> '';
CREATE INDEX IF NOT EXISTS idx_feedback_tenant_alert
  ON alert_feedback (tenant_id,alert_id,created_at DESC);
CREATE INDEX IF NOT EXISTS idx_feedback_tenant_time
  ON alert_feedback (tenant_id,created_at DESC);
CREATE INDEX IF NOT EXISTS idx_feedback_label
  ON alert_feedback (tenant_id,label,created_at DESC);

DO $$
BEGIN
  IF NOT EXISTS (
    SELECT 1 FROM pg_constraint
    WHERE conrelid='alert_feedback'::regclass
      AND conname='alert_feedback_label_check'
  ) THEN
    ALTER TABLE alert_feedback
      ADD CONSTRAINT alert_feedback_label_check
      CHECK (label IN ('TP','FP')) NOT VALID;
  END IF;
END $$;

CREATE TABLE IF NOT EXISTS alert_feedback_outbox (
  outbox_id          BIGSERIAL PRIMARY KEY,
  event_id           UUID NOT NULL UNIQUE,
  feedback_id        UUID NOT NULL REFERENCES alert_feedback(feedback_id) ON DELETE RESTRICT,
  tenant_id          TEXT NOT NULL,
  alert_id           TEXT NOT NULL,
  partition_key      TEXT NOT NULL,
  schema_version     INTEGER NOT NULL DEFAULT 1 CHECK (schema_version = 1),
  aggregate_version  BIGINT NOT NULL DEFAULT 1 CHECK (aggregate_version = 1),
  payload            JSONB NOT NULL,
  published          BOOLEAN NOT NULL DEFAULT false,
  attempts           INTEGER NOT NULL DEFAULT 0 CHECK (attempts >= 0),
  last_error         TEXT NOT NULL DEFAULT '',
  next_attempt_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
  locked_until       TIMESTAMPTZ,
  locked_by          TEXT NOT NULL DEFAULT '',
  created_at         TIMESTAMPTZ NOT NULL DEFAULT now(),
  published_at       TIMESTAMPTZ
);
CREATE INDEX IF NOT EXISTS idx_alert_feedback_outbox_pending
  ON alert_feedback_outbox (next_attempt_at,outbox_id)
  WHERE published=false;

CREATE TABLE IF NOT EXISTS alignment_schema_migrations (
  version     TEXT PRIMARY KEY,
  description TEXT NOT NULL,
  applied_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
  applied_by  TEXT NOT NULL DEFAULT current_user
);
INSERT INTO alignment_schema_migrations(version,description)
VALUES ('202607302230','alert feedback authoritative transaction and Kafka outbox')
ON CONFLICT (version) DO NOTHING;

COMMIT;
