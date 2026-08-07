-- F-ALERT-001 / F-MODEL-001 / T-KAFKA-001 / WP-07
-- Expand: add a replayable alert.feedback.v1 projection and a durable MLOps
-- inbox. The inbox is a training-input handoff, not proof that a model was
-- trained or deployed.
-- Backfill: replay alert.feedback.v1 by tenant+alert key after the consumer
-- group and DLQ are approved. Reconcile feedback_id and Kafka offsets.
-- Rollback: set ALERT_FEEDBACK_PROJECTION_V1_ENABLED=false and retain both
-- rebuildable tables until the consumer group is retired.

BEGIN;

CREATE TABLE IF NOT EXISTS alert_feedback_event_projection (
  event_id           UUID PRIMARY KEY,
  feedback_id        UUID NOT NULL UNIQUE,
  tenant_id          TEXT NOT NULL,
  alert_id           TEXT NOT NULL,
  user_id            TEXT NOT NULL DEFAULT '',
  label              TEXT NOT NULL CHECK (label IN ('TP','FP')),
  reason_code        TEXT NOT NULL DEFAULT '',
  event_timestamp_ms BIGINT NOT NULL CHECK (event_timestamp_ms > 0),
  payload             JSONB NOT NULL,
  kafka_partition     INTEGER NOT NULL,
  kafka_offset        BIGINT NOT NULL,
  projected_at        TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE (kafka_partition,kafka_offset)
);
CREATE INDEX IF NOT EXISTS idx_alert_feedback_event_tenant_alert
  ON alert_feedback_event_projection (tenant_id,alert_id,event_timestamp_ms);

CREATE TABLE IF NOT EXISTS model_feedback_inbox (
  feedback_id        UUID PRIMARY KEY,
  event_id           UUID NOT NULL UNIQUE,
  tenant_id          TEXT NOT NULL,
  alert_id           TEXT NOT NULL,
  user_id            TEXT NOT NULL DEFAULT '',
  label              TEXT NOT NULL CHECK (label IN ('TP','FP')),
  reason_code        TEXT NOT NULL DEFAULT '',
  model_version      TEXT NOT NULL DEFAULT '',
  rule_version       TEXT NOT NULL DEFAULT '',
  event_timestamp_ms BIGINT NOT NULL CHECK (event_timestamp_ms > 0),
  payload             JSONB NOT NULL,
  status              TEXT NOT NULL DEFAULT 'pending'
                      CHECK (status IN ('pending','processing','applied','failed')),
  attempts            INTEGER NOT NULL DEFAULT 0 CHECK (attempts >= 0),
  last_error          TEXT NOT NULL DEFAULT '',
  next_attempt_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
  created_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at          TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_model_feedback_inbox_pending
  ON model_feedback_inbox (next_attempt_at,created_at,feedback_id)
  WHERE status IN ('pending','failed');
CREATE INDEX IF NOT EXISTS idx_model_feedback_inbox_tenant_model
  ON model_feedback_inbox (tenant_id,model_version,event_timestamp_ms);

CREATE TABLE IF NOT EXISTS alignment_schema_migrations (
  version     TEXT PRIMARY KEY,
  description TEXT NOT NULL,
  applied_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
  applied_by  TEXT NOT NULL DEFAULT current_user
);
INSERT INTO alignment_schema_migrations(version,description)
VALUES ('202607302215','alert.feedback.v1 replayable MLOps inbox projection')
ON CONFLICT (version) DO NOTHING;

COMMIT;
