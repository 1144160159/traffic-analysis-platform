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
  user_id     TEXT,
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

-- F-PLAYBOOK-001 additive live execution workflow. Legacy/drill rows remain
-- valid; no historical row is promoted to an external effect.
ALTER TABLE alert_playbook_executions DROP CONSTRAINT IF EXISTS alert_playbook_execution_mode_check;
ALTER TABLE alert_playbook_executions DROP CONSTRAINT IF EXISTS alert_playbook_execution_status_check;
ALTER TABLE alert_playbook_executions
  ADD COLUMN IF NOT EXISTS playbook_version INTEGER NOT NULL DEFAULT 0,
  ADD COLUMN IF NOT EXISTS workflow_revision BIGINT NOT NULL DEFAULT 1,
  ADD COLUMN IF NOT EXISTS approval_status TEXT NOT NULL DEFAULT 'not_required',
  ADD COLUMN IF NOT EXISTS executor_status TEXT NOT NULL DEFAULT 'simulated',
  ADD COLUMN IF NOT EXISTS idempotency_key TEXT NOT NULL DEFAULT '',
  ADD COLUMN IF NOT EXISTS request_sha256 TEXT NOT NULL DEFAULT '',
  ADD COLUMN IF NOT EXISTS reason TEXT NOT NULL DEFAULT '',
  ADD COLUMN IF NOT EXISTS trace_id TEXT NOT NULL DEFAULT '',
  ADD COLUMN IF NOT EXISTS approved_by TEXT NOT NULL DEFAULT '',
  ADD COLUMN IF NOT EXISTS approved_at TIMESTAMPTZ,
  ADD COLUMN IF NOT EXISTS execution_receipt JSONB NOT NULL DEFAULT '{}'::jsonb,
  ADD COLUMN IF NOT EXISTS compensation_receipt JSONB NOT NULL DEFAULT '{}'::jsonb,
  ADD COLUMN IF NOT EXISTS attempts INTEGER NOT NULL DEFAULT 0,
  ADD COLUMN IF NOT EXISTS next_attempt_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  ADD COLUMN IF NOT EXISTS locked_until TIMESTAMPTZ,
  ADD COLUMN IF NOT EXISTS locked_by TEXT NOT NULL DEFAULT '',
  ADD COLUMN IF NOT EXISTS updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  ADD COLUMN IF NOT EXISTS completed_at TIMESTAMPTZ;
ALTER TABLE alert_playbook_executions ADD CONSTRAINT alert_playbook_execution_mode_check
  CHECK (mode IN ('legacy','drill','live'));
ALTER TABLE alert_playbook_executions ADD CONSTRAINT alert_playbook_execution_status_check CHECK (status IN (
  'succeeded','rolled_back','rollback_recorded','pending_approval','approved_awaiting_executor','running',
  'completed','partial','failed','cancelled','compensation_queued','compensating','compensated','compensation_failed'
));
ALTER TABLE alert_playbook_executions DROP CONSTRAINT IF EXISTS alert_playbook_execution_workflow_revision_check;
ALTER TABLE alert_playbook_executions ADD CONSTRAINT alert_playbook_execution_workflow_revision_check CHECK (workflow_revision>0);
ALTER TABLE alert_playbook_executions DROP CONSTRAINT IF EXISTS alert_playbook_execution_approval_status_check;
ALTER TABLE alert_playbook_executions ADD CONSTRAINT alert_playbook_execution_approval_status_check
  CHECK (approval_status IN ('not_required','pending','approved','rejected','cancelled'));
ALTER TABLE alert_playbook_executions DROP CONSTRAINT IF EXISTS alert_playbook_execution_executor_status_check;
ALTER TABLE alert_playbook_executions ADD CONSTRAINT alert_playbook_execution_executor_status_check CHECK (executor_status IN (
  'simulated','not_dispatched','not_configured','queued','running','succeeded','partial','failed','cancelled',
  'compensating','compensated','compensation_failed'
));
ALTER TABLE alert_playbook_executions DROP CONSTRAINT IF EXISTS alert_playbook_execution_attempts_check;
ALTER TABLE alert_playbook_executions ADD CONSTRAINT alert_playbook_execution_attempts_check CHECK (attempts>=0);
CREATE UNIQUE INDEX IF NOT EXISTS uq_alert_playbook_execution_idempotency
  ON alert_playbook_executions(tenant_id,idempotency_key) WHERE idempotency_key<>'';
CREATE INDEX IF NOT EXISTS idx_alert_playbook_execution_dispatch
  ON alert_playbook_executions(next_attempt_at,created_at,execution_id)
  WHERE mode='live' AND status IN ('approved_awaiting_executor','compensation_queued');

CREATE TABLE IF NOT EXISTS alert_playbook_execution_approvals (
  approval_id UUID PRIMARY KEY, execution_id TEXT NOT NULL, tenant_id TEXT NOT NULL, playbook_name TEXT NOT NULL,
  decision TEXT NOT NULL CHECK (decision IN ('approve','reject')), expected_revision BIGINT NOT NULL CHECK (expected_revision>0),
  idempotency_key TEXT NOT NULL CHECK (length(idempotency_key) BETWEEN 16 AND 200),
  reason TEXT NOT NULL CHECK (length(reason) BETWEEN 8 AND 1000), decided_by TEXT NOT NULL CHECK (decided_by<>''),
  resulting_revision BIGINT NOT NULL CHECK (resulting_revision>0), resulting_status TEXT NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(), UNIQUE (tenant_id,idempotency_key),
  FOREIGN KEY (tenant_id,execution_id) REFERENCES alert_playbook_executions(tenant_id,execution_id) ON DELETE RESTRICT
);
CREATE INDEX IF NOT EXISTS idx_alert_playbook_execution_approvals_job
  ON alert_playbook_execution_approvals(tenant_id,execution_id,created_at DESC);

CREATE TABLE IF NOT EXISTS alert_playbook_execution_controls (
  request_id UUID PRIMARY KEY, execution_id TEXT NOT NULL, tenant_id TEXT NOT NULL, playbook_name TEXT NOT NULL,
  operation TEXT NOT NULL CHECK (operation IN ('cancel','compensate')), expected_revision BIGINT NOT NULL CHECK (expected_revision>0),
  idempotency_key TEXT NOT NULL CHECK (length(idempotency_key) BETWEEN 16 AND 200),
  reason TEXT NOT NULL CHECK (length(reason) BETWEEN 8 AND 1000), requested_by TEXT NOT NULL CHECK (requested_by<>''),
  resulting_revision BIGINT NOT NULL CHECK (resulting_revision>0), resulting_status TEXT NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(), UNIQUE (tenant_id,idempotency_key),
  FOREIGN KEY (tenant_id,execution_id) REFERENCES alert_playbook_executions(tenant_id,execution_id) ON DELETE RESTRICT
);
CREATE INDEX IF NOT EXISTS idx_alert_playbook_execution_controls_job
  ON alert_playbook_execution_controls(tenant_id,execution_id,created_at DESC);

CREATE TABLE IF NOT EXISTS alert_playbook_step_receipts (
  receipt_id UUID PRIMARY KEY, execution_id TEXT NOT NULL, tenant_id TEXT NOT NULL, playbook_name TEXT NOT NULL,
  phase TEXT NOT NULL CHECK (phase IN ('execute','compensate')), attempt INTEGER NOT NULL CHECK (attempt>0),
  step_index INTEGER NOT NULL CHECK (step_index>=0), action_type TEXT NOT NULL, provider TEXT NOT NULL,
  provider_receipt_id TEXT NOT NULL, status TEXT NOT NULL CHECK (status IN ('succeeded','partial','failed')),
  external_effect BOOLEAN NOT NULL DEFAULT false, payload JSONB NOT NULL, payload_sha256 TEXT NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(), UNIQUE (execution_id,phase,attempt,step_index),
  UNIQUE (tenant_id,provider,provider_receipt_id,phase),
  FOREIGN KEY (tenant_id,execution_id) REFERENCES alert_playbook_executions(tenant_id,execution_id) ON DELETE RESTRICT
);
CREATE INDEX IF NOT EXISTS idx_alert_playbook_step_receipts_job
  ON alert_playbook_step_receipts(tenant_id,execution_id,created_at DESC);

CREATE TABLE IF NOT EXISTS alert_playbook_execution_outbox (
  outbox_id BIGSERIAL PRIMARY KEY, event_id UUID NOT NULL UNIQUE, execution_id TEXT NOT NULL, tenant_id TEXT NOT NULL,
  playbook_name TEXT NOT NULL, event_type TEXT NOT NULL, schema_version INTEGER NOT NULL DEFAULT 2,
  aggregate_version BIGINT NOT NULL CHECK (aggregate_version>0), partition_key TEXT NOT NULL, payload JSONB NOT NULL,
  published BOOLEAN NOT NULL DEFAULT false, attempts INTEGER NOT NULL DEFAULT 0 CHECK (attempts>=0),
  next_attempt_at TIMESTAMPTZ NOT NULL DEFAULT now(), locked_until TIMESTAMPTZ, locked_by TEXT NOT NULL DEFAULT '',
  last_error TEXT NOT NULL DEFAULT '', created_at TIMESTAMPTZ NOT NULL DEFAULT now(), published_at TIMESTAMPTZ,
  FOREIGN KEY (tenant_id,execution_id) REFERENCES alert_playbook_executions(tenant_id,execution_id) ON DELETE RESTRICT
);
CREATE INDEX IF NOT EXISTS idx_alert_playbook_execution_outbox_dispatch
  ON alert_playbook_execution_outbox(next_attempt_at,outbox_id) WHERE published=false;
ALTER TABLE alert_playbook_execution_outbox ADD COLUMN IF NOT EXISTS status TEXT NOT NULL DEFAULT 'pending';
ALTER TABLE alert_playbook_execution_outbox ADD COLUMN IF NOT EXISTS last_attempt_at TIMESTAMPTZ;
ALTER TABLE alert_playbook_execution_outbox ADD COLUMN IF NOT EXISTS dead_at TIMESTAMPTZ;
UPDATE alert_playbook_execution_outbox SET status=CASE WHEN published THEN 'published' ELSE 'pending' END WHERE status NOT IN ('pending','processing','published','dead') OR (published AND status<>'published');
ALTER TABLE alert_playbook_execution_outbox DROP CONSTRAINT IF EXISTS alert_playbook_execution_outbox_status_check;
ALTER TABLE alert_playbook_execution_outbox ADD CONSTRAINT alert_playbook_execution_outbox_status_check CHECK(status IN ('pending','processing','published','dead'));
CREATE INDEX IF NOT EXISTS idx_alert_playbook_execution_outbox_ready ON alert_playbook_execution_outbox(next_attempt_at,created_at,outbox_id) WHERE published=false AND status IN ('pending','processing');
CREATE TABLE IF NOT EXISTS alert_playbook_execution_event_projection (
  event_id UUID PRIMARY KEY, tenant_id TEXT NOT NULL, execution_id TEXT NOT NULL, playbook_name TEXT NOT NULL,
  event_type TEXT NOT NULL, schema_version INTEGER NOT NULL CHECK(schema_version=2), aggregate_version BIGINT NOT NULL CHECK(aggregate_version>0),
  partition_key TEXT NOT NULL CHECK(partition_key<>''), trace_id TEXT NOT NULL CHECK(trace_id<>''), payload JSONB NOT NULL,
  payload_sha256 TEXT NOT NULL CHECK(length(payload_sha256)=64), kafka_topic TEXT NOT NULL CHECK(kafka_topic<>''),
  kafka_partition INTEGER NOT NULL CHECK(kafka_partition>=0), kafka_offset BIGINT NOT NULL CHECK(kafka_offset>=0),
  projected_at TIMESTAMPTZ NOT NULL DEFAULT now(), UNIQUE(kafka_topic,kafka_partition,kafka_offset), UNIQUE(tenant_id,execution_id,aggregate_version)
);
CREATE INDEX IF NOT EXISTS idx_playbook_event_projection_execution ON alert_playbook_execution_event_projection(tenant_id,execution_id,aggregate_version);
CREATE TABLE IF NOT EXISTS alert_playbook_execution_state_projection (
  tenant_id TEXT NOT NULL, execution_id TEXT NOT NULL, playbook_name TEXT NOT NULL, playbook_version INTEGER NOT NULL DEFAULT 0 CHECK(playbook_version>=0),
  alert_id TEXT NOT NULL DEFAULT '', status TEXT NOT NULL, approval_status TEXT NOT NULL DEFAULT '', executor_status TEXT NOT NULL DEFAULT '',
  aggregate_version BIGINT NOT NULL CHECK(aggregate_version>0), event_type TEXT NOT NULL, trace_id TEXT NOT NULL CHECK(trace_id<>''),
  last_event_id UUID NOT NULL, payload JSONB NOT NULL, payload_sha256 TEXT NOT NULL CHECK(length(payload_sha256)=64),
  kafka_topic TEXT NOT NULL CHECK(kafka_topic<>''), kafka_partition INTEGER NOT NULL CHECK(kafka_partition>=0),
  kafka_offset BIGINT NOT NULL CHECK(kafka_offset>=0), updated_at TIMESTAMPTZ NOT NULL DEFAULT now(), PRIMARY KEY(tenant_id,execution_id)
);
CREATE INDEX IF NOT EXISTS idx_playbook_state_projection_status ON alert_playbook_execution_state_projection(tenant_id,status,updated_at DESC);

