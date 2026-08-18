-- M07 static/dynamic behavior-baseline authority, immutable versions and activation receipts.
-- Existing behavior_baseline_* compatibility tables remain unchanged.
BEGIN;

CREATE EXTENSION IF NOT EXISTS "uuid-ossp";

CREATE TABLE IF NOT EXISTS behavior_baseline_definitions_v1 (
  tenant_id               TEXT NOT NULL REFERENCES tenants(tenant_id) ON DELETE RESTRICT,
  baseline_id             TEXT NOT NULL,
  baseline_kind           TEXT NOT NULL CHECK (baseline_kind IN ('static','dynamic')),
  entity_type             TEXT NOT NULL,
  entity_id               TEXT NOT NULL,
  lifecycle_state         TEXT NOT NULL DEFAULT 'learning' CHECK (lifecycle_state IN ('learning','frozen','active','retired','failed')),
  revision                BIGINT NOT NULL DEFAULT 1 CHECK (revision > 0),
  next_version            BIGINT NOT NULL DEFAULT 1 CHECK (next_version > 0),
  active_version          BIGINT,
  previous_stable_version BIGINT,
  algorithm_version       TEXT NOT NULL,
  sample_policy           JSONB NOT NULL,
  threshold_spec          JSONB NOT NULL,
  expected_consumers      TEXT[] NOT NULL,
  created_by              TEXT NOT NULL,
  updated_by              TEXT NOT NULL,
  created_at              TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at              TIMESTAMPTZ NOT NULL DEFAULT now(),
  PRIMARY KEY (tenant_id,baseline_id),
  CHECK (jsonb_typeof(sample_policy)='object'),
  CHECK (jsonb_typeof(threshold_spec)='object'),
  CHECK (cardinality(expected_consumers)>0),
  CHECK (active_version IS NULL OR active_version>0),
  CHECK (previous_stable_version IS NULL OR previous_stable_version>0),
  CHECK ((lifecycle_state='active' AND active_version IS NOT NULL) OR lifecycle_state<>'active'),
  CHECK (active_version IS NULL OR previous_stable_version IS NULL OR active_version<>previous_stable_version)
);

CREATE TABLE IF NOT EXISTS behavior_baseline_sample_snapshots_v1 (
  sample_snapshot_id       UUID PRIMARY KEY,
  tenant_id                TEXT NOT NULL,
  baseline_id              TEXT NOT NULL,
  window_start             TIMESTAMPTZ NOT NULL,
  window_end               TIMESTAMPTZ NOT NULL,
  as_of                    TIMESTAMPTZ NOT NULL,
  max_event_time           TIMESTAMPTZ,
  row_count                BIGINT NOT NULL CHECK (row_count>=0),
  eligible_row_count       BIGINT NOT NULL CHECK (eligible_row_count>=0 AND eligible_row_count<=row_count),
  minimum_eligible_rows    BIGINT NOT NULL CHECK (minimum_eligible_rows>0),
  quality_status           TEXT NOT NULL CHECK (quality_status IN ('complete','partial','failed')),
  partial_reasons          TEXT[] NOT NULL DEFAULT ARRAY[]::TEXT[],
  source_watermark         JSONB NOT NULL,
  source_query_sha256      TEXT NOT NULL CHECK (source_query_sha256 ~ '^[0-9a-f]{64}$'),
  canonical_sha256         TEXT NOT NULL CHECK (canonical_sha256 ~ '^[0-9a-f]{64}$'),
  sample_object_uri        TEXT NOT NULL DEFAULT '',
  sample_object_sha256     TEXT NOT NULL DEFAULT '' CHECK (sample_object_sha256='' OR sample_object_sha256 ~ '^[0-9a-f]{64}$'),
  provenance               JSONB NOT NULL,
  created_at               TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE (tenant_id,baseline_id,sample_snapshot_id),
  FOREIGN KEY (tenant_id,baseline_id) REFERENCES behavior_baseline_definitions_v1(tenant_id,baseline_id) ON DELETE RESTRICT,
  CHECK (window_start<window_end AND as_of>=window_end),
  CHECK (max_event_time IS NULL OR max_event_time<=window_end),
  CHECK (jsonb_typeof(source_watermark)='object'),
  CHECK (jsonb_typeof(provenance)='object'),
  CHECK ((quality_status='complete' AND cardinality(partial_reasons)=0 AND eligible_row_count>=minimum_eligible_rows)
      OR quality_status<>'complete'),
  CHECK ((sample_object_uri='' AND sample_object_sha256='') OR (sample_object_uri<>'' AND sample_object_sha256<>''))
);

