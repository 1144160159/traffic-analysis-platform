-- Durable source-offset acknowledgement after canonical DLQ broker ACK.
BEGIN;
SET LOCAL lock_timeout='5s';
SET LOCAL statement_timeout='2min';

CREATE TABLE IF NOT EXISTS kafka_dlq_acknowledgement_receipts (
  consumer_group   TEXT NOT NULL,
  source_topic     TEXT NOT NULL,
  source_partition INTEGER NOT NULL CHECK (source_partition >= 0),
  source_offset    BIGINT NOT NULL CHECK (source_offset >= 0),
  source_key_sha256 TEXT NOT NULL CHECK (source_key_sha256 ~ '^[0-9a-f]{64}$'),
  payload_sha256   TEXT NOT NULL CHECK (payload_sha256 ~ '^[0-9a-f]{64}$'),
  headers_sha256   TEXT NOT NULL CHECK (headers_sha256 ~ '^[0-9a-f]{64}$'),
  error_sha256     TEXT NOT NULL CHECK (error_sha256 ~ '^[0-9a-f]{64}$'),
  acknowledged_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
  PRIMARY KEY (consumer_group,source_topic,source_partition,source_offset)
);

CREATE TABLE IF NOT EXISTS alignment_schema_migrations (
  version TEXT PRIMARY KEY,
  description TEXT NOT NULL,
  applied_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  applied_by TEXT NOT NULL DEFAULT current_user
);
INSERT INTO alignment_schema_migrations(version,description)
VALUES ('202608131330','durable Kafka DLQ source acknowledgement receipts')
ON CONFLICT (version) DO NOTHING;

COMMIT;
