-- =============================================================================
-- 任务管理 + 审计日志 (PostgreSQL)
-- 来源: common/old/postgres_ddl.sql
-- =============================================================================
BEGIN;

CREATE TABLE IF NOT EXISTS tasks (
  task_id         UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
  tenant_id       TEXT NOT NULL REFERENCES tenants(tenant_id) ON DELETE CASCADE,
  name            TEXT NOT NULL DEFAULT '',
  task_type       TEXT NOT NULL,
  params          JSONB NOT NULL DEFAULT '{}'::jsonb,
  status          TEXT NOT NULL DEFAULT 'queued',
  progress        INT NOT NULL DEFAULT 0,
  result_file_key TEXT NOT NULL DEFAULT '',
  result_sha256   TEXT NOT NULL DEFAULT '',
  result_packets  BIGINT NOT NULL DEFAULT 0,
  result_bytes    BIGINT NOT NULL DEFAULT 0,
  files_scanned   INT NOT NULL DEFAULT 0,
  error_message   TEXT NOT NULL DEFAULT '',
  run_id          TEXT NOT NULL DEFAULT '',
  created_by      TEXT NOT NULL DEFAULT '',
  created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
  started_at      TIMESTAMPTZ,
  completed_at    TIMESTAMPTZ
);

ALTER TABLE tasks ADD COLUMN IF NOT EXISTS name TEXT NOT NULL DEFAULT '';
ALTER TABLE tasks ADD COLUMN IF NOT EXISTS progress INT NOT NULL DEFAULT 0;
ALTER TABLE tasks ADD COLUMN IF NOT EXISTS result_file_key TEXT NOT NULL DEFAULT '';
ALTER TABLE tasks ADD COLUMN IF NOT EXISTS result_sha256 TEXT NOT NULL DEFAULT '';
ALTER TABLE tasks ADD COLUMN IF NOT EXISTS result_packets BIGINT NOT NULL DEFAULT 0;
ALTER TABLE tasks ADD COLUMN IF NOT EXISTS result_bytes BIGINT NOT NULL DEFAULT 0;
ALTER TABLE tasks ADD COLUMN IF NOT EXISTS files_scanned INT NOT NULL DEFAULT 0;
ALTER TABLE tasks ADD COLUMN IF NOT EXISTS error_message TEXT NOT NULL DEFAULT '';
ALTER TABLE tasks ADD COLUMN IF NOT EXISTS run_id TEXT NOT NULL DEFAULT '';
ALTER TABLE tasks ADD COLUMN IF NOT EXISTS created_by TEXT NOT NULL DEFAULT '';
ALTER TABLE tasks ADD COLUMN IF NOT EXISTS updated_at TIMESTAMPTZ NOT NULL DEFAULT now();
ALTER TABLE tasks ADD COLUMN IF NOT EXISTS started_at TIMESTAMPTZ;
ALTER TABLE tasks ADD COLUMN IF NOT EXISTS completed_at TIMESTAMPTZ;

DO $$
DECLARE
  constraint_name TEXT;
BEGIN
  FOR constraint_name IN
    SELECT conname
    FROM pg_constraint
    WHERE conrelid = 'tasks'::regclass
      AND contype = 'f'
      AND pg_get_constraintdef(oid) LIKE '%created_by%'
  LOOP
    EXECUTE format('ALTER TABLE tasks DROP CONSTRAINT %I', constraint_name);
  END LOOP;
END $$;

ALTER TABLE tasks ALTER COLUMN created_by TYPE TEXT USING created_by::TEXT;
ALTER TABLE tasks ALTER COLUMN created_by SET DEFAULT '';
ALTER TABLE tasks ALTER COLUMN created_by SET NOT NULL;
ALTER TABLE tasks ALTER COLUMN run_id SET DEFAULT '';
UPDATE tasks SET run_id = '' WHERE run_id IS NULL;
ALTER TABLE tasks ALTER COLUMN run_id SET NOT NULL;

