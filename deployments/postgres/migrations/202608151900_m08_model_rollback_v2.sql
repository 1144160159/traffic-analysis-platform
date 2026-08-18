-- T1-M08-N016 governed model rollback v2.
--
-- Expand only.  The writer is separately guarded by
-- MODEL_ROLLBACK_V2_ENABLED=false.  A rollback request records the exact
-- currently-serving and target immutable packages.  The serving pointer may
-- move only after the exact Flink subtask set acknowledges the target.  A
-- failed/expired attempt is compensated back to the recorded source package;
-- partial compensation is never represented as recovered.

BEGIN;

CREATE TABLE IF NOT EXISTS model_rollback_requests (
    rollback_id                    UUID PRIMARY KEY,
    action_job_id                  TEXT NOT NULL UNIQUE
                                      REFERENCES model_action_jobs(job_id) ON DELETE RESTRICT,
    rollback_event_id              TEXT NOT NULL UNIQUE
                                      REFERENCES model_update_outbox(event_id) ON DELETE RESTRICT,
    compensation_event_id          TEXT UNIQUE
                                      REFERENCES model_update_outbox(event_id) ON DELETE RESTRICT,
    tenant_id                      TEXT NOT NULL
                                      REFERENCES tenants(tenant_id) ON DELETE RESTRICT,
    model_id                       UUID NOT NULL
                                      REFERENCES models(model_id) ON DELETE RESTRICT,
    from_model_version             TEXT NOT NULL
                                      REFERENCES model_versions(model_version) ON DELETE RESTRICT,
    from_revision                  BIGINT NOT NULL CHECK (from_revision > 0),
    from_package_sha256            TEXT NOT NULL CHECK (from_package_sha256 ~ '^[0-9a-f]{64}$'),
    target_model_version           TEXT NOT NULL
                                      REFERENCES model_versions(model_version) ON DELETE RESTRICT,
    target_revision                BIGINT NOT NULL CHECK (target_revision > 0),
    target_package_sha256          TEXT NOT NULL CHECK (target_package_sha256 ~ '^[0-9a-f]{64}$'),
    consumer_deployment_id         TEXT NOT NULL,
    consumer_profile_sha256        TEXT NOT NULL CHECK (consumer_profile_sha256 ~ '^[0-9a-f]{64}$'),
    expected_parallelism           INTEGER NOT NULL CHECK (expected_parallelism > 0),
    request_sha256                 TEXT NOT NULL CHECK (request_sha256 ~ '^[0-9a-f]{64}$'),
    reason                         TEXT NOT NULL CHECK (length(reason) BETWEEN 8 AND 1000),
    requested_by                   TEXT NOT NULL,
    state                          TEXT NOT NULL DEFAULT 'PENDING_ACK' CHECK (
                                      state IN (
                                        'PENDING_ACK','PARTIAL','COMPENSATING','RECOVERED',
                                        'FAILED_UNPUBLISHED','FAILED_RESTORED','COMPENSATION_FAILED'
                                      )
                                    ),
    applied_subtasks               INTEGER NOT NULL DEFAULT 0 CHECK (
                                      applied_subtasks BETWEEN 0 AND expected_parallelism
                                    ),
    compensation_applied_subtasks  INTEGER NOT NULL DEFAULT 0 CHECK (
                                      compensation_applied_subtasks BETWEEN 0 AND expected_parallelism
                                    ),
    active_switched                BOOLEAN NOT NULL DEFAULT false,
    failure_reason                 TEXT NOT NULL DEFAULT '',
    created_at                     TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at                     TIMESTAMPTZ NOT NULL DEFAULT now(),
    recovered_at                   TIMESTAMPTZ,
    CHECK (from_model_version <> target_model_version),
    CHECK (active_switched = (state = 'RECOVERED')),
    CHECK (state <> 'RECOVERED' OR applied_subtasks = expected_parallelism),
    CHECK (state <> 'FAILED_RESTORED' OR compensation_applied_subtasks = expected_parallelism),
    CHECK (compensation_event_id IS NOT NULL OR state NOT IN (
      'COMPENSATING','FAILED_RESTORED','COMPENSATION_FAILED'
    ))
);

CREATE UNIQUE INDEX IF NOT EXISTS uq_model_rollback_inflight
    ON model_rollback_requests (tenant_id,model_id)
    WHERE state IN ('PENDING_ACK','PARTIAL','COMPENSATING');

CREATE INDEX IF NOT EXISTS idx_model_rollback_status
    ON model_rollback_requests (tenant_id,model_id,created_at DESC);