CREATE TABLE IF NOT EXISTS behavior_baseline_versions_v1 (
  version_id             UUID PRIMARY KEY,
  tenant_id              TEXT NOT NULL,
  baseline_id            TEXT NOT NULL,
  baseline_version       BIGINT NOT NULL CHECK (baseline_version>0),
  baseline_kind          TEXT NOT NULL CHECK (baseline_kind IN ('static','dynamic')),
  definition_revision    BIGINT NOT NULL CHECK (definition_revision>0),
  lifecycle_state        TEXT NOT NULL CHECK (lifecycle_state IN ('learning','frozen','active','retired','failed')),
  sample_snapshot_id     UUID,
  window_start           TIMESTAMPTZ,
  window_end             TIMESTAMPTZ,
  algorithm_version      TEXT NOT NULL,
  threshold_spec         JSONB NOT NULL,
  statistics             JSONB NOT NULL,
  quality_status         TEXT NOT NULL CHECK (quality_status IN ('complete','partial','failed')),
  snapshot_sha256        TEXT NOT NULL CHECK (snapshot_sha256 ~ '^[0-9a-f]{64}$'),
  candidate_sha256       TEXT NOT NULL CHECK (candidate_sha256 ~ '^[0-9a-f]{64}$'),
  rollback_of_version    BIGINT,
  created_by             TEXT NOT NULL,
  created_at             TIMESTAMPTZ NOT NULL DEFAULT now(),
  frozen_at              TIMESTAMPTZ,
  activated_at           TIMESTAMPTZ,
  retired_at             TIMESTAMPTZ,
  UNIQUE (tenant_id,baseline_id,baseline_version),
  UNIQUE (tenant_id,baseline_id,version_id),
  FOREIGN KEY (tenant_id,baseline_id) REFERENCES behavior_baseline_definitions_v1(tenant_id,baseline_id) ON DELETE RESTRICT,
  FOREIGN KEY (tenant_id,baseline_id,sample_snapshot_id) REFERENCES behavior_baseline_sample_snapshots_v1(tenant_id,baseline_id,sample_snapshot_id) ON DELETE RESTRICT,
  CHECK (jsonb_typeof(threshold_spec)='object'),
  CHECK (jsonb_typeof(statistics)='object'),
  CHECK ((window_start IS NULL AND window_end IS NULL) OR (window_start IS NOT NULL AND window_end IS NOT NULL AND window_start<window_end)),
  CHECK ((baseline_kind='dynamic' AND sample_snapshot_id IS NOT NULL AND window_start IS NOT NULL)
      OR (baseline_kind='static' AND sample_snapshot_id IS NULL)),
  CHECK ((lifecycle_state='learning' AND frozen_at IS NULL AND activated_at IS NULL AND retired_at IS NULL)
      OR (lifecycle_state='frozen' AND frozen_at IS NOT NULL AND activated_at IS NULL AND retired_at IS NULL)
      OR (lifecycle_state='active' AND frozen_at IS NOT NULL AND activated_at IS NOT NULL AND retired_at IS NULL)
      OR (lifecycle_state='retired' AND frozen_at IS NOT NULL AND retired_at IS NOT NULL)
      OR lifecycle_state='failed'),
  CHECK (lifecycle_state NOT IN ('frozen','active') OR quality_status='complete'),
  CHECK (rollback_of_version IS NULL OR (rollback_of_version>0 AND rollback_of_version<>baseline_version))
);
CREATE INDEX IF NOT EXISTS idx_behavior_baseline_versions_v1_state
  ON behavior_baseline_versions_v1(tenant_id,baseline_id,lifecycle_state,baseline_version DESC);

