-- M07 three-level fusion snapshots, append-only resolution history and projection delivery.
-- Existing fusion workbench tables remain compatibility read/write surfaces.
BEGIN;

CREATE EXTENSION IF NOT EXISTS "uuid-ossp";

CREATE TABLE IF NOT EXISTS fusion_source_snapshots (
  snapshot_id       UUID PRIMARY KEY,
  tenant_id         TEXT NOT NULL REFERENCES tenants(tenant_id) ON DELETE RESTRICT,
  source_id         TEXT NOT NULL,
  source_kind       TEXT NOT NULL CHECK (source_kind IN ('flow','asset','device_log','user_event','threat_intel','analyst','other')),
  source_cursor     JSONB NOT NULL,
  as_of             TIMESTAMPTZ NOT NULL,
  window_start      TIMESTAMPTZ NOT NULL,
  window_end        TIMESTAMPTZ NOT NULL,
  source_version    BIGINT NOT NULL CHECK (source_version > 0),
  quality_status    TEXT NOT NULL CHECK (quality_status IN ('complete','partial','failed')),
  partial_reasons   TEXT[] NOT NULL DEFAULT ARRAY[]::TEXT[],
  row_count         BIGINT NOT NULL CHECK (row_count >= 0),
  canonical_sha256  TEXT NOT NULL CHECK (canonical_sha256 ~ '^[0-9a-f]{64}$'),
  provenance        JSONB NOT NULL,
  trace_id          TEXT NOT NULL,
  created_at        TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE (tenant_id,snapshot_id),
  UNIQUE (tenant_id,source_id,source_version),
  CHECK (jsonb_typeof(source_cursor)='object'),
  CHECK (jsonb_typeof(provenance)='object'),
  CHECK (window_start < window_end AND as_of >= window_end),
  CHECK ((quality_status='complete' AND cardinality(partial_reasons)=0) OR quality_status<>'complete')
);
CREATE INDEX IF NOT EXISTS idx_fusion_source_snapshots_as_of
  ON fusion_source_snapshots(tenant_id,source_id,window_start,window_end,as_of DESC,source_version DESC);

CREATE TABLE IF NOT EXISTS fusion_source_entity_facts (
  tenant_id           TEXT NOT NULL,
  source_snapshot_id  UUID NOT NULL,
  source_entity_id    TEXT NOT NULL,
  entity_kind         TEXT NOT NULL CHECK (entity_kind IN ('ip','asset','device','user','host','service','unknown')),
  identifiers         JSONB NOT NULL,
  evidence_event_ids  JSONB NOT NULL,
  provenance          JSONB NOT NULL,
  canonical_sha256    TEXT NOT NULL CHECK (canonical_sha256 ~ '^[0-9a-f]{64}$'),
  created_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
  PRIMARY KEY (tenant_id,source_snapshot_id,source_entity_id),
  FOREIGN KEY (tenant_id,source_snapshot_id) REFERENCES fusion_source_snapshots(tenant_id,snapshot_id) ON DELETE RESTRICT,
  CHECK (jsonb_typeof(identifiers)='object'),
  CHECK (jsonb_typeof(evidence_event_ids)='array'),
  CHECK (jsonb_typeof(provenance)='object')
);

