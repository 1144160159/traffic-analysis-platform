-- T-DQ-001: idempotent dataset/rule governance and immutable command history.
--
-- Expand only. Runtime code must not create these objects. Rollback disables
-- the governance routes and retains history, outbox and command receipts.
BEGIN;

CREATE TABLE IF NOT EXISTS data_quality_dataset_history (
  history_id BIGSERIAL PRIMARY KEY,
  event_id UUID NOT NULL UNIQUE,
  tenant_id TEXT NOT NULL,
  dataset_id TEXT NOT NULL,
  revision BIGINT NOT NULL CHECK (revision > 0),
  operation TEXT NOT NULL CHECK (operation IN ('created','updated','retired')),
  actor_id TEXT NOT NULL CHECK (actor_id <> ''),
  reason TEXT NOT NULL CHECK (length(reason) BETWEEN 8 AND 1000),
  trace_id TEXT NOT NULL CHECK (trace_id <> ''),
  snapshot JSONB NOT NULL CHECK (jsonb_typeof(snapshot) = 'object'),
  occurred_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE (tenant_id,dataset_id,revision),
  FOREIGN KEY (tenant_id,dataset_id)
    REFERENCES data_quality_datasets(tenant_id,dataset_id) ON DELETE RESTRICT
);
CREATE INDEX IF NOT EXISTS idx_data_quality_dataset_history_lookup
  ON data_quality_dataset_history (tenant_id,dataset_id,revision DESC);

CREATE TABLE IF NOT EXISTS data_quality_rule_history (
  history_id BIGSERIAL PRIMARY KEY,
  event_id UUID NOT NULL UNIQUE,
  tenant_id TEXT NOT NULL,
  rule_id UUID NOT NULL,
  rule_version BIGINT NOT NULL CHECK (rule_version > 0),
  revision BIGINT NOT NULL CHECK (revision > 0),
  operation TEXT NOT NULL CHECK (operation IN (
    'created','shadow_started','approval_submitted','approved','rejected','retired'
  )),
  previous_status TEXT NOT NULL,
  resulting_status TEXT NOT NULL,
  actor_id TEXT NOT NULL CHECK (actor_id <> ''),
  reason TEXT NOT NULL CHECK (length(reason) BETWEEN 8 AND 1000),
  trace_id TEXT NOT NULL CHECK (trace_id <> ''),
  snapshot JSONB NOT NULL CHECK (jsonb_typeof(snapshot) = 'object'),
  occurred_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE (tenant_id,rule_id,rule_version,revision),
  FOREIGN KEY (tenant_id,rule_id,rule_version)
    REFERENCES data_quality_rules(tenant_id,rule_id,rule_version) ON DELETE RESTRICT
);
CREATE INDEX IF NOT EXISTS idx_data_quality_rule_history_lookup
  ON data_quality_rule_history (tenant_id,rule_id,rule_version DESC,revision DESC);

CREATE TABLE IF NOT EXISTS data_quality_command_requests (
  tenant_id TEXT NOT NULL,
  idempotency_key TEXT NOT NULL CHECK (length(idempotency_key) BETWEEN 16 AND 200),
  request_sha256 TEXT NOT NULL CHECK (length(request_sha256) = 64),
  action_id TEXT NOT NULL CHECK (action_id <> ''),
  operation TEXT NOT NULL CHECK (operation <> ''),
  aggregate_type TEXT NOT NULL CHECK (aggregate_type IN ('dataset','rule')),
  aggregate_id TEXT NOT NULL CHECK (aggregate_id <> ''),
  resulting_revision BIGINT NOT NULL CHECK (resulting_revision > 0),
  event_id UUID NOT NULL REFERENCES data_quality_outbox(event_id) ON DELETE RESTRICT,
  response_payload JSONB NOT NULL CHECK (jsonb_typeof(response_payload) = 'object'),
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  PRIMARY KEY (tenant_id,idempotency_key),
  UNIQUE (event_id)
);
CREATE INDEX IF NOT EXISTS idx_data_quality_command_requests_aggregate
  ON data_quality_command_requests (tenant_id,aggregate_type,aggregate_id,created_at DESC);

INSERT INTO alignment_schema_migrations(version,description)
VALUES ('202608041500','idempotent data quality dataset and rule governance history')
ON CONFLICT (version) DO NOTHING;

COMMIT;
