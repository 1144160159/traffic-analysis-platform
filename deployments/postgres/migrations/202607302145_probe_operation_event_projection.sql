-- F-PROBE-001 / T-KAFKA-001 / WP-10
-- Expand: add an idempotent read projection for probe.events.v2.
-- Verify:
--   SELECT event_type,count(*) FROM probe_operation_event_projection GROUP BY event_type;
--   SELECT tenant_id,status,count(*) FROM probe_operation_state_projection GROUP BY tenant_id,status;
-- Cutover: enable the consumer only after this migration is present.
-- Rollback: stop the consumer; both projections are rebuildable from Kafka.

BEGIN;

CREATE TABLE IF NOT EXISTS probe_operation_event_projection (
  event_id          UUID PRIMARY KEY,
  operation_id      UUID NOT NULL,
  tenant_id         TEXT NOT NULL,
  probe_id          TEXT NOT NULL,
  event_type        TEXT NOT NULL CHECK (
    event_type = 'traffic.probe.v2.OperationAcknowledged'
  ),
  revision          BIGINT NOT NULL CHECK (revision > 0),
  status            TEXT NOT NULL,
  trace_id           TEXT NOT NULL,
  payload            JSONB NOT NULL,
  kafka_partition    INTEGER NOT NULL,
  kafka_offset       BIGINT NOT NULL,
  projected_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE (kafka_partition,kafka_offset)
);
CREATE INDEX IF NOT EXISTS idx_probe_operation_event_projection_tenant_operation
  ON probe_operation_event_projection (tenant_id,operation_id,revision);

CREATE TABLE IF NOT EXISTS probe_operation_state_projection (
  tenant_id         TEXT NOT NULL,
  operation_id      UUID NOT NULL,
  probe_id          TEXT NOT NULL,
  revision          BIGINT NOT NULL CHECK (revision > 0),
  event_type        TEXT NOT NULL,
  status            TEXT NOT NULL,
  trace_id           TEXT NOT NULL,
  last_event_id      UUID NOT NULL,
  payload            JSONB NOT NULL,
  kafka_partition    INTEGER NOT NULL,
  kafka_offset       BIGINT NOT NULL,
  updated_at         TIMESTAMPTZ NOT NULL DEFAULT now(),
  PRIMARY KEY (tenant_id,operation_id)
);
CREATE INDEX IF NOT EXISTS idx_probe_operation_state_projection_tenant_probe
  ON probe_operation_state_projection (tenant_id,probe_id,updated_at DESC);

CREATE TABLE IF NOT EXISTS alignment_schema_migrations (
  version     TEXT PRIMARY KEY,
  description TEXT NOT NULL,
  applied_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
  applied_by  TEXT NOT NULL DEFAULT current_user
);
INSERT INTO alignment_schema_migrations(version,description)
VALUES ('202607302145','probe.events.v2 idempotent lifecycle projection')
ON CONFLICT (version) DO NOTHING;

COMMIT;
