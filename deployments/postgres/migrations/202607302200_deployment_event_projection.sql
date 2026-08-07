-- F-MODEL-001 / T-KAFKA-001 / WP-10
-- Expand: add a replayable lifecycle projection for deployment.events.v1.
-- This projection is read-only with respect to authoritative deployments and
-- must never be treated as proof that a deployment effect was applied.
-- Rollback: set DEPLOYMENT_EVENT_PROJECTION_V1_ENABLED=false and retain the
-- rebuildable projection tables until the consumer group is retired.

BEGIN;

CREATE TABLE IF NOT EXISTS deployment_event_projection (
  event_id          UUID PRIMARY KEY,
  deployment_id     TEXT NOT NULL,
  tenant_id         TEXT NOT NULL,
  action            TEXT NOT NULL,
  status            TEXT NOT NULL,
  operator_id       TEXT NOT NULL DEFAULT '',
  event_timestamp_ms BIGINT NOT NULL CHECK (event_timestamp_ms > 0),
  payload            JSONB NOT NULL,
  kafka_partition    INTEGER NOT NULL,
  kafka_offset       BIGINT NOT NULL,
  projected_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE (kafka_partition,kafka_offset)
);
CREATE INDEX IF NOT EXISTS idx_deployment_event_projection_tenant_deployment
  ON deployment_event_projection (tenant_id,deployment_id,event_timestamp_ms);

CREATE TABLE IF NOT EXISTS deployment_state_projection (
  tenant_id          TEXT NOT NULL,
  deployment_id      TEXT NOT NULL,
  action             TEXT NOT NULL,
  status             TEXT NOT NULL,
  operator_id        TEXT NOT NULL DEFAULT '',
  event_timestamp_ms BIGINT NOT NULL CHECK (event_timestamp_ms > 0),
  last_event_id      UUID NOT NULL,
  payload             JSONB NOT NULL,
  kafka_partition     INTEGER NOT NULL,
  kafka_offset        BIGINT NOT NULL,
  updated_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
  PRIMARY KEY (tenant_id,deployment_id)
);
CREATE INDEX IF NOT EXISTS idx_deployment_state_projection_tenant_status
  ON deployment_state_projection (tenant_id,status,updated_at DESC);

CREATE TABLE IF NOT EXISTS alignment_schema_migrations (
  version     TEXT PRIMARY KEY,
  description TEXT NOT NULL,
  applied_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
  applied_by  TEXT NOT NULL DEFAULT current_user
);
INSERT INTO alignment_schema_migrations(version,description)
VALUES ('202607302200','deployment.events.v1 replayable lifecycle projection')
ON CONFLICT (version) DO NOTHING;

COMMIT;