CREATE INDEX IF NOT EXISTS idx_model_rollback_deadline
    ON model_rollback_requests (updated_at,rollback_id)
    WHERE state IN ('PENDING_ACK','PARTIAL');

CREATE OR REPLACE FUNCTION enforce_model_rollback_state_v2()
RETURNS TRIGGER LANGUAGE plpgsql AS $$
DECLARE
  transition_key TEXT;
BEGIN
  IF TG_OP = 'DELETE' THEN
    RAISE EXCEPTION 'model rollback receipts are append-only';
  END IF;

  IF OLD.rollback_id IS DISTINCT FROM NEW.rollback_id
     OR OLD.action_job_id IS DISTINCT FROM NEW.action_job_id
     OR OLD.rollback_event_id IS DISTINCT FROM NEW.rollback_event_id
     OR OLD.tenant_id IS DISTINCT FROM NEW.tenant_id
     OR OLD.model_id IS DISTINCT FROM NEW.model_id
     OR OLD.from_model_version IS DISTINCT FROM NEW.from_model_version
     OR OLD.from_revision IS DISTINCT FROM NEW.from_revision
     OR OLD.from_package_sha256 IS DISTINCT FROM NEW.from_package_sha256
     OR OLD.target_model_version IS DISTINCT FROM NEW.target_model_version
     OR OLD.target_revision IS DISTINCT FROM NEW.target_revision
     OR OLD.target_package_sha256 IS DISTINCT FROM NEW.target_package_sha256
     OR OLD.consumer_deployment_id IS DISTINCT FROM NEW.consumer_deployment_id
     OR OLD.consumer_profile_sha256 IS DISTINCT FROM NEW.consumer_profile_sha256
     OR OLD.expected_parallelism IS DISTINCT FROM NEW.expected_parallelism
     OR OLD.request_sha256 IS DISTINCT FROM NEW.request_sha256
     OR OLD.reason IS DISTINCT FROM NEW.reason
     OR OLD.requested_by IS DISTINCT FROM NEW.requested_by
     OR OLD.created_at IS DISTINCT FROM NEW.created_at THEN
    RAISE EXCEPTION 'model rollback immutable identity changed';
  END IF;

  transition_key := OLD.state || '->' || NEW.state;
  IF transition_key NOT IN (
      'PENDING_ACK->PENDING_ACK','PENDING_ACK->PARTIAL',
      'PENDING_ACK->COMPENSATING','PENDING_ACK->RECOVERED',
      'PENDING_ACK->FAILED_UNPUBLISHED',
      'PARTIAL->PARTIAL','PARTIAL->COMPENSATING','PARTIAL->RECOVERED',
      'COMPENSATING->COMPENSATING','COMPENSATING->FAILED_RESTORED',
      'COMPENSATING->COMPENSATION_FAILED',
      'RECOVERED->RECOVERED','FAILED_UNPUBLISHED->FAILED_UNPUBLISHED',
      'FAILED_RESTORED->FAILED_RESTORED',
      'COMPENSATION_FAILED->COMPENSATION_FAILED'
  ) THEN
    RAISE EXCEPTION 'invalid model rollback state transition %', transition_key;
  END IF;

  IF NEW.applied_subtasks < OLD.applied_subtasks
     OR NEW.compensation_applied_subtasks < OLD.compensation_applied_subtasks THEN
    RAISE EXCEPTION 'model rollback acknowledgement counts cannot decrease';
  END IF;
  IF OLD.compensation_event_id IS NOT NULL
     AND OLD.compensation_event_id IS DISTINCT FROM NEW.compensation_event_id THEN
    RAISE EXCEPTION 'model rollback compensation identity changed';
  END IF;
  RETURN NEW;
END;
$$;

DROP TRIGGER IF EXISTS model_rollback_state_v2 ON model_rollback_requests;
CREATE TRIGGER model_rollback_state_v2
BEFORE UPDATE OR DELETE ON model_rollback_requests
FOR EACH ROW EXECUTE FUNCTION enforce_model_rollback_state_v2();

CREATE TABLE IF NOT EXISTS alignment_schema_migrations (
    version TEXT PRIMARY KEY,
    description TEXT NOT NULL,
    applied_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    applied_by TEXT NOT NULL DEFAULT current_user
);
INSERT INTO alignment_schema_migrations(version, description)
VALUES ('202608151900', 'M08 two-phase exact-quorum model rollback v2')
ON CONFLICT (version) DO NOTHING;

COMMIT;