CREATE OR REPLACE FUNCTION protect_behavior_baseline_version_v1()
RETURNS TRIGGER AS $$
BEGIN
  IF ROW(NEW.version_id,NEW.tenant_id,NEW.baseline_id,NEW.baseline_version,NEW.baseline_kind,
         NEW.definition_revision,NEW.sample_snapshot_id,NEW.window_start,NEW.window_end,
         NEW.algorithm_version,NEW.threshold_spec,NEW.statistics,NEW.quality_status,
         NEW.snapshot_sha256,NEW.candidate_sha256,NEW.rollback_of_version,NEW.created_by,NEW.created_at)
     IS DISTINCT FROM
     ROW(OLD.version_id,OLD.tenant_id,OLD.baseline_id,OLD.baseline_version,OLD.baseline_kind,
         OLD.definition_revision,OLD.sample_snapshot_id,OLD.window_start,OLD.window_end,
         OLD.algorithm_version,OLD.threshold_spec,OLD.statistics,OLD.quality_status,
         OLD.snapshot_sha256,OLD.candidate_sha256,OLD.rollback_of_version,OLD.created_by,OLD.created_at) THEN
    RAISE EXCEPTION 'behavior baseline immutable version fields cannot change';
  END IF;
  IF OLD.lifecycle_state='retired' AND NEW IS DISTINCT FROM OLD THEN
    RAISE EXCEPTION 'retired behavior baseline version is immutable';
  END IF;
  IF NEW.lifecycle_state<>OLD.lifecycle_state AND NOT (
       (OLD.lifecycle_state='learning' AND NEW.lifecycle_state IN ('frozen','failed'))
    OR (OLD.lifecycle_state='frozen' AND NEW.lifecycle_state IN ('active','failed'))
    OR (OLD.lifecycle_state='active' AND NEW.lifecycle_state='retired')
  ) THEN
    RAISE EXCEPTION 'invalid behavior baseline version state transition: % -> %', OLD.lifecycle_state, NEW.lifecycle_state;
  END IF;
  RETURN NEW;
END;
$$ LANGUAGE plpgsql;

DO $$
BEGIN
  IF NOT EXISTS (
    SELECT 1 FROM pg_trigger
    WHERE tgname='trg_protect_behavior_baseline_version_v1'
      AND tgrelid='behavior_baseline_versions_v1'::regclass
  ) THEN
    EXECUTE 'CREATE TRIGGER trg_protect_behavior_baseline_version_v1
      BEFORE UPDATE ON behavior_baseline_versions_v1
      FOR EACH ROW EXECUTE FUNCTION protect_behavior_baseline_version_v1()';
  END IF;
END;
$$;

