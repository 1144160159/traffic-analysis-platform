-- Canonical bootstrap counterpart of migration 202608091500_alert_response_reconciliation_compensation_v1.sql.
BEGIN;

ALTER TABLE alert_response_control_requests
  DROP CONSTRAINT IF EXISTS alert_response_control_requests_state_check;
ALTER TABLE alert_response_control_requests
  ADD CONSTRAINT alert_response_control_requests_state_check
    CHECK (state IN ('cancelled','blocked_external_executor','queued'));

CREATE UNIQUE INDEX IF NOT EXISTS uq_alert_response_actions_tenant_idempotency
  ON alert_response_actions(tenant_id,idempotency_key)
  WHERE idempotency_key<>'';

CREATE TABLE IF NOT EXISTS alert_response_execution_authority_rechecks (
  recheck_id UUID PRIMARY KEY,
  event_id UUID NOT NULL UNIQUE REFERENCES alert_response_execution_receipts(event_id) ON DELETE RESTRICT,
  job_id TEXT NOT NULL REFERENCES alert_response_actions(job_id) ON DELETE RESTRICT,
  tenant_id TEXT NOT NULL,
  trace_id TEXT NOT NULL,
  status TEXT NOT NULL CHECK (status IN ('pending','checking','resolved','exhausted')),
  attempts INTEGER NOT NULL DEFAULT 0 CHECK (attempts >= 0),
  max_attempts INTEGER NOT NULL CHECK (max_attempts BETWEEN 1 AND 100),
  next_attempt_at TIMESTAMPTZ NOT NULL,
  locked_until TIMESTAMPTZ,
  locked_by TEXT NOT NULL DEFAULT '',
  last_authority_state TEXT NOT NULL DEFAULT '',
  last_error TEXT NOT NULL DEFAULT '',
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  resolved_at TIMESTAMPTZ
);
CREATE INDEX IF NOT EXISTS idx_alert_response_execution_rechecks_due
  ON alert_response_execution_authority_rechecks(next_attempt_at,recheck_id)
  WHERE status IN ('pending','checking');

CREATE TABLE IF NOT EXISTS alert_response_compensation_attempts (
  request_id UUID PRIMARY KEY REFERENCES alert_response_control_requests(request_id) ON DELETE RESTRICT,
  event_id UUID NOT NULL REFERENCES alert_response_execution_receipts(event_id) ON DELETE RESTRICT,
  job_id TEXT NOT NULL REFERENCES alert_response_actions(job_id) ON DELETE RESTRICT,
  tenant_id TEXT NOT NULL,
  alert_id TEXT NOT NULL,
  original_action_id TEXT NOT NULL,
  compensation_action_id TEXT NOT NULL,
  original_provider TEXT NOT NULL,
  original_provider_receipt_id TEXT NOT NULL,
  original_effect_ids JSONB NOT NULL CHECK (jsonb_typeof(original_effect_ids)='array' AND jsonb_array_length(original_effect_ids)>0),
  requested_by TEXT NOT NULL,
  reason TEXT NOT NULL,
  trace_id TEXT NOT NULL,
  aggregate_version BIGINT NOT NULL CHECK (aggregate_version>0),
  provider_idempotency_key TEXT NOT NULL UNIQUE,
  status TEXT NOT NULL CHECK (status IN ('pending','executing','authority_pending','compensated','failed','exhausted_unknown')),
  attempts INTEGER NOT NULL DEFAULT 0 CHECK (attempts>=0),
  max_attempts INTEGER NOT NULL DEFAULT 8 CHECK (max_attempts BETWEEN 1 AND 100),
  next_attempt_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  locked_until TIMESTAMPTZ,
  locked_by TEXT NOT NULL DEFAULT '',
  last_authority_state TEXT NOT NULL DEFAULT '',
  last_error TEXT NOT NULL DEFAULT '',
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  completed_at TIMESTAMPTZ
);
CREATE INDEX IF NOT EXISTS idx_alert_response_compensation_due
  ON alert_response_compensation_attempts(next_attempt_at,request_id)
  WHERE status IN ('pending','executing','authority_pending');
CREATE INDEX IF NOT EXISTS idx_alert_response_compensation_tenant_job
  ON alert_response_compensation_attempts(tenant_id,job_id,created_at DESC);

CREATE TABLE IF NOT EXISTS alert_response_compensation_receipts (
  request_id UUID PRIMARY KEY REFERENCES alert_response_compensation_attempts(request_id) ON DELETE RESTRICT,
  event_id UUID NOT NULL,
  job_id TEXT NOT NULL,
  tenant_id TEXT NOT NULL,
  provider TEXT NOT NULL,
  provider_receipt_id TEXT NOT NULL,
  state TEXT NOT NULL CHECK (state IN ('compensated','failed')),
  effect_state TEXT NOT NULL CHECK (effect_state IN ('compensated','none')),
  compensated_effect_ids JSONB NOT NULL CHECK (jsonb_typeof(compensated_effect_ids)='array'),
  result JSONB NOT NULL DEFAULT '{}'::jsonb,
  error_code TEXT NOT NULL DEFAULT '',
  error_message TEXT NOT NULL DEFAULT '',
  receipt_sha256 TEXT NOT NULL CHECK (receipt_sha256 ~ '^[0-9a-f]{64}$'),
  authority_lookup JSONB NOT NULL DEFAULT '{}'::jsonb,
  trace_id TEXT NOT NULL,
  compensated_at TIMESTAMPTZ NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE(provider,provider_receipt_id)
);
CREATE INDEX IF NOT EXISTS idx_alert_response_compensation_receipts_trace
  ON alert_response_compensation_receipts(tenant_id,trace_id,compensated_at DESC);

CREATE TABLE IF NOT EXISTS alert_response_authority_check_history (
  subject_type TEXT NOT NULL CHECK (subject_type IN ('execution','compensation')),
  subject_id TEXT NOT NULL,
  attempt INTEGER NOT NULL CHECK (attempt>0),
  tenant_id TEXT NOT NULL,
  job_id TEXT NOT NULL,
  trace_id TEXT NOT NULL,
  provider TEXT NOT NULL DEFAULT '',
  authority_state TEXT NOT NULL,
  error_code TEXT NOT NULL DEFAULT '',
  error_message TEXT NOT NULL DEFAULT '',
  detail JSONB NOT NULL DEFAULT '{}'::jsonb,
  checked_at TIMESTAMPTZ NOT NULL,
  PRIMARY KEY(subject_type,subject_id,attempt)
);
CREATE INDEX IF NOT EXISTS idx_alert_response_authority_history_trace
  ON alert_response_authority_check_history(tenant_id,trace_id,checked_at DESC);

INSERT INTO alignment_schema_migrations(version,description)
VALUES ('202608091500','bounded alert response authority reconciliation and external compensation')
ON CONFLICT (version) DO NOTHING;

COMMIT;
