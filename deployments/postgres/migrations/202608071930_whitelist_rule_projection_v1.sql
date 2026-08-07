-- Expand-only migration. The event pipeline remains disabled until this
-- migration, the canonical topic and both service candidates are available.
BEGIN;

CREATE TABLE IF NOT EXISTS whitelist_rule_projection (
  tenant_id TEXT NOT NULL,
  entry_id UUID NOT NULL REFERENCES whitelist(id) ON DELETE RESTRICT,
  entry_version BIGINT NOT NULL CHECK (entry_version > 0),
  source_event_id UUID NOT NULL UNIQUE REFERENCES whitelist_event_outbox(event_id) ON DELETE RESTRICT,
  desired_state TEXT NOT NULL CHECK (desired_state IN ('effective','revoked')),
  entry_type TEXT NOT NULL CHECK (entry_type IN ('ip','domain','fingerprint','subnet','asset','account','rule','model')),
  match_value TEXT NOT NULL CHECK (match_value <> ''),
  scope TEXT NOT NULL DEFAULT '',
  expires_at TIMESTAMPTZ,
  rule_revision TEXT NOT NULL CHECK (length(rule_revision) = 64),
  payload_sha256 TEXT NOT NULL CHECK (length(payload_sha256) = 64),
  kafka_partition INTEGER NOT NULL CHECK (kafka_partition >= 0),
  kafka_offset BIGINT NOT NULL CHECK (kafka_offset >= 0),
  applied_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  PRIMARY KEY (tenant_id,entry_id)
);
CREATE INDEX IF NOT EXISTS idx_whitelist_rule_projection_effective
  ON whitelist_rule_projection (tenant_id,entry_type,match_value)
  WHERE desired_state='effective';

INSERT INTO alignment_schema_migrations(version,description)
VALUES ('202608071930','whitelist rule-manager projection and ingestion enforcement')
ON CONFLICT (version) DO NOTHING;

COMMIT;
