-- F-MODEL-001 / F-MLOPS-001 / T1-M08-N015
-- Evidence-bound drift decisions may request candidate training only.  Serving
-- activation remains owned by the independent model activation authority.
-- Rollback: set MLOPS_AUTOMATIC_CANDIDATE_V1_ENABLED=false and retain both
-- append-only receipt tables.  Do not delete governance evidence.

BEGIN;

CREATE TABLE IF NOT EXISTS model_drift_evaluation_receipt (
  evaluation_id          UUID PRIMARY KEY,
  tenant_id              TEXT NOT NULL REFERENCES tenants(tenant_id) ON DELETE RESTRICT,
  model_id               UUID NOT NULL REFERENCES models(model_id) ON DELETE RESTRICT,
  model_version          TEXT NOT NULL REFERENCES model_versions(model_version) ON DELETE RESTRICT,
  feature_set_id         TEXT NOT NULL REFERENCES feature_sets(feature_set_id) ON DELETE RESTRICT,
  policy_sha256          CHAR(64) NOT NULL CHECK (policy_sha256 ~ '^[0-9a-f]{64}$'),
  signal_sha256          CHAR(64) NOT NULL CHECK (signal_sha256 ~ '^[0-9a-f]{64}$'),
  decision_state         TEXT NOT NULL CHECK (decision_state IN ('BLOCKED','NO_ACTION','CANDIDATE')),
  reasons                JSONB NOT NULL CHECK (jsonb_typeof(reasons)='array'),
  psi                    JSONB NOT NULL CHECK (jsonb_typeof(psi)='object'),
  max_observed_psi       DOUBLE PRECISION NOT NULL CHECK (max_observed_psi >= 0),
  false_positive_rate    DOUBLE PRECISION NOT NULL CHECK (false_positive_rate BETWEEN 0 AND 1),
  signal_snapshot        JSONB NOT NULL CHECK (jsonb_typeof(signal_snapshot)='object'),
  feature_watermark      TIMESTAMPTZ,
  feedback_watermark     TIMESTAMPTZ,
  baseline_window_start  TIMESTAMPTZ NOT NULL,
  baseline_window_end    TIMESTAMPTZ NOT NULL,
  current_window_start   TIMESTAMPTZ NOT NULL,
  current_window_end     TIMESTAMPTZ NOT NULL,
  evaluated_at           TIMESTAMPTZ NOT NULL,
  activation_authorized  BOOLEAN NOT NULL DEFAULT false CHECK (activation_authorized=false),
  created_at             TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE (policy_sha256,signal_sha256),
  CHECK (baseline_window_start < baseline_window_end),
  CHECK (baseline_window_end = current_window_start),
  CHECK (current_window_start < current_window_end),
  CHECK (current_window_end <= evaluated_at),
  CHECK ((decision_state='CANDIDATE' AND jsonb_array_length(reasons)>0)
      OR decision_state<>'CANDIDATE')
);

CREATE INDEX IF NOT EXISTS idx_model_drift_evaluation_receipt_scope
  ON model_drift_evaluation_receipt (tenant_id,model_id,evaluated_at DESC);

CREATE TABLE IF NOT EXISTS model_retrain_candidate_request (
  candidate_id            UUID PRIMARY KEY,
  evaluation_id           UUID NOT NULL UNIQUE REFERENCES model_drift_evaluation_receipt(evaluation_id) ON DELETE RESTRICT,
  tenant_id               TEXT NOT NULL REFERENCES tenants(tenant_id) ON DELETE RESTRICT,
  model_id                UUID NOT NULL REFERENCES models(model_id) ON DELETE RESTRICT,
  baseline_model_version  TEXT NOT NULL REFERENCES model_versions(model_version) ON DELETE RESTRICT,
  feature_set_id          TEXT NOT NULL REFERENCES feature_sets(feature_set_id) ON DELETE RESTRICT,
  workflow_name           TEXT NOT NULL UNIQUE CHECK (workflow_name ~ '^mlops-drift-[0-9a-f]{20}$'),
  candidate_state         TEXT NOT NULL CHECK (candidate_state IN ('PENDING_WORKFLOW','WORKFLOW_SUBMITTED','WORKFLOW_FAILED')),
  argo_namespace          TEXT NOT NULL,
  workflow_template       TEXT NOT NULL,
  approval_state          TEXT NOT NULL DEFAULT 'NOT_REQUESTED' CHECK (approval_state='NOT_REQUESTED'),
  activation_authorized   BOOLEAN NOT NULL DEFAULT false CHECK (activation_authorized=false),
  dispatch_attempts       INTEGER NOT NULL DEFAULT 0 CHECK (dispatch_attempts >= 0),
  last_error              TEXT NOT NULL DEFAULT '',
  workflow_submitted_at   TIMESTAMPTZ,
  created_at              TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at              TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE (tenant_id,model_id,baseline_model_version),
  CHECK ((candidate_state='WORKFLOW_SUBMITTED' AND workflow_submitted_at IS NOT NULL AND last_error='')
      OR candidate_state<>'WORKFLOW_SUBMITTED')
);