CREATE TABLE IF NOT EXISTS behavior_baseline_build_jobs_v1 (
  job_id                    UUID PRIMARY KEY,
  tenant_id                 TEXT NOT NULL,
  baseline_id               TEXT NOT NULL,
  baseline_kind             TEXT NOT NULL CHECK (baseline_kind IN ('static','dynamic')),
  definition_revision       BIGINT NOT NULL CHECK (definition_revision>0),
  target_version            BIGINT NOT NULL CHECK (target_version>0),
  idempotency_key           TEXT NOT NULL,
  request_sha256            TEXT NOT NULL CHECK (request_sha256 ~ '^[0-9a-f]{64}$'),
  candidate_sha256          TEXT NOT NULL CHECK (candidate_sha256 ~ '^[0-9a-f]{64}$'),
  requested_window_start    TIMESTAMPTZ,
  requested_window_end      TIMESTAMPTZ,
  status                    TEXT NOT NULL CHECK (status IN ('queued','running','succeeded','failed','cancelled')),
  result_sample_snapshot_id UUID,
  result_version_id         UUID,
  error_code                TEXT NOT NULL DEFAULT '',
  error_detail              TEXT NOT NULL DEFAULT '',
  requested_by              TEXT NOT NULL,
  reason                    TEXT NOT NULL,
  trace_id                  TEXT NOT NULL,
  created_at                TIMESTAMPTZ NOT NULL DEFAULT now(),
  started_at                TIMESTAMPTZ,
  completed_at              TIMESTAMPTZ,
  UNIQUE (tenant_id,baseline_id,idempotency_key),
  UNIQUE (tenant_id,baseline_id,target_version),
  UNIQUE (tenant_id,baseline_id,job_id),
  FOREIGN KEY (tenant_id,baseline_id) REFERENCES behavior_baseline_definitions_v1(tenant_id,baseline_id) ON DELETE RESTRICT,
  FOREIGN KEY (tenant_id,baseline_id,result_sample_snapshot_id) REFERENCES behavior_baseline_sample_snapshots_v1(tenant_id,baseline_id,sample_snapshot_id) ON DELETE RESTRICT,
  FOREIGN KEY (tenant_id,baseline_id,result_version_id) REFERENCES behavior_baseline_versions_v1(tenant_id,baseline_id,version_id) ON DELETE RESTRICT,
  CHECK ((requested_window_start IS NULL AND requested_window_end IS NULL)
      OR (requested_window_start IS NOT NULL AND requested_window_end IS NOT NULL AND requested_window_start<requested_window_end)),
  CHECK ((baseline_kind='dynamic' AND requested_window_start IS NOT NULL) OR baseline_kind='static'),
  CHECK ((status IN ('succeeded','failed','cancelled') AND completed_at IS NOT NULL) OR status NOT IN ('succeeded','failed','cancelled')),
  CHECK ((status='succeeded' AND result_version_id IS NOT NULL AND error_code=''
      AND ((baseline_kind='dynamic' AND result_sample_snapshot_id IS NOT NULL) OR baseline_kind='static')) OR status<>'succeeded'),
  CHECK ((status='failed' AND error_code<>'') OR status<>'failed')
);
CREATE INDEX IF NOT EXISTS idx_behavior_baseline_build_jobs_v1_ready
  ON behavior_baseline_build_jobs_v1(created_at,job_id) WHERE status='queued';

CREATE TABLE IF NOT EXISTS behavior_baseline_approval_requests_v1 (
  approval_id          UUID PRIMARY KEY,
  tenant_id            TEXT NOT NULL,
  baseline_id          TEXT NOT NULL,
  baseline_version     BIGINT NOT NULL,
  definition_revision  BIGINT NOT NULL CHECK (definition_revision>0),
  snapshot_sha256      TEXT NOT NULL CHECK (snapshot_sha256 ~ '^[0-9a-f]{64}$'),
  candidate_sha256     TEXT NOT NULL CHECK (candidate_sha256 ~ '^[0-9a-f]{64}$'),
  status               TEXT NOT NULL CHECK (status IN ('pending','approved','rejected','expired','consumed','revoked')),
  requested_by         TEXT NOT NULL,
  decided_by           TEXT,
  reason               TEXT NOT NULL,
  requested_at         TIMESTAMPTZ NOT NULL DEFAULT now(),
  expires_at           TIMESTAMPTZ NOT NULL,
  decided_at           TIMESTAMPTZ,
  consumed_at          TIMESTAMPTZ,
  UNIQUE (tenant_id,baseline_id,baseline_version,approval_id),
  FOREIGN KEY (tenant_id,baseline_id,baseline_version) REFERENCES behavior_baseline_versions_v1(tenant_id,baseline_id,baseline_version) ON DELETE RESTRICT,
  CHECK (expires_at>requested_at),
  CHECK ((status='pending' AND decided_by IS NULL AND decided_at IS NULL AND consumed_at IS NULL)
      OR (status IN ('approved','rejected','revoked') AND decided_by IS NOT NULL AND decided_at IS NOT NULL AND consumed_at IS NULL)
      OR (status='consumed' AND decided_by IS NOT NULL AND decided_at IS NOT NULL AND consumed_at IS NOT NULL)
      OR status='expired'),
  CHECK (decided_by IS NULL OR decided_by<>requested_by)
);

