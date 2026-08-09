-- F-ALERT-003 / T-KAFKA-003: durable alert-response poison-event receipt.
-- The common Kafka consumer invokes this PostgreSQL barrier only after the
-- canonical DLQ write is acknowledged and before committing the source offset.
BEGIN;

CREATE TABLE IF NOT EXISTS alert_response_dlq_receipts (
  source_topic TEXT NOT NULL CHECK (source_topic = 'alert.response.requested.v1'),
  source_partition INTEGER NOT NULL CHECK (source_partition >= 0),
  source_offset BIGINT NOT NULL CHECK (source_offset >= 0),
  dlq_topic TEXT NOT NULL CHECK (dlq_topic = 'dlq.v1'),
  event_id TEXT NOT NULL DEFAULT '', tenant_id TEXT NOT NULL DEFAULT '',
  job_id TEXT NOT NULL DEFAULT '', alert_id TEXT NOT NULL DEFAULT '',
  action_id TEXT NOT NULL DEFAULT '', aggregate_version BIGINT CHECK (aggregate_version > 0),
  trace_id TEXT NOT NULL DEFAULT '',
  error_code TEXT NOT NULL CHECK (error_code <> ''), error_message TEXT NOT NULL DEFAULT '',
  payload_sha256 TEXT NOT NULL CHECK (payload_sha256 ~ '^[0-9a-f]{64}$'),
  headers_sha256 TEXT NOT NULL CHECK (headers_sha256 ~ '^[0-9a-f]{64}$'),
  headers JSONB NOT NULL DEFAULT '{}'::jsonb CHECK (jsonb_typeof(headers) = 'object'),
  acknowledged_at TIMESTAMPTZ NOT NULL,
  PRIMARY KEY (source_topic,source_partition,source_offset)
);
CREATE INDEX IF NOT EXISTS idx_alert_response_dlq_receipts_tenant_trace
  ON alert_response_dlq_receipts (tenant_id,trace_id,acknowledged_at DESC)
  WHERE tenant_id <> '' AND trace_id <> '';
CREATE INDEX IF NOT EXISTS idx_alert_response_dlq_receipts_event
  ON alert_response_dlq_receipts (event_id,acknowledged_at DESC) WHERE event_id <> '';

INSERT INTO alignment_schema_migrations(version,description)
VALUES ('202608091230','alert response DLQ acknowledgement PostgreSQL and audit barrier')
ON CONFLICT (version) DO NOTHING;

COMMIT;
