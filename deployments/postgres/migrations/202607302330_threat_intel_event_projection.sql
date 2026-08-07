-- T-KAFKA-001 / T-SCHEMA-001
-- Expand: add a replayable downstream receipt for threat.intel.v1. The
-- consumer verifies every indicator against authoritative PostgreSQL
-- threat_intel before committing the Kafka identity and offset.
-- Backfill: none. Existing Kafka records can be replayed; the consumer accepts
-- the legacy v1 key/envelope while all newly published records use the
-- canonical tenant key and explicit schema/aggregate headers.
-- Rollback: set THREAT_INTEL_EVENT_PROJECTION_V1_ENABLED=false. Keep the
-- additive projection as reconciliation evidence.

BEGIN;

CREATE TABLE IF NOT EXISTS threat_intel_event_projection (
  event_id          TEXT PRIMARY KEY,
  event_type        TEXT NOT NULL,
  schema_version    INTEGER NOT NULL CHECK (schema_version=1),
  aggregate_version BIGINT NOT NULL CHECK (aggregate_version>=1),
  tenant_id         TEXT NOT NULL,
  source            TEXT NOT NULL,
  indicator_count   INTEGER NOT NULL CHECK (indicator_count>=0),
  payload           JSONB NOT NULL,
  trace_id          TEXT NOT NULL,
  occurred_at       TIMESTAMPTZ NOT NULL,
  kafka_partition   INTEGER NOT NULL,
  kafka_offset      BIGINT NOT NULL,
  received_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE (kafka_partition,kafka_offset)
);
CREATE INDEX IF NOT EXISTS idx_threat_intel_event_projection_tenant
  ON threat_intel_event_projection(tenant_id,occurred_at DESC);
CREATE INDEX IF NOT EXISTS idx_threat_intel_event_projection_source
  ON threat_intel_event_projection(tenant_id,source,occurred_at DESC);

CREATE TABLE IF NOT EXISTS alignment_schema_migrations (
  version     TEXT PRIMARY KEY,
  description TEXT NOT NULL,
  applied_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
  applied_by  TEXT NOT NULL DEFAULT current_user
);
INSERT INTO alignment_schema_migrations(version,description)
VALUES ('202607302330','threat intel authoritative event projection')
ON CONFLICT (version) DO NOTHING;

COMMIT;