CREATE TABLE IF NOT EXISTS behavior_baseline_activation_targets_v1 (
  tenant_id           TEXT NOT NULL,
  baseline_id         TEXT NOT NULL,
  baseline_version    BIGINT NOT NULL,
  consumer_id         TEXT NOT NULL,
  required            BOOLEAN NOT NULL DEFAULT true,
  status              TEXT NOT NULL CHECK (status IN ('pending','acked','failed','revoked')),
  candidate_sha256    TEXT NOT NULL CHECK (candidate_sha256 ~ '^[0-9a-f]{64}$'),
  ack_event_id        UUID UNIQUE,
  ack_sha256          TEXT NOT NULL DEFAULT '' CHECK (ack_sha256='' OR ack_sha256 ~ '^[0-9a-f]{64}$'),
  error_detail        TEXT NOT NULL DEFAULT '',
  acknowledged_at     TIMESTAMPTZ,
  updated_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
  PRIMARY KEY (tenant_id,baseline_id,baseline_version,consumer_id),
  FOREIGN KEY (tenant_id,baseline_id,baseline_version) REFERENCES behavior_baseline_versions_v1(tenant_id,baseline_id,baseline_version) ON DELETE RESTRICT,
  CHECK ((status='acked' AND ack_event_id IS NOT NULL AND ack_sha256<>'' AND acknowledged_at IS NOT NULL AND error_detail='')
      OR (status='failed' AND ack_event_id IS NOT NULL AND error_detail<>'' AND acknowledged_at IS NOT NULL)
      OR status IN ('pending','revoked'))
);

CREATE TABLE IF NOT EXISTS behavior_baseline_ack_readiness_history_v1 (
  receipt_id       UUID PRIMARY KEY,
  pipeline_id      TEXT NOT NULL CHECK (pipeline_id='baseline-activation-ack-v1'),
  consumer_group   TEXT NOT NULL CHECK (consumer_group='alert-service-baseline-activation-acks-v1'),
  observed_topic   TEXT NOT NULL CHECK (observed_topic='baseline.activation-acks.v1'),
  candidate_sha256 TEXT NOT NULL CHECK (candidate_sha256 ~ '^[0-9a-f]{64}$'),
  owner_id         TEXT NOT NULL,
  owner_epoch      BIGINT NOT NULL CHECK (owner_epoch>0),
  generation_id    INTEGER NOT NULL CHECK (generation_id>=0),
  state            TEXT NOT NULL CHECK (state IN ('ASSIGNED','READY','REVOKED','STOPPED')),
  assignments      JSONB NOT NULL,
  observed_at      TIMESTAMPTZ NOT NULL,
  lease_expires_at TIMESTAMPTZ,
  created_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
  CHECK (jsonb_typeof(assignments)='array'),
  CHECK ((state='READY' AND lease_expires_at>observed_at) OR (state<>'READY' AND lease_expires_at IS NULL))
);

CREATE TABLE IF NOT EXISTS behavior_baseline_ack_readiness_current_v1 (
  pipeline_id      TEXT NOT NULL CHECK (pipeline_id='baseline-activation-ack-v1'),
  consumer_group   TEXT NOT NULL CHECK (consumer_group='alert-service-baseline-activation-acks-v1'),
  observed_topic   TEXT NOT NULL CHECK (observed_topic='baseline.activation-acks.v1'),
  candidate_sha256 TEXT NOT NULL CHECK (candidate_sha256 ~ '^[0-9a-f]{64}$'),
  receipt_id       UUID NOT NULL REFERENCES behavior_baseline_ack_readiness_history_v1(receipt_id) ON DELETE RESTRICT,
  owner_id         TEXT NOT NULL,
  owner_epoch      BIGINT NOT NULL CHECK (owner_epoch>0),
  generation_id    INTEGER NOT NULL CHECK (generation_id>=0),
  state            TEXT NOT NULL CHECK (state IN ('ASSIGNED','READY','REVOKED','STOPPED')),
  assignments      JSONB NOT NULL,
  observed_at      TIMESTAMPTZ NOT NULL,
  lease_expires_at TIMESTAMPTZ,
  updated_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
  PRIMARY KEY (pipeline_id,consumer_group),
  CHECK (jsonb_typeof(assignments)='array'),
  CHECK ((state='READY' AND lease_expires_at>observed_at) OR (state<>'READY' AND lease_expires_at IS NULL))
);

