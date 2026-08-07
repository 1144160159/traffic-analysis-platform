-- T-PG-002 / T-KAFKA-001 threat-intel command atomicity expansion.
-- Expand only: retain all existing rows and the v1 event outbox.
BEGIN;

ALTER TABLE threat_intel
  ADD COLUMN IF NOT EXISTS revision BIGINT NOT NULL DEFAULT 1;
ALTER TABLE threat_intel
  DROP CONSTRAINT IF EXISTS threat_intel_revision_positive;
ALTER TABLE threat_intel
  ADD CONSTRAINT threat_intel_revision_positive CHECK (revision > 0);

ALTER TABLE threat_intel_feeds
  ADD COLUMN IF NOT EXISTS revision BIGINT NOT NULL DEFAULT 1;
ALTER TABLE threat_intel_feeds
  DROP CONSTRAINT IF EXISTS threat_intel_feeds_revision_positive;
ALTER TABLE threat_intel_feeds
  ADD CONSTRAINT threat_intel_feeds_revision_positive CHECK (revision > 0);

CREATE TABLE IF NOT EXISTS threat_intel_command_history (
  history_id        BIGSERIAL PRIMARY KEY,
  event_id          TEXT NOT NULL REFERENCES threat_intel_event_outbox(event_id) ON DELETE RESTRICT,
  tenant_id         TEXT NOT NULL,
  aggregate_type    TEXT NOT NULL CHECK (aggregate_type IN ('entry','feed')),
  aggregate_id      TEXT NOT NULL,
  revision          BIGINT NOT NULL CHECK (revision > 0),
  action_id         TEXT NOT NULL,
  operation         TEXT NOT NULL,
  reason            TEXT NOT NULL,
  trace_id          TEXT NOT NULL,
  compatibility_mode BOOLEAN NOT NULL DEFAULT false,
  snapshot          JSONB NOT NULL,
  created_at        TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE (tenant_id,aggregate_type,aggregate_id,revision),
  UNIQUE (event_id,aggregate_type,aggregate_id)
);
CREATE INDEX IF NOT EXISTS idx_threat_intel_command_history_lookup
  ON threat_intel_command_history (tenant_id,aggregate_type,aggregate_id,revision DESC);

CREATE TABLE IF NOT EXISTS threat_intel_command_requests (
  tenant_id          TEXT NOT NULL,
  idempotency_key    TEXT NOT NULL CHECK (length(idempotency_key) BETWEEN 16 AND 200),
  request_sha256     TEXT NOT NULL CHECK (length(request_sha256) = 64),
  action_id          TEXT NOT NULL,
  command_type       TEXT NOT NULL,
  expected_revision  BIGINT NOT NULL CHECK (expected_revision >= 0),
  resulting_revision BIGINT NOT NULL CHECK (resulting_revision > 0),
  event_id            TEXT NOT NULL REFERENCES threat_intel_event_outbox(event_id) ON DELETE RESTRICT,
  reason              TEXT NOT NULL,
  trace_id            TEXT NOT NULL,
  compatibility_mode  BOOLEAN NOT NULL DEFAULT false,
  response_payload    JSONB NOT NULL,
  created_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
  PRIMARY KEY (tenant_id,idempotency_key),
  UNIQUE (event_id)
);
CREATE INDEX IF NOT EXISTS idx_threat_intel_command_requests_created
  ON threat_intel_command_requests (tenant_id,created_at DESC);

CREATE TABLE IF NOT EXISTS alignment_schema_migrations (
  version     TEXT PRIMARY KEY,
  description TEXT NOT NULL,
  applied_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
  applied_by  TEXT NOT NULL DEFAULT current_user
);
INSERT INTO alignment_schema_migrations(version,description)
VALUES ('202608031550','threat intel revision history audit outbox and durable command replay')
ON CONFLICT (version) DO NOTHING;

COMMIT;