CREATE TABLE IF NOT EXISTS alert_saved_views (view_id UUID PRIMARY KEY DEFAULT uuid_generate_v4(), tenant_id TEXT NOT NULL, name TEXT NOT NULL, filters JSONB NOT NULL DEFAULT '{}'::jsonb, created_by TEXT NOT NULL DEFAULT '', created_at TIMESTAMPTZ NOT NULL DEFAULT now(), updated_at TIMESTAMPTZ NOT NULL DEFAULT now(), UNIQUE(tenant_id,name));
CREATE TABLE IF NOT EXISTS alert_response_actions (job_id TEXT PRIMARY KEY, tenant_id TEXT NOT NULL, alert_id TEXT NOT NULL, action TEXT NOT NULL, target TEXT NOT NULL, reason TEXT NOT NULL, dry_run BOOLEAN NOT NULL DEFAULT true, status TEXT NOT NULL, detail JSONB NOT NULL DEFAULT '{}'::jsonb, requested_by TEXT NOT NULL DEFAULT '', created_at TIMESTAMPTZ NOT NULL DEFAULT now(), updated_at TIMESTAMPTZ NOT NULL DEFAULT now());
CREATE TABLE IF NOT EXISTS alert_response_outbox (outbox_id BIGSERIAL PRIMARY KEY, job_id TEXT NOT NULL REFERENCES alert_response_actions(job_id) ON DELETE CASCADE, tenant_id TEXT NOT NULL, event_type TEXT NOT NULL, payload JSONB NOT NULL, published BOOLEAN NOT NULL DEFAULT false, attempts INTEGER NOT NULL DEFAULT 0, last_error TEXT NOT NULL DEFAULT '', next_attempt_at TIMESTAMPTZ NOT NULL DEFAULT now(), locked_until TIMESTAMPTZ, locked_by TEXT NOT NULL DEFAULT '', created_at TIMESTAMPTZ NOT NULL DEFAULT now(), published_at TIMESTAMPTZ);
CREATE INDEX IF NOT EXISTS idx_alert_response_outbox_retry ON alert_response_outbox (next_attempt_at, outbox_id) WHERE published=false;
ALTER TABLE alert_response_actions ADD COLUMN IF NOT EXISTS event_id UUID;
ALTER TABLE alert_response_actions ADD COLUMN IF NOT EXISTS action_id TEXT NOT NULL DEFAULT '';
ALTER TABLE alert_response_actions ADD COLUMN IF NOT EXISTS revision BIGINT NOT NULL DEFAULT 1;
ALTER TABLE alert_response_actions ADD COLUMN IF NOT EXISTS trace_id TEXT NOT NULL DEFAULT '';
ALTER TABLE alert_response_actions ADD COLUMN IF NOT EXISTS idempotency_key TEXT NOT NULL DEFAULT '';
ALTER TABLE alert_response_actions ADD COLUMN IF NOT EXISTS approval_status TEXT NOT NULL DEFAULT 'not_required';
ALTER TABLE alert_response_actions ADD COLUMN IF NOT EXISTS approved_by TEXT NOT NULL DEFAULT '';
ALTER TABLE alert_response_actions ADD COLUMN IF NOT EXISTS approved_at TIMESTAMPTZ;
ALTER TABLE alert_response_actions ADD COLUMN IF NOT EXISTS result JSONB NOT NULL DEFAULT '{}'::jsonb;
ALTER TABLE alert_response_actions ADD COLUMN IF NOT EXISTS error TEXT NOT NULL DEFAULT '';
ALTER TABLE alert_response_outbox ADD COLUMN IF NOT EXISTS event_id UUID;
ALTER TABLE alert_response_outbox ADD COLUMN IF NOT EXISTS schema_version INTEGER NOT NULL DEFAULT 1;
ALTER TABLE alert_response_outbox ADD COLUMN IF NOT EXISTS aggregate_version BIGINT NOT NULL DEFAULT 1;
ALTER TABLE alert_response_outbox ADD COLUMN IF NOT EXISTS partition_key TEXT NOT NULL DEFAULT '';
CREATE TABLE IF NOT EXISTS alert_response_execution_receipts (event_id UUID PRIMARY KEY, job_id TEXT NOT NULL UNIQUE REFERENCES alert_response_actions(job_id) ON DELETE RESTRICT, tenant_id TEXT NOT NULL, alert_id TEXT NOT NULL, action_id TEXT NOT NULL, state TEXT NOT NULL CHECK (state IN ('simulated_completed','blocked_external_executor','failed')), simulated BOOLEAN NOT NULL, external_effect BOOLEAN NOT NULL DEFAULT false, result JSONB NOT NULL DEFAULT '{}'::jsonb, error TEXT NOT NULL DEFAULT '', kafka_partition INTEGER NOT NULL, kafka_offset BIGINT NOT NULL, created_at TIMESTAMPTZ NOT NULL DEFAULT now(), UNIQUE (kafka_partition,kafka_offset));
ALTER TABLE alert_response_actions ADD COLUMN IF NOT EXISTS expected_revision BIGINT NOT NULL DEFAULT 0;
ALTER TABLE alert_response_outbox ADD COLUMN IF NOT EXISTS cancelled_at TIMESTAMPTZ;
ALTER TABLE alert_response_execution_receipts ADD COLUMN IF NOT EXISTS aggregate_version BIGINT NOT NULL DEFAULT 1;
CREATE INDEX IF NOT EXISTS idx_alert_response_outbox_delivery ON alert_response_outbox(next_attempt_at,outbox_id) WHERE published=false AND cancelled_at IS NULL;
CREATE TABLE IF NOT EXISTS alert_response_approvals (approval_id UUID PRIMARY KEY, job_id TEXT NOT NULL REFERENCES alert_response_actions(job_id) ON DELETE RESTRICT, tenant_id TEXT NOT NULL, alert_id TEXT NOT NULL, decision TEXT NOT NULL CHECK (decision IN ('approve','reject')), expected_revision BIGINT NOT NULL CHECK (expected_revision>0), idempotency_key TEXT NOT NULL CHECK (length(idempotency_key) BETWEEN 16 AND 200), reason TEXT NOT NULL CHECK (length(reason) BETWEEN 8 AND 1000), decided_by TEXT NOT NULL CHECK (decided_by<>''), resulting_revision BIGINT NOT NULL, resulting_status TEXT NOT NULL, approval_status TEXT NOT NULL CHECK (approval_status IN ('approved','rejected')), created_at TIMESTAMPTZ NOT NULL DEFAULT now(), UNIQUE (tenant_id,idempotency_key));
CREATE INDEX IF NOT EXISTS idx_alert_response_approvals_job ON alert_response_approvals(tenant_id,job_id,created_at);
CREATE TABLE IF NOT EXISTS alert_response_control_requests (request_id UUID PRIMARY KEY, job_id TEXT NOT NULL REFERENCES alert_response_actions(job_id) ON DELETE RESTRICT, tenant_id TEXT NOT NULL, alert_id TEXT NOT NULL, operation TEXT NOT NULL CHECK (operation IN ('cancel','compensate')), expected_revision BIGINT NOT NULL CHECK (expected_revision>0), idempotency_key TEXT NOT NULL CHECK (length(idempotency_key) BETWEEN 16 AND 200), reason TEXT NOT NULL CHECK (length(reason) BETWEEN 8 AND 1000), requested_by TEXT NOT NULL CHECK (requested_by<>''), state TEXT NOT NULL CHECK (state IN ('cancelled','blocked_external_executor')), resulting_revision BIGINT NOT NULL, resulting_status TEXT NOT NULL, created_at TIMESTAMPTZ NOT NULL DEFAULT now(), UNIQUE (tenant_id,idempotency_key));
CREATE INDEX IF NOT EXISTS idx_alert_response_control_job ON alert_response_control_requests(tenant_id,job_id,created_at);
CREATE INDEX IF NOT EXISTS idx_alert_response_receipts_tenant_job ON alert_response_execution_receipts(tenant_id,job_id,created_at);
-- BEGIN GENERATED F-ALERT-003 EXTERNAL EXECUTOR
ALTER TABLE alert_response_execution_receipts
  ADD COLUMN IF NOT EXISTS provider TEXT NOT NULL DEFAULT '',
  ADD COLUMN IF NOT EXISTS provider_receipt_id TEXT NOT NULL DEFAULT '',
  ADD COLUMN IF NOT EXISTS effect_state TEXT NOT NULL DEFAULT 'none',
  ADD COLUMN IF NOT EXISTS effect_ids JSONB NOT NULL DEFAULT '[]'::jsonb,
  ADD COLUMN IF NOT EXISTS trace_id TEXT NOT NULL DEFAULT '',
  ADD COLUMN IF NOT EXISTS receipt_sha256 TEXT NOT NULL DEFAULT repeat('0',64),
  ADD COLUMN IF NOT EXISTS authority_lookup JSONB NOT NULL DEFAULT '{}'::jsonb,
  ADD COLUMN IF NOT EXISTS executed_at TIMESTAMPTZ NOT NULL DEFAULT now();
ALTER TABLE alert_response_execution_receipts
  DROP CONSTRAINT IF EXISTS alert_response_execution_receipts_state_check,
  DROP CONSTRAINT IF EXISTS alert_response_execution_receipts_effect_state_check,
  DROP CONSTRAINT IF EXISTS alert_response_execution_receipts_effect_ids_check,
  DROP CONSTRAINT IF EXISTS alert_response_execution_receipts_receipt_sha256_check,
  DROP CONSTRAINT IF EXISTS alert_response_execution_receipts_completed_effect_check,
  DROP CONSTRAINT IF EXISTS alert_response_execution_receipts_failed_effect_check;
ALTER TABLE alert_response_execution_receipts
  ADD CONSTRAINT alert_response_execution_receipts_state_check CHECK (state IN ('simulated_completed','blocked_external_executor','completed','partial','failed')),
  ADD CONSTRAINT alert_response_execution_receipts_effect_state_check CHECK (effect_state IN ('confirmed','none','unknown')),
  ADD CONSTRAINT alert_response_execution_receipts_effect_ids_check CHECK (jsonb_typeof(effect_ids)='array'),
  ADD CONSTRAINT alert_response_execution_receipts_receipt_sha256_check CHECK (receipt_sha256 ~ '^[0-9a-f]{64}$'),
  ADD CONSTRAINT alert_response_execution_receipts_completed_effect_check CHECK (state<>'completed' OR (external_effect AND effect_state='confirmed' AND jsonb_array_length(effect_ids)>0)),
  ADD CONSTRAINT alert_response_execution_receipts_failed_effect_check CHECK (state<>'failed' OR (NOT external_effect AND effect_state='none'));