CREATE TABLE IF NOT EXISTS behavior_baseline_lifecycle_history_v1 (
  history_id          UUID PRIMARY KEY,
  tenant_id           TEXT NOT NULL,
  baseline_id         TEXT NOT NULL,
  definition_revision BIGINT NOT NULL CHECK (definition_revision>0),
  baseline_version    BIGINT,
  from_state          TEXT,
  to_state            TEXT NOT NULL CHECK (to_state IN ('learning','frozen','active','retired','failed')),
  event_type          TEXT NOT NULL,
  reason              TEXT NOT NULL,
  actor_id             TEXT NOT NULL,
  trace_id             TEXT NOT NULL,
  metadata             JSONB NOT NULL,
  created_at           TIMESTAMPTZ NOT NULL DEFAULT now(),
  FOREIGN KEY (tenant_id,baseline_id) REFERENCES behavior_baseline_definitions_v1(tenant_id,baseline_id) ON DELETE RESTRICT,
  CHECK (jsonb_typeof(metadata)='object'),
  CHECK (from_state IS NULL OR from_state IN ('learning','frozen','active','retired','failed'))
);

CREATE TABLE IF NOT EXISTS behavior_baseline_lifecycle_outbox_v1 (
  outbox_sequence   BIGSERIAL NOT NULL UNIQUE,
  event_id          UUID PRIMARY KEY,
  tenant_id         TEXT NOT NULL,
  baseline_id       TEXT NOT NULL,
  aggregate_type    TEXT NOT NULL CHECK (aggregate_type IN ('baseline_build_job','baseline_version')),
  aggregate_id      TEXT NOT NULL,
  aggregate_version BIGINT NOT NULL CHECK (aggregate_version>0),
  event_type        TEXT NOT NULL,
  partition_key     TEXT NOT NULL,
  payload           JSONB NOT NULL,
  payload_sha256    TEXT NOT NULL CHECK (payload_sha256 ~ '^[0-9a-f]{64}$'),
  publish_state     TEXT NOT NULL DEFAULT 'PENDING' CHECK (publish_state IN ('PENDING','OUTCOME_UNKNOWN','KAFKA_ACKED')),
  broker_topic      TEXT NOT NULL DEFAULT '',
  broker_partition  INTEGER,
  broker_offset     BIGINT,
  attempts          INTEGER NOT NULL DEFAULT 0 CHECK (attempts>=0),
  next_attempt_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
  claim_token       UUID,
  claimed_at        TIMESTAMPTZ,
  last_error        TEXT NOT NULL DEFAULT '',
  trace_id          TEXT NOT NULL,
  created_at        TIMESTAMPTZ NOT NULL DEFAULT now(),
  acked_at          TIMESTAMPTZ,
  UNIQUE (tenant_id,aggregate_type,aggregate_id,aggregate_version,event_type),
  FOREIGN KEY (tenant_id,baseline_id) REFERENCES behavior_baseline_definitions_v1(tenant_id,baseline_id) ON DELETE RESTRICT,
  CHECK (jsonb_typeof(payload)='object'),
  CHECK ((publish_state='OUTCOME_UNKNOWN' AND claim_token IS NOT NULL AND claimed_at IS NOT NULL)
      OR (publish_state<>'OUTCOME_UNKNOWN' AND claim_token IS NULL AND claimed_at IS NULL)),
  CHECK ((publish_state='KAFKA_ACKED' AND broker_topic<>'' AND broker_partition IS NOT NULL AND broker_offset IS NOT NULL AND acked_at IS NOT NULL)
      OR publish_state<>'KAFKA_ACKED')
);
CREATE INDEX IF NOT EXISTS idx_behavior_baseline_lifecycle_outbox_v1_ready
  ON behavior_baseline_lifecycle_outbox_v1(next_attempt_at,outbox_sequence) WHERE publish_state IN ('PENDING','OUTCOME_UNKNOWN');