CREATE TABLE IF NOT EXISTS fusion_source_relation_facts (
  tenant_id              TEXT NOT NULL,
  source_snapshot_id     UUID NOT NULL,
  source_relation_id     TEXT NOT NULL,
  source_entity_id       TEXT NOT NULL,
  target_entity_id       TEXT NOT NULL,
  relation_kind          TEXT NOT NULL,
  event_time             TIMESTAMPTZ NOT NULL,
  evidence_event_ids     JSONB NOT NULL,
  provenance             JSONB NOT NULL,
  canonical_sha256       TEXT NOT NULL CHECK (canonical_sha256 ~ '^[0-9a-f]{64}$'),
  created_at             TIMESTAMPTZ NOT NULL DEFAULT now(),
  PRIMARY KEY (tenant_id,source_snapshot_id,source_relation_id),
  FOREIGN KEY (tenant_id,source_snapshot_id,source_entity_id)
    REFERENCES fusion_source_entity_facts(tenant_id,source_snapshot_id,source_entity_id) ON DELETE RESTRICT,
  FOREIGN KEY (tenant_id,source_snapshot_id,target_entity_id)
    REFERENCES fusion_source_entity_facts(tenant_id,source_snapshot_id,source_entity_id) ON DELETE RESTRICT,
  CHECK (source_entity_id <> target_entity_id),
  CHECK (jsonb_typeof(evidence_event_ids)='array'),
  CHECK (jsonb_typeof(provenance)='object')
);

CREATE TABLE IF NOT EXISTS fusion_snapshots (
  snapshot_id          UUID PRIMARY KEY,
  tenant_id            TEXT NOT NULL REFERENCES tenants(tenant_id) ON DELETE RESTRICT,
  fusion_level         TEXT NOT NULL CHECK (fusion_level IN ('data','feature','knowledge')),
  snapshot_version     BIGINT NOT NULL CHECK (snapshot_version > 0),
  as_of                TIMESTAMPTZ NOT NULL,
  window_start         TIMESTAMPTZ,
  window_end           TIMESTAMPTZ,
  status               TEXT NOT NULL CHECK (status IN ('complete','partial','failed','withdrawn')),
  partial_sources      TEXT[] NOT NULL DEFAULT ARRAY[]::TEXT[],
  entity_count         BIGINT NOT NULL CHECK (entity_count >= 0),
  relation_count       BIGINT NOT NULL DEFAULT 0 CHECK (relation_count >= 0),
  canonical_sha256     TEXT NOT NULL CHECK (canonical_sha256 ~ '^[0-9a-f]{64}$'),
  quality_summary      JSONB NOT NULL,
  provenance           JSONB NOT NULL,
  rollback_snapshot_id UUID,
  trace_id             TEXT NOT NULL,
  created_by           TEXT NOT NULL,
  created_at           TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE (tenant_id,snapshot_id),
  UNIQUE (tenant_id,fusion_level,snapshot_version),
  FOREIGN KEY (tenant_id,rollback_snapshot_id) REFERENCES fusion_snapshots(tenant_id,snapshot_id) ON DELETE RESTRICT,
  CHECK (jsonb_typeof(quality_summary)='object'),
  CHECK (jsonb_typeof(provenance)='object'),
  CHECK ((window_start IS NULL AND window_end IS NULL) OR (window_start IS NOT NULL AND window_end IS NOT NULL AND window_start < window_end AND as_of >= window_end)),
  CHECK ((status='complete' AND cardinality(partial_sources)=0) OR status<>'complete'),
  CHECK (rollback_snapshot_id IS NULL OR rollback_snapshot_id <> snapshot_id)
);
CREATE INDEX IF NOT EXISTS idx_fusion_snapshots_level_version
  ON fusion_snapshots(tenant_id,fusion_level,snapshot_version DESC);