CREATE UNIQUE INDEX IF NOT EXISTS uq_alert_response_provider_receipt ON alert_response_execution_receipts(provider,provider_receipt_id) WHERE provider<>'' AND provider_receipt_id<>'';
CREATE INDEX IF NOT EXISTS idx_alert_response_receipts_trace ON alert_response_execution_receipts(tenant_id,trace_id,executed_at DESC) WHERE trace_id<>'';
INSERT INTO alignment_schema_migrations(version,description) VALUES ('202608091130','provider authoritative alert response execution receipts and audit') ON CONFLICT (version) DO NOTHING;
-- END GENERATED F-ALERT-003 EXTERNAL EXECUTOR

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
CREATE INDEX IF NOT EXISTS idx_tasks_status ON tasks(status) WHERE status IN ('queued','processing');
CREATE INDEX IF NOT EXISTS idx_tasks_type ON tasks(task_type);
CREATE INDEX IF NOT EXISTS idx_tasks_run_id ON tasks(run_id) WHERE run_id IS NOT NULL;

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
ALTER TABLE campaign_action_jobs ADD COLUMN IF NOT EXISTS idempotency_key TEXT NOT NULL DEFAULT '';
ALTER TABLE campaign_action_jobs ADD COLUMN IF NOT EXISTS request_sha256 TEXT NOT NULL DEFAULT '';
ALTER TABLE campaign_action_jobs ADD COLUMN IF NOT EXISTS expected_revision BIGINT NOT NULL DEFAULT 0;
ALTER TABLE campaign_action_jobs ADD COLUMN IF NOT EXISTS resource_revision BIGINT NOT NULL DEFAULT 0;
ALTER TABLE campaign_action_jobs ADD COLUMN IF NOT EXISTS reason TEXT NOT NULL DEFAULT '';
ALTER TABLE campaign_action_jobs DROP CONSTRAINT IF EXISTS campaign_action_jobs_status_check;
ALTER TABLE campaign_action_jobs ADD CONSTRAINT campaign_action_jobs_status_check
  CHECK (status IN ('queued','accepted','pending_approval','approved_awaiting_executor','running','completed','succeeded','partial','failed','cancelled','compensation_queued','compensating','compensated','compensation_failed'));
CREATE UNIQUE INDEX IF NOT EXISTS uq_campaign_action_jobs_tenant_idempotency
  ON campaign_action_jobs (tenant_id, idempotency_key) WHERE idempotency_key<>'';
CREATE UNIQUE INDEX IF NOT EXISTS uq_campaign_action_jobs_tenant_job
  ON campaign_action_jobs (tenant_id,job_id);

CREATE TABLE IF NOT EXISTS campaign_soar_jobs (
  job_id TEXT PRIMARY KEY REFERENCES campaign_action_jobs(job_id) ON DELETE RESTRICT,
  tenant_id TEXT NOT NULL, campaign_id TEXT NOT NULL, playbook_id TEXT NOT NULL, target TEXT NOT NULL,
  source_snapshot_id TEXT NOT NULL, campaign_revision BIGINT NOT NULL CHECK (campaign_revision>0),
  status TEXT NOT NULL CHECK (status IN ('pending_approval','approved_awaiting_executor','running','completed','partial','failed','cancelled','compensation_queued','compensating','compensated','compensation_failed')),
  approval_status TEXT NOT NULL CHECK (approval_status IN ('pending','approved','rejected','cancelled')),
  executor_status TEXT NOT NULL CHECK (executor_status IN ('not_dispatched','not_configured','queued','running','succeeded','partial','failed','cancelled','compensating','compensated','compensation_failed')),
  revision BIGINT NOT NULL DEFAULT 1 CHECK (revision>0), request JSONB NOT NULL DEFAULT '{}'::jsonb,
  execution_receipt JSONB NOT NULL DEFAULT '{}'::jsonb, compensation_receipt JSONB NOT NULL DEFAULT '{}'::jsonb,
  error_message TEXT NOT NULL DEFAULT '', attempts INTEGER NOT NULL DEFAULT 0 CHECK (attempts>=0),
  next_attempt_at TIMESTAMPTZ NOT NULL DEFAULT now(), locked_until TIMESTAMPTZ, locked_by TEXT NOT NULL DEFAULT '',
  requested_by TEXT NOT NULL, approved_by TEXT NOT NULL DEFAULT '', approved_at TIMESTAMPTZ,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(), updated_at TIMESTAMPTZ NOT NULL DEFAULT now(), completed_at TIMESTAMPTZ,
  UNIQUE (tenant_id,job_id), FOREIGN KEY (tenant_id,job_id) REFERENCES campaign_action_jobs(tenant_id,job_id) ON DELETE RESTRICT
);
CREATE INDEX IF NOT EXISTS idx_campaign_soar_jobs_campaign ON campaign_soar_jobs(tenant_id,campaign_id,created_at DESC);
CREATE INDEX IF NOT EXISTS idx_campaign_soar_jobs_dispatch ON campaign_soar_jobs(next_attempt_at,created_at) WHERE status IN ('approved_awaiting_executor','compensation_queued');
CREATE TABLE IF NOT EXISTS campaign_soar_approvals (
  approval_id UUID PRIMARY KEY, job_id TEXT NOT NULL REFERENCES campaign_soar_jobs(job_id) ON DELETE RESTRICT,
  tenant_id TEXT NOT NULL, campaign_id TEXT NOT NULL, decision TEXT NOT NULL CHECK (decision IN ('approve','reject')),
  expected_revision BIGINT NOT NULL CHECK (expected_revision>0), idempotency_key TEXT NOT NULL CHECK (length(idempotency_key) BETWEEN 16 AND 200),
  reason TEXT NOT NULL CHECK (length(reason) BETWEEN 8 AND 1000), decided_by TEXT NOT NULL CHECK (decided_by<>''),
  resulting_revision BIGINT NOT NULL CHECK (resulting_revision>0), resulting_status TEXT NOT NULL,
  approval_status TEXT NOT NULL, created_at TIMESTAMPTZ NOT NULL DEFAULT now(), UNIQUE (tenant_id,idempotency_key)
);
CREATE INDEX IF NOT EXISTS idx_campaign_soar_approvals_job ON campaign_soar_approvals(tenant_id,job_id,created_at DESC);
CREATE TABLE IF NOT EXISTS campaign_soar_execution_receipts (
  receipt_id UUID PRIMARY KEY, job_id TEXT NOT NULL REFERENCES campaign_soar_jobs(job_id) ON DELETE RESTRICT,
  tenant_id TEXT NOT NULL, campaign_id TEXT NOT NULL, phase TEXT NOT NULL CHECK (phase IN ('execute','compensate')),
  attempt INTEGER NOT NULL CHECK (attempt>0), provider TEXT NOT NULL, provider_receipt_id TEXT NOT NULL,
  status TEXT NOT NULL CHECK (status IN ('succeeded','partial','failed')), external_effect BOOLEAN NOT NULL DEFAULT false,
  payload JSONB NOT NULL, payload_sha256 TEXT NOT NULL, created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE (job_id,phase,attempt), UNIQUE (tenant_id,provider,provider_receipt_id,phase)
);
CREATE INDEX IF NOT EXISTS idx_campaign_soar_receipts_job ON campaign_soar_execution_receipts(tenant_id,job_id,created_at DESC);
CREATE TABLE IF NOT EXISTS campaign_soar_control_requests (
  request_id UUID PRIMARY KEY, job_id TEXT NOT NULL REFERENCES campaign_soar_jobs(job_id) ON DELETE RESTRICT,
  tenant_id TEXT NOT NULL, campaign_id TEXT NOT NULL, operation TEXT NOT NULL CHECK (operation IN ('cancel','compensate')),
  expected_revision BIGINT NOT NULL CHECK (expected_revision>0), idempotency_key TEXT NOT NULL CHECK (length(idempotency_key) BETWEEN 16 AND 200),
  reason TEXT NOT NULL CHECK (length(reason) BETWEEN 8 AND 1000), requested_by TEXT NOT NULL CHECK (requested_by<>''),
  resulting_revision BIGINT NOT NULL CHECK (resulting_revision>0), resulting_status TEXT NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(), UNIQUE (tenant_id,idempotency_key)
);
CREATE INDEX IF NOT EXISTS idx_campaign_soar_control_job ON campaign_soar_control_requests(tenant_id,job_id,created_at DESC);

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
ALTER TABLE campaign_workbench_state ADD COLUMN IF NOT EXISTS member_count INTEGER NOT NULL DEFAULT 0;
ALTER TABLE campaign_workbench_state ADD COLUMN IF NOT EXISTS last_event_id UUID;

CREATE TABLE IF NOT EXISTS campaign_aggregate_history (
  event_id UUID PRIMARY KEY,
  tenant_id TEXT NOT NULL,
  campaign_id TEXT NOT NULL,
  aggregate_revision BIGINT NOT NULL CHECK (aggregate_revision > 0),
  event_type TEXT NOT NULL,
  status TEXT NOT NULL,
  assignee TEXT NOT NULL DEFAULT '',
  member_count INTEGER NOT NULL DEFAULT 0 CHECK (member_count >= 0),
  payload JSONB NOT NULL,
  reason TEXT NOT NULL,
  created_by TEXT NOT NULL DEFAULT '',
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE (tenant_id, campaign_id, aggregate_revision)
);
CREATE INDEX IF NOT EXISTS idx_campaign_aggregate_history_revision
  ON campaign_aggregate_history (tenant_id, campaign_id, aggregate_revision DESC);

CREATE TABLE IF NOT EXISTS campaign_aggregate_outbox (
  event_id UUID PRIMARY KEY REFERENCES campaign_aggregate_history(event_id) ON DELETE RESTRICT,
  tenant_id TEXT NOT NULL,
  aggregate_id TEXT NOT NULL,
  aggregate_revision BIGINT NOT NULL CHECK (aggregate_revision > 0),
  event_type TEXT NOT NULL,
  schema_version INTEGER NOT NULL DEFAULT 2 CHECK (schema_version > 0),
  partition_key TEXT NOT NULL,
  payload JSONB NOT NULL,
  status TEXT NOT NULL DEFAULT 'pending' CHECK (status IN ('pending','processing','published','dead')),
  published BOOLEAN NOT NULL DEFAULT false,
  attempts INTEGER NOT NULL DEFAULT 0 CHECK (attempts >= 0),
  last_error TEXT NOT NULL DEFAULT '',
  next_attempt_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  locked_until TIMESTAMPTZ,
  locked_by TEXT NOT NULL DEFAULT '',
  last_attempt_at TIMESTAMPTZ,
  dead_at TIMESTAMPTZ,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  published_at TIMESTAMPTZ,
  UNIQUE (tenant_id, aggregate_id, aggregate_revision)
);
CREATE INDEX IF NOT EXISTS idx_campaign_aggregate_outbox_pending
  ON campaign_aggregate_outbox (next_attempt_at, created_at) WHERE published=false;
CREATE INDEX IF NOT EXISTS idx_campaign_aggregate_outbox_delivery
  ON campaign_aggregate_outbox (next_attempt_at,created_at,event_id)
  WHERE status IN ('pending','processing');

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
ALTER TABLE campaign_reports ADD COLUMN IF NOT EXISTS campaign_revision BIGINT NOT NULL DEFAULT 0;
ALTER TABLE campaign_reports ADD COLUMN IF NOT EXISTS snapshot_id UUID;
ALTER TABLE campaign_reports ADD COLUMN IF NOT EXISTS snapshot JSONB NOT NULL DEFAULT '{}'::jsonb;
ALTER TABLE campaign_reports ADD COLUMN IF NOT EXISTS snapshot_sha256 TEXT NOT NULL DEFAULT '';
ALTER TABLE campaign_reports ADD COLUMN IF NOT EXISTS object_manifest JSONB NOT NULL DEFAULT '{}'::jsonb;
ALTER TABLE campaign_reports ADD COLUMN IF NOT EXISTS error_message TEXT NOT NULL DEFAULT '';
ALTER TABLE campaign_reports ADD COLUMN IF NOT EXISTS idempotency_key TEXT NOT NULL DEFAULT '';
ALTER TABLE campaign_reports DROP CONSTRAINT IF EXISTS campaign_reports_status_check;
ALTER TABLE campaign_reports ADD CONSTRAINT campaign_reports_status_check
  CHECK (status IN ('queued','accepted','running','completed','succeeded','partial','failed','cancelled'));
CREATE UNIQUE INDEX IF NOT EXISTS uq_campaign_reports_tenant_idempotency
  ON campaign_reports (tenant_id, idempotency_key) WHERE idempotency_key<>'';
