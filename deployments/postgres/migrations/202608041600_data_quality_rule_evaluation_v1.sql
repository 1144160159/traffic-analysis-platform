-- T-DQ-001: durable bounded rule evaluations and detected quality events.
-- Expand only. The evaluator remains default-off until this migration and the
-- approved active-rule catalog are verified in a candidate environment.
BEGIN;

CREATE TABLE IF NOT EXISTS data_quality_rule_evaluations (
  evaluation_id UUID PRIMARY KEY,
  tenant_id TEXT NOT NULL,
  rule_id UUID NOT NULL,
  rule_version BIGINT NOT NULL CHECK (rule_version > 0),
  dataset_id TEXT NOT NULL,
  window_start TIMESTAMPTZ NOT NULL,
  window_end TIMESTAMPTZ NOT NULL CHECK (window_end > window_start),
  status TEXT NOT NULL CHECK (status IN ('pass','fail','unknown')),
  total_count BIGINT NOT NULL CHECK (total_count >= 0),
  passed_count BIGINT NOT NULL CHECK (passed_count >= 0 AND passed_count <= total_count),
  affected_count BIGINT NOT NULL CHECK (affected_count = total_count - passed_count),
  measured_value JSONB NOT NULL CHECK (jsonb_typeof(measured_value) = 'object'),
  source_watermarks JSONB NOT NULL CHECK (jsonb_typeof(source_watermarks) = 'object'),
  quality_event_id UUID,
  trace_id TEXT NOT NULL CHECK (trace_id <> ''),
  evaluated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE (tenant_id,rule_id,rule_version,window_start,window_end),
  FOREIGN KEY (tenant_id,rule_id,rule_version)
    REFERENCES data_quality_rules(tenant_id,rule_id,rule_version) ON DELETE RESTRICT,
  FOREIGN KEY (tenant_id,dataset_id)
    REFERENCES data_quality_datasets(tenant_id,dataset_id) ON DELETE RESTRICT,
  FOREIGN KEY (quality_event_id)
    REFERENCES data_quality_events(quality_event_id) ON DELETE RESTRICT
);
CREATE INDEX IF NOT EXISTS idx_data_quality_rule_evaluations_window
  ON data_quality_rule_evaluations (tenant_id,dataset_id,window_end DESC,evaluation_id);

INSERT INTO alignment_schema_migrations(version,description)
VALUES ('202608041600','durable bounded data quality rule evaluations and event detection')
ON CONFLICT (version) DO NOTHING;

COMMIT;
