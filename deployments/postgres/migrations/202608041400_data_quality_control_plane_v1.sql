-- T-DQ-001: persistent, versioned data-quality control plane.
--
-- Expand: add dataset, rule, baseline, watermark, event, repair and outbox tables.
-- Backfill: register datasets explicitly per tenant; do not infer green status from
--           an absent rule, baseline or watermark.
-- Verify: compare rule/baseline versions, real hand-off watermarks and outbox/audit
--         rows before enabling any release-blocking policy.
-- Cutover: first shadow evaluation, then approved per-rule activation.
-- Rollback: disable evaluators and retain all immutable versions and evidence;
--           no destructive table or column rollback is permitted.
BEGIN;

CREATE TABLE IF NOT EXISTS data_quality_datasets (
  tenant_id TEXT NOT NULL REFERENCES tenants(tenant_id) ON DELETE CASCADE,
  dataset_id TEXT NOT NULL CHECK (dataset_id <> ''),
  display_name TEXT NOT NULL,
  owner TEXT NOT NULL CHECK (owner <> ''),
  schema_version BIGINT NOT NULL CHECK (schema_version > 0),
  signal_contract_version TEXT NOT NULL CHECK (signal_contract_version <> ''),
  business_keys JSONB NOT NULL CHECK (jsonb_typeof(business_keys) = 'array'),
  allowed_lateness_seconds BIGINT NOT NULL CHECK (allowed_lateness_seconds >= 0),
  retention_seconds BIGINT NOT NULL CHECK (retention_seconds > 0),
  upstreams JSONB NOT NULL CHECK (jsonb_typeof(upstreams) = 'array'),
  downstreams JSONB NOT NULL CHECK (jsonb_typeof(downstreams) = 'array'),
  slo_target NUMERIC(8,7) NOT NULL CHECK (slo_target > 0 AND slo_target <= 1),
  status TEXT NOT NULL DEFAULT 'active' CHECK (status IN ('active','paused','retired')),
  revision BIGINT NOT NULL DEFAULT 1 CHECK (revision > 0),
  trace_id TEXT NOT NULL CHECK (trace_id <> ''),
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  PRIMARY KEY (tenant_id,dataset_id)
);
CREATE INDEX IF NOT EXISTS idx_data_quality_datasets_owner_status
  ON data_quality_datasets (tenant_id,owner,status,dataset_id);

CREATE TABLE IF NOT EXISTS data_quality_rules (
  tenant_id TEXT NOT NULL,
  rule_id UUID NOT NULL,
  rule_key TEXT NOT NULL CHECK (rule_key <> ''),
  dataset_id TEXT NOT NULL,
  rule_version BIGINT NOT NULL CHECK (rule_version > 0),
  dimension TEXT NOT NULL CHECK (dimension IN (
    'completeness','uniqueness','timeliness','validity','referential_integrity',
    'ordering','duplicate','lateness','tenant_ownership','object_availability'
  )),
  field_path TEXT NOT NULL DEFAULT '',
  predicate JSONB NOT NULL CHECK (jsonb_typeof(predicate) = 'object'),
  threshold JSONB NOT NULL CHECK (jsonb_typeof(threshold) = 'object'),
  window_seconds BIGINT NOT NULL CHECK (window_seconds > 0),
  sampling JSONB NOT NULL CHECK (jsonb_typeof(sampling) = 'object'),
  severity TEXT NOT NULL CHECK (severity IN ('info','warning','high','critical')),
  owner TEXT NOT NULL CHECK (owner <> ''),
  exemption_policy JSONB NOT NULL DEFAULT '{}'::jsonb CHECK (jsonb_typeof(exemption_policy) = 'object'),
  repair_action TEXT NOT NULL DEFAULT '',
  gate_policy TEXT NOT NULL DEFAULT 'observe'
    CHECK (gate_policy IN ('observe','degrade','quarantine','release_block')),
  status TEXT NOT NULL DEFAULT 'draft'
    CHECK (status IN ('draft','shadow','approval_pending','active','superseded','rejected','retired')),
  revision BIGINT NOT NULL DEFAULT 1 CHECK (revision > 0),
  created_by TEXT NOT NULL,
  approved_by TEXT NOT NULL DEFAULT '',
  reason TEXT NOT NULL,
  trace_id TEXT NOT NULL CHECK (trace_id <> ''),
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  PRIMARY KEY (tenant_id,rule_id,rule_version),
  UNIQUE (tenant_id,rule_key,rule_version),
  FOREIGN KEY (tenant_id,dataset_id)
    REFERENCES data_quality_datasets(tenant_id,dataset_id) ON DELETE RESTRICT
);
CREATE UNIQUE INDEX IF NOT EXISTS idx_data_quality_rules_one_active
  ON data_quality_rules (tenant_id,rule_id) WHERE status='active';