ALTER TABLE campaign_reports ADD COLUMN IF NOT EXISTS job_id TEXT;
ALTER TABLE campaign_reports ADD COLUMN IF NOT EXISTS object_bucket TEXT NOT NULL DEFAULT '';
ALTER TABLE campaign_reports ADD COLUMN IF NOT EXISTS object_key TEXT NOT NULL DEFAULT '';
ALTER TABLE campaign_reports ADD COLUMN IF NOT EXISTS mime_type TEXT NOT NULL DEFAULT '';
ALTER TABLE campaign_reports ADD COLUMN IF NOT EXISTS artifact_sha256 TEXT NOT NULL DEFAULT '';
ALTER TABLE campaign_reports ADD COLUMN IF NOT EXISTS size_bytes BIGINT NOT NULL DEFAULT 0 CHECK (size_bytes>=0);
ALTER TABLE campaign_reports ADD COLUMN IF NOT EXISTS attempts INTEGER NOT NULL DEFAULT 0 CHECK (attempts>=0);
ALTER TABLE campaign_reports ADD COLUMN IF NOT EXISTS next_attempt_at TIMESTAMPTZ NOT NULL DEFAULT now();
ALTER TABLE campaign_reports ADD COLUMN IF NOT EXISTS locked_until TIMESTAMPTZ;
ALTER TABLE campaign_reports ADD COLUMN IF NOT EXISTS locked_by TEXT NOT NULL DEFAULT '';
ALTER TABLE campaign_reports ADD COLUMN IF NOT EXISTS updated_at TIMESTAMPTZ NOT NULL DEFAULT now();
CREATE UNIQUE INDEX IF NOT EXISTS uq_campaign_reports_job_id ON campaign_reports(job_id) WHERE job_id IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_campaign_reports_executor_pending
  ON campaign_reports(next_attempt_at,created_at,report_id) WHERE status IN ('accepted','running');

-- F-ALERT-002 authoritative campaign membership relation. ClickHouse campaign
-- arrays are projections and must not be mutated by the command path.
CREATE TABLE IF NOT EXISTS campaign_alert_links (
  relation_id UUID PRIMARY KEY,
  tenant_id TEXT NOT NULL REFERENCES tenants(tenant_id) ON DELETE CASCADE,
  campaign_id TEXT NOT NULL,
  alert_id TEXT NOT NULL,
  status TEXT NOT NULL DEFAULT 'linked' CHECK (status IN ('linked','unlinked')),
  revision BIGINT NOT NULL DEFAULT 1 CHECK (revision > 0),
  campaign_revision BIGINT NOT NULL DEFAULT 0 CHECK (campaign_revision >= 0),
  reason TEXT NOT NULL,
  idempotency_key TEXT NOT NULL,
  created_by TEXT NOT NULL DEFAULT '',
  updated_by TEXT NOT NULL DEFAULT '',
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE (tenant_id, campaign_id, alert_id),
  UNIQUE (tenant_id, idempotency_key)
);
CREATE INDEX IF NOT EXISTS idx_campaign_alert_links_alert ON campaign_alert_links (tenant_id, alert_id, updated_at DESC) WHERE status='linked';
CREATE INDEX IF NOT EXISTS idx_campaign_alert_links_campaign ON campaign_alert_links (tenant_id, campaign_id, updated_at DESC) WHERE status='linked';
CREATE TABLE IF NOT EXISTS campaign_alert_link_history (
  event_id UUID PRIMARY KEY,
  relation_id UUID NOT NULL REFERENCES campaign_alert_links(relation_id) ON DELETE RESTRICT,
  tenant_id TEXT NOT NULL REFERENCES tenants(tenant_id) ON DELETE CASCADE,
  campaign_id TEXT NOT NULL,
  alert_id TEXT NOT NULL,
  event_type TEXT NOT NULL CHECK (event_type IN ('linked','unlinked')),
  revision BIGINT NOT NULL CHECK (revision > 0),
  campaign_revision BIGINT NOT NULL DEFAULT 0 CHECK (campaign_revision >= 0),
  payload JSONB NOT NULL,
  created_by TEXT NOT NULL DEFAULT '',
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE (relation_id, revision)
);
CREATE INDEX IF NOT EXISTS idx_campaign_alert_link_history_relation ON campaign_alert_link_history (tenant_id, relation_id, revision);
CREATE TABLE IF NOT EXISTS campaign_alert_link_outbox (
  event_id UUID PRIMARY KEY REFERENCES campaign_alert_link_history(event_id) ON DELETE RESTRICT,
  tenant_id TEXT NOT NULL REFERENCES tenants(tenant_id) ON DELETE CASCADE,
  aggregate_id UUID NOT NULL REFERENCES campaign_alert_links(relation_id) ON DELETE RESTRICT,
  aggregate_version BIGINT NOT NULL CHECK (aggregate_version > 0),
  event_type TEXT NOT NULL,
  schema_version INTEGER NOT NULL DEFAULT 2 CHECK (schema_version > 0),
  partition_key TEXT NOT NULL,
  payload JSONB NOT NULL,
  status TEXT NOT NULL DEFAULT 'pending' CHECK (status IN ('pending','processing','published','dead')),
  published BOOLEAN NOT NULL DEFAULT false,
  attempts INTEGER NOT NULL DEFAULT 0 CHECK (attempts >= 0),
  last_error TEXT NOT NULL DEFAULT '',
  next_attempt_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  locked_until TIMESTAMPTZ,
  locked_by TEXT NOT NULL DEFAULT '',
  last_attempt_at TIMESTAMPTZ,
  dead_at TIMESTAMPTZ,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  published_at TIMESTAMPTZ,
  UNIQUE (aggregate_id, aggregate_version)
);
CREATE INDEX IF NOT EXISTS idx_campaign_alert_link_outbox_pending ON campaign_alert_link_outbox (next_attempt_at, created_at) WHERE published=false;
CREATE INDEX IF NOT EXISTS idx_campaign_alert_link_outbox_delivery
  ON campaign_alert_link_outbox (next_attempt_at,created_at,event_id)
  WHERE status IN ('pending','processing');

CREATE TABLE IF NOT EXISTS campaign_membership_commands (
  command_id UUID PRIMARY KEY,
  tenant_id TEXT NOT NULL REFERENCES tenants(tenant_id) ON DELETE CASCADE,
  relation_id UUID NOT NULL REFERENCES campaign_alert_links(relation_id) ON DELETE RESTRICT,
  campaign_id TEXT NOT NULL,
  alert_id TEXT NOT NULL,
  operation TEXT NOT NULL CHECK (operation IN ('link','unlink')),
  idempotency_key TEXT NOT NULL CHECK (length(idempotency_key) BETWEEN 16 AND 200),
  request_sha256 TEXT NOT NULL CHECK (length(request_sha256)=64),
  expected_relation_revision BIGINT NOT NULL CHECK (expected_relation_revision >= 0),
  expected_campaign_revision BIGINT CHECK (expected_campaign_revision >= 0),
  relation_revision BIGINT NOT NULL CHECK (relation_revision > 0),
  campaign_revision BIGINT NOT NULL CHECK (campaign_revision >= 0),
  result JSONB NOT NULL,
  created_by TEXT NOT NULL DEFAULT '',
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE (tenant_id,idempotency_key)
);
CREATE INDEX IF NOT EXISTS idx_campaign_membership_commands_relation ON campaign_membership_commands (tenant_id,relation_id,created_at DESC);
CREATE INDEX IF NOT EXISTS idx_campaign_membership_commands_campaign ON campaign_membership_commands (tenant_id,campaign_id,campaign_revision DESC);

CREATE TABLE IF NOT EXISTS campaign_merge_receipts (
  merge_id UUID PRIMARY KEY,
  job_id TEXT NOT NULL UNIQUE,
  tenant_id TEXT NOT NULL REFERENCES tenants(tenant_id) ON DELETE CASCADE,
  source_campaign_id TEXT NOT NULL,
  target_campaign_id TEXT NOT NULL,
  idempotency_key TEXT NOT NULL CHECK (length(idempotency_key) BETWEEN 16 AND 200),
  request_sha256 TEXT NOT NULL CHECK (length(request_sha256)=64),
  source_expected_revision BIGINT NOT NULL CHECK (source_expected_revision>=0),
  target_expected_revision BIGINT NOT NULL CHECK (target_expected_revision>=0),
  source_revision BIGINT NOT NULL CHECK (source_revision>source_expected_revision),
  target_revision BIGINT NOT NULL CHECK (target_revision>target_expected_revision),
  source_member_count INTEGER NOT NULL CHECK (source_member_count>0 AND source_member_count<=1000),
  target_member_count_before INTEGER NOT NULL CHECK (target_member_count_before>=0),
  target_member_count_after INTEGER NOT NULL CHECK (target_member_count_after>=target_member_count_before),
  moved_count INTEGER NOT NULL CHECK (moved_count>=0),
  relinked_count INTEGER NOT NULL CHECK (relinked_count>=0),
  deduplicated_count INTEGER NOT NULL CHECK (deduplicated_count>=0),
  manifest JSONB NOT NULL CHECK (jsonb_typeof(manifest)='object'),
  manifest_sha256 TEXT NOT NULL CHECK (length(manifest_sha256)=64),
  reason TEXT NOT NULL CHECK (length(reason) BETWEEN 8 AND 1000),
  trace_id TEXT NOT NULL CHECK (length(trace_id)>0),
  created_by TEXT NOT NULL DEFAULT '',
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE (tenant_id,idempotency_key),
  UNIQUE (tenant_id,source_campaign_id),
  CHECK (source_campaign_id<>target_campaign_id),
  CHECK (moved_count+relinked_count+deduplicated_count=source_member_count),
  CHECK (target_member_count_after=target_member_count_before+moved_count+relinked_count)
);
CREATE INDEX IF NOT EXISTS idx_campaign_merge_receipts_target ON campaign_merge_receipts (tenant_id,target_campaign_id,created_at DESC);
CREATE TABLE IF NOT EXISTS campaign_merge_items (
  merge_id UUID NOT NULL REFERENCES campaign_merge_receipts(merge_id) ON DELETE RESTRICT DEFERRABLE INITIALLY DEFERRED,
  tenant_id TEXT NOT NULL REFERENCES tenants(tenant_id) ON DELETE CASCADE,
  source_relation_id UUID NOT NULL REFERENCES campaign_alert_links(relation_id) ON DELETE RESTRICT,
  target_relation_id UUID NOT NULL REFERENCES campaign_alert_links(relation_id) ON DELETE RESTRICT,
  alert_id TEXT NOT NULL,
  outcome TEXT NOT NULL CHECK (outcome IN ('moved','relinked','deduplicated')),
  source_relation_revision BIGINT NOT NULL CHECK (source_relation_revision>0),
  target_relation_revision BIGINT NOT NULL CHECK (target_relation_revision>0),
  source_event_id UUID NOT NULL REFERENCES campaign_alert_link_history(event_id) ON DELETE RESTRICT,
  target_event_id UUID REFERENCES campaign_alert_link_history(event_id) ON DELETE RESTRICT,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  PRIMARY KEY (merge_id,alert_id),
  UNIQUE (merge_id,source_relation_id),
  CHECK ((outcome='deduplicated' AND target_event_id IS NULL) OR (outcome IN ('moved','relinked') AND target_event_id IS NOT NULL))
);
CREATE INDEX IF NOT EXISTS idx_campaign_merge_items_tenant_alert ON campaign_merge_items (tenant_id,alert_id,created_at DESC);