CREATE TABLE IF NOT EXISTS audit_logs (
  id          BIGSERIAL PRIMARY KEY,
  event_id    TEXT NOT NULL DEFAULT ('audit-' || uuid_generate_v4()::text),
  tenant_id   TEXT NOT NULL,
  user_id     UUID,
  action      TEXT NOT NULL,
  object_type TEXT NOT NULL,
  object_id   TEXT,
  detail      JSONB NOT NULL DEFAULT '{}'::jsonb,
  ip_addr     TEXT,
  user_agent  TEXT,
  created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

ALTER TABLE audit_logs ADD COLUMN IF NOT EXISTS event_id TEXT;
UPDATE audit_logs SET event_id = 'audit-' || id::TEXT WHERE event_id IS NULL OR event_id = '';
ALTER TABLE audit_logs ALTER COLUMN event_id SET DEFAULT ('audit-' || uuid_generate_v4()::text);
ALTER TABLE audit_logs ALTER COLUMN event_id SET NOT NULL;
CREATE UNIQUE INDEX IF NOT EXISTS idx_audit_event_id ON audit_logs(event_id);
CREATE INDEX IF NOT EXISTS idx_audit_tenant_time ON audit_logs (tenant_id, created_at DESC);

ALTER TABLE audit_logs ADD COLUMN IF NOT EXISTS request_id TEXT;
ALTER TABLE audit_logs ADD COLUMN IF NOT EXISTS trace_id TEXT;
ALTER TABLE audit_logs ADD COLUMN IF NOT EXISTS success BOOLEAN NOT NULL DEFAULT true;
ALTER TABLE audit_logs ADD COLUMN IF NOT EXISTS error_message TEXT;
ALTER TABLE audit_logs ADD COLUMN IF NOT EXISTS risk_level TEXT;
ALTER TABLE audit_logs ADD COLUMN IF NOT EXISTS result TEXT;
UPDATE audit_logs SET request_id=detail->>'request_id' WHERE COALESCE(request_id,'')='' AND detail ? 'request_id';
UPDATE audit_logs SET trace_id=detail->>'trace_id' WHERE COALESCE(trace_id,'')='' AND detail ? 'trace_id';
UPDATE audit_logs SET result=COALESCE(NULLIF(detail->>'result',''), CASE WHEN success THEN 'success' ELSE 'failure' END) WHERE COALESCE(result,'')='';
UPDATE audit_logs SET risk_level=COALESCE(NULLIF(detail->>'risk',''),NULLIF(detail->>'risk_level',''),'low') WHERE COALESCE(risk_level,'')='';
UPDATE audit_logs SET success=false WHERE lower(result) IN ('failure','failed','error','denied');
CREATE INDEX IF NOT EXISTS idx_audit_tenant_request ON audit_logs (tenant_id, request_id) WHERE request_id IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_audit_tenant_trace ON audit_logs (tenant_id, trace_id) WHERE trace_id IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_audit_tenant_result_risk_time ON audit_logs (tenant_id, result, risk_level, created_at DESC);

CREATE TABLE IF NOT EXISTS audit_saved_queries (
  saved_query_id UUID PRIMARY KEY DEFAULT uuid_generate_v4(), tenant_id TEXT NOT NULL,
  name TEXT NOT NULL, filters JSONB NOT NULL DEFAULT '{}'::jsonb, created_by TEXT NOT NULL DEFAULT '',
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(), updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE (tenant_id, name)
);
CREATE INDEX IF NOT EXISTS idx_audit_saved_queries_tenant_time ON audit_saved_queries (tenant_id, created_at DESC);

CREATE TABLE IF NOT EXISTS audit_exports (
  export_id UUID PRIMARY KEY DEFAULT uuid_generate_v4(), tenant_id TEXT NOT NULL,
  format TEXT NOT NULL CHECK (format IN ('pdf','csv','json')), filters JSONB NOT NULL DEFAULT '{}'::jsonb,
  row_count INTEGER NOT NULL DEFAULT 0, total_matching INTEGER NOT NULL DEFAULT 0,
  truncated BOOLEAN NOT NULL DEFAULT false, mask_sensitive BOOLEAN NOT NULL DEFAULT true,
  filename TEXT NOT NULL, mime_type TEXT NOT NULL,
  sha256 TEXT NOT NULL, size_bytes BIGINT NOT NULL, created_by TEXT NOT NULL DEFAULT '',
  created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
ALTER TABLE audit_exports ADD COLUMN IF NOT EXISTS total_matching INTEGER NOT NULL DEFAULT 0;
ALTER TABLE audit_exports ADD COLUMN IF NOT EXISTS truncated BOOLEAN NOT NULL DEFAULT false;
ALTER TABLE audit_exports ADD COLUMN IF NOT EXISTS mask_sensitive BOOLEAN NOT NULL DEFAULT true;
CREATE INDEX IF NOT EXISTS idx_audit_exports_tenant_time ON audit_exports (tenant_id, created_at DESC);

CREATE TABLE IF NOT EXISTS audit_reviews (
  review_id UUID PRIMARY KEY DEFAULT uuid_generate_v4(), tenant_id TEXT NOT NULL, audit_log_id TEXT NOT NULL,
  decision TEXT NOT NULL CHECK (decision IN ('pending','approved','rejected','escalated')), comment TEXT NOT NULL DEFAULT '',
  risk_level TEXT NOT NULL CHECK (risk_level IN ('low','medium','high','critical')),
  reviewed_by TEXT NOT NULL DEFAULT '', created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_audit_reviews_tenant_log_time ON audit_reviews (tenant_id, audit_log_id, created_at DESC);

CREATE TABLE IF NOT EXISTS audit_integrity_checks (
  check_id UUID PRIMARY KEY DEFAULT uuid_generate_v4(), tenant_id TEXT NOT NULL,
  time_start TIMESTAMPTZ NOT NULL, time_end TIMESTAMPTZ NOT NULL, filters JSONB NOT NULL DEFAULT '{}'::jsonb, row_count BIGINT NOT NULL,
  root_sha256 TEXT NOT NULL, status TEXT NOT NULL CHECK (status IN ('passed','failed','baseline_created','no_records')),
  matched_count BIGINT NOT NULL DEFAULT 0, baselined_count BIGINT NOT NULL DEFAULT 0,
  mismatched_count BIGINT NOT NULL DEFAULT 0, added_count BIGINT NOT NULL DEFAULT 0,
  missing_count BIGINT NOT NULL DEFAULT 0,
  requested_by TEXT NOT NULL DEFAULT '', created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
ALTER TABLE audit_integrity_checks ADD COLUMN IF NOT EXISTS matched_count BIGINT NOT NULL DEFAULT 0;
ALTER TABLE audit_integrity_checks ADD COLUMN IF NOT EXISTS baselined_count BIGINT NOT NULL DEFAULT 0;
ALTER TABLE audit_integrity_checks ADD COLUMN IF NOT EXISTS mismatched_count BIGINT NOT NULL DEFAULT 0;
ALTER TABLE audit_integrity_checks ADD COLUMN IF NOT EXISTS filters JSONB NOT NULL DEFAULT '{}'::jsonb;
ALTER TABLE audit_integrity_checks ADD COLUMN IF NOT EXISTS added_count BIGINT NOT NULL DEFAULT 0;
ALTER TABLE audit_integrity_checks ADD COLUMN IF NOT EXISTS missing_count BIGINT NOT NULL DEFAULT 0;
ALTER TABLE audit_integrity_checks DROP CONSTRAINT IF EXISTS audit_integrity_checks_status_check;
ALTER TABLE audit_integrity_checks ADD CONSTRAINT audit_integrity_checks_status_check CHECK (status IN ('passed','failed','baseline_created','no_records'));
CREATE INDEX IF NOT EXISTS idx_audit_integrity_checks_tenant_time ON audit_integrity_checks (tenant_id, created_at DESC);

CREATE TABLE IF NOT EXISTS audit_log_integrity_baselines (
  tenant_id TEXT NOT NULL, audit_log_id TEXT NOT NULL, root_sha256 TEXT NOT NULL,
  established_at TIMESTAMPTZ NOT NULL DEFAULT now(), last_checked_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  PRIMARY KEY (tenant_id, audit_log_id)
);
CREATE INDEX IF NOT EXISTS idx_audit_log_integrity_baselines_checked ON audit_log_integrity_baselines (tenant_id, last_checked_at DESC);

CREATE TABLE IF NOT EXISTS audit_integrity_manifest_entries (
  check_id UUID NOT NULL REFERENCES audit_integrity_checks(check_id) ON DELETE RESTRICT,
  tenant_id TEXT NOT NULL, audit_log_id TEXT NOT NULL, root_sha256 TEXT NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  PRIMARY KEY (check_id, audit_log_id)
);
CREATE INDEX IF NOT EXISTS idx_audit_integrity_manifest_tenant_log ON audit_integrity_manifest_entries (tenant_id, audit_log_id);

-- SOAR playbook definitions, drill-only executions and tenant overrides.
-- High-risk response actions are persisted as definitions, but this schema's
-- executable workflow records simulations only until a real provider exists.
CREATE TABLE IF NOT EXISTS alert_playbook_executions (
  execution_id    TEXT PRIMARY KEY,
  tenant_id       TEXT NOT NULL,
  playbook_name   TEXT NOT NULL,
  alert_id        TEXT NOT NULL,
  success_actions INTEGER NOT NULL DEFAULT 0,
  failed_actions  INTEGER NOT NULL DEFAULT 0,
  duration_ms     BIGINT NOT NULL DEFAULT 0,
  request_payload JSONB NOT NULL DEFAULT '{}'::jsonb,
  result_payload  JSONB NOT NULL DEFAULT '{}'::jsonb,
  mode            TEXT NOT NULL DEFAULT 'legacy',
  status          TEXT NOT NULL DEFAULT 'succeeded',
  rollback_of     TEXT,
  effect_payload  JSONB NOT NULL DEFAULT '{}'::jsonb,
  requested_by    TEXT NOT NULL DEFAULT '',
  rolled_back_at  TIMESTAMPTZ,
  created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
  CONSTRAINT alert_playbook_execution_mode_check CHECK (mode IN ('legacy', 'drill')),
  CONSTRAINT alert_playbook_execution_status_check CHECK (status IN ('succeeded', 'failed', 'rolled_back', 'rollback_recorded'))
);
ALTER TABLE alert_playbook_executions ADD COLUMN IF NOT EXISTS mode TEXT NOT NULL DEFAULT 'legacy';
ALTER TABLE alert_playbook_executions ADD COLUMN IF NOT EXISTS status TEXT NOT NULL DEFAULT 'succeeded';
ALTER TABLE alert_playbook_executions ADD COLUMN IF NOT EXISTS rollback_of TEXT;
ALTER TABLE alert_playbook_executions ADD COLUMN IF NOT EXISTS effect_payload JSONB NOT NULL DEFAULT '{}'::jsonb;
ALTER TABLE alert_playbook_executions ADD COLUMN IF NOT EXISTS requested_by TEXT NOT NULL DEFAULT '';
ALTER TABLE alert_playbook_executions ADD COLUMN IF NOT EXISTS rolled_back_at TIMESTAMPTZ;
DO $do$ BEGIN ALTER TABLE alert_playbook_executions ADD CONSTRAINT alert_playbook_execution_mode_check CHECK (mode IN ('legacy', 'drill')); EXCEPTION WHEN duplicate_object THEN NULL; END $do$;
DO $do$ BEGIN ALTER TABLE alert_playbook_executions ADD CONSTRAINT alert_playbook_execution_status_check CHECK (status IN ('succeeded', 'failed', 'rolled_back', 'rollback_recorded')); EXCEPTION WHEN duplicate_object THEN NULL; END $do$;
CREATE UNIQUE INDEX IF NOT EXISTS idx_alert_playbook_executions_tenant_id ON alert_playbook_executions (tenant_id, execution_id);
DO $do$ BEGIN ALTER TABLE alert_playbook_executions ADD CONSTRAINT alert_playbook_execution_rollback_fk FOREIGN KEY (tenant_id, rollback_of) REFERENCES alert_playbook_executions (tenant_id, execution_id); EXCEPTION WHEN duplicate_object THEN NULL; END $do$;
CREATE INDEX IF NOT EXISTS idx_alert_playbook_executions_tenant_created
  ON alert_playbook_executions (tenant_id, created_at DESC);

CREATE TABLE IF NOT EXISTS alert_saved_views (view_id UUID PRIMARY KEY DEFAULT uuid_generate_v4(), tenant_id TEXT NOT NULL, name TEXT NOT NULL, filters JSONB NOT NULL DEFAULT '{}'::jsonb, created_by TEXT NOT NULL DEFAULT '', created_at TIMESTAMPTZ NOT NULL DEFAULT now(), updated_at TIMESTAMPTZ NOT NULL DEFAULT now(), UNIQUE(tenant_id,name));
CREATE TABLE IF NOT EXISTS alert_response_actions (job_id TEXT PRIMARY KEY, tenant_id TEXT NOT NULL, alert_id TEXT NOT NULL, action TEXT NOT NULL, target TEXT NOT NULL, reason TEXT NOT NULL, dry_run BOOLEAN NOT NULL DEFAULT true, status TEXT NOT NULL, detail JSONB NOT NULL DEFAULT '{}'::jsonb, requested_by TEXT NOT NULL DEFAULT '', created_at TIMESTAMPTZ NOT NULL DEFAULT now(), updated_at TIMESTAMPTZ NOT NULL DEFAULT now());
CREATE TABLE IF NOT EXISTS alert_response_outbox (outbox_id BIGSERIAL PRIMARY KEY, job_id TEXT NOT NULL REFERENCES alert_response_actions(job_id) ON DELETE CASCADE, tenant_id TEXT NOT NULL, event_type TEXT NOT NULL, payload JSONB NOT NULL, published BOOLEAN NOT NULL DEFAULT false, attempts INTEGER NOT NULL DEFAULT 0, last_error TEXT NOT NULL DEFAULT '', created_at TIMESTAMPTZ NOT NULL DEFAULT now(), published_at TIMESTAMPTZ);
CREATE INDEX IF NOT EXISTS idx_alert_response_outbox_pending ON alert_response_outbox (published, created_at) WHERE published=false;

CREATE TABLE IF NOT EXISTS alert_playbook_overrides (
  tenant_id        TEXT NOT NULL,
  name             TEXT NOT NULL,
  enabled          BOOLEAN NOT NULL DEFAULT true,
  max_runs         INTEGER NOT NULL DEFAULT 0,
  cooldown_seconds BIGINT NOT NULL DEFAULT 0,
  updated_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
  PRIMARY KEY (tenant_id, name)
);

CREATE TABLE IF NOT EXISTS alert_playbook_definitions (
  tenant_id          TEXT NOT NULL,
  name               TEXT NOT NULL,
  display_name       TEXT NOT NULL,
  description        TEXT NOT NULL DEFAULT '',
  version            INTEGER NOT NULL DEFAULT 1,
  stage              TEXT NOT NULL DEFAULT 'draft',
  enabled            BOOLEAN NOT NULL DEFAULT false,
  risk_level         TEXT NOT NULL DEFAULT 'medium',
  definition_payload JSONB NOT NULL,
  created_by         TEXT NOT NULL DEFAULT '',
  submitted_by       TEXT NOT NULL DEFAULT '',
  approved_by        TEXT NOT NULL DEFAULT '',
  rejection_reason   TEXT NOT NULL DEFAULT '',
  created_at         TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at         TIMESTAMPTZ NOT NULL DEFAULT now(),
  PRIMARY KEY (tenant_id, name),
  CONSTRAINT alert_playbook_definition_stage_check
    CHECK (stage IN ('draft', 'approval_pending', 'approved', 'rejected')),
  CONSTRAINT alert_playbook_definition_risk_check
    CHECK (risk_level IN ('low', 'medium', 'high', 'critical'))
);
CREATE INDEX IF NOT EXISTS idx_alert_playbook_definitions_tenant_stage
  ON alert_playbook_definitions (tenant_id, stage, updated_at DESC);

CREATE TABLE IF NOT EXISTS data_quality_actions (
  action_id       UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
  tenant_id       TEXT NOT NULL,
  view_name       TEXT NOT NULL,
  action_name     TEXT NOT NULL,
  target          TEXT NOT NULL,
  dry_run         BOOLEAN NOT NULL DEFAULT TRUE,
  status          TEXT NOT NULL DEFAULT 'dry_run',
  requested_by    TEXT NOT NULL DEFAULT '',
  request_payload JSONB NOT NULL DEFAULT '{}'::jsonb,
  created_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_data_quality_actions_tenant_created ON data_quality_actions (tenant_id, created_at DESC);

-- Explicitly activated canonical UI dataset for the eight data-quality views.
-- Default schema creation does not activate or seed any tenant.
CREATE TABLE IF NOT EXISTS data_quality_ui_fixtures (
  tenant_id       TEXT PRIMARY KEY,
  fixture_version TEXT NOT NULL,
  payload         JSONB NOT NULL,
  active          BOOLEAN NOT NULL DEFAULT false,
  updated_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_data_quality_ui_fixtures_active
  ON data_quality_ui_fixtures (tenant_id, active);
CREATE INDEX IF NOT EXISTS idx_tasks_tenant_time ON tasks (tenant_id, created_at DESC);

-- Explicitly activated, database-backed canonical UI fixture for encrypted traffic.
-- No row is installed by the default schema; live APIs remain the fallback.
CREATE TABLE IF NOT EXISTS encrypted_traffic_ui_fixtures (
  tenant_id      TEXT NOT NULL REFERENCES tenants(tenant_id) ON DELETE CASCADE,
  endpoint       TEXT NOT NULL CHECK (endpoint IN ('stats','sessions','ja3','tunnels','exfiltration','evidence')),
  fixture_version TEXT NOT NULL,
  payload        JSONB NOT NULL,
  active         BOOLEAN NOT NULL DEFAULT false,
  updated_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
  PRIMARY KEY (tenant_id, endpoint)
);
CREATE INDEX IF NOT EXISTS idx_encrypted_traffic_ui_fixtures_active
  ON encrypted_traffic_ui_fixtures (tenant_id, active, endpoint);

-- Explicitly activated, database-backed canonical UI fixture for the forensics workbench.
-- No rows are installed by the default schema; production data remains the fallback.
CREATE TABLE IF NOT EXISTS forensics_ui_fixtures (
  tenant_id       TEXT NOT NULL REFERENCES tenants(tenant_id) ON DELETE CASCADE,
  endpoint        TEXT NOT NULL CHECK (endpoint IN ('jobs','stats')),
  fixture_version TEXT NOT NULL,
  payload         JSONB NOT NULL,
  active          BOOLEAN NOT NULL DEFAULT false,
  updated_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
  PRIMARY KEY (tenant_id, endpoint)
);
CREATE INDEX IF NOT EXISTS idx_forensics_ui_fixtures_active
  ON forensics_ui_fixtures (tenant_id, active, endpoint);

CREATE TABLE IF NOT EXISTS campaign_action_jobs (
  job_id        TEXT PRIMARY KEY,
  tenant_id     TEXT NOT NULL,
  campaign_id   TEXT NOT NULL,
  action_id     TEXT NOT NULL,
  target        TEXT NOT NULL,
  metadata      JSONB NOT NULL DEFAULT '{}'::jsonb,
  simulation    BOOLEAN NOT NULL DEFAULT true,
  dry_run       BOOLEAN NOT NULL DEFAULT true,
  status        TEXT NOT NULL CHECK (status IN ('queued', 'running', 'completed', 'failed')),
  result        JSONB NOT NULL DEFAULT '{}'::jsonb,
  error_message TEXT NOT NULL DEFAULT '',
  created_by    TEXT NOT NULL DEFAULT '',
  created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
  completed_at  TIMESTAMPTZ
);
CREATE INDEX IF NOT EXISTS idx_campaign_action_jobs_tenant_time
  ON campaign_action_jobs (tenant_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_campaign_action_jobs_campaign_time
  ON campaign_action_jobs (tenant_id, campaign_id, created_at DESC);

-- Mutable SOC workbench state is intentionally kept in PostgreSQL.  The
-- ClickHouse campaigns table remains the immutable detection/aggregation
-- record while analysts can assign and advance a campaign without rewriting
-- analytical history.
CREATE TABLE IF NOT EXISTS campaign_workbench_state (
  tenant_id     TEXT NOT NULL,
  campaign_id   TEXT NOT NULL,
  assignee      TEXT NOT NULL DEFAULT '',
  status        TEXT NOT NULL DEFAULT 'active'
                CHECK (status IN ('active','investigating','contained','closed')),
  state_version BIGINT NOT NULL DEFAULT 1,
  updated_by    TEXT NOT NULL DEFAULT '',
  updated_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
  PRIMARY KEY (tenant_id, campaign_id)
);
CREATE INDEX IF NOT EXISTS idx_campaign_workbench_state_tenant_status
  ON campaign_workbench_state (tenant_id, status, updated_at DESC);

CREATE TABLE IF NOT EXISTS campaign_reports (
  report_id      TEXT PRIMARY KEY,
  tenant_id      TEXT NOT NULL,
  campaign_id    TEXT NOT NULL,
  format         TEXT NOT NULL DEFAULT 'pdf' CHECK (format IN ('pdf','word','json')),
  status         TEXT NOT NULL DEFAULT 'completed'
                 CHECK (status IN ('queued','running','completed','failed')),
  sections       JSONB NOT NULL DEFAULT '[]'::jsonb,
  evidence_count INTEGER NOT NULL DEFAULT 0,
  created_by     TEXT NOT NULL DEFAULT '',
  created_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
  completed_at   TIMESTAMPTZ
);
CREATE INDEX IF NOT EXISTS idx_campaign_reports_campaign_time
  ON campaign_reports (tenant_id, campaign_id, created_at DESC);

CREATE TABLE IF NOT EXISTS whitelist (
  id              UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
  tenant_id       TEXT NOT NULL,
  type            TEXT NOT NULL CHECK (type IN ('ip','domain','fingerprint','subnet','asset','account','rule','model')),
  value           TEXT NOT NULL,
  reason          TEXT NOT NULL DEFAULT '',
  description     TEXT NOT NULL DEFAULT '',
  status          TEXT NOT NULL DEFAULT 'draft',
  approval_status TEXT NOT NULL DEFAULT 'draft',
  source_alert_id TEXT NOT NULL DEFAULT '',
  feedback_id     TEXT NOT NULL DEFAULT '',
  owner_role      TEXT NOT NULL DEFAULT '',
  scope           TEXT NOT NULL DEFAULT '',
  risk_level      TEXT NOT NULL DEFAULT 'medium',
  covered_alerts  INTEGER NOT NULL DEFAULT 0,
  covered_assets  INTEGER NOT NULL DEFAULT 0,
  version         INTEGER NOT NULL DEFAULT 1,
  created_by      TEXT NOT NULL DEFAULT '',
  approved_by     TEXT NOT NULL DEFAULT '',
  approved_at     TIMESTAMPTZ,
  disabled_at     TIMESTAMPTZ,
  expires_at      TIMESTAMPTZ,
  created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE (tenant_id, type, value)
);

ALTER TABLE whitelist ADD COLUMN IF NOT EXISTS status TEXT NOT NULL DEFAULT 'draft';
ALTER TABLE whitelist ADD COLUMN IF NOT EXISTS approval_status TEXT NOT NULL DEFAULT 'draft';
ALTER TABLE whitelist ALTER COLUMN status SET DEFAULT 'draft';
ALTER TABLE whitelist ALTER COLUMN approval_status SET DEFAULT 'draft';
ALTER TABLE whitelist ADD COLUMN IF NOT EXISTS source_alert_id TEXT NOT NULL DEFAULT '';
ALTER TABLE whitelist ADD COLUMN IF NOT EXISTS feedback_id TEXT NOT NULL DEFAULT '';
ALTER TABLE whitelist ADD COLUMN IF NOT EXISTS owner_role TEXT NOT NULL DEFAULT '';
ALTER TABLE whitelist ADD COLUMN IF NOT EXISTS scope TEXT NOT NULL DEFAULT '';
ALTER TABLE whitelist ADD COLUMN IF NOT EXISTS risk_level TEXT NOT NULL DEFAULT 'medium';
ALTER TABLE whitelist ADD COLUMN IF NOT EXISTS covered_alerts INTEGER NOT NULL DEFAULT 0;
ALTER TABLE whitelist ADD COLUMN IF NOT EXISTS covered_assets INTEGER NOT NULL DEFAULT 0;
ALTER TABLE whitelist ADD COLUMN IF NOT EXISTS version INTEGER NOT NULL DEFAULT 1;
ALTER TABLE whitelist ADD COLUMN IF NOT EXISTS approved_by TEXT NOT NULL DEFAULT '';
ALTER TABLE whitelist ADD COLUMN IF NOT EXISTS approved_at TIMESTAMPTZ;
ALTER TABLE whitelist ADD COLUMN IF NOT EXISTS disabled_at TIMESTAMPTZ;
ALTER TABLE whitelist ADD COLUMN IF NOT EXISTS updated_at TIMESTAMPTZ NOT NULL DEFAULT now();
ALTER TABLE whitelist DROP CONSTRAINT IF EXISTS whitelist_type_check;
ALTER TABLE whitelist ADD CONSTRAINT whitelist_type_check CHECK (type IN ('ip','domain','fingerprint','subnet','asset','account','rule','model'));
ALTER TABLE whitelist DROP CONSTRAINT IF EXISTS whitelist_governance_state_check;
ALTER TABLE whitelist ADD CONSTRAINT whitelist_governance_state_check CHECK (
  (status='draft' AND approval_status='draft') OR
  (status='pending' AND approval_status='pending') OR
  (status='active' AND approval_status='approved') OR
  (status='disabled' AND approval_status IN ('approved','rejected'))
);
CREATE INDEX IF NOT EXISTS idx_whitelist_tenant ON whitelist (tenant_id);
CREATE INDEX IF NOT EXISTS idx_whitelist_entries_tenant_status ON whitelist (tenant_id, status, updated_at DESC);
CREATE INDEX IF NOT EXISTS idx_whitelist_entries_approval ON whitelist (tenant_id, approval_status, updated_at DESC);
CREATE INDEX IF NOT EXISTS idx_whitelist_source_alert ON whitelist (tenant_id, source_alert_id) WHERE source_alert_id <> '';
CREATE INDEX IF NOT EXISTS idx_whitelist_expires ON whitelist (expires_at) WHERE expires_at IS NOT NULL;

CREATE TABLE IF NOT EXISTS alert_notification_settings (
  tenant_id  TEXT PRIMARY KEY,
  settings   JSONB NOT NULL DEFAULT '{}'::jsonb,
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS notification_rules (
  rule_id    UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
  tenant_id  TEXT NOT NULL,
  name       TEXT NOT NULL,
  conditions JSONB NOT NULL DEFAULT '{}'::jsonb,
  channels   JSONB NOT NULL DEFAULT '[]'::jsonb,
  enabled    BOOLEAN NOT NULL DEFAULT true,
  created_by UUID,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE (tenant_id, name)
);
CREATE INDEX IF NOT EXISTS idx_notification_rules_tenant_enabled
  ON notification_rules (tenant_id, enabled, updated_at DESC);

CREATE TABLE IF NOT EXISTS notification_history (
  notification_id BIGSERIAL PRIMARY KEY,
  tenant_id       TEXT NOT NULL,
  rule_id         UUID REFERENCES notification_rules(rule_id) ON DELETE SET NULL,
  alert_id        TEXT NOT NULL DEFAULT '',
  target_name     TEXT NOT NULL DEFAULT '',
  channel         TEXT NOT NULL,
  alert_type      TEXT NOT NULL DEFAULT '',
  status          TEXT NOT NULL,
  error_message   TEXT,
  retry_count     INTEGER NOT NULL DEFAULT 0,
  trace_id        TEXT NOT NULL DEFAULT '',
  sent_at         TIMESTAMPTZ,
  created_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);
ALTER TABLE notification_history ADD COLUMN IF NOT EXISTS target_name TEXT NOT NULL DEFAULT '';
ALTER TABLE notification_history ADD COLUMN IF NOT EXISTS alert_type TEXT NOT NULL DEFAULT '';
ALTER TABLE notification_history ADD COLUMN IF NOT EXISTS retry_count INTEGER NOT NULL DEFAULT 0;
ALTER TABLE notification_history ADD COLUMN IF NOT EXISTS trace_id TEXT NOT NULL DEFAULT '';
CREATE INDEX IF NOT EXISTS idx_notification_history_tenant_created
  ON notification_history (tenant_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_notification_history_tenant_status
  ON notification_history (tenant_id, status, created_at DESC);

CREATE TABLE IF NOT EXISTS notification_escalation_policies (
  policy_id  UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
  tenant_id  TEXT NOT NULL,
  name       TEXT NOT NULL,
  stages     JSONB NOT NULL DEFAULT '[]'::jsonb,
  enabled    BOOLEAN NOT NULL DEFAULT true,
  created_by TEXT NOT NULL DEFAULT '',
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE (tenant_id, name)
);
CREATE INDEX IF NOT EXISTS idx_notification_escalation_tenant_enabled
  ON notification_escalation_policies (tenant_id, enabled, updated_at DESC);

CREATE TABLE IF NOT EXISTS notification_escalation_jobs (
  job_id BIGSERIAL PRIMARY KEY,
  tenant_id TEXT NOT NULL,
  alert_key TEXT NOT NULL,
  alert_id TEXT NOT NULL DEFAULT '',
  rule_id UUID NOT NULL REFERENCES notification_rules(rule_id) ON DELETE CASCADE,
  stage_index INTEGER NOT NULL,
  policy_id UUID,
  policy_updated_at TIMESTAMPTZ,
  stage_after_minutes DOUBLE PRECISION,
  stage_fingerprint TEXT NOT NULL DEFAULT '',
  target_role TEXT NOT NULL,
  channel TEXT NOT NULL,
  due_at TIMESTAMPTZ NOT NULL,
  alert_payload JSONB NOT NULL,
  status TEXT NOT NULL DEFAULT 'pending',
  attempts INTEGER NOT NULL DEFAULT 0,
  last_error TEXT,
  locked_at TIMESTAMPTZ,
  lock_token TEXT NOT NULL DEFAULT '',
  trace_id TEXT NOT NULL DEFAULT '',
  completed_at TIMESTAMPTZ,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE (tenant_id, alert_key, rule_id, stage_index, channel)
);
ALTER TABLE notification_escalation_jobs ADD COLUMN IF NOT EXISTS locked_at TIMESTAMPTZ;
ALTER TABLE notification_escalation_jobs ADD COLUMN IF NOT EXISTS lock_token TEXT NOT NULL DEFAULT '';
ALTER TABLE notification_escalation_jobs ADD COLUMN IF NOT EXISTS policy_id UUID;
ALTER TABLE notification_escalation_jobs ADD COLUMN IF NOT EXISTS policy_updated_at TIMESTAMPTZ;
ALTER TABLE notification_escalation_jobs ADD COLUMN IF NOT EXISTS stage_after_minutes DOUBLE PRECISION;
ALTER TABLE notification_escalation_jobs ADD COLUMN IF NOT EXISTS stage_fingerprint TEXT NOT NULL DEFAULT '';
CREATE INDEX IF NOT EXISTS idx_notification_escalation_jobs_due
  ON notification_escalation_jobs (status, due_at, job_id);

CREATE TABLE IF NOT EXISTS notification_templates (
  template_id      UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
  tenant_id        TEXT NOT NULL,
  template_type    TEXT NOT NULL,
  name             TEXT NOT NULL,
  version          INTEGER NOT NULL DEFAULT 1,
  subject          TEXT NOT NULL DEFAULT '',
  body             TEXT NOT NULL DEFAULT '',
  variable_schema  JSONB NOT NULL DEFAULT '{}'::jsonb,
  validation_status TEXT NOT NULL DEFAULT 'passed',
  enabled          BOOLEAN NOT NULL DEFAULT true,
  created_by       TEXT NOT NULL DEFAULT '',
  created_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE (tenant_id, name)
);
CREATE INDEX IF NOT EXISTS idx_notification_templates_tenant_enabled
  ON notification_templates (tenant_id, enabled, updated_at DESC);

CREATE TABLE IF NOT EXISTS notification_silence_rules (
  rule_id          TEXT PRIMARY KEY,
  tenant_id        TEXT NOT NULL,
  name             TEXT NOT NULL,
  scope            TEXT NOT NULL DEFAULT '',
  starts_at        TIMESTAMPTZ NOT NULL,
  ends_at          TIMESTAMPTZ NOT NULL,
  affected_targets JSONB NOT NULL DEFAULT '[]'::jsonb,
  policy           TEXT NOT NULL DEFAULT 'all',
  reason           TEXT NOT NULL DEFAULT '',
  enabled          BOOLEAN NOT NULL DEFAULT true,
  created_by       TEXT NOT NULL DEFAULT '',
  created_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at       TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_notification_silence_tenant_time
  ON notification_silence_rules (tenant_id, starts_at DESC);
CREATE INDEX IF NOT EXISTS idx_notification_silence_tenant_enabled
  ON notification_silence_rules (tenant_id, enabled, starts_at DESC);


CREATE OR REPLACE FUNCTION notification_governance_atomic_audit()
RETURNS TRIGGER AS $$
DECLARE
  row_data JSONB;
  tenant_value TEXT;
  object_value TEXT;
  action_prefix TEXT;
BEGIN
  row_data := CASE WHEN TG_OP = 'DELETE' THEN to_jsonb(OLD) ELSE to_jsonb(NEW) END;
  tenant_value := COALESCE(row_data->>'tenant_id', 'default');
  object_value := CASE
    WHEN TG_TABLE_NAME = 'notification_escalation_jobs' THEN COALESCE(row_data->>'job_id', tenant_value)
    ELSE COALESCE(row_data->>'rule_id', row_data->>'template_id', row_data->>'policy_id', row_data->>'notification_id', tenant_value)
  END;
  action_prefix := CASE TG_TABLE_NAME
    WHEN 'alert_notification_settings' THEN 'NOTIFICATION_SETTINGS'
    WHEN 'notification_rules' THEN 'NOTIFICATION_RULE'
    WHEN 'notification_templates' THEN 'NOTIFICATION_TEMPLATE'
    WHEN 'notification_escalation_policies' THEN 'NOTIFICATION_ESCALATION'
    WHEN 'notification_escalation_jobs' THEN 'NOTIFICATION_ESCALATION_JOB'
    WHEN 'notification_silence_rules' THEN 'NOTIFICATION_SILENCE_RULE'
    WHEN 'notification_history' THEN 'NOTIFICATION_DELIVERY'
    ELSE 'NOTIFICATION_GOVERNANCE'
  END;
  INSERT INTO audit_logs (event_id, tenant_id, user_id, action, object_type, object_id, detail)
  VALUES (
    'audit-' || uuid_generate_v4()::TEXT,
    tenant_value,
    NULL,
    action_prefix || '_DB_' || TG_OP,
    TG_TABLE_NAME,
    object_value,
    jsonb_build_object('atomic', true, 'operation', TG_OP)
  );
  RETURN CASE WHEN TG_OP = 'DELETE' THEN OLD ELSE NEW END;
END;
$$ LANGUAGE plpgsql;

DO $$
DECLARE
  table_name TEXT;
  trigger_name TEXT;
BEGIN
  FOREACH table_name IN ARRAY ARRAY[
    'alert_notification_settings',
    'notification_rules',
    'notification_templates',
    'notification_escalation_policies',
    'notification_escalation_jobs',
    'notification_silence_rules',
    'notification_history'
  ]
  LOOP
    trigger_name := 'trg_' || table_name || '_atomic_audit';
    EXECUTE format('DROP TRIGGER IF EXISTS %I ON %I', trigger_name, table_name);
    EXECUTE format(
      'CREATE TRIGGER %I AFTER INSERT OR UPDATE OR DELETE ON %I FOR EACH ROW EXECUTE FUNCTION notification_governance_atomic_audit()',
      trigger_name,
      table_name
    );
  END LOOP;
END $$;


CREATE TABLE IF NOT EXISTS topic_saved_views (
  view_id     UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
  tenant_id   TEXT NOT NULL,
  topic       TEXT NOT NULL,
  name        TEXT NOT NULL,
  filters     JSONB NOT NULL DEFAULT '{}'::jsonb,
  visibility  TEXT NOT NULL DEFAULT 'private',
  favorite    BOOLEAN NOT NULL DEFAULT false,
  shared      BOOLEAN NOT NULL DEFAULT false,
  share_token TEXT,
  created_by  TEXT NOT NULL DEFAULT '',
  created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_topic_saved_views_tenant_topic
  ON topic_saved_views (tenant_id, topic, updated_at DESC);

CREATE TABLE IF NOT EXISTS topic_scope_overrides (
  tenant_id       TEXT NOT NULL,
  topic           TEXT NOT NULL,
  scope_name      TEXT NOT NULL DEFAULT '',
  included_assets JSONB NOT NULL DEFAULT '[]'::jsonb,
  excluded_assets JSONB NOT NULL DEFAULT '[]'::jsonb,
  risk_levels     JSONB NOT NULL DEFAULT '[]'::jsonb,
  time_window     TEXT NOT NULL DEFAULT '24h',
  detail          JSONB NOT NULL DEFAULT '{}'::jsonb,
  updated_by      TEXT NOT NULL DEFAULT '',
  updated_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
  PRIMARY KEY (tenant_id, topic)
);

CREATE TABLE IF NOT EXISTS topic_subscriptions (
  subscription_id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
  tenant_id        TEXT NOT NULL,
  topic            TEXT NOT NULL,
  channel          TEXT NOT NULL,
  threshold        TEXT NOT NULL DEFAULT 'high',
  schedule         TEXT NOT NULL DEFAULT 'realtime',
  recipients       JSONB NOT NULL DEFAULT '[]'::jsonb,
  enabled          BOOLEAN NOT NULL DEFAULT true,
  created_by       TEXT NOT NULL DEFAULT '',
  created_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
  detail           JSONB NOT NULL DEFAULT '{}'::jsonb
);

CREATE INDEX IF NOT EXISTS idx_topic_subscriptions_tenant_topic
  ON topic_subscriptions (tenant_id, topic, updated_at DESC);

CREATE TABLE IF NOT EXISTS topic_exports (
  export_id    UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
  tenant_id    TEXT NOT NULL,
  topic        TEXT NOT NULL,
  export_type  TEXT NOT NULL,
  status       TEXT NOT NULL DEFAULT 'completed',
  parameters   JSONB NOT NULL DEFAULT '{}'::jsonb,
  result       JSONB NOT NULL DEFAULT '{}'::jsonb,
  generated_by TEXT NOT NULL DEFAULT '',
  generated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_topic_exports_tenant_time
  ON topic_exports (tenant_id, generated_at DESC);

COMMIT;
