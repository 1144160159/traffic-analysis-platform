-- T-PG-001 / T-KAFKA-001 / T-SCHEMA-001
-- Expand: threat-intel business rows, minimum audit and this outbox are written
-- in one transaction. Kafka delivery is asynchronous and never determines
-- whether the business mutation committed.
-- Backfill: none. Historical direct publishes remain immutable evidence.
-- Rollback: set THREAT_INTEL_OUTBOX_V1_ENABLED=false to stop dispatch while
-- retaining committed pending rows. Do not return to synchronous side effects.

BEGIN;

CREATE TABLE IF NOT EXISTS threat_intel_event_outbox (
  event_id       TEXT PRIMARY KEY,
  tenant_id      TEXT NOT NULL,
  partition_key  TEXT NOT NULL,
  payload        JSONB NOT NULL,
  status         TEXT NOT NULL DEFAULT 'pending'
                 CHECK (status IN ('pending','processing','published','dead')),
  attempt_count  INTEGER NOT NULL DEFAULT 0 CHECK (attempt_count>=0),
  available_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
  locked_until   TIMESTAMPTZ,
  locked_by      TEXT NOT NULL DEFAULT '',
  last_error     TEXT NOT NULL DEFAULT '',
  created_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
  published_at   TIMESTAMPTZ
);
CREATE INDEX IF NOT EXISTS idx_threat_intel_event_outbox_pending
  ON threat_intel_event_outbox(available_at,created_at)
  WHERE status='pending';
CREATE INDEX IF NOT EXISTS idx_threat_intel_event_outbox_reclaim
  ON threat_intel_event_outbox(locked_until,created_at)
  WHERE status='processing';

CREATE TABLE IF NOT EXISTS alignment_schema_migrations (
  version     TEXT PRIMARY KEY,
  description TEXT NOT NULL,
  applied_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
  applied_by  TEXT NOT NULL DEFAULT current_user
);
INSERT INTO alignment_schema_migrations(version,description)
VALUES ('202607302345','threat intel business audit transactional outbox')
ON CONFLICT (version) DO NOTHING;

COMMIT;