CREATE TABLE IF NOT EXISTS campaign_membership_backfill_runs (
  run_id UUID PRIMARY KEY, tenant_id TEXT NOT NULL REFERENCES tenants(tenant_id) ON DELETE CASCADE,
  source_kind TEXT NOT NULL CHECK (source_kind='clickhouse_export'), source_uri TEXT NOT NULL CHECK (length(source_uri)>0),
  source_sha256 TEXT NOT NULL CHECK (length(source_sha256)=64), source_snapshot_id TEXT NOT NULL CHECK (length(source_snapshot_id)>0),
  source_as_of TIMESTAMPTZ NOT NULL, manifest JSONB NOT NULL CHECK (jsonb_typeof(manifest)='object'),
  manifest_sha256 TEXT NOT NULL CHECK (length(manifest_sha256)=64), reason TEXT NOT NULL CHECK (length(reason) BETWEEN 8 AND 1000),
  status TEXT NOT NULL CHECK (status IN ('running','completed','partial','failed')),
  campaign_count INTEGER NOT NULL CHECK (campaign_count>0 AND campaign_count<=100), source_member_count INTEGER NOT NULL CHECK (source_member_count>=0),
  completed_campaign_count INTEGER NOT NULL DEFAULT 0 CHECK (completed_campaign_count>=0), failed_campaign_count INTEGER NOT NULL DEFAULT 0 CHECK (failed_campaign_count>=0),
  inserted_count INTEGER NOT NULL DEFAULT 0 CHECK (inserted_count>=0), bound_count INTEGER NOT NULL DEFAULT 0 CHECK (bound_count>=0),
  unchanged_count INTEGER NOT NULL DEFAULT 0 CHECK (unchanged_count>=0), skipped_unlinked_count INTEGER NOT NULL DEFAULT 0 CHECK (skipped_unlinked_count>=0),
  created_by TEXT NOT NULL DEFAULT '', created_at TIMESTAMPTZ NOT NULL DEFAULT now(), updated_at TIMESTAMPTZ NOT NULL DEFAULT now(), completed_at TIMESTAMPTZ,
  UNIQUE (tenant_id,manifest_sha256), CHECK (completed_campaign_count+failed_campaign_count<=campaign_count)
);
CREATE INDEX IF NOT EXISTS idx_campaign_membership_backfill_runs_tenant ON campaign_membership_backfill_runs (tenant_id,created_at DESC);
CREATE TABLE IF NOT EXISTS campaign_membership_backfill_campaigns (
  run_id UUID NOT NULL REFERENCES campaign_membership_backfill_runs(run_id) ON DELETE RESTRICT,
  tenant_id TEXT NOT NULL REFERENCES tenants(tenant_id) ON DELETE CASCADE, campaign_id TEXT NOT NULL,
  manifest_sha256 TEXT NOT NULL CHECK (length(manifest_sha256)=64), expected_campaign_revision BIGINT NOT NULL CHECK (expected_campaign_revision>=0),
  starting_campaign_revision BIGINT CHECK (starting_campaign_revision>=0), resulting_campaign_revision BIGINT CHECK (resulting_campaign_revision>=0),
  source_member_count INTEGER NOT NULL CHECK (source_member_count>=0 AND source_member_count<=1000), resulting_member_count INTEGER CHECK (resulting_member_count>=0),
  inserted_count INTEGER NOT NULL DEFAULT 0 CHECK (inserted_count>=0), bound_count INTEGER NOT NULL DEFAULT 0 CHECK (bound_count>=0),
  unchanged_count INTEGER NOT NULL DEFAULT 0 CHECK (unchanged_count>=0), skipped_unlinked_count INTEGER NOT NULL DEFAULT 0 CHECK (skipped_unlinked_count>=0),
  status TEXT NOT NULL CHECK (status IN ('running','completed','failed')), error_code TEXT NOT NULL DEFAULT '', error_message TEXT NOT NULL DEFAULT '',
  aggregate_event_id UUID REFERENCES campaign_aggregate_history(event_id) ON DELETE RESTRICT,
  started_at TIMESTAMPTZ NOT NULL DEFAULT now(), completed_at TIMESTAMPTZ, PRIMARY KEY (run_id,campaign_id),
  CHECK (inserted_count+bound_count+unchanged_count+skipped_unlinked_count<=source_member_count)
);
CREATE INDEX IF NOT EXISTS idx_campaign_membership_backfill_campaigns_tenant ON campaign_membership_backfill_campaigns (tenant_id,campaign_id,started_at DESC);
CREATE TABLE IF NOT EXISTS campaign_membership_backfill_items (
  run_id UUID NOT NULL, tenant_id TEXT NOT NULL REFERENCES tenants(tenant_id) ON DELETE CASCADE, campaign_id TEXT NOT NULL,
  alert_id TEXT NOT NULL, source_ordinal INTEGER NOT NULL CHECK (source_ordinal>=0),
  outcome TEXT NOT NULL CHECK (outcome IN ('inserted','bound','unchanged','skipped_explicit_unlink')),
  relation_id UUID NOT NULL REFERENCES campaign_alert_links(relation_id) ON DELETE RESTRICT,
  relation_revision BIGINT NOT NULL CHECK (relation_revision>0), campaign_revision BIGINT NOT NULL CHECK (campaign_revision>=0),
  event_id UUID REFERENCES campaign_alert_link_history(event_id) ON DELETE RESTRICT, created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  PRIMARY KEY (run_id,campaign_id,alert_id), UNIQUE (run_id,campaign_id,source_ordinal),
  FOREIGN KEY (run_id,campaign_id) REFERENCES campaign_membership_backfill_campaigns(run_id,campaign_id) ON DELETE RESTRICT DEFERRABLE INITIALLY DEFERRED,
  CHECK ((outcome IN ('inserted','bound') AND event_id IS NOT NULL) OR (outcome IN ('unchanged','skipped_explicit_unlink') AND event_id IS NULL))
);
CREATE INDEX IF NOT EXISTS idx_campaign_membership_backfill_items_tenant_alert ON campaign_membership_backfill_items (tenant_id,alert_id,created_at DESC);

CREATE TABLE IF NOT EXISTS campaign_event_projection_inbox (
  stream TEXT NOT NULL CHECK (stream IN ('aggregate','membership')),
  event_id UUID NOT NULL,
  tenant_id TEXT NOT NULL REFERENCES tenants(tenant_id) ON DELETE CASCADE,
  aggregate_id TEXT NOT NULL,
  campaign_id TEXT NOT NULL,
  relation_id UUID,
  alert_id TEXT NOT NULL DEFAULT '',
  event_type TEXT NOT NULL,
  schema_version INTEGER NOT NULL CHECK (schema_version=2),
  aggregate_revision BIGINT NOT NULL CHECK (aggregate_revision>=0),
  relation_revision BIGINT NOT NULL DEFAULT 0 CHECK (relation_revision>=0),
  partition_key TEXT NOT NULL,
  trace_id TEXT NOT NULL,
  payload JSONB NOT NULL,
  projection_status TEXT NOT NULL DEFAULT 'pending' CHECK (projection_status IN ('pending','processing','partial','applied','dead')),
  target_status JSONB NOT NULL DEFAULT '{"clickhouse":"pending","opensearch":"pending","nebulagraph":"pending"}'::jsonb
    CHECK (jsonb_typeof(target_status)='object'
      AND target_status->>'clickhouse' IN ('pending','applied','dead')
      AND target_status->>'opensearch' IN ('pending','applied','dead')
      AND target_status->>'nebulagraph' IN ('pending','applied','dead')
      AND target_status-'clickhouse'-'opensearch'-'nebulagraph'='{}'::jsonb),
  first_kafka_topic TEXT NOT NULL,
  first_kafka_partition INTEGER NOT NULL CHECK (first_kafka_partition>=0),
  first_kafka_offset BIGINT NOT NULL CHECK (first_kafka_offset>=0),
  attempt_count INTEGER NOT NULL DEFAULT 0 CHECK (attempt_count>=0),
  available_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  locked_by TEXT NOT NULL DEFAULT '',
  locked_until TIMESTAMPTZ,
  last_error TEXT NOT NULL DEFAULT '',
  applied_at TIMESTAMPTZ,
  received_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  PRIMARY KEY (stream,event_id),
  CHECK ((stream='aggregate' AND aggregate_revision>0 AND relation_revision=0 AND relation_id IS NULL)
      OR (stream='membership' AND relation_revision>0 AND relation_id IS NOT NULL))
);
CREATE INDEX IF NOT EXISTS idx_campaign_event_projection_pending ON campaign_event_projection_inbox (available_at,received_at,stream,event_id) WHERE projection_status IN ('pending','processing','partial');
CREATE INDEX IF NOT EXISTS idx_campaign_event_projection_campaign ON campaign_event_projection_inbox (tenant_id,campaign_id,aggregate_revision DESC,relation_revision DESC);
CREATE TABLE IF NOT EXISTS campaign_event_projection_deliveries (
  kafka_topic TEXT NOT NULL,
  kafka_partition INTEGER NOT NULL CHECK (kafka_partition>=0),
  kafka_offset BIGINT NOT NULL CHECK (kafka_offset>=0),
  stream TEXT NOT NULL,
  event_id UUID NOT NULL,
  received_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  PRIMARY KEY (kafka_topic,kafka_partition,kafka_offset),
  FOREIGN KEY (stream,event_id) REFERENCES campaign_event_projection_inbox(stream,event_id) ON DELETE RESTRICT
);
CREATE INDEX IF NOT EXISTS idx_campaign_event_projection_delivery_event ON campaign_event_projection_deliveries (stream,event_id,received_at);
CREATE TABLE IF NOT EXISTS campaign_event_projection_watermarks (
  kafka_topic TEXT NOT NULL,
  kafka_partition INTEGER NOT NULL CHECK (kafka_partition>=0),
  last_offset BIGINT NOT NULL CHECK (last_offset>=0),
  last_event_id UUID NOT NULL,
  last_stream TEXT NOT NULL CHECK (last_stream IN ('aggregate','membership')),
  last_received_at TIMESTAMPTZ NOT NULL,
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  PRIMARY KEY (kafka_topic,kafka_partition)
);
CREATE TABLE IF NOT EXISTS campaign_target_projection_watermarks (
  tenant_id TEXT NOT NULL REFERENCES tenants(tenant_id) ON DELETE CASCADE,
  projection_key TEXT NOT NULL,
  target TEXT NOT NULL CHECK (target IN ('clickhouse','opensearch','nebulagraph')),
  stream TEXT NOT NULL CHECK (stream IN ('aggregate','membership')),
  projection_version BIGINT NOT NULL CHECK (projection_version>0),
  event_id UUID NOT NULL,
  projection_sha256 TEXT NOT NULL CHECK (length(projection_sha256)=64),
  applied_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  PRIMARY KEY (tenant_id,projection_key,target),
  FOREIGN KEY (stream,event_id) REFERENCES campaign_event_projection_inbox(stream,event_id) ON DELETE RESTRICT
);
CREATE INDEX IF NOT EXISTS idx_campaign_target_projection_event ON campaign_target_projection_watermarks (stream,event_id,target);
CREATE INDEX IF NOT EXISTS idx_campaign_target_projection_version ON campaign_target_projection_watermarks (tenant_id,target,projection_version DESC);

CREATE TABLE IF NOT EXISTS alert_report_jobs (
  job_id TEXT PRIMARY KEY,
  tenant_id TEXT NOT NULL REFERENCES tenants(tenant_id) ON DELETE CASCADE,
  alert_id TEXT NOT NULL,
  format TEXT NOT NULL CHECK (format IN ('json','pdf','docx')),
  status TEXT NOT NULL DEFAULT 'accepted' CHECK (status IN ('accepted','running','cancel_requested','completed','partial','failed','cancelled','compensating','compensated','compensation_failed')),
  revision BIGINT NOT NULL DEFAULT 1 CHECK (revision > 0),
  idempotency_key TEXT NOT NULL,
  requested_snapshot_id TEXT NOT NULL,
  snapshot JSONB NOT NULL,
  snapshot_sha256 TEXT NOT NULL,
  missing_sections JSONB NOT NULL DEFAULT '[]'::jsonb,
  source_watermarks JSONB NOT NULL DEFAULT '{}'::jsonb,
  object_bucket TEXT NOT NULL DEFAULT '',
  object_key TEXT NOT NULL DEFAULT '',
  mime_type TEXT NOT NULL DEFAULT '',
  artifact_sha256 TEXT NOT NULL DEFAULT '',
  size_bytes BIGINT NOT NULL DEFAULT 0 CHECK (size_bytes >= 0),
  error_message TEXT NOT NULL DEFAULT '',
  attempts INTEGER NOT NULL DEFAULT 0 CHECK (attempts >= 0),
  next_attempt_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  locked_until TIMESTAMPTZ,
  locked_by TEXT NOT NULL DEFAULT '',
  created_by TEXT NOT NULL DEFAULT '',
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  completed_at TIMESTAMPTZ,
  cancellation_reason TEXT NOT NULL DEFAULT '',
  cancel_requested_at TIMESTAMPTZ,
  cancelled_at TIMESTAMPTZ,
  UNIQUE (tenant_id,idempotency_key)
);
ALTER TABLE alert_report_jobs
  ADD COLUMN IF NOT EXISTS revision BIGINT NOT NULL DEFAULT 1,
  ADD COLUMN IF NOT EXISTS cancellation_reason TEXT NOT NULL DEFAULT '',
  ADD COLUMN IF NOT EXISTS cancel_requested_at TIMESTAMPTZ,
  ADD COLUMN IF NOT EXISTS cancelled_at TIMESTAMPTZ;
