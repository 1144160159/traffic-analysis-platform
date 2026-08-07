-- T-DQ-001: authoritative bounded replay projection and commit receipts.
-- Expand only. The projection consumer and executor remain default-off.
BEGIN;

CREATE TABLE IF NOT EXISTS data_quality_flow_replay_projection (
  tenant_id TEXT NOT NULL,
  repair_id UUID NOT NULL REFERENCES data_quality_repairs(repair_id) ON DELETE RESTRICT,
  event_id TEXT NOT NULL CHECK (event_id <> ''),
  idempotency_key TEXT NOT NULL CHECK (length(idempotency_key) BETWEEN 16 AND 300),
  source_event_sha256 TEXT NOT NULL CHECK (length(source_event_sha256) = 64),
  flow_payload BYTEA NOT NULL CHECK (octet_length(flow_payload) > 0),
  source_event_ts BIGINT NOT NULL,
  source_ingest_ts BIGINT NOT NULL,
  projection_version TEXT NOT NULL DEFAULT 'flow-replay-pg-v1'
    CHECK (projection_version = 'flow-replay-pg-v1'),
  trace_id TEXT NOT NULL CHECK (trace_id <> ''),
  committed_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  PRIMARY KEY (tenant_id,repair_id,event_id),
  UNIQUE (tenant_id,repair_id,idempotency_key)
);
CREATE INDEX IF NOT EXISTS idx_data_quality_flow_replay_projection_repair
  ON data_quality_flow_replay_projection (tenant_id,repair_id,committed_at,event_id);

CREATE TABLE IF NOT EXISTS data_quality_replay_projection_receipts (
  tenant_id TEXT NOT NULL,
  repair_id UUID NOT NULL,
  event_id TEXT NOT NULL,
  projection_id TEXT NOT NULL CHECK (projection_id = 'flow-replay-pg-v1'),
  target_store TEXT NOT NULL CHECK (target_store = 'postgresql'),
  target_object_id TEXT NOT NULL CHECK (target_object_id <> ''),
  target_version TEXT NOT NULL CHECK (target_version = 'flow-replay-pg-v1'),
  source_event_sha256 TEXT NOT NULL CHECK (length(source_event_sha256) = 64),
  target_payload_sha256 TEXT NOT NULL CHECK (length(target_payload_sha256) = 64),
  kafka_topic TEXT NOT NULL CHECK (kafka_topic = 'flow.projection-replay.v1'),
  kafka_partition INTEGER NOT NULL CHECK (kafka_partition >= 0),
  kafka_offset BIGINT NOT NULL CHECK (kafka_offset >= 0),
  trace_id TEXT NOT NULL CHECK (trace_id <> ''),
  committed_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  PRIMARY KEY (tenant_id,repair_id,event_id,projection_id),
  UNIQUE (kafka_topic,kafka_partition,kafka_offset),
  FOREIGN KEY (tenant_id,repair_id,event_id)
    REFERENCES data_quality_flow_replay_projection(tenant_id,repair_id,event_id)
    ON DELETE RESTRICT
);
CREATE INDEX IF NOT EXISTS idx_data_quality_replay_projection_receipts_repair
  ON data_quality_replay_projection_receipts (tenant_id,repair_id,committed_at,event_id);

INSERT INTO alignment_schema_migrations(version,description)
VALUES ('202608041800','authoritative bounded flow replay projection and commit receipts')
ON CONFLICT (version) DO NOTHING;

COMMIT;
