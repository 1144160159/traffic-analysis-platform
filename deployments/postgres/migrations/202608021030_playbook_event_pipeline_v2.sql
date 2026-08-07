-- F-PLAYBOOK-001 / T-KAFKA-001 / T-PG-001
-- Expand-only durable Kafka delivery and idempotent PostgreSQL projection for
-- generic playbook execution lifecycle events.
BEGIN;
SET LOCAL lock_timeout = '5s';
SET LOCAL statement_timeout = '2min';

ALTER TABLE alert_playbook_execution_outbox
  ADD COLUMN IF NOT EXISTS status TEXT NOT NULL DEFAULT 'pending',
  ADD COLUMN IF NOT EXISTS last_attempt_at TIMESTAMPTZ,
  ADD COLUMN IF NOT EXISTS dead_at TIMESTAMPTZ;

UPDATE alert_playbook_execution_outbox
SET status = CASE WHEN published THEN 'published' ELSE 'pending' END
WHERE status NOT IN ('pending','processing','published','dead')
   OR (published AND status <> 'published');

ALTER TABLE alert_playbook_execution_outbox
  DROP CONSTRAINT IF EXISTS alert_playbook_execution_outbox_status_check;
ALTER TABLE alert_playbook_execution_outbox
  ADD CONSTRAINT alert_playbook_execution_outbox_status_check
  CHECK (status IN ('pending','processing','published','dead'));

CREATE INDEX IF NOT EXISTS idx_alert_playbook_execution_outbox_ready
  ON alert_playbook_execution_outbox(next_attempt_at,created_at,outbox_id)
  WHERE published=false AND status IN ('pending','processing');

CREATE TABLE IF NOT EXISTS alert_playbook_execution_event_projection (
  event_id             UUID PRIMARY KEY,
  tenant_id            TEXT NOT NULL,
  execution_id         TEXT NOT NULL,
  playbook_name        TEXT NOT NULL,
  event_type           TEXT NOT NULL,
  schema_version       INTEGER NOT NULL CHECK (schema_version=2),
  aggregate_version    BIGINT NOT NULL CHECK (aggregate_version>0),
  partition_key        TEXT NOT NULL CHECK (partition_key<>''),
  trace_id             TEXT NOT NULL CHECK (trace_id<>''),
  payload              JSONB NOT NULL,
  payload_sha256       TEXT NOT NULL CHECK (length(payload_sha256)=64),
  kafka_topic          TEXT NOT NULL CHECK (kafka_topic<>''),
  kafka_partition      INTEGER NOT NULL CHECK (kafka_partition>=0),
  kafka_offset         BIGINT NOT NULL CHECK (kafka_offset>=0),
  projected_at         TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE (kafka_topic,kafka_partition,kafka_offset),
  UNIQUE (tenant_id,execution_id,aggregate_version)
);
CREATE INDEX IF NOT EXISTS idx_playbook_event_projection_execution
  ON alert_playbook_execution_event_projection(tenant_id,execution_id,aggregate_version);

CREATE TABLE IF NOT EXISTS alert_playbook_execution_state_projection (
  tenant_id            TEXT NOT NULL,
  execution_id         TEXT NOT NULL,
  playbook_name        TEXT NOT NULL,
  playbook_version     INTEGER NOT NULL DEFAULT 0 CHECK (playbook_version>=0),
  alert_id             TEXT NOT NULL DEFAULT '',
  status               TEXT NOT NULL,
  approval_status      TEXT NOT NULL DEFAULT '',
  executor_status      TEXT NOT NULL DEFAULT '',
  aggregate_version    BIGINT NOT NULL CHECK (aggregate_version>0),
  event_type           TEXT NOT NULL,
  trace_id             TEXT NOT NULL CHECK (trace_id<>''),
  last_event_id        UUID NOT NULL,
  payload              JSONB NOT NULL,
  payload_sha256       TEXT NOT NULL CHECK (length(payload_sha256)=64),
  kafka_topic          TEXT NOT NULL CHECK (kafka_topic<>''),
  kafka_partition      INTEGER NOT NULL CHECK (kafka_partition>=0),
  kafka_offset         BIGINT NOT NULL CHECK (kafka_offset>=0),
  updated_at           TIMESTAMPTZ NOT NULL DEFAULT now(),
  PRIMARY KEY (tenant_id,execution_id)
);
CREATE INDEX IF NOT EXISTS idx_playbook_state_projection_status
  ON alert_playbook_execution_state_projection(tenant_id,status,updated_at DESC);

CREATE TABLE IF NOT EXISTS alignment_schema_migrations (
  version TEXT PRIMARY KEY,
  description TEXT NOT NULL,
  applied_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  applied_by TEXT NOT NULL DEFAULT current_user
);
INSERT INTO alignment_schema_migrations(version,description)
VALUES ('202608021030','playbook execution Kafka delivery lease and idempotent state projection')
ON CONFLICT (version) DO NOTHING;

COMMIT;