CREATE INDEX IF NOT EXISTS idx_behavior_baseline_lifecycle_outbox_v1_partition_order
  ON behavior_baseline_lifecycle_outbox_v1(partition_key,outbox_sequence) WHERE publish_state<>'KAFKA_ACKED';

CREATE TABLE IF NOT EXISTS behavior_baseline_command_receipts_v1 (
  tenant_id          TEXT NOT NULL,
  idempotency_key    TEXT NOT NULL,
  command_type       TEXT NOT NULL,
  request_sha256     TEXT NOT NULL CHECK (request_sha256 ~ '^[0-9a-f]{64}$'),
  response_status    INTEGER NOT NULL CHECK (response_status BETWEEN 200 AND 299),
  response_body      JSONB NOT NULL,
  created_at         TIMESTAMPTZ NOT NULL DEFAULT now(),
  PRIMARY KEY (tenant_id,idempotency_key),
  CHECK (jsonb_typeof(response_body)='object')
);

CREATE TABLE IF NOT EXISTS behavior_baseline_drift_evaluations_v1 (
  evaluation_id       UUID PRIMARY KEY,
  tenant_id           TEXT NOT NULL,
  baseline_id         TEXT NOT NULL,
  baseline_version    BIGINT,
  snapshot_sha256     TEXT NOT NULL DEFAULT '' CHECK (snapshot_sha256='' OR snapshot_sha256 ~ '^[0-9a-f]{64}$'),
  metric_name         TEXT NOT NULL,
  observed_value      DOUBLE PRECISION NOT NULL,
  observed_at         TIMESTAMPTZ NOT NULL,
  window_start        TIMESTAMPTZ,
  window_end          TIMESTAMPTZ,
  mean_value          DOUBLE PRECISION,
  stddev_value        DOUBLE PRECISION,
  deviation_score     DOUBLE PRECISION,
  warning_threshold   DOUBLE PRECISION,
  alert_threshold     DOUBLE PRECISION,
  disposition         TEXT NOT NULL CHECK (disposition IN ('normal','warning','alert','missing','partial','stale','failed')),
  quality_status      TEXT NOT NULL CHECK (quality_status IN ('complete','partial','failed','unavailable','stale')),
  failure_code        TEXT NOT NULL DEFAULT '',
  evidence_refs       JSONB NOT NULL,
  provenance          JSONB NOT NULL,
  trace_id            TEXT NOT NULL,
  created_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
  FOREIGN KEY (tenant_id,baseline_id) REFERENCES behavior_baseline_definitions_v1(tenant_id,baseline_id) ON DELETE RESTRICT,
  FOREIGN KEY (tenant_id,baseline_id,baseline_version) REFERENCES behavior_baseline_versions_v1(tenant_id,baseline_id,baseline_version) ON DELETE RESTRICT,
  CHECK ((window_start IS NULL AND window_end IS NULL) OR (window_start IS NOT NULL AND window_end IS NOT NULL AND window_start<window_end)),
  CHECK (jsonb_typeof(evidence_refs)='array'),
  CHECK (jsonb_typeof(provenance)='object'),
  CHECK ((disposition IN ('normal','warning','alert') AND baseline_version IS NOT NULL AND snapshot_sha256<>''
      AND deviation_score IS NOT NULL AND quality_status='complete' AND failure_code='')
      OR disposition NOT IN ('normal','warning','alert')),
  CHECK ((disposition IN ('missing','partial','stale','failed') AND failure_code<>'') OR disposition NOT IN ('missing','partial','stale','failed'))
);
CREATE INDEX IF NOT EXISTS idx_behavior_baseline_drift_evaluations_v1_time
  ON behavior_baseline_drift_evaluations_v1(tenant_id,baseline_id,observed_at DESC);

INSERT INTO alignment_schema_migrations(version,description)
VALUES ('202608141800','M07 static and dynamic behavior baseline authority')
ON CONFLICT(version) DO NOTHING;

COMMIT;
