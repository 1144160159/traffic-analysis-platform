-- M02-N012 GAP-08/GAP-10/GAP-13
-- Durable consumer ownership epochs fence every probe outbox claim.

BEGIN;
SET LOCAL lock_timeout='5s';
SET LOCAL statement_timeout='2min';

CREATE TABLE IF NOT EXISTS probe_pipeline_readiness_epochs (
  pipeline_id      TEXT NOT NULL,
  consumer_role    TEXT NOT NULL CHECK (consumer_role IN (
    'COMMAND_DELIVERY','ACK_AUTHORITY','LIFECYCLE_PROJECTION'
  )),
  consumer_group   TEXT NOT NULL,
  owner_id         TEXT NOT NULL,
  owner_epoch      BIGINT NOT NULL CHECK (owner_epoch > 0),
  ready            BOOLEAN NOT NULL,
  observed_at      TIMESTAMPTZ NOT NULL,
  lease_expires_at TIMESTAMPTZ,
  revoked_at       TIMESTAMPTZ,
  updated_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
  PRIMARY KEY (pipeline_id,consumer_role),
  CHECK (
    (ready AND lease_expires_at IS NOT NULL AND revoked_at IS NULL) OR
    (NOT ready AND revoked_at IS NOT NULL)
  )
);
CREATE INDEX IF NOT EXISTS idx_probe_pipeline_readiness_live
  ON probe_pipeline_readiness_epochs (pipeline_id,lease_expires_at)
  WHERE ready;

CREATE TABLE IF NOT EXISTS alignment_schema_migrations (
  version     TEXT PRIMARY KEY,
  description TEXT NOT NULL,
  applied_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
  applied_by  TEXT NOT NULL DEFAULT current_user
);
INSERT INTO alignment_schema_migrations(version,description)
VALUES ('202608131030','durable probe consumer readiness epoch claim fence')
ON CONFLICT (version) DO NOTHING;

COMMIT;