CREATE TABLE IF NOT EXISTS fusion_source_sync_jobs (
  job_id                   UUID PRIMARY KEY,
  tenant_id                TEXT NOT NULL REFERENCES tenants(tenant_id) ON DELETE RESTRICT,
  source_id                TEXT NOT NULL CHECK (source_id IN ('traffic','asset','log','behavior')),
  source_kind              TEXT NOT NULL CHECK (source_kind IN ('flow','asset','device_log','user_event')),
  idempotency_key          TEXT NOT NULL,
  idempotency_request_sha256 TEXT NOT NULL CHECK (idempotency_request_sha256 ~ '^[0-9a-f]{64}$'),
  request_sha256           TEXT NOT NULL CHECK (request_sha256 ~ '^[0-9a-f]{64}$'),
  requested_window_start   TIMESTAMPTZ NOT NULL,
  requested_window_end     TIMESTAMPTZ NOT NULL,
  expected_source_version  BIGINT CHECK (expected_source_version IS NULL OR expected_source_version > 0),
  status                   TEXT NOT NULL CHECK (status IN ('queued','running','succeeded','failed')),
  revision                 BIGINT NOT NULL DEFAULT 1 CHECK (revision > 0),
  reason                   TEXT NOT NULL,
  requested_by             TEXT NOT NULL,
  trace_id                 TEXT NOT NULL,
  result_source_snapshot_id UUID,
  result_data_snapshot_id  UUID,
  result_feature_snapshot_id UUID,
  error_code               TEXT NOT NULL DEFAULT '',
  error_detail             TEXT NOT NULL DEFAULT '',
  created_at               TIMESTAMPTZ NOT NULL DEFAULT now(),
  started_at               TIMESTAMPTZ,
  completed_at             TIMESTAMPTZ,
  UNIQUE (tenant_id,idempotency_key),
  UNIQUE (tenant_id,job_id),
  FOREIGN KEY (tenant_id,result_source_snapshot_id) REFERENCES fusion_source_snapshots(tenant_id,snapshot_id) ON DELETE RESTRICT,
  FOREIGN KEY (tenant_id,result_data_snapshot_id) REFERENCES fusion_snapshots(tenant_id,snapshot_id) ON DELETE RESTRICT,
  FOREIGN KEY (tenant_id,result_feature_snapshot_id) REFERENCES fusion_snapshots(tenant_id,snapshot_id) ON DELETE RESTRICT,
  CHECK (requested_window_start < requested_window_end),
  CHECK ((status='queued' AND started_at IS NULL AND completed_at IS NULL) OR status<>'queued'),
  CHECK ((status IN ('succeeded','failed') AND completed_at IS NOT NULL) OR status NOT IN ('succeeded','failed')),
  CHECK ((status='succeeded' AND result_source_snapshot_id IS NOT NULL AND result_data_snapshot_id IS NOT NULL AND result_feature_snapshot_id IS NOT NULL AND error_code='') OR status<>'succeeded'),
  CHECK ((status='failed' AND error_code<>'') OR status<>'failed')
);

CREATE INDEX IF NOT EXISTS idx_fusion_source_sync_jobs_ready
  ON fusion_source_sync_jobs(created_at,job_id) WHERE status='queued';

CREATE TABLE IF NOT EXISTS fusion_projection_readiness_history (
  receipt_id          UUID PRIMARY KEY,
  pipeline_id         TEXT NOT NULL CHECK (pipeline_id='fusion-projection-v1'),
  consumer_group      TEXT NOT NULL,
  observed_topic      TEXT NOT NULL CHECK (observed_topic='fusion.commands.v1'),
  candidate_sha256    TEXT NOT NULL CHECK (candidate_sha256 ~ '^[0-9a-f]{64}$'),
  owner_id            TEXT NOT NULL,
  owner_epoch         BIGINT NOT NULL CHECK (owner_epoch > 0),
  generation_id       INTEGER NOT NULL CHECK (generation_id >= 0),
  state               TEXT NOT NULL CHECK (state IN ('ASSIGNED','READY','REVOKED','STOPPED')),
  assignments         JSONB NOT NULL,
  observed_at         TIMESTAMPTZ NOT NULL,
  lease_expires_at    TIMESTAMPTZ,
  created_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE (pipeline_id,consumer_group,owner_id,owner_epoch,state,observed_at),
  CHECK (jsonb_typeof(assignments)='array'),
  CHECK ((state='READY' AND lease_expires_at>observed_at) OR (state<>'READY' AND lease_expires_at IS NULL))
);