CREATE INDEX IF NOT EXISTS idx_model_retrain_candidate_request_pending
  ON model_retrain_candidate_request (updated_at,candidate_id)
  WHERE candidate_state IN ('PENDING_WORKFLOW','WORKFLOW_FAILED');

CREATE OR REPLACE FUNCTION enforce_model_drift_evaluation_append_only()
RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
  RAISE EXCEPTION 'model drift evaluation receipts are append-only';
END;
$$;
DROP TRIGGER IF EXISTS model_drift_evaluation_append_only
  ON model_drift_evaluation_receipt;
CREATE TRIGGER model_drift_evaluation_append_only
BEFORE UPDATE OR DELETE ON model_drift_evaluation_receipt
FOR EACH ROW EXECUTE FUNCTION enforce_model_drift_evaluation_append_only();

CREATE OR REPLACE FUNCTION enforce_model_retrain_candidate_guard()
RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE
  evaluation model_drift_evaluation_receipt%ROWTYPE;
BEGIN
  IF TG_OP = 'DELETE' THEN
    RAISE EXCEPTION 'model retrain candidate requests are append-only';
  END IF;
  IF TG_OP = 'UPDATE' THEN
    IF NEW.candidate_id<>OLD.candidate_id OR NEW.evaluation_id<>OLD.evaluation_id
       OR NEW.tenant_id<>OLD.tenant_id OR NEW.model_id<>OLD.model_id
       OR NEW.baseline_model_version<>OLD.baseline_model_version
       OR NEW.feature_set_id<>OLD.feature_set_id OR NEW.workflow_name<>OLD.workflow_name
       OR NEW.argo_namespace<>OLD.argo_namespace OR NEW.workflow_template<>OLD.workflow_template
       OR NEW.approval_state<>OLD.approval_state OR NEW.activation_authorized<>OLD.activation_authorized
       OR NEW.created_at<>OLD.created_at THEN
      RAISE EXCEPTION 'model retrain candidate immutable identity changed';
    END IF;
    IF NOT ((OLD.candidate_state='PENDING_WORKFLOW' AND NEW.candidate_state IN ('PENDING_WORKFLOW','WORKFLOW_SUBMITTED','WORKFLOW_FAILED'))
         OR (OLD.candidate_state='WORKFLOW_FAILED' AND NEW.candidate_state IN ('WORKFLOW_FAILED','WORKFLOW_SUBMITTED'))
         OR (OLD.candidate_state='WORKFLOW_SUBMITTED' AND NEW.candidate_state='WORKFLOW_SUBMITTED')) THEN
      RAISE EXCEPTION 'invalid model retrain candidate state transition % -> %', OLD.candidate_state, NEW.candidate_state;
    END IF;
  END IF;

  SELECT * INTO evaluation FROM model_drift_evaluation_receipt
  WHERE evaluation_id=NEW.evaluation_id;
  IF NOT FOUND OR evaluation.decision_state<>'CANDIDATE' THEN
    RAISE EXCEPTION 'model retrain candidate requires a CANDIDATE evaluation';
  END IF;
  IF evaluation.tenant_id<>NEW.tenant_id OR evaluation.model_id<>NEW.model_id
     OR evaluation.model_version<>NEW.baseline_model_version
     OR evaluation.feature_set_id<>NEW.feature_set_id
     OR evaluation.activation_authorized OR NEW.activation_authorized
     OR NEW.approval_state<>'NOT_REQUESTED' THEN
    RAISE EXCEPTION 'model retrain candidate evaluation identity mismatch';
  END IF;
  IF TG_OP='INSERT' AND NOT EXISTS (
    SELECT 1 FROM model_versions active
    WHERE active.model_version=NEW.baseline_model_version
      AND active.tenant_id=NEW.tenant_id AND active.model_id=NEW.model_id
      AND active.status='active'
  ) THEN
    RAISE EXCEPTION 'model retrain candidate baseline is not the active model version';
  END IF;
  RETURN NEW;
END;
$$;
DROP TRIGGER IF EXISTS model_retrain_candidate_guard
  ON model_retrain_candidate_request;
CREATE TRIGGER model_retrain_candidate_guard
BEFORE INSERT OR UPDATE OR DELETE ON model_retrain_candidate_request
FOR EACH ROW EXECUTE FUNCTION enforce_model_retrain_candidate_guard();

CREATE TABLE IF NOT EXISTS alignment_schema_migrations (
  version      TEXT PRIMARY KEY,
  description  TEXT NOT NULL,
  applied_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
  applied_by   TEXT NOT NULL DEFAULT current_user
);
INSERT INTO alignment_schema_migrations(version,description)
VALUES ('202608151700','M08 governed model drift evaluation and candidate-only retrain request')
ON CONFLICT (version) DO NOTHING;

COMMIT;
