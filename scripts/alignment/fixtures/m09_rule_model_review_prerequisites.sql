-- Run-scoped PostgreSQL fixture for T1-M09-N019 Kubernetes acceptance.
-- This schema contains only the runtime receipt authority read by the
-- deployment expansion gate.  It must never be applied to a shared database.

CREATE TABLE codex_ephemeral_rule_model_review_sentinel (
  marker TEXT PRIMARY KEY CHECK (marker = 'ephemeral-only')
);
INSERT INTO codex_ephemeral_rule_model_review_sentinel(marker) VALUES ('ephemeral-only');

CREATE TABLE tenants (
  tenant_id TEXT PRIMARY KEY,
  name TEXT NOT NULL
);

CREATE TABLE rule_versions (
  rule_version TEXT PRIMARY KEY,
  rule_id TEXT NOT NULL,
  tenant_id TEXT NOT NULL REFERENCES tenants(tenant_id) ON DELETE CASCADE,
  version BIGINT NOT NULL
);

CREATE TABLE rule_outbox (
  id BIGSERIAL PRIMARY KEY,
  rule_id TEXT NOT NULL,
  payload JSONB NOT NULL,
  published BOOLEAN NOT NULL DEFAULT false,
  runtime_status TEXT NOT NULL DEFAULT 'pending',
  runtime_applied_at TIMESTAMPTZ,
  runtime_last_error TEXT NOT NULL DEFAULT ''
);

CREATE TABLE rule_update_applied_acks (
  event_id TEXT NOT NULL,
  subtask_index INT NOT NULL,
  parallelism INT NOT NULL,
  status TEXT NOT NULL,
  PRIMARY KEY (event_id, subtask_index)
);

CREATE TABLE model_versions (
  model_version TEXT PRIMARY KEY,
  model_id UUID NOT NULL,
  tenant_id TEXT NOT NULL REFERENCES tenants(tenant_id) ON DELETE CASCADE
);

CREATE TABLE model_update_outbox (
  id BIGSERIAL PRIMARY KEY,
  event_id TEXT NOT NULL UNIQUE,
  tenant_id TEXT NOT NULL,
  model_id TEXT NOT NULL,
  model_version TEXT NOT NULL,
  action TEXT NOT NULL,
  status TEXT NOT NULL DEFAULT 'pending',
  published_at TIMESTAMPTZ,
  last_error TEXT NOT NULL DEFAULT ''
);

CREATE TABLE model_update_applied_acks (
  event_id TEXT NOT NULL,
  subtask_index INT NOT NULL,
  parallelism INT NOT NULL,
  status TEXT NOT NULL,
  applied_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  PRIMARY KEY (event_id, subtask_index)
);

CREATE TABLE deployments (
  deployment_id TEXT PRIMARY KEY,
  tenant_id TEXT NOT NULL REFERENCES tenants(tenant_id) ON DELETE CASCADE,
  rule_version TEXT,
  model_version TEXT,
  status TEXT NOT NULL
);

CREATE TABLE deployment_outbox (
  id BIGSERIAL PRIMARY KEY,
  event_id TEXT NOT NULL UNIQUE,
  deployment_id TEXT NOT NULL,
  tenant_id TEXT NOT NULL,
  event_type TEXT NOT NULL,
  status TEXT NOT NULL DEFAULT 'pending',
  last_error TEXT NOT NULL DEFAULT ''
);

CREATE TABLE deployment_event_projection (
  event_id UUID PRIMARY KEY,
  deployment_id TEXT NOT NULL,
  tenant_id TEXT NOT NULL,
  action TEXT NOT NULL,
  status TEXT NOT NULL,
  projected_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  kafka_partition INT NOT NULL,
  kafka_offset BIGINT NOT NULL,
  UNIQUE (kafka_partition, kafka_offset)
);