CREATE TABLE IF NOT EXISTS fusion_projection_readiness_current (
  pipeline_id         TEXT NOT NULL CHECK (pipeline_id='fusion-projection-v1'),
  consumer_group      TEXT NOT NULL,
  observed_topic      TEXT NOT NULL CHECK (observed_topic='fusion.commands.v1'),
  candidate_sha256    TEXT NOT NULL CHECK (candidate_sha256 ~ '^[0-9a-f]{64}$'),
  receipt_id          UUID NOT NULL REFERENCES fusion_projection_readiness_history(receipt_id) ON DELETE RESTRICT,
  owner_id            TEXT NOT NULL,
  owner_epoch         BIGINT NOT NULL CHECK (owner_epoch > 0),
  generation_id       INTEGER NOT NULL CHECK (generation_id >= 0),
  state               TEXT NOT NULL CHECK (state IN ('ASSIGNED','READY','REVOKED','STOPPED')),
  assignments         JSONB NOT NULL,
  observed_at         TIMESTAMPTZ NOT NULL,
  lease_expires_at    TIMESTAMPTZ,
  updated_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
  PRIMARY KEY (pipeline_id,consumer_group),
  CHECK (jsonb_typeof(assignments)='array'),
  CHECK ((state='READY' AND lease_expires_at>observed_at) OR (state<>'READY' AND lease_expires_at IS NULL))
);

CREATE TABLE IF NOT EXISTS fusion_snapshot_entities (
  tenant_id            TEXT NOT NULL,
  fusion_snapshot_id   UUID NOT NULL,
  entity_id            TEXT NOT NULL,
  entity_kind          TEXT NOT NULL CHECK (entity_kind IN ('ip','asset','device','user','host','service','unknown')),
  identifiers          JSONB NOT NULL,
  source_entity_refs   JSONB NOT NULL,
  source_count         INTEGER NOT NULL CHECK (source_count > 0),
  confidence           DOUBLE PRECISION NOT NULL CHECK (confidence >= 0 AND confidence <= 1),
  provenance           JSONB NOT NULL,
  canonical_sha256     TEXT NOT NULL CHECK (canonical_sha256 ~ '^[0-9a-f]{64}$'),
  created_at           TIMESTAMPTZ NOT NULL DEFAULT now(),
  PRIMARY KEY (tenant_id,fusion_snapshot_id,entity_id),
  FOREIGN KEY (tenant_id,fusion_snapshot_id) REFERENCES fusion_snapshots(tenant_id,snapshot_id) ON DELETE RESTRICT,
  CHECK (jsonb_typeof(identifiers)='object'),
  CHECK (jsonb_typeof(source_entity_refs)='array'),
  CHECK (jsonb_typeof(provenance)='object')
);

CREATE TABLE IF NOT EXISTS fusion_snapshot_relations (
  tenant_id            TEXT NOT NULL,
  fusion_snapshot_id   UUID NOT NULL,
  relation_id          TEXT NOT NULL,
  source_entity_id     TEXT NOT NULL,
  target_entity_id     TEXT NOT NULL,
  relation_kind        TEXT NOT NULL,
  edge_origin          TEXT NOT NULL CHECK (edge_origin IN ('observed','derived','analyst')),
  event_time           TIMESTAMPTZ NOT NULL,
  confidence           DOUBLE PRECISION NOT NULL CHECK (confidence >= 0 AND confidence <= 1),
  evidence_refs        JSONB NOT NULL,
  provenance           JSONB NOT NULL,
  canonical_sha256     TEXT NOT NULL CHECK (canonical_sha256 ~ '^[0-9a-f]{64}$'),
  created_at           TIMESTAMPTZ NOT NULL DEFAULT now(),
  PRIMARY KEY (tenant_id,fusion_snapshot_id,relation_id),
  FOREIGN KEY (tenant_id,fusion_snapshot_id,source_entity_id)
    REFERENCES fusion_snapshot_entities(tenant_id,fusion_snapshot_id,entity_id) ON DELETE RESTRICT,
  FOREIGN KEY (tenant_id,fusion_snapshot_id,target_entity_id)
    REFERENCES fusion_snapshot_entities(tenant_id,fusion_snapshot_id,entity_id) ON DELETE RESTRICT,
  CHECK (source_entity_id <> target_entity_id),
  CHECK (jsonb_typeof(evidence_refs)='array'),
  CHECK (jsonb_typeof(provenance)='object')
);