CREATE INDEX IF NOT EXISTS idx_data_quality_rules_dataset_status
  ON data_quality_rules (tenant_id,dataset_id,status,updated_at DESC);

CREATE TABLE IF NOT EXISTS data_quality_baselines (
  baseline_id UUID PRIMARY KEY,
  tenant_id TEXT NOT NULL,
  dataset_id TEXT NOT NULL,
  baseline_version BIGINT NOT NULL CHECK (baseline_version > 0),
  status TEXT NOT NULL DEFAULT 'active' CHECK (status IN ('shadow','active','superseded','rejected')),
  window_start TIMESTAMPTZ NOT NULL,
  window_end TIMESTAMPTZ NOT NULL CHECK (window_end > window_start),
  sample_count BIGINT NOT NULL CHECK (sample_count >= 0),
  metrics JSONB NOT NULL CHECK (jsonb_typeof(metrics) = 'object'),
  schema_columns JSONB NOT NULL CHECK (jsonb_typeof(schema_columns) = 'array'),
  schema_sha256 TEXT NOT NULL CHECK (length(schema_sha256) = 64),
  source_watermarks JSONB NOT NULL CHECK (jsonb_typeof(source_watermarks) = 'object'),
  created_by TEXT NOT NULL,
  trace_id TEXT NOT NULL CHECK (trace_id <> ''),
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE (tenant_id,dataset_id,baseline_version),
  FOREIGN KEY (tenant_id,dataset_id)
    REFERENCES data_quality_datasets(tenant_id,dataset_id) ON DELETE RESTRICT
);
CREATE UNIQUE INDEX IF NOT EXISTS idx_data_quality_baselines_one_active
  ON data_quality_baselines (tenant_id,dataset_id) WHERE status='active';

CREATE TABLE IF NOT EXISTS data_quality_watermarks (
  tenant_id TEXT NOT NULL,
  dataset_id TEXT NOT NULL,
  source_kind TEXT NOT NULL CHECK (source_kind IN (
    'kafka_offset','flink_watermark','sink_commit','business_version','object_manifest'
  )),
  source_id TEXT NOT NULL CHECK (source_id <> ''),
  partition_id TEXT NOT NULL DEFAULT '',
  measurement_status TEXT NOT NULL CHECK (measurement_status IN (
    'measured','unknown','not_applicable','error'
  )),
  watermark_value TEXT,
  observed_at TIMESTAMPTZ,
  collected_at TIMESTAMPTZ NOT NULL,
  trace_id TEXT NOT NULL CHECK (trace_id <> ''),
  measurement_error TEXT NOT NULL DEFAULT '',
  metadata JSONB NOT NULL DEFAULT '{}'::jsonb CHECK (jsonb_typeof(metadata) = 'object'),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  CHECK (
    (measurement_status = 'measured' AND watermark_value IS NOT NULL AND watermark_value <> '' AND observed_at IS NOT NULL AND measurement_error = '') OR
    (measurement_status = 'error' AND watermark_value IS NULL AND observed_at IS NULL AND measurement_error <> '') OR
    (measurement_status IN ('unknown','not_applicable') AND watermark_value IS NULL AND observed_at IS NULL AND measurement_error = '')
  ),
  PRIMARY KEY (tenant_id,dataset_id,source_kind,source_id,partition_id),
  FOREIGN KEY (tenant_id,dataset_id)
    REFERENCES data_quality_datasets(tenant_id,dataset_id) ON DELETE RESTRICT
);
CREATE INDEX IF NOT EXISTS idx_data_quality_watermarks_freshness
  ON data_quality_watermarks (tenant_id,dataset_id,collected_at DESC);

CREATE TABLE IF NOT EXISTS data_quality_events (
  quality_event_id UUID PRIMARY KEY,
  tenant_id TEXT NOT NULL,
  dataset_id TEXT NOT NULL,
  rule_id UUID NOT NULL,
  rule_version BIGINT NOT NULL CHECK (rule_version > 0),
  status TEXT NOT NULL DEFAULT 'detected' CHECK (status IN (
    'detected','triaged','repair_planned','dry_run_passed','approved',
    'replaying','reconciled','closed','failed'
  )),
  severity TEXT NOT NULL CHECK (severity IN ('info','warning','high','critical')),
  window_start TIMESTAMPTZ NOT NULL,
  window_end TIMESTAMPTZ NOT NULL CHECK (window_end > window_start),
  affected_count BIGINT NOT NULL DEFAULT 0 CHECK (affected_count >= 0),
  measured_value JSONB NOT NULL CHECK (jsonb_typeof(measured_value) = 'object'),
  source_watermarks JSONB NOT NULL CHECK (jsonb_typeof(source_watermarks) = 'object'),
  sample_manifest JSONB NOT NULL DEFAULT '{}'::jsonb CHECK (jsonb_typeof(sample_manifest) = 'object'),
  owner TEXT NOT NULL CHECK (owner <> ''),
  root_cause TEXT NOT NULL DEFAULT '',
  revision BIGINT NOT NULL DEFAULT 1 CHECK (revision > 0),
  trace_id TEXT NOT NULL CHECK (trace_id <> ''),
  detected_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  closed_at TIMESTAMPTZ,
  FOREIGN KEY (tenant_id,rule_id,rule_version)
    REFERENCES data_quality_rules(tenant_id,rule_id,rule_version) ON DELETE RESTRICT,
  FOREIGN KEY (tenant_id,dataset_id)
    REFERENCES data_quality_datasets(tenant_id,dataset_id) ON DELETE RESTRICT
);
CREATE INDEX IF NOT EXISTS idx_data_quality_events_work_queue
  ON data_quality_events (tenant_id,status,severity,detected_at DESC);