ALTER TABLE alert_report_jobs
  DROP CONSTRAINT IF EXISTS alert_report_jobs_status_check,
  DROP CONSTRAINT IF EXISTS chk_alert_report_status,
  DROP CONSTRAINT IF EXISTS chk_alert_report_revision;
ALTER TABLE alert_report_jobs
  ADD CONSTRAINT chk_alert_report_status CHECK (status IN ('accepted','running','cancel_requested','completed','partial','failed','cancelled','compensating','compensated','compensation_failed')),
  ADD CONSTRAINT chk_alert_report_revision CHECK (revision > 0);
CREATE INDEX IF NOT EXISTS idx_alert_report_jobs_alert_time ON alert_report_jobs (tenant_id,alert_id,created_at DESC);
CREATE INDEX IF NOT EXISTS idx_alert_report_jobs_queue ON alert_report_jobs (status,next_attempt_at,created_at) WHERE status IN ('accepted','running');
CREATE INDEX IF NOT EXISTS idx_alert_report_jobs_cancel_cleanup ON alert_report_jobs(next_attempt_at,created_at) WHERE status='cancel_requested';
CREATE TABLE IF NOT EXISTS alert_report_outbox (
  event_id UUID PRIMARY KEY,
  job_id TEXT NOT NULL REFERENCES alert_report_jobs(job_id) ON DELETE RESTRICT,
  tenant_id TEXT NOT NULL REFERENCES tenants(tenant_id) ON DELETE CASCADE,
  event_type TEXT NOT NULL,
  aggregate_version BIGINT NOT NULL CHECK (aggregate_version > 0),
  schema_version INTEGER NOT NULL DEFAULT 2 CHECK (schema_version > 0),
  partition_key TEXT NOT NULL,
  payload JSONB NOT NULL,
  published BOOLEAN NOT NULL DEFAULT false,
  attempts INTEGER NOT NULL DEFAULT 0 CHECK (attempts >= 0),
  last_error TEXT NOT NULL DEFAULT '',
  next_attempt_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  locked_until TIMESTAMPTZ,
  locked_by TEXT NOT NULL DEFAULT '',
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  published_at TIMESTAMPTZ,
  UNIQUE (job_id,aggregate_version)
);
CREATE INDEX IF NOT EXISTS idx_alert_report_outbox_pending ON alert_report_outbox (next_attempt_at,created_at) WHERE published=false;
CREATE TABLE IF NOT EXISTS alert_report_job_history (
  transition_id BIGSERIAL PRIMARY KEY,
  job_id TEXT NOT NULL REFERENCES alert_report_jobs(job_id) ON DELETE RESTRICT,
  tenant_id TEXT NOT NULL REFERENCES tenants(tenant_id) ON DELETE CASCADE,
  from_status TEXT NOT NULL,
  to_status TEXT NOT NULL,
  revision BIGINT NOT NULL CHECK (revision > 0),
  actor TEXT NOT NULL,
  reason TEXT NOT NULL,
  trace_id TEXT NOT NULL,
  detail JSONB NOT NULL DEFAULT '{}'::jsonb,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE (job_id,revision)
);
CREATE INDEX IF NOT EXISTS idx_alert_report_history_tenant_job ON alert_report_job_history(tenant_id,job_id,revision);
INSERT INTO alert_report_job_history(job_id,tenant_id,from_status,to_status,revision,actor,reason,trace_id,detail)
SELECT job_id,tenant_id,'',status,revision,COALESCE(NULLIF(created_by,''),'migration'),
       'legacy alert report lifecycle backfill','',jsonb_build_object('backfilled',true)
FROM alert_report_jobs ON CONFLICT (job_id,revision) DO NOTHING;
CREATE TABLE IF NOT EXISTS alert_report_control_requests (
  request_id UUID PRIMARY KEY,
  tenant_id TEXT NOT NULL REFERENCES tenants(tenant_id) ON DELETE CASCADE,
  job_id TEXT NOT NULL REFERENCES alert_report_jobs(job_id) ON DELETE RESTRICT,
  operation TEXT NOT NULL CHECK (operation IN ('cancel','compensate')),
  idempotency_key TEXT NOT NULL CHECK (length(idempotency_key) BETWEEN 16 AND 200),
  request_hash TEXT NOT NULL CHECK (length(request_hash)=64),
  expected_revision BIGINT NOT NULL CHECK (expected_revision > 0),
  resulting_revision BIGINT NOT NULL CHECK (resulting_revision > 0),
  result_payload JSONB NOT NULL DEFAULT '{}'::jsonb,
  actor TEXT NOT NULL,
  reason TEXT NOT NULL CHECK (length(reason) BETWEEN 8 AND 1000),
  trace_id TEXT NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE (tenant_id,idempotency_key)
);
CREATE INDEX IF NOT EXISTS idx_alert_report_control_job ON alert_report_control_requests(tenant_id,job_id,created_at);
INSERT INTO alignment_schema_migrations(version,description)
VALUES ('202608041900','F-ALERT revisioned cooperative report cancellation')
ON CONFLICT (version) DO NOTHING;