CREATE TABLE IF NOT EXISTS fusion_feature_metrics (
  tenant_id            TEXT NOT NULL,
  feature_snapshot_id  UUID NOT NULL,
  metric_name          TEXT NOT NULL,
  metric_value         DOUBLE PRECISION NOT NULL,
  metric_unit          TEXT NOT NULL,
  metric_semantics     TEXT NOT NULL,
  canonical_sha256     TEXT NOT NULL CHECK (canonical_sha256 ~ '^[0-9a-f]{64}$'),
  created_at           TIMESTAMPTZ NOT NULL DEFAULT now(),
  PRIMARY KEY (tenant_id,feature_snapshot_id,metric_name),
  FOREIGN KEY (tenant_id,feature_snapshot_id) REFERENCES fusion_snapshots(tenant_id,snapshot_id) ON DELETE RESTRICT
);

CREATE TABLE IF NOT EXISTS fusion_feature_ablation_results (
  tenant_id              TEXT NOT NULL,
  feature_snapshot_id    UUID NOT NULL,
  omitted_source_id      TEXT NOT NULL CHECK (omitted_source_id IN ('traffic','asset','log','behavior')),
  status                 TEXT NOT NULL CHECK (status IN ('complete','partial','not_applicable')),
  included_source_count  INTEGER NOT NULL CHECK (included_source_count >= 0),
  entity_count           BIGINT NOT NULL CHECK (entity_count >= 0),
  relation_count         BIGINT NOT NULL CHECK (relation_count >= 0),
  entity_delta           BIGINT NOT NULL,
  relation_delta         BIGINT NOT NULL,
  result                 JSONB NOT NULL,
  canonical_sha256       TEXT NOT NULL CHECK (canonical_sha256 ~ '^[0-9a-f]{64}$'),
  created_at             TIMESTAMPTZ NOT NULL DEFAULT now(),
  PRIMARY KEY (tenant_id,feature_snapshot_id,omitted_source_id),
  FOREIGN KEY (tenant_id,feature_snapshot_id) REFERENCES fusion_snapshots(tenant_id,snapshot_id) ON DELETE RESTRICT,
  CHECK (jsonb_typeof(result)='object')
);

CREATE TABLE IF NOT EXISTS fusion_snapshot_sources (
  tenant_id          TEXT NOT NULL,
  fusion_snapshot_id UUID NOT NULL,
  source_snapshot_id UUID NOT NULL,
  source_role        TEXT NOT NULL CHECK (source_role IN ('primary','supporting','ablation_control')),
  included           BOOLEAN NOT NULL,
  exclusion_reason   TEXT NOT NULL DEFAULT '',
  source_order       INTEGER NOT NULL CHECK (source_order >= 0),
  PRIMARY KEY (tenant_id,fusion_snapshot_id,source_snapshot_id),
  UNIQUE (tenant_id,fusion_snapshot_id,source_order),
  FOREIGN KEY (tenant_id,fusion_snapshot_id) REFERENCES fusion_snapshots(tenant_id,snapshot_id) ON DELETE RESTRICT,
  FOREIGN KEY (tenant_id,source_snapshot_id) REFERENCES fusion_source_snapshots(tenant_id,snapshot_id) ON DELETE RESTRICT,
  CHECK (included OR exclusion_reason <> '')
);
CREATE INDEX IF NOT EXISTS idx_fusion_snapshot_sources_source
  ON fusion_snapshot_sources(tenant_id,source_snapshot_id,fusion_snapshot_id);

