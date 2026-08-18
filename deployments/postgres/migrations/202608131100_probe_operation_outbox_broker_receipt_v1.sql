-- M02-N012 GAP-01/GAP-11
-- Expand the probe outbox with an attempt-correlated broker receipt. The
-- compatibility boolean remains until readers have cut over to publish_state.

BEGIN;
SET LOCAL lock_timeout='5s';
SET LOCAL statement_timeout='2min';

ALTER TABLE probe_operation_outbox
  ADD COLUMN IF NOT EXISTS publish_state TEXT NOT NULL DEFAULT 'PENDING';
ALTER TABLE probe_operation_outbox
  ADD COLUMN IF NOT EXISTS broker_topic TEXT NOT NULL DEFAULT '';
ALTER TABLE probe_operation_outbox
  ADD COLUMN IF NOT EXISTS broker_partition INTEGER;
ALTER TABLE probe_operation_outbox
  ADD COLUMN IF NOT EXISTS broker_offset BIGINT;
ALTER TABLE probe_operation_outbox
  ADD COLUMN IF NOT EXISTS publish_attempt UUID;
ALTER TABLE probe_operation_outbox
  ADD COLUMN IF NOT EXISTS acked_at TIMESTAMPTZ;

UPDATE probe_operation_outbox
SET publish_state=CASE WHEN published THEN 'KAFKA_ACKED' ELSE 'PENDING' END
WHERE publish_state NOT IN ('OUTCOME_UNKNOWN','KAFKA_ACKED') OR published;

ALTER TABLE probe_operation_outbox
  DROP CONSTRAINT IF EXISTS probe_operation_outbox_publish_state_check;
ALTER TABLE probe_operation_outbox
  ADD CONSTRAINT probe_operation_outbox_publish_state_check
  CHECK (publish_state IN ('PENDING','OUTCOME_UNKNOWN','KAFKA_ACKED')) NOT VALID;
ALTER TABLE probe_operation_outbox
  VALIDATE CONSTRAINT probe_operation_outbox_publish_state_check;

ALTER TABLE probe_operation_outbox
  DROP CONSTRAINT IF EXISTS probe_operation_outbox_publish_compatibility_check;
ALTER TABLE probe_operation_outbox
  ADD CONSTRAINT probe_operation_outbox_publish_compatibility_check
  CHECK (
    (published AND publish_state='KAFKA_ACKED') OR
    (NOT published AND publish_state IN ('PENDING','OUTCOME_UNKNOWN'))
  ) NOT VALID;
ALTER TABLE probe_operation_outbox
  VALIDATE CONSTRAINT probe_operation_outbox_publish_compatibility_check;

DROP INDEX IF EXISTS idx_probe_operation_outbox_pending;
CREATE INDEX idx_probe_operation_outbox_pending
  ON probe_operation_outbox (next_attempt_at,created_at)
  WHERE publish_state IN ('PENDING','OUTCOME_UNKNOWN');

CREATE TABLE IF NOT EXISTS alignment_schema_migrations (
  version     TEXT PRIMARY KEY,
  description TEXT NOT NULL,
  applied_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
  applied_by  TEXT NOT NULL DEFAULT current_user
);
INSERT INTO alignment_schema_migrations(version,description)
VALUES ('202608131100','probe outbox attempt-correlated Kafka broker receipt')
ON CONFLICT (version) DO NOTHING;

COMMIT;
