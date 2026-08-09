-- Canonical bootstrap counterpart of migration 202608091130_alert_response_external_executor_v1.sql.
BEGIN;

ALTER TABLE alert_response_execution_receipts
  ADD COLUMN IF NOT EXISTS provider TEXT NOT NULL DEFAULT '',
  ADD COLUMN IF NOT EXISTS provider_receipt_id TEXT NOT NULL DEFAULT '',
  ADD COLUMN IF NOT EXISTS effect_state TEXT NOT NULL DEFAULT 'none',
  ADD COLUMN IF NOT EXISTS effect_ids JSONB NOT NULL DEFAULT '[]'::jsonb,
  ADD COLUMN IF NOT EXISTS trace_id TEXT NOT NULL DEFAULT '',
  ADD COLUMN IF NOT EXISTS receipt_sha256 TEXT NOT NULL DEFAULT repeat('0',64),
  ADD COLUMN IF NOT EXISTS authority_lookup JSONB NOT NULL DEFAULT '{}'::jsonb,
  ADD COLUMN IF NOT EXISTS executed_at TIMESTAMPTZ NOT NULL DEFAULT now();

UPDATE alert_response_execution_receipts receipt
SET provider=CASE WHEN receipt.simulated THEN 'internal-simulation' ELSE 'legacy-approval-guard' END,
    provider_receipt_id=CASE WHEN receipt.simulated THEN 'simulation:' ELSE 'blocked:' END || receipt.event_id::text,
    effect_state=CASE WHEN receipt.external_effect THEN 'confirmed' ELSE 'none' END,
    trace_id=action.trace_id,
    receipt_sha256=repeat('0',64), executed_at=receipt.created_at
FROM alert_response_actions action
WHERE action.job_id=receipt.job_id
  AND (receipt.provider='' OR receipt.provider_receipt_id='' OR receipt.trace_id='');

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
CREATE UNIQUE INDEX IF NOT EXISTS uq_alert_response_provider_receipt
  ON alert_response_execution_receipts(provider,provider_receipt_id) WHERE provider<>'' AND provider_receipt_id<>'';
CREATE INDEX IF NOT EXISTS idx_alert_response_receipts_trace
  ON alert_response_execution_receipts(tenant_id,trace_id,executed_at DESC) WHERE trace_id<>'';
INSERT INTO alignment_schema_migrations(version,description)
VALUES ('202608091130','provider authoritative alert response execution receipts and audit')
ON CONFLICT (version) DO NOTHING;

COMMIT;
