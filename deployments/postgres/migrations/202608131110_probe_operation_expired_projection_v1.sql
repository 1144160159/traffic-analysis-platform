-- M02-N012 GAP-09/GAP-12
-- Expand the probe lifecycle projection before OperationExpired emission starts.
-- Rollback requires stopping expiry emission and draining existing expiry facts first.

BEGIN;
SET LOCAL lock_timeout='5s';
SET LOCAL statement_timeout='2min';

ALTER TABLE probe_operation_event_projection
  DROP CONSTRAINT IF EXISTS probe_operation_event_projection_event_type_check;

ALTER TABLE probe_operation_event_projection
  ADD CONSTRAINT probe_operation_event_projection_event_type_check
  CHECK (event_type IN (
    'traffic.probe.v2.OperationAcknowledged',
    'traffic.probe.v2.OperationExpired'
  )) NOT VALID;

ALTER TABLE probe_operation_event_projection
  VALIDATE CONSTRAINT probe_operation_event_projection_event_type_check;

CREATE TABLE IF NOT EXISTS alignment_schema_migrations (
  version     TEXT PRIMARY KEY,
  description TEXT NOT NULL,
  applied_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
  applied_by  TEXT NOT NULL DEFAULT current_user
);
INSERT INTO alignment_schema_migrations(version,description)
VALUES ('202608131110','admit distinct OperationExpired lifecycle projection facts')
ON CONFLICT (version) DO NOTHING;

COMMIT;