CREATE TABLE IF NOT EXISTS fusion_resolution_history (
  resolution_id         UUID PRIMARY KEY,
  tenant_id              TEXT NOT NULL,
  conflict_id            TEXT NOT NULL,
  resolution_version     BIGINT NOT NULL CHECK (resolution_version > 0),
  conflict_state_version BIGINT NOT NULL CHECK (conflict_state_version > 0),
  lifecycle_state        TEXT NOT NULL CHECK (lifecycle_state IN ('applied','revoked','superseded')),
  selected_source        TEXT NOT NULL,
  selected_value         TEXT NOT NULL,
  strategy               TEXT NOT NULL CHECK (strategy IN ('authoritative-source','manual-repair-task','accept-primary','revoke')),
  reason                 TEXT NOT NULL,
  source_snapshot_id     UUID,
  supersedes_id          UUID REFERENCES fusion_resolution_history(resolution_id) ON DELETE RESTRICT,
  provenance             JSONB NOT NULL,
  resolved_by            TEXT NOT NULL,
  trace_id               TEXT NOT NULL,
  created_at             TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE (tenant_id,conflict_id,resolution_version),
  FOREIGN KEY (tenant_id,conflict_id) REFERENCES fusion_conflicts(tenant_id,conflict_id) ON DELETE RESTRICT,
  FOREIGN KEY (tenant_id,source_snapshot_id) REFERENCES fusion_source_snapshots(tenant_id,snapshot_id) ON DELETE RESTRICT,
  CHECK (jsonb_typeof(provenance)='object'),
  CHECK ((resolution_version=1 AND supersedes_id IS NULL) OR (resolution_version>1 AND supersedes_id IS NOT NULL))
);
CREATE INDEX IF NOT EXISTS idx_fusion_resolution_history_current
  ON fusion_resolution_history(tenant_id,conflict_id,resolution_version DESC);

CREATE TABLE IF NOT EXISTS fusion_projection_outbox (
  event_id         UUID PRIMARY KEY,
  tenant_id        TEXT NOT NULL REFERENCES tenants(tenant_id) ON DELETE RESTRICT,
  aggregate_type   TEXT NOT NULL CHECK (aggregate_type IN ('source_sync_job','source_snapshot','fusion_snapshot','resolution')),
  aggregate_id     UUID NOT NULL,
  aggregate_version BIGINT NOT NULL CHECK (aggregate_version > 0),
  event_type       TEXT NOT NULL,
  partition_key    TEXT NOT NULL,
  payload          JSONB NOT NULL,
  payload_sha256   TEXT NOT NULL CHECK (payload_sha256 ~ '^[0-9a-f]{64}$'),
  publish_state    TEXT NOT NULL DEFAULT 'PENDING' CHECK (publish_state IN ('PENDING','OUTCOME_UNKNOWN','KAFKA_ACKED')),
  broker_topic     TEXT NOT NULL DEFAULT '',
  broker_partition INTEGER,
  broker_offset    BIGINT,
  attempts         INTEGER NOT NULL DEFAULT 0 CHECK (attempts >= 0),
  next_attempt_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
  claim_token      UUID,
  claimed_at       TIMESTAMPTZ,
  last_error       TEXT NOT NULL DEFAULT '',
  trace_id         TEXT NOT NULL,
  created_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
  acked_at         TIMESTAMPTZ,
  UNIQUE (tenant_id,aggregate_type,aggregate_id,aggregate_version,event_type),
  CHECK (jsonb_typeof(payload)='object'),
  CHECK ((publish_state='OUTCOME_UNKNOWN' AND claim_token IS NOT NULL AND claimed_at IS NOT NULL)
      OR (publish_state<>'OUTCOME_UNKNOWN' AND claim_token IS NULL AND claimed_at IS NULL)),
  CHECK ((publish_state='KAFKA_ACKED' AND broker_topic<>'' AND broker_partition IS NOT NULL AND broker_offset IS NOT NULL AND acked_at IS NOT NULL) OR publish_state<>'KAFKA_ACKED')
);
CREATE INDEX IF NOT EXISTS idx_fusion_projection_outbox_ready
  ON fusion_projection_outbox(next_attempt_at,created_at) WHERE publish_state IN ('PENDING','OUTCOME_UNKNOWN');

