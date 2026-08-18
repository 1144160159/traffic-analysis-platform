-- F-MODEL-001 / F-MLOPS-001 / T1-M08-N014
-- Expand-only consumer-first storage for versioned, adjudicated model feedback.
-- No producer or training trigger is enabled by this migration.
-- Rollback: disable MODEL_FEEDBACK_REVISION_CONSUMER_V1_ENABLED and retain all
-- four tables as audit evidence. Destructive rollback requires data-owner
-- approval after M09-N017 has been proven absent.

BEGIN;

CREATE TABLE IF NOT EXISTS model_feedback_revision_head (
  feedback_id          UUID PRIMARY KEY,
  tenant_id            TEXT NOT NULL,
  prediction_id        TEXT NOT NULL,
  alert_id             TEXT NOT NULL,
  model_version        TEXT NOT NULL,
  rule_version         TEXT NOT NULL,
  label                TEXT NOT NULL CHECK (label IN ('TP','FP')),
  label_revision       BIGINT NOT NULL CHECK (label_revision >= 1),
  adjudication_state   TEXT NOT NULL CHECK (adjudication_state IN ('PROPOSED','ADJUDICATED','RETRACTED')),
  last_event_id        UUID NOT NULL,
  payload_sha256       CHAR(64) NOT NULL CHECK (payload_sha256 ~ '^[0-9a-f]{64}$'),
  occurred_at_ms       BIGINT NOT NULL CHECK (occurred_at_ms > 0),
  created_at           TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at           TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE (tenant_id,prediction_id)
);

CREATE TABLE IF NOT EXISTS model_feedback_revision_inbox (
  event_id             UUID PRIMARY KEY,
  feedback_id          UUID NOT NULL,
  tenant_id            TEXT NOT NULL,
  prediction_id        TEXT NOT NULL,
  alert_id             TEXT NOT NULL,
  label                TEXT NOT NULL CHECK (label IN ('TP','FP')),
  label_revision       BIGINT NOT NULL CHECK (label_revision >= 1),
  adjudication_state   TEXT NOT NULL CHECK (adjudication_state IN ('PROPOSED','ADJUDICATED','RETRACTED')),
  reason_code          TEXT NOT NULL DEFAULT '',
  model_version        TEXT NOT NULL,
  rule_version         TEXT NOT NULL,
  adjudicated_by       TEXT NOT NULL,
  previous_event_id    UUID,
  occurred_at_ms       BIGINT NOT NULL CHECK (occurred_at_ms > 0),
  trace_id             TEXT NOT NULL DEFAULT '',
  payload              JSONB NOT NULL,
  payload_sha256       CHAR(64) NOT NULL CHECK (payload_sha256 ~ '^[0-9a-f]{64}$'),
  kafka_topic          TEXT NOT NULL CHECK (kafka_topic='model.feedback.v1'),
  kafka_partition      INTEGER NOT NULL CHECK (kafka_partition >= 0),
  kafka_offset         BIGINT NOT NULL CHECK (kafka_offset >= 0),
  status               TEXT NOT NULL DEFAULT 'pending' CHECK (status IN ('pending','processing','applied','failed','dead_letter','retracted')),
  attempts             INTEGER NOT NULL DEFAULT 0 CHECK (attempts >= 0),
  last_error           TEXT NOT NULL DEFAULT '',
  next_attempt_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
  locked_until         TIMESTAMPTZ,
  locked_by            TEXT NOT NULL DEFAULT '',
  applied_at           TIMESTAMPTZ,
  created_at           TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at           TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE (feedback_id,label_revision),
  UNIQUE (kafka_topic,kafka_partition,kafka_offset),
  CHECK ((label_revision=1 AND previous_event_id IS NULL) OR
         (label_revision>1 AND previous_event_id IS NOT NULL))
);

CREATE INDEX IF NOT EXISTS idx_model_feedback_revision_inbox_pending
  ON model_feedback_revision_inbox (next_attempt_at,created_at,event_id)
  WHERE status IN ('pending','failed');
CREATE INDEX IF NOT EXISTS idx_model_feedback_revision_inbox_prediction
  ON model_feedback_revision_inbox (tenant_id,prediction_id,label_revision DESC);

CREATE TABLE IF NOT EXISTS model_feedback_revision_receipt (
  event_id             UUID PRIMARY KEY REFERENCES model_feedback_revision_inbox(event_id) ON DELETE RESTRICT,
  feedback_id          UUID NOT NULL,
  tenant_id            TEXT NOT NULL,
  label_revision       BIGINT NOT NULL CHECK (label_revision >= 1),
  outcome              TEXT NOT NULL CHECK (outcome IN ('ACCEPTED')),
  payload_sha256       CHAR(64) NOT NULL CHECK (payload_sha256 ~ '^[0-9a-f]{64}$'),
  kafka_topic          TEXT NOT NULL CHECK (kafka_topic='model.feedback.v1'),
  kafka_partition      INTEGER NOT NULL CHECK (kafka_partition >= 0),
  kafka_offset         BIGINT NOT NULL CHECK (kafka_offset >= 0),
  recorded_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE (feedback_id,label_revision),
  UNIQUE (kafka_topic,kafka_partition,kafka_offset)
);

CREATE TABLE IF NOT EXISTS model_feedback_consumer_readiness_receipt (
  consumer_group       TEXT NOT NULL,
  candidate_sha256     CHAR(64) NOT NULL CHECK (candidate_sha256 ~ '^[0-9a-f]{64}$'),
  contract_sha256      CHAR(64) NOT NULL CHECK (contract_sha256 ~ '^[0-9a-f]{64}$'),
  kafka_topic          TEXT NOT NULL CHECK (kafka_topic='model.feedback.v1'),
  state                TEXT NOT NULL CHECK (state IN ('READY','STOPPED')),
  event_id             UUID,
  kafka_partition      INTEGER CHECK (kafka_partition IS NULL OR kafka_partition >= 0),
  kafka_offset         BIGINT CHECK (kafka_offset IS NULL OR kafka_offset >= 0),
  observed_at          TIMESTAMPTZ NOT NULL,
  updated_at           TIMESTAMPTZ NOT NULL DEFAULT now(),
  PRIMARY KEY (consumer_group,candidate_sha256),
  UNIQUE (kafka_topic,consumer_group,candidate_sha256),
  CHECK ((state='READY' AND event_id IS NOT NULL AND kafka_partition IS NOT NULL AND kafka_offset IS NOT NULL)
      OR (state='STOPPED' AND event_id IS NULL AND kafka_partition IS NULL AND kafka_offset IS NULL))
);

CREATE TABLE IF NOT EXISTS alignment_schema_migrations (
  version      TEXT PRIMARY KEY,
  description  TEXT NOT NULL,
  applied_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
  applied_by   TEXT NOT NULL DEFAULT current_user
);
INSERT INTO alignment_schema_migrations(version,description)
VALUES ('202608151430','M08 consumer-first model feedback revision inbox and receipt')
ON CONFLICT (version) DO NOTHING;

COMMIT;