CREATE TABLE IF NOT EXISTS alert_feedback (
  feedback_id       UUID PRIMARY KEY,
  event_id          UUID NOT NULL UNIQUE,
  tenant_id         TEXT NOT NULL,
  alert_id          TEXT NOT NULL,
  user_id           UUID,
  label             TEXT NOT NULL CHECK (label IN ('TP','FP')),
  reason_code       TEXT,
  comment           TEXT,
  add_to_whitelist  BOOLEAN NOT NULL DEFAULT false,
  alert_type        TEXT NOT NULL DEFAULT '',
  severity          TEXT NOT NULL DEFAULT '',
  model_version     TEXT NOT NULL DEFAULT '',
  rule_version      TEXT NOT NULL DEFAULT '',
  idempotency_key   TEXT NOT NULL DEFAULT '',
  trace_id          TEXT NOT NULL DEFAULT '',
  payload           JSONB NOT NULL DEFAULT '{}'::jsonb,
  status            TEXT NOT NULL DEFAULT 'accepted',
  created_at        TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at        TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX IF NOT EXISTS uq_alert_feedback_tenant_idempotency ON alert_feedback (tenant_id,idempotency_key) WHERE idempotency_key <> '';
CREATE INDEX IF NOT EXISTS idx_feedback_tenant_alert ON alert_feedback (tenant_id,alert_id,created_at DESC);
CREATE INDEX IF NOT EXISTS idx_feedback_tenant_time ON alert_feedback (tenant_id,created_at DESC);
CREATE INDEX IF NOT EXISTS idx_feedback_label ON alert_feedback (tenant_id,label,created_at DESC);
CREATE TABLE IF NOT EXISTS alert_feedback_outbox (
  outbox_id BIGSERIAL PRIMARY KEY,
  event_id UUID NOT NULL UNIQUE,
  feedback_id UUID NOT NULL REFERENCES alert_feedback(feedback_id) ON DELETE RESTRICT,
  tenant_id TEXT NOT NULL,
  alert_id TEXT NOT NULL,
  partition_key TEXT NOT NULL,
  schema_version INTEGER NOT NULL DEFAULT 1 CHECK (schema_version = 1),
  aggregate_version BIGINT NOT NULL DEFAULT 1 CHECK (aggregate_version = 1),
  payload JSONB NOT NULL,
  published BOOLEAN NOT NULL DEFAULT false,
  attempts INTEGER NOT NULL DEFAULT 0 CHECK (attempts >= 0),
  last_error TEXT NOT NULL DEFAULT '',
  next_attempt_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  locked_until TIMESTAMPTZ,
  locked_by TEXT NOT NULL DEFAULT '',
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  published_at TIMESTAMPTZ
);
CREATE INDEX IF NOT EXISTS idx_alert_feedback_outbox_pending ON alert_feedback_outbox (next_attempt_at,outbox_id) WHERE published=false;

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
  revision   BIGINT NOT NULL DEFAULT 1 CHECK (revision > 0),
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
  revision         BIGINT NOT NULL DEFAULT 1 CHECK (revision > 0),
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

CREATE TABLE IF NOT EXISTS topic_actions (
  action_id    UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
  tenant_id    TEXT NOT NULL,
  topic        TEXT NOT NULL,
  action       TEXT NOT NULL,
  target       TEXT NOT NULL,
  status       TEXT NOT NULL DEFAULT 'queued',
  detail       JSONB NOT NULL DEFAULT '{}'::jsonb,
  requested_by TEXT NOT NULL DEFAULT '',
  created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at   TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_topic_actions_tenant_topic
  ON topic_actions (tenant_id, topic, created_at DESC);

CREATE TABLE IF NOT EXISTS topic_snapshot_manifests (
  snapshot_id UUID PRIMARY KEY,
  tenant_id TEXT NOT NULL,
  topic TEXT NOT NULL CHECK (topic IN ('tunnel','exfil','apt')),
  resource_revision BIGINT NOT NULL CHECK (resource_revision > 0),
  as_of TIMESTAMPTZ NOT NULL,
  range_start BIGINT NOT NULL,
  range_end BIGINT NOT NULL,
  payload JSONB NOT NULL,
  payload_sha256 TEXT NOT NULL CHECK (length(payload_sha256)=64),
  partial BOOLEAN NOT NULL DEFAULT false,
  missing_sections TEXT[] NOT NULL DEFAULT ARRAY[]::TEXT[],
  source_watermarks JSONB NOT NULL DEFAULT '{}'::jsonb,
  trace_id TEXT NOT NULL DEFAULT '',
  created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_topic_snapshot_manifest_tenant_topic ON topic_snapshot_manifests (tenant_id,topic,created_at DESC);
CREATE INDEX IF NOT EXISTS idx_topic_snapshot_manifest_trace ON topic_snapshot_manifests (tenant_id,trace_id) WHERE trace_id<>'';

ALTER TABLE topic_actions ADD COLUMN IF NOT EXISTS idempotency_key TEXT;
ALTER TABLE topic_actions ADD COLUMN IF NOT EXISTS snapshot_id UUID;
ALTER TABLE topic_actions ADD COLUMN IF NOT EXISTS expected_revision BIGINT NOT NULL DEFAULT 0;
ALTER TABLE topic_actions ADD COLUMN IF NOT EXISTS revision BIGINT NOT NULL DEFAULT 1;
ALTER TABLE topic_actions ADD COLUMN IF NOT EXISTS reason TEXT NOT NULL DEFAULT '';
ALTER TABLE topic_actions ADD COLUMN IF NOT EXISTS trace_id TEXT NOT NULL DEFAULT '';
ALTER TABLE topic_actions ADD COLUMN IF NOT EXISTS executor TEXT NOT NULL DEFAULT 'legacy';
ALTER TABLE topic_actions ADD COLUMN IF NOT EXISTS receipt JSONB NOT NULL DEFAULT '{}'::jsonb;
ALTER TABLE topic_actions ADD COLUMN IF NOT EXISTS error JSONB NOT NULL DEFAULT '{}'::jsonb;
ALTER TABLE topic_actions ADD COLUMN IF NOT EXISTS attempts INTEGER NOT NULL DEFAULT 0;
ALTER TABLE topic_actions ADD COLUMN IF NOT EXISTS lease_until TIMESTAMPTZ;
CREATE UNIQUE INDEX IF NOT EXISTS uq_topic_actions_tenant_idempotency ON topic_actions (tenant_id,idempotency_key) WHERE idempotency_key IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_topic_actions_executor_queue ON topic_actions (executor,status,created_at) WHERE status IN ('accepted','running');

CREATE TABLE IF NOT EXISTS topic_action_history (
  history_id BIGSERIAL PRIMARY KEY,
  job_id UUID NOT NULL REFERENCES topic_actions(action_id) ON DELETE RESTRICT,
  tenant_id TEXT NOT NULL,
  revision BIGINT NOT NULL CHECK (revision > 0),
  from_status TEXT NOT NULL,
  to_status TEXT NOT NULL,
  detail JSONB NOT NULL DEFAULT '{}'::jsonb,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE (job_id,revision)
);
CREATE INDEX IF NOT EXISTS idx_topic_action_history_tenant_job ON topic_action_history (tenant_id,job_id,revision);

CREATE TABLE IF NOT EXISTS topic_action_receipts (
  receipt_id UUID PRIMARY KEY,
  job_id UUID NOT NULL REFERENCES topic_actions(action_id) ON DELETE RESTRICT,
  tenant_id TEXT NOT NULL,
  executor TEXT NOT NULL,
  effect_type TEXT NOT NULL,
  effect_ref TEXT NOT NULL,
  receipt JSONB NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE (job_id,effect_type,effect_ref)
);
CREATE INDEX IF NOT EXISTS idx_topic_action_receipts_tenant_job ON topic_action_receipts (tenant_id,job_id);

CREATE TABLE IF NOT EXISTS topic_action_outbox (
  event_id UUID PRIMARY KEY,
  job_id UUID NOT NULL REFERENCES topic_actions(action_id) ON DELETE RESTRICT,
  tenant_id TEXT NOT NULL,
  event_type TEXT NOT NULL,
  aggregate_version BIGINT NOT NULL DEFAULT 1 CHECK (aggregate_version > 0),
  schema_version INTEGER NOT NULL DEFAULT 2 CHECK (schema_version > 0),
  partition_key TEXT NOT NULL,
  payload JSONB NOT NULL,
  published BOOLEAN NOT NULL DEFAULT false,
  attempts INTEGER NOT NULL DEFAULT 0 CHECK (attempts >= 0),
  last_error TEXT NOT NULL DEFAULT '',
  next_attempt_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  locked_until TIMESTAMPTZ,
  locked_by TEXT NOT NULL DEFAULT '',
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  published_at TIMESTAMPTZ,
  UNIQUE (job_id,event_type)
);
CREATE INDEX IF NOT EXISTS idx_topic_action_outbox_pending ON topic_action_outbox (next_attempt_at,created_at) WHERE published=false;

CREATE TABLE IF NOT EXISTS topic_action_event_projection (
    event_id UUID PRIMARY KEY,
    job_id UUID NOT NULL,
    tenant_id TEXT NOT NULL,
    topic TEXT NOT NULL,
    event_type TEXT NOT NULL CHECK (event_type IN ('traffic.topic.v2.ActionRequested','traffic.topic.v2.ActionResult')),
    revision BIGINT NOT NULL CHECK (revision > 0),
    action_id TEXT NOT NULL DEFAULT '',
    status TEXT NOT NULL DEFAULT '',
    trace_id TEXT NOT NULL,
    payload JSONB NOT NULL,
    kafka_partition INTEGER NOT NULL,
    kafka_offset BIGINT NOT NULL,
    projected_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (kafka_partition,kafka_offset)
);
CREATE INDEX IF NOT EXISTS idx_topic_action_event_projection_tenant_job ON topic_action_event_projection (tenant_id,job_id,revision);

CREATE TABLE IF NOT EXISTS topic_action_job_projection (
    tenant_id TEXT NOT NULL,
    job_id UUID NOT NULL,
    topic TEXT NOT NULL,
    revision BIGINT NOT NULL CHECK (revision > 0),
    event_type TEXT NOT NULL,
    action_id TEXT NOT NULL DEFAULT '',
    status TEXT NOT NULL DEFAULT '',
    trace_id TEXT NOT NULL,
    last_event_id UUID NOT NULL,
    payload JSONB NOT NULL,
    kafka_partition INTEGER NOT NULL,
    kafka_offset BIGINT NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (tenant_id,job_id)
);
CREATE INDEX IF NOT EXISTS idx_topic_action_job_projection_tenant_topic ON topic_action_job_projection (tenant_id,topic,updated_at DESC);

CREATE TABLE IF NOT EXISTS probe_operation_event_projection (
    event_id UUID PRIMARY KEY,
    operation_id UUID NOT NULL,
    tenant_id TEXT NOT NULL,
    probe_id TEXT NOT NULL,
    event_type TEXT NOT NULL CHECK (event_type = 'traffic.probe.v2.OperationAcknowledged'),
    revision BIGINT NOT NULL CHECK (revision > 0),
    status TEXT NOT NULL,
    trace_id TEXT NOT NULL,
    payload JSONB NOT NULL,
    kafka_partition INTEGER NOT NULL,
    kafka_offset BIGINT NOT NULL,
    projected_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (kafka_partition,kafka_offset)
);
CREATE INDEX IF NOT EXISTS idx_probe_operation_event_projection_tenant_operation ON probe_operation_event_projection (tenant_id,operation_id,revision);

CREATE TABLE IF NOT EXISTS probe_operation_state_projection (
    tenant_id TEXT NOT NULL,
    operation_id UUID NOT NULL,
    probe_id TEXT NOT NULL,
    revision BIGINT NOT NULL CHECK (revision > 0),
    event_type TEXT NOT NULL,
    status TEXT NOT NULL,
    trace_id TEXT NOT NULL,
    last_event_id UUID NOT NULL,
    payload JSONB NOT NULL,
    kafka_partition INTEGER NOT NULL,
    kafka_offset BIGINT NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (tenant_id,operation_id)
);
CREATE INDEX IF NOT EXISTS idx_probe_operation_state_projection_tenant_probe ON probe_operation_state_projection (tenant_id,probe_id,updated_at DESC);

-- Database-backed presentation fixtures for the three topic workbenches.  These
-- rows are read through the normal topic APIs; the Web UI never imports fixture
-- constants or bypasses the backend.
CREATE TABLE IF NOT EXISTS topic_panel_simulations (
  simulation_id TEXT PRIMARY KEY,
  tenant_id     TEXT NOT NULL DEFAULT '*',
  topic         TEXT NOT NULL CHECK (topic IN ('tunnel', 'exfil', 'apt')),
  version       TEXT NOT NULL,
  enabled       BOOLEAN NOT NULL DEFAULT true,
  payload       JSONB NOT NULL,
  created_by    TEXT NOT NULL DEFAULT '',
  created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE (tenant_id, topic, version)
);

CREATE INDEX IF NOT EXISTS idx_topic_panel_simulations_active
  ON topic_panel_simulations (tenant_id, topic, enabled, updated_at DESC);
CREATE UNIQUE INDEX IF NOT EXISTS uq_topic_panel_simulations_one_enabled
  ON topic_panel_simulations (tenant_id, topic)
  WHERE enabled;

COMMIT;

-- T-PG-002 threat-intel command authority for fresh schema entrypoints.
BEGIN;
CREATE TABLE IF NOT EXISTS threat_intel (
  id UUID PRIMARY KEY DEFAULT uuid_generate_v4(), tenant_id TEXT NOT NULL DEFAULT 'default',
  type TEXT NOT NULL CHECK (type IN ('ip','domain','hash')), value TEXT NOT NULL,
  reputation TEXT NOT NULL DEFAULT 'unknown', category TEXT NOT NULL DEFAULT '',
  source TEXT NOT NULL DEFAULT 'manual', description TEXT NOT NULL DEFAULT '',
  last_seen TIMESTAMPTZ NOT NULL DEFAULT now(), updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  revision BIGINT NOT NULL DEFAULT 1,
  UNIQUE (tenant_id,type,value)
);
CREATE INDEX IF NOT EXISTS idx_threat_intel_value ON threat_intel(type,value);
CREATE INDEX IF NOT EXISTS idx_threat_intel_tenant_value ON threat_intel(tenant_id,type,value);
CREATE INDEX IF NOT EXISTS idx_threat_intel_rep ON threat_intel(reputation);
CREATE INDEX IF NOT EXISTS idx_threat_intel_source ON threat_intel(source);
CREATE TABLE IF NOT EXISTS threat_intel_feeds (
  id UUID PRIMARY KEY DEFAULT uuid_generate_v4(), tenant_id TEXT NOT NULL DEFAULT 'default',
  name TEXT NOT NULL, enabled BOOLEAN NOT NULL DEFAULT true,
  interval_seconds INTEGER NOT NULL DEFAULT 3600 CHECK (interval_seconds >= 1),
  entries JSONB NOT NULL DEFAULT '[]'::jsonb, last_run_at TIMESTAMPTZ,
  next_run_at TIMESTAMPTZ NOT NULL DEFAULT now(), last_status TEXT NOT NULL DEFAULT 'never',
  last_error TEXT NOT NULL DEFAULT '', run_count INTEGER NOT NULL DEFAULT 0,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(), updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  revision BIGINT NOT NULL DEFAULT 1, UNIQUE (tenant_id,name)
);
CREATE INDEX IF NOT EXISTS idx_threat_intel_feeds_due ON threat_intel_feeds(enabled,next_run_at);
CREATE INDEX IF NOT EXISTS idx_threat_intel_feeds_tenant ON threat_intel_feeds(tenant_id,name);
CREATE TABLE IF NOT EXISTS threat_intel_event_outbox (
  event_id TEXT PRIMARY KEY, tenant_id TEXT NOT NULL, partition_key TEXT NOT NULL,
  payload JSONB NOT NULL, status TEXT NOT NULL DEFAULT 'pending'
    CHECK (status IN ('pending','processing','published','dead')),
  attempt_count INTEGER NOT NULL DEFAULT 0 CHECK (attempt_count >= 0),
  available_at TIMESTAMPTZ NOT NULL DEFAULT now(), locked_until TIMESTAMPTZ,
  locked_by TEXT NOT NULL DEFAULT '', last_error TEXT NOT NULL DEFAULT '',
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(), published_at TIMESTAMPTZ
);
CREATE INDEX IF NOT EXISTS idx_threat_intel_event_outbox_pending
  ON threat_intel_event_outbox(available_at,created_at) WHERE status='pending';
CREATE INDEX IF NOT EXISTS idx_threat_intel_event_outbox_reclaim
  ON threat_intel_event_outbox(locked_until,created_at) WHERE status='processing';
ALTER TABLE threat_intel DROP CONSTRAINT IF EXISTS threat_intel_revision_positive;
ALTER TABLE threat_intel ADD CONSTRAINT threat_intel_revision_positive CHECK (revision > 0);
ALTER TABLE threat_intel_feeds DROP CONSTRAINT IF EXISTS threat_intel_feeds_revision_positive;
ALTER TABLE threat_intel_feeds ADD CONSTRAINT threat_intel_feeds_revision_positive CHECK (revision > 0);
CREATE TABLE IF NOT EXISTS threat_intel_command_history (
  history_id BIGSERIAL PRIMARY KEY,
  event_id TEXT NOT NULL REFERENCES threat_intel_event_outbox(event_id) ON DELETE RESTRICT,
  tenant_id TEXT NOT NULL, aggregate_type TEXT NOT NULL CHECK (aggregate_type IN ('entry','feed')),
  aggregate_id TEXT NOT NULL, revision BIGINT NOT NULL CHECK (revision > 0),
  action_id TEXT NOT NULL, operation TEXT NOT NULL, reason TEXT NOT NULL, trace_id TEXT NOT NULL,
  compatibility_mode BOOLEAN NOT NULL DEFAULT false, snapshot JSONB NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE (tenant_id,aggregate_type,aggregate_id,revision),
  UNIQUE (event_id,aggregate_type,aggregate_id)
);
CREATE INDEX IF NOT EXISTS idx_threat_intel_command_history_lookup
  ON threat_intel_command_history (tenant_id,aggregate_type,aggregate_id,revision DESC);
CREATE TABLE IF NOT EXISTS threat_intel_command_requests (
  tenant_id TEXT NOT NULL,
  idempotency_key TEXT NOT NULL CHECK (length(idempotency_key) BETWEEN 16 AND 200),
  request_sha256 TEXT NOT NULL CHECK (length(request_sha256) = 64),
  action_id TEXT NOT NULL, command_type TEXT NOT NULL,
  expected_revision BIGINT NOT NULL CHECK (expected_revision >= 0),
  resulting_revision BIGINT NOT NULL CHECK (resulting_revision > 0),
  event_id TEXT NOT NULL REFERENCES threat_intel_event_outbox(event_id) ON DELETE RESTRICT,
  reason TEXT NOT NULL, trace_id TEXT NOT NULL,
  compatibility_mode BOOLEAN NOT NULL DEFAULT false, response_payload JSONB NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  PRIMARY KEY (tenant_id,idempotency_key), UNIQUE (event_id)
);
CREATE INDEX IF NOT EXISTS idx_threat_intel_command_requests_created
  ON threat_intel_command_requests (tenant_id,created_at DESC);
CREATE TABLE IF NOT EXISTS alignment_schema_migrations (
  version TEXT PRIMARY KEY,
  description TEXT NOT NULL,
  applied_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  applied_by TEXT NOT NULL DEFAULT current_user
);
INSERT INTO alignment_schema_migrations(version,description)
VALUES ('202608031550','threat intel revision history audit outbox and durable command replay')
ON CONFLICT (version) DO NOTHING;
COMMIT;

-- T-PG-002 notification rule atomic command boundary (202608031300).
BEGIN;
ALTER TABLE notification_rules
  ADD COLUMN IF NOT EXISTS revision BIGINT NOT NULL DEFAULT 1,
  ADD COLUMN IF NOT EXISTS trace_id TEXT NOT NULL DEFAULT '';
ALTER TABLE notification_escalation_policies
  ADD COLUMN IF NOT EXISTS revision BIGINT NOT NULL DEFAULT 1;
ALTER TABLE alert_notification_settings
  ADD COLUMN IF NOT EXISTS revision BIGINT NOT NULL DEFAULT 1;
ALTER TABLE notification_silence_rules
  ADD COLUMN IF NOT EXISTS revision BIGINT NOT NULL DEFAULT 1;
CREATE TABLE IF NOT EXISTS notification_governance_history (
  history_id BIGSERIAL PRIMARY KEY,
  event_id UUID NOT NULL UNIQUE,
  tenant_id TEXT NOT NULL,
  aggregate_type TEXT NOT NULL,
  aggregate_id TEXT NOT NULL,
  revision BIGINT NOT NULL CHECK (revision > 0),
  action TEXT NOT NULL,
  snapshot JSONB NOT NULL,
  changed_by TEXT NOT NULL DEFAULT '',
  reason TEXT NOT NULL,
  trace_id TEXT NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE (tenant_id, aggregate_type, aggregate_id, revision)
);
CREATE INDEX IF NOT EXISTS idx_notification_governance_history_aggregate
  ON notification_governance_history (tenant_id,aggregate_type,aggregate_id,revision DESC);
CREATE TABLE IF NOT EXISTS notification_governance_outbox (
  outbox_id BIGSERIAL PRIMARY KEY,
  event_id UUID NOT NULL UNIQUE,
  aggregate_type TEXT NOT NULL,
  aggregate_id TEXT NOT NULL,
  aggregate_version BIGINT NOT NULL CHECK (aggregate_version > 0),
  tenant_id TEXT NOT NULL,
  event_type TEXT NOT NULL,
  schema_version INTEGER NOT NULL DEFAULT 1 CHECK (schema_version > 0),
  partition_key TEXT NOT NULL,
  payload JSONB NOT NULL,
  trace_id TEXT NOT NULL,
  status TEXT NOT NULL DEFAULT 'pending' CHECK (status IN ('pending','processing','published','dead')),
  publish_attempts INTEGER NOT NULL DEFAULT 0 CHECK (publish_attempts >= 0),
  last_error TEXT NOT NULL DEFAULT '',
  next_retry_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  locked_until TIMESTAMPTZ,
  locked_by TEXT NOT NULL DEFAULT '',
  occurred_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  published_at TIMESTAMPTZ
);
CREATE INDEX IF NOT EXISTS idx_notification_governance_outbox_ready
  ON notification_governance_outbox (next_retry_at,occurred_at,outbox_id)
  WHERE status IN ('pending','processing');
CREATE TABLE IF NOT EXISTS alignment_schema_migrations (
  version TEXT PRIMARY KEY,
  description TEXT NOT NULL,
  applied_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  applied_by TEXT NOT NULL DEFAULT current_user
);
CREATE TABLE IF NOT EXISTS notification_governance_requests (
  tenant_id TEXT NOT NULL,
  idempotency_key TEXT NOT NULL,
  payload_sha256 TEXT NOT NULL CHECK (length(payload_sha256)=64),
  action_id TEXT NOT NULL,
  aggregate_type TEXT NOT NULL,
  aggregate_id TEXT NOT NULL,
  resulting_revision BIGINT NOT NULL CHECK (resulting_revision > 0),
  event_id UUID NOT NULL REFERENCES notification_governance_outbox(event_id) ON DELETE RESTRICT,
  response_payload JSONB NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  PRIMARY KEY (tenant_id,idempotency_key),
  UNIQUE (event_id)
);
INSERT INTO alignment_schema_migrations(version,description)
VALUES ('202608031300','T-PG-002 notification rule atomic command history outbox idempotency')
ON CONFLICT (version) DO NOTHING;
COMMIT;


-- T-PG-002 / F-ALERT-006 atomic saved-view transaction schema.
BEGIN;
ALTER TABLE alert_saved_views
  ADD COLUMN IF NOT EXISTS revision BIGINT NOT NULL DEFAULT 1,
  ADD COLUMN IF NOT EXISTS updated_by TEXT NOT NULL DEFAULT '',
  ADD COLUMN IF NOT EXISTS trace_id TEXT NOT NULL DEFAULT '';
CREATE TABLE IF NOT EXISTS alert_saved_view_requests (
  tenant_id TEXT NOT NULL,
  idempotency_key TEXT NOT NULL CHECK (length(idempotency_key) BETWEEN 16 AND 200),
  payload_sha256 TEXT NOT NULL CHECK (length(payload_sha256) = 64),
  view_id UUID NOT NULL REFERENCES alert_saved_views(view_id) ON DELETE CASCADE,
  resulting_revision BIGINT NOT NULL CHECK (resulting_revision > 0),
  event_id UUID NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  PRIMARY KEY (tenant_id, idempotency_key),
  UNIQUE (event_id)
);
CREATE TABLE IF NOT EXISTS alert_saved_view_history (
  history_id BIGSERIAL PRIMARY KEY,
  event_id UUID NOT NULL,
  tenant_id TEXT NOT NULL,
  view_id UUID NOT NULL REFERENCES alert_saved_views(view_id) ON DELETE CASCADE,
  revision BIGINT NOT NULL CHECK (revision > 0),
  name TEXT NOT NULL,
  filters JSONB NOT NULL,
  action TEXT NOT NULL,
  changed_by TEXT NOT NULL,
  trace_id TEXT NOT NULL,
  occurred_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE (tenant_id, view_id, revision),
  UNIQUE (event_id)
);
CREATE TABLE IF NOT EXISTS alert_saved_view_outbox (
  outbox_id BIGSERIAL PRIMARY KEY,
  event_id UUID NOT NULL UNIQUE,
  aggregate_type TEXT NOT NULL DEFAULT 'alert_saved_view',
  aggregate_id UUID NOT NULL,
  aggregate_version BIGINT NOT NULL CHECK (aggregate_version > 0),
  tenant_id TEXT NOT NULL,
  event_type TEXT NOT NULL,
  schema_version INTEGER NOT NULL DEFAULT 1 CHECK (schema_version = 1),
  partition_key TEXT NOT NULL CHECK (partition_key <> ''),
  payload JSONB NOT NULL,
  trace_id TEXT NOT NULL,
  occurred_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  publish_attempts INTEGER NOT NULL DEFAULT 0 CHECK (publish_attempts >= 0),
  next_retry_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  status TEXT NOT NULL DEFAULT 'pending' CHECK (status IN ('pending','processing','published','dead')),
  locked_by TEXT NOT NULL DEFAULT '',
  locked_until TIMESTAMPTZ,
  published_at TIMESTAMPTZ,
  last_error TEXT NOT NULL DEFAULT ''
);
CREATE INDEX IF NOT EXISTS idx_alert_saved_view_outbox_ready
  ON alert_saved_view_outbox (next_retry_at, occurred_at, outbox_id)
  WHERE status IN ('pending','processing');
CREATE TABLE IF NOT EXISTS alignment_schema_migrations (
  version TEXT PRIMARY KEY,
  description TEXT NOT NULL,
  applied_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  applied_by TEXT NOT NULL DEFAULT current_user
);
INSERT INTO alignment_schema_migrations(version, description)
VALUES ('202608031100', 'alert saved view atomic history audit outbox and idempotency')
ON CONFLICT (version) DO NOTHING;
COMMIT;

-- BEGIN GENERATED T-DQ-001 DATA QUALITY CONTROL PLANE
-- Source: deployments/postgres/migrations/202608041400_data_quality_control_plane_v1.sql
-- Do not edit this block directly; run scripts/alignment/sync_data_quality_postgres_entrypoints.py --write.
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

-- T-DQ-001: immutable repair lifecycle commands and bounded replay evidence.
-- Expand only. Replay execution remains default-off and no runtime DDL is used.
BEGIN;

CREATE TABLE IF NOT EXISTS data_quality_repair_history (
  history_id BIGSERIAL PRIMARY KEY,
  event_id UUID NOT NULL UNIQUE,
  tenant_id TEXT NOT NULL,
  repair_id UUID NOT NULL REFERENCES data_quality_repairs(repair_id) ON DELETE RESTRICT,
  revision BIGINT NOT NULL CHECK (revision > 0),
  operation TEXT NOT NULL CHECK (operation IN (
    'planned','dry_run_completed','approval_submitted','approved','rejected',
    'execution_started','execution_completed','execution_partial','execution_failed','reconciled','cancelled'
  )),
  previous_status TEXT NOT NULL,
  resulting_status TEXT NOT NULL,
  actor_id TEXT NOT NULL CHECK (actor_id <> ''),
  reason TEXT NOT NULL CHECK (length(reason) BETWEEN 8 AND 1000),
  trace_id TEXT NOT NULL CHECK (trace_id <> ''),
  snapshot JSONB NOT NULL CHECK (jsonb_typeof(snapshot) = 'object'),
  occurred_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE (tenant_id,repair_id,revision)
);
CREATE INDEX IF NOT EXISTS idx_data_quality_repair_history_lookup
  ON data_quality_repair_history (tenant_id,repair_id,revision DESC);

CREATE TABLE IF NOT EXISTS data_quality_repair_requests (
  tenant_id TEXT NOT NULL,
  idempotency_key TEXT NOT NULL CHECK (length(idempotency_key) BETWEEN 16 AND 200),
  request_sha256 TEXT NOT NULL CHECK (length(request_sha256) = 64),
  action_id TEXT NOT NULL CHECK (action_id <> ''),
  operation TEXT NOT NULL CHECK (operation <> ''),
  repair_id UUID NOT NULL REFERENCES data_quality_repairs(repair_id) ON DELETE RESTRICT,
  resulting_revision BIGINT NOT NULL CHECK (resulting_revision > 0),
  event_id UUID NOT NULL REFERENCES data_quality_outbox(event_id) ON DELETE RESTRICT,
  response_payload JSONB NOT NULL CHECK (jsonb_typeof(response_payload) = 'object'),
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  PRIMARY KEY (tenant_id,idempotency_key),
  UNIQUE (event_id)
);
CREATE INDEX IF NOT EXISTS idx_data_quality_repair_requests_lookup
  ON data_quality_repair_requests (tenant_id,repair_id,created_at DESC);

INSERT INTO alignment_schema_migrations(version,description)
VALUES ('202608041700','immutable data quality repair dry-run approval replay and reconcile lifecycle')
ON CONFLICT (version) DO NOTHING;

COMMIT;

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
-- END GENERATED T-DQ-001 DATA QUALITY CONTROL PLANE