CREATE TABLE IF NOT EXISTS data_quality_repairs (
  repair_id UUID PRIMARY KEY,
  tenant_id TEXT NOT NULL,
  quality_event_id UUID NOT NULL REFERENCES data_quality_events(quality_event_id) ON DELETE RESTRICT,
  operation_id TEXT NOT NULL CHECK (operation_id <> ''),
  idempotency_key TEXT NOT NULL CHECK (length(idempotency_key) BETWEEN 16 AND 200),
  status TEXT NOT NULL DEFAULT 'planned' CHECK (status IN (
    'planned','dry_run_running','dry_run_passed','approval_pending','approved',
    'executing','executed','partial','failed','reconciled','cancelled','compensating','compensated'
  )),
  input_scope JSONB NOT NULL CHECK (jsonb_typeof(input_scope) = 'object'),
  resource_budget JSONB NOT NULL CHECK (jsonb_typeof(resource_budget) = 'object'),
  repair_summary JSONB NOT NULL DEFAULT '{}'::jsonb CHECK (jsonb_typeof(repair_summary) = 'object'),
  reconcile_summary JSONB NOT NULL DEFAULT '{}'::jsonb CHECK (jsonb_typeof(reconcile_summary) = 'object'),
  requested_by TEXT NOT NULL,
  approved_by TEXT NOT NULL DEFAULT '',
  reason TEXT NOT NULL,
  revision BIGINT NOT NULL DEFAULT 1 CHECK (revision > 0),
  trace_id TEXT NOT NULL CHECK (trace_id <> ''),
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  completed_at TIMESTAMPTZ,
  UNIQUE (tenant_id,idempotency_key),
  CHECK (approved_by = '' OR approved_by <> requested_by)
);
CREATE INDEX IF NOT EXISTS idx_data_quality_repairs_status
  ON data_quality_repairs (tenant_id,status,updated_at,repair_id);

CREATE TABLE IF NOT EXISTS data_quality_outbox (
  outbox_id BIGSERIAL PRIMARY KEY,
  event_id UUID NOT NULL UNIQUE,
  tenant_id TEXT NOT NULL,
  aggregate_type TEXT NOT NULL CHECK (aggregate_type IN ('dataset','rule','baseline','quality_event','repair')),
  aggregate_id TEXT NOT NULL CHECK (aggregate_id <> ''),
  aggregate_version BIGINT NOT NULL CHECK (aggregate_version > 0),
  event_type TEXT NOT NULL CHECK (event_type <> ''),
  schema_version INTEGER NOT NULL DEFAULT 1 CHECK (schema_version = 1),
  partition_key TEXT NOT NULL CHECK (partition_key <> ''),
  payload JSONB NOT NULL CHECK (jsonb_typeof(payload) = 'object'),
  trace_id TEXT NOT NULL CHECK (trace_id <> ''),
  status TEXT NOT NULL DEFAULT 'pending' CHECK (status IN ('pending','processing','published','dead')),
  attempt_count INTEGER NOT NULL DEFAULT 0 CHECK (attempt_count >= 0),
  available_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  locked_until TIMESTAMPTZ,
  locked_by TEXT NOT NULL DEFAULT '',
  last_error TEXT NOT NULL DEFAULT '',
  occurred_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  published_at TIMESTAMPTZ
);
CREATE INDEX IF NOT EXISTS idx_data_quality_outbox_ready
  ON data_quality_outbox (available_at,occurred_at,outbox_id) WHERE status='pending';
CREATE INDEX IF NOT EXISTS idx_data_quality_outbox_reclaim
  ON data_quality_outbox (locked_until,outbox_id) WHERE status='processing';

CREATE TABLE IF NOT EXISTS alignment_schema_migrations (
  version TEXT PRIMARY KEY,
  description TEXT NOT NULL,
  applied_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  applied_by TEXT NOT NULL DEFAULT current_user
);
INSERT INTO alignment_schema_migrations(version,description)
VALUES ('202608041400','persistent versioned data quality control plane and real handoff watermarks')
ON CONFLICT (version) DO NOTHING;

COMMIT;