CREATE TABLE IF NOT EXISTS fusion_projection_inbox (
  event_id             UUID PRIMARY KEY,
  tenant_id            TEXT NOT NULL,
  job_id               UUID NOT NULL,
  source_id            TEXT NOT NULL,
  event_sha256         TEXT NOT NULL CHECK (event_sha256 ~ '^[0-9a-f]{64}$'),
  source_topic         TEXT NOT NULL,
  source_partition     INTEGER NOT NULL CHECK (source_partition >= 0),
  source_offset        BIGINT NOT NULL CHECK (source_offset >= 0),
  disposition          TEXT NOT NULL CHECK (disposition IN ('applied','failed')),
  failure_code         TEXT NOT NULL DEFAULT '',
  source_snapshot_id   UUID,
  data_snapshot_id     UUID,
  feature_snapshot_id  UUID,
  trace_id             TEXT NOT NULL,
  applied_at           TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE (source_topic,source_partition,source_offset),
  UNIQUE (tenant_id,job_id),
  FOREIGN KEY (tenant_id,job_id) REFERENCES fusion_source_sync_jobs(tenant_id,job_id) ON DELETE RESTRICT,
  FOREIGN KEY (tenant_id,source_snapshot_id) REFERENCES fusion_source_snapshots(tenant_id,snapshot_id) ON DELETE RESTRICT,
  FOREIGN KEY (tenant_id,data_snapshot_id) REFERENCES fusion_snapshots(tenant_id,snapshot_id) ON DELETE RESTRICT,
  FOREIGN KEY (tenant_id,feature_snapshot_id) REFERENCES fusion_snapshots(tenant_id,snapshot_id) ON DELETE RESTRICT,
  CHECK ((disposition='applied' AND failure_code='' AND source_snapshot_id IS NOT NULL AND data_snapshot_id IS NOT NULL AND feature_snapshot_id IS NOT NULL)
      OR (disposition='failed' AND failure_code<>'' AND source_snapshot_id IS NULL AND data_snapshot_id IS NULL AND feature_snapshot_id IS NULL))
);

CREATE TABLE IF NOT EXISTS fusion_projection_watermarks (
  tenant_id          TEXT NOT NULL REFERENCES tenants(tenant_id) ON DELETE RESTRICT,
  target             TEXT NOT NULL CHECK (target IN ('clickhouse','opensearch','nebulagraph')),
  fusion_level       TEXT NOT NULL CHECK (fusion_level IN ('data','feature','knowledge')),
  snapshot_id        UUID NOT NULL,
  snapshot_version   BIGINT NOT NULL CHECK (snapshot_version > 0),
  projection_sha256  TEXT NOT NULL CHECK (projection_sha256 ~ '^[0-9a-f]{64}$'),
  row_count          BIGINT NOT NULL CHECK (row_count >= 0),
  applied_at         TIMESTAMPTZ NOT NULL,
  trace_id           TEXT NOT NULL,
  PRIMARY KEY (tenant_id,target,fusion_level),
  FOREIGN KEY (tenant_id,snapshot_id) REFERENCES fusion_snapshots(tenant_id,snapshot_id) ON DELETE RESTRICT
);

INSERT INTO alignment_schema_migrations(version,description)
VALUES ('202608141700','M07 three-level fusion snapshots, append-only resolutions and projection delivery')
ON CONFLICT(version) DO NOTHING;

COMMIT;
