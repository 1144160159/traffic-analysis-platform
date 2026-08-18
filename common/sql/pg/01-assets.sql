-- =============================================================================
-- 资产表 (PostgreSQL) — Asset Service
-- 来源: common/old/postgres_ddl.sql (已合并)
-- =============================================================================
BEGIN;

CREATE TABLE IF NOT EXISTS asset_groups (
  group_id   UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
  tenant_id  TEXT NOT NULL REFERENCES tenants(tenant_id) ON DELETE CASCADE,
  name       TEXT NOT NULL,
  selector   JSONB NOT NULL DEFAULT '{}'::jsonb,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE (tenant_id, name)
);

CREATE TABLE IF NOT EXISTS assets (
  asset_id    UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
  revision    BIGINT NOT NULL DEFAULT 1 CHECK (revision > 0),
  display_code TEXT,
  tenant_id   TEXT NOT NULL REFERENCES tenants(tenant_id) ON DELETE CASCADE,
  asset_type  TEXT NOT NULL DEFAULT 'unknown',
  status      TEXT NOT NULL DEFAULT 'active',
  ip          TEXT,
  ip_address  TEXT,
  mac_address TEXT,
  hostname    TEXT,
  vendor      TEXT,
  os_type     TEXT,
  source      TEXT NOT NULL DEFAULT 'manual',
  vlan_id     TEXT,
  switch_port TEXT,
  department  TEXT,
  campus      TEXT,
  owner       TEXT,
  tags        JSONB NOT NULL DEFAULT '{}'::jsonb,
  criticality INT NOT NULL DEFAULT 0,
  metadata    JSONB NOT NULL DEFAULT '{}'::jsonb,
  first_seen  TIMESTAMPTZ NOT NULL DEFAULT now(),
  last_seen   TIMESTAMPTZ NOT NULL DEFAULT now(),
  created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

ALTER TABLE assets ADD COLUMN IF NOT EXISTS ip TEXT;
ALTER TABLE assets ADD COLUMN IF NOT EXISTS revision BIGINT NOT NULL DEFAULT 1;
ALTER TABLE assets ADD COLUMN IF NOT EXISTS display_code TEXT;
ALTER TABLE assets ADD COLUMN IF NOT EXISTS asset_type TEXT NOT NULL DEFAULT 'unknown';
ALTER TABLE assets ADD COLUMN IF NOT EXISTS status TEXT NOT NULL DEFAULT 'active';
ALTER TABLE assets ADD COLUMN IF NOT EXISTS ip_address TEXT;
ALTER TABLE assets ADD COLUMN IF NOT EXISTS mac_address TEXT;
ALTER TABLE assets ADD COLUMN IF NOT EXISTS hostname TEXT;
ALTER TABLE assets ADD COLUMN IF NOT EXISTS vendor TEXT;
ALTER TABLE assets ADD COLUMN IF NOT EXISTS os_type TEXT;
ALTER TABLE assets ADD COLUMN IF NOT EXISTS source TEXT NOT NULL DEFAULT 'manual';
ALTER TABLE assets ADD COLUMN IF NOT EXISTS vlan_id TEXT;
ALTER TABLE assets ADD COLUMN IF NOT EXISTS switch_port TEXT;
ALTER TABLE assets ADD COLUMN IF NOT EXISTS department TEXT;
ALTER TABLE assets ADD COLUMN IF NOT EXISTS campus TEXT;
ALTER TABLE assets ADD COLUMN IF NOT EXISTS owner TEXT;
ALTER TABLE assets ADD COLUMN IF NOT EXISTS tags JSONB NOT NULL DEFAULT '{}'::jsonb;
ALTER TABLE assets ADD COLUMN IF NOT EXISTS criticality INT NOT NULL DEFAULT 0;
ALTER TABLE assets ADD COLUMN IF NOT EXISTS metadata JSONB NOT NULL DEFAULT '{}'::jsonb;
ALTER TABLE assets ADD COLUMN IF NOT EXISTS first_seen TIMESTAMPTZ NOT NULL DEFAULT now();
ALTER TABLE assets ADD COLUMN IF NOT EXISTS last_seen TIMESTAMPTZ NOT NULL DEFAULT now();
ALTER TABLE assets ADD COLUMN IF NOT EXISTS created_at TIMESTAMPTZ NOT NULL DEFAULT now();
ALTER TABLE assets ADD COLUMN IF NOT EXISTS updated_at TIMESTAMPTZ NOT NULL DEFAULT now();
ALTER TABLE assets ADD COLUMN IF NOT EXISTS lifecycle_state TEXT NOT NULL DEFAULT 'managed';
ALTER TABLE assets ADD COLUMN IF NOT EXISTS merged_into_asset_id UUID REFERENCES assets(asset_id) ON DELETE RESTRICT;
ALTER TABLE assets DROP CONSTRAINT IF EXISTS chk_assets_lifecycle_state;
ALTER TABLE assets ADD CONSTRAINT chk_assets_lifecycle_state CHECK (lifecycle_state IN ('candidate','confirmed','managed','isolated','retired','merged'));
ALTER TABLE assets DROP CONSTRAINT IF EXISTS chk_assets_merge_target;
ALTER TABLE assets ADD CONSTRAINT chk_assets_merge_target CHECK ((lifecycle_state='merged' AND merged_into_asset_id IS NOT NULL AND merged_into_asset_id<>asset_id) OR (lifecycle_state<>'merged' AND merged_into_asset_id IS NULL));
ALTER TABLE assets ALTER COLUMN ip DROP NOT NULL;
UPDATE assets SET ip_address = ip WHERE (ip_address IS NULL OR ip_address = '') AND ip IS NOT NULL;
UPDATE assets AS candidate
SET ip = candidate.ip_address
WHERE candidate.ip IS NULL
  AND candidate.ip_address IS NOT NULL
  AND (SELECT COUNT(*) FROM assets AS peer WHERE peer.tenant_id = candidate.tenant_id AND peer.ip_address = candidate.ip_address) = 1
  AND NOT EXISTS (SELECT 1 FROM assets AS peer WHERE peer.tenant_id = candidate.tenant_id AND peer.ip = candidate.ip_address);

-- 资产变更事件
CREATE TABLE IF NOT EXISTS asset_events (
  event_id   SERIAL PRIMARY KEY,
  event_uuid UUID DEFAULT uuid_generate_v4(),
  asset_id   UUID NOT NULL REFERENCES assets(asset_id) ON DELETE CASCADE,
  tenant_id  TEXT NOT NULL,
  event_type TEXT NOT NULL,
  revision   BIGINT DEFAULT 1,
  trace_id   TEXT NOT NULL DEFAULT '',
  old_value  JSONB,
  new_value  JSONB,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_asset_events_asset ON asset_events(asset_id, created_at DESC);
ALTER TABLE asset_events ADD COLUMN IF NOT EXISTS event_uuid UUID;
ALTER TABLE asset_events ADD COLUMN IF NOT EXISTS revision BIGINT;
ALTER TABLE asset_events ADD COLUMN IF NOT EXISTS trace_id TEXT NOT NULL DEFAULT '';
UPDATE asset_events event
SET event_uuid=COALESCE(event.event_uuid,uuid_generate_v5(uuid_ns_url(),'traffic.asset.history:'||event.tenant_id||':'||event.asset_id::text||':'||event.event_id::text)),
    revision=COALESCE(event.revision,asset.revision)
FROM assets asset
WHERE asset.asset_id=event.asset_id AND (event.event_uuid IS NULL OR event.revision IS NULL);
ALTER TABLE asset_events ALTER COLUMN event_uuid SET NOT NULL;
ALTER TABLE asset_events ALTER COLUMN event_uuid SET DEFAULT uuid_generate_v4();
ALTER TABLE asset_events ALTER COLUMN revision SET NOT NULL;
ALTER TABLE asset_events ALTER COLUMN revision SET DEFAULT 1;
CREATE UNIQUE INDEX IF NOT EXISTS uq_asset_events_event_uuid ON asset_events(event_uuid);
CREATE INDEX IF NOT EXISTS idx_asset_events_tenant_revision ON asset_events(tenant_id,asset_id,revision DESC);
CREATE TABLE IF NOT EXISTS asset_event_outbox (
  outbox_id BIGSERIAL PRIMARY KEY, event_id UUID NOT NULL UNIQUE, tenant_id TEXT NOT NULL,
  asset_id UUID NOT NULL REFERENCES assets(asset_id) ON DELETE RESTRICT,
  aggregate_version BIGINT NOT NULL CHECK (aggregate_version>0),
  schema_version INTEGER NOT NULL DEFAULT 2 CHECK (schema_version>0),
  partition_key TEXT NOT NULL CHECK (partition_key<>''), event_type TEXT NOT NULL, payload JSONB NOT NULL,
  status TEXT NOT NULL DEFAULT 'pending' CHECK (status IN ('pending','processing','published','dead','cancelled')),
  attempt_count INTEGER NOT NULL DEFAULT 0, available_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  locked_by TEXT NOT NULL DEFAULT '', locked_until TIMESTAMPTZ, last_error TEXT NOT NULL DEFAULT '',
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(), published_at TIMESTAMPTZ
);
CREATE UNIQUE INDEX IF NOT EXISTS uq_asset_event_outbox_aggregate ON asset_event_outbox(tenant_id,asset_id,aggregate_version);
CREATE INDEX IF NOT EXISTS idx_asset_event_outbox_ready ON asset_event_outbox(available_at,outbox_id) WHERE status='pending';
CREATE INDEX IF NOT EXISTS idx_asset_event_outbox_reclaim ON asset_event_outbox(locked_until,outbox_id) WHERE status='processing';
CREATE TABLE IF NOT EXISTS asset_upsert_requests (
  request_id UUID PRIMARY KEY, tenant_id TEXT NOT NULL,
  idempotency_key TEXT NOT NULL CHECK (length(idempotency_key) BETWEEN 16 AND 200),
  request_hash TEXT NOT NULL CHECK (length(request_hash)=64), actor TEXT NOT NULL CHECK (actor<>''),
  expected_revision BIGINT NOT NULL CHECK (expected_revision>=0),
  asset_id UUID NOT NULL REFERENCES assets(asset_id) ON DELETE RESTRICT, created BOOLEAN NOT NULL,
  resulting_revision BIGINT NOT NULL CHECK (resulting_revision>0),
  event_id UUID NOT NULL REFERENCES asset_event_outbox(event_id) ON DELETE RESTRICT,
  outbox_id BIGINT NOT NULL REFERENCES asset_event_outbox(outbox_id) ON DELETE RESTRICT,
  trace_id TEXT NOT NULL CHECK (trace_id<>''), created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE (tenant_id,idempotency_key)
);
CREATE INDEX IF NOT EXISTS idx_asset_upsert_requests_asset ON asset_upsert_requests(tenant_id,asset_id,created_at DESC);
CREATE TABLE IF NOT EXISTS asset_inactive_requests (
  request_id UUID PRIMARY KEY, tenant_id TEXT NOT NULL,
  idempotency_key TEXT NOT NULL CHECK (length(idempotency_key) BETWEEN 16 AND 200),
  request_hash TEXT NOT NULL CHECK (length(request_hash)=64),
  actor TEXT NOT NULL CHECK (actor<>''),
  action_id TEXT NOT NULL CHECK (action_id='asset-inactive-sweep'),
  reason TEXT NOT NULL CHECK (reason<>''), cutoff TIMESTAMPTZ NOT NULL,
  affected_count INTEGER NOT NULL CHECK (affected_count>=0),
  result_payload JSONB NOT NULL, trace_id TEXT NOT NULL CHECK (trace_id<>''),
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(), UNIQUE (tenant_id,idempotency_key)
);
CREATE INDEX IF NOT EXISTS idx_asset_inactive_requests_cutoff ON asset_inactive_requests(tenant_id,cutoff DESC);
CREATE TABLE IF NOT EXISTS asset_projection_inbox (
  event_id UUID PRIMARY KEY, tenant_id TEXT NOT NULL,
  asset_id UUID NOT NULL REFERENCES assets(asset_id) ON DELETE RESTRICT,
  aggregate_version BIGINT NOT NULL CHECK (aggregate_version>0),
  schema_version INTEGER NOT NULL CHECK (schema_version=2),
  partition_key TEXT NOT NULL CHECK (partition_key<>''),
  trace_id TEXT NOT NULL DEFAULT '', payload JSONB NOT NULL,
  payload_sha256 TEXT NOT NULL CHECK (length(payload_sha256)=64),
  kafka_partition INTEGER NOT NULL, kafka_offset BIGINT NOT NULL,
  os_status TEXT NOT NULL DEFAULT 'pending' CHECK (os_status IN ('pending','applied','dead')),
  nebula_status TEXT NOT NULL DEFAULT 'pending' CHECK (nebula_status IN ('pending','applied','dead')),
  status TEXT NOT NULL DEFAULT 'pending' CHECK (status IN ('pending','processing','applied','dead')),
  attempt_count INTEGER NOT NULL DEFAULT 0, available_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  locked_by TEXT NOT NULL DEFAULT '', locked_until TIMESTAMPTZ, last_error TEXT NOT NULL DEFAULT '',
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(), updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  applied_at TIMESTAMPTZ, UNIQUE (tenant_id,asset_id,aggregate_version)
);
CREATE INDEX IF NOT EXISTS idx_asset_projection_inbox_ready ON asset_projection_inbox(available_at,created_at) WHERE status='pending';
CREATE INDEX IF NOT EXISTS idx_asset_projection_inbox_reclaim ON asset_projection_inbox(locked_until,created_at) WHERE status='processing';
CREATE INDEX IF NOT EXISTS idx_asset_projection_inbox_dead ON asset_projection_inbox(updated_at) WHERE status='dead';
ALTER TABLE asset_projection_inbox
  ADD COLUMN IF NOT EXISTS kafka_topic TEXT NOT NULL DEFAULT 'asset.events.v2',
  ADD COLUMN IF NOT EXISTS kafka_timestamp_ms BIGINT NOT NULL DEFAULT 1 CHECK (kafka_timestamp_ms>0),
  ADD COLUMN IF NOT EXISTS raw_payload BYTEA,
  ADD COLUMN IF NOT EXISTS source_sha256 TEXT NOT NULL DEFAULT repeat('0',64) CHECK (length(source_sha256)=64),
  ADD COLUMN IF NOT EXISTS ch_status TEXT NOT NULL DEFAULT 'disabled' CHECK (ch_status IN ('disabled','pending','applied','dead'));
CREATE INDEX IF NOT EXISTS idx_asset_projection_inbox_ch_ready ON asset_projection_inbox(available_at,created_at) WHERE ch_status='pending';
CREATE TABLE IF NOT EXISTS asset_projection_watermarks (
  tenant_id TEXT NOT NULL,
  asset_id UUID NOT NULL REFERENCES assets(asset_id) ON DELETE RESTRICT,
  target TEXT NOT NULL CHECK (target IN ('opensearch','nebulagraph')),
  aggregate_version BIGINT NOT NULL CHECK (aggregate_version>0),
  event_id UUID NOT NULL REFERENCES asset_projection_inbox(event_id) ON DELETE RESTRICT,
  projection_sha256 TEXT NOT NULL CHECK (length(projection_sha256)=64),
  applied_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  PRIMARY KEY (tenant_id,asset_id,target)
);
CREATE INDEX IF NOT EXISTS idx_asset_projection_watermarks_target_version ON asset_projection_watermarks(target,aggregate_version);
ALTER TABLE asset_projection_watermarks
  DROP CONSTRAINT IF EXISTS asset_projection_watermarks_target_check;
ALTER TABLE asset_projection_watermarks
  ADD CONSTRAINT asset_projection_watermarks_target_check
  CHECK (target IN ('opensearch','nebulagraph','clickhouse'));
CREATE TABLE IF NOT EXISTS asset_discovery_credentials (
  credential_id TEXT PRIMARY KEY, tenant_id TEXT NOT NULL, name TEXT NOT NULL,
  protocol TEXT NOT NULL, endpoint TEXT, secret_ref TEXT NOT NULL, created_by TEXT,
  revision BIGINT NOT NULL DEFAULT 1 CHECK(revision>0),
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(), updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE(tenant_id,name)
);
CREATE TABLE IF NOT EXISTS asset_discovery_runs (
  run_id TEXT PRIMARY KEY, tenant_id TEXT NOT NULL, mode TEXT NOT NULL, target_cidr TEXT,
  target_network CIDR, credential_id TEXT, action_id TEXT NOT NULL DEFAULT 'asset-active-discovery-run',
  status TEXT NOT NULL DEFAULT 'queued' CHECK(status IN ('queued','running','cancel_requested','cancelled','succeeded','completed','partial','failed','blocked')),
  revision BIGINT NOT NULL DEFAULT 1 CHECK(revision>0), requested_by TEXT,
  reason TEXT NOT NULL DEFAULT 'legacy-compatible', rate_limit_per_second INT NOT NULL DEFAULT 10 CHECK(rate_limit_per_second BETWEEN 1 AND 10000),
  security_window_start TIMESTAMPTZ, security_window_end TIMESTAMPTZ, approved_by TEXT NOT NULL DEFAULT '',
  idempotency_key TEXT, request_hash TEXT, trace_id TEXT NOT NULL DEFAULT '',
  cancel_requested BOOLEAN NOT NULL DEFAULT false, discovered_assets INT NOT NULL DEFAULT 0,
  discovered_links INT NOT NULL DEFAULT 0, discovered_candidates INT NOT NULL DEFAULT 0,
  rejected_records INT NOT NULL DEFAULT 0, result_watermark TEXT NOT NULL DEFAULT '',
  error_message TEXT, queued_at TIMESTAMPTZ NOT NULL DEFAULT now(), started_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(), completed_at TIMESTAMPTZ,
  locked_by TEXT NOT NULL DEFAULT '', locked_until TIMESTAMPTZ
);
CREATE UNIQUE INDEX IF NOT EXISTS uq_asset_discovery_run_idempotency ON asset_discovery_runs(tenant_id,idempotency_key) WHERE idempotency_key IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_asset_discovery_run_ready ON asset_discovery_runs(queued_at,run_id) WHERE status='queued';
CREATE INDEX IF NOT EXISTS idx_asset_discovery_run_reclaim ON asset_discovery_runs(locked_until,run_id) WHERE status='running';
CREATE INDEX IF NOT EXISTS idx_asset_discovery_run_overlap ON asset_discovery_runs USING gist(target_network inet_ops) WHERE status IN ('queued','running','cancel_requested');
CREATE TABLE IF NOT EXISTS asset_discovery_run_history (
  transition_id BIGSERIAL PRIMARY KEY, run_id TEXT NOT NULL REFERENCES asset_discovery_runs(run_id) ON DELETE RESTRICT,
  tenant_id TEXT NOT NULL, from_status TEXT NOT NULL, to_status TEXT NOT NULL,
  revision BIGINT NOT NULL CHECK(revision>0), actor TEXT NOT NULL, reason TEXT NOT NULL,
  trace_id TEXT NOT NULL, detail JSONB NOT NULL DEFAULT '{}'::jsonb,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(), UNIQUE(run_id,revision)
);
CREATE TABLE IF NOT EXISTS asset_discovery_control_requests (
  request_id UUID PRIMARY KEY, tenant_id TEXT NOT NULL,
  run_id TEXT NOT NULL REFERENCES asset_discovery_runs(run_id) ON DELETE RESTRICT,
  operation TEXT NOT NULL CHECK(operation IN ('cancel','merge_candidate')),
  candidate_id UUID,
  idempotency_key TEXT NOT NULL CHECK(length(idempotency_key) BETWEEN 16 AND 200),
  request_hash TEXT NOT NULL CHECK(length(request_hash)=64),
  expected_revision BIGINT NOT NULL CHECK(expected_revision>0),
  resulting_revision BIGINT NOT NULL CHECK(resulting_revision>0),
  result_payload JSONB NOT NULL DEFAULT '{}'::jsonb,
  actor TEXT NOT NULL, reason TEXT NOT NULL, trace_id TEXT NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(), UNIQUE(tenant_id,idempotency_key)
);
CREATE INDEX IF NOT EXISTS idx_asset_discovery_control_run ON asset_discovery_control_requests(tenant_id,run_id,created_at);
CREATE TABLE IF NOT EXISTS asset_discovery_candidates (
  candidate_id UUID PRIMARY KEY, run_id TEXT NOT NULL REFERENCES asset_discovery_runs(run_id) ON DELETE RESTRICT,
  tenant_id TEXT NOT NULL, fingerprint TEXT NOT NULL CHECK(length(fingerprint)=64), observation JSONB NOT NULL,
  status TEXT NOT NULL DEFAULT 'pending' CHECK(status IN ('pending','approved','merged','rejected')),
  revision BIGINT NOT NULL DEFAULT 1 CHECK(revision>0), source_asset_id UUID REFERENCES assets(asset_id) ON DELETE RESTRICT,
  decision_reason TEXT NOT NULL DEFAULT '', decided_by TEXT NOT NULL DEFAULT '',
  discovered_at TIMESTAMPTZ NOT NULL DEFAULT now(), decided_at TIMESTAMPTZ,
  UNIQUE(tenant_id,run_id,fingerprint)
);
CREATE INDEX IF NOT EXISTS idx_asset_discovery_candidates_run ON asset_discovery_candidates(tenant_id,run_id,status,discovered_at,candidate_id);
CREATE TABLE IF NOT EXISTS asset_discovery_outbox (
  outbox_id BIGSERIAL PRIMARY KEY, event_id UUID NOT NULL UNIQUE,
  run_id TEXT REFERENCES asset_discovery_runs(run_id) ON DELETE RESTRICT,
  resource_type TEXT NOT NULL DEFAULT 'run' CHECK(resource_type IN ('run','credential','topology_link')),
  resource_id TEXT NOT NULL, action_id TEXT NOT NULL,
  tenant_id TEXT NOT NULL, aggregate_version BIGINT NOT NULL CHECK(aggregate_version>0),
  schema_version INT NOT NULL DEFAULT 1 CHECK(schema_version>0), partition_key TEXT NOT NULL CHECK(partition_key<>''),
  event_type TEXT NOT NULL, payload JSONB NOT NULL,
  status TEXT NOT NULL DEFAULT 'pending' CHECK(status IN ('pending','processing','published','dead','cancelled')),
  attempt_count INT NOT NULL DEFAULT 0, available_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  locked_by TEXT NOT NULL DEFAULT '', locked_until TIMESTAMPTZ, last_error TEXT NOT NULL DEFAULT '',
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(), published_at TIMESTAMPTZ,
  UNIQUE(resource_type,resource_id,aggregate_version,event_type),
  CHECK((resource_type='run' AND run_id=resource_id) OR (resource_type<>'run' AND run_id IS NULL))
);
CREATE INDEX IF NOT EXISTS idx_asset_discovery_outbox_ready ON asset_discovery_outbox(available_at,outbox_id) WHERE status='pending';
CREATE TABLE IF NOT EXISTS asset_discovery_resource_requests (
  request_id UUID PRIMARY KEY, tenant_id TEXT NOT NULL,
  resource_type TEXT NOT NULL CHECK(resource_type IN ('credential','topology_link')),
  resource_id TEXT NOT NULL, action_id TEXT NOT NULL,
  idempotency_key TEXT NOT NULL CHECK(length(idempotency_key) BETWEEN 16 AND 200),
  request_hash TEXT NOT NULL CHECK(length(request_hash)=64), expected_revision BIGINT NOT NULL CHECK(expected_revision>=0),
  resulting_revision BIGINT NOT NULL CHECK(resulting_revision>0), result_payload JSONB NOT NULL,
  event_id UUID NOT NULL, outbox_id BIGINT NOT NULL REFERENCES asset_discovery_outbox(outbox_id) ON DELETE RESTRICT,
  actor TEXT NOT NULL, reason TEXT NOT NULL, trace_id TEXT NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(), UNIQUE(tenant_id,idempotency_key)
);
CREATE TABLE IF NOT EXISTS asset_discovery_resource_history (
  history_id BIGSERIAL PRIMARY KEY, tenant_id TEXT NOT NULL,
  resource_type TEXT NOT NULL CHECK(resource_type IN ('credential','topology_link')),
  resource_id TEXT NOT NULL, revision BIGINT NOT NULL CHECK(revision>0), action_id TEXT NOT NULL,
  actor TEXT NOT NULL, reason TEXT NOT NULL, trace_id TEXT NOT NULL,
  old_value JSONB NOT NULL DEFAULT '{}'::jsonb, new_value JSONB NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(), UNIQUE(resource_type,resource_id,revision)
);
CREATE TABLE IF NOT EXISTS asset_topology_links (
  link_id TEXT PRIMARY KEY, tenant_id TEXT NOT NULL, run_id TEXT,
  source_asset_id TEXT, source_mac TEXT, source_ip TEXT, source_interface TEXT NOT NULL DEFAULT '',
  neighbor_asset_id TEXT, neighbor_mac TEXT NOT NULL DEFAULT '', neighbor_ip TEXT,
  neighbor_interface TEXT NOT NULL DEFAULT '', protocol TEXT NOT NULL,
  confidence INT NOT NULL DEFAULT 80, revision BIGINT NOT NULL DEFAULT 1 CHECK(revision>0),
  observed_at TIMESTAMPTZ NOT NULL DEFAULT now(), created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE(tenant_id,source_mac,neighbor_mac,protocol,source_interface,neighbor_interface)
);
CREATE INDEX IF NOT EXISTS idx_asset_topology_links_tenant ON asset_topology_links(tenant_id,observed_at DESC);
CREATE INDEX IF NOT EXISTS idx_asset_topology_links_asset ON asset_topology_links(tenant_id,source_asset_id,neighbor_asset_id);
CREATE TABLE IF NOT EXISTS asset_export_jobs (
  job_id UUID PRIMARY KEY, tenant_id TEXT NOT NULL REFERENCES tenants(tenant_id) ON DELETE RESTRICT,
  action_id TEXT NOT NULL CHECK(action_id='asset-inventory-export'), format TEXT NOT NULL CHECK(format IN ('csv','jsonl')),
  status TEXT NOT NULL DEFAULT 'accepted' CHECK(status IN ('accepted','running','completed','failed','cancelled')),
  revision BIGINT NOT NULL DEFAULT 1 CHECK(revision>0), columns JSONB NOT NULL, query JSONB NOT NULL,
  query_sha256 TEXT NOT NULL CHECK(length(query_sha256)=64), idempotency_key TEXT NOT NULL CHECK(length(idempotency_key) BETWEEN 16 AND 200),
  reason TEXT NOT NULL, snapshot_id TEXT NOT NULL DEFAULT '', as_of TIMESTAMPTZ, source_watermarks JSONB NOT NULL DEFAULT '{}'::jsonb,
  row_count INTEGER NOT NULL DEFAULT 0 CHECK(row_count>=0), object_bucket TEXT NOT NULL DEFAULT '', object_key TEXT NOT NULL DEFAULT '',
  mime_type TEXT NOT NULL DEFAULT '', artifact_sha256 TEXT NOT NULL DEFAULT '', size_bytes BIGINT NOT NULL DEFAULT 0 CHECK(size_bytes>=0),
  retention_until TIMESTAMPTZ, error_message TEXT NOT NULL DEFAULT '', attempts INTEGER NOT NULL DEFAULT 0 CHECK(attempts>=0),
  next_attempt_at TIMESTAMPTZ NOT NULL DEFAULT now(), locked_until TIMESTAMPTZ, locked_by TEXT NOT NULL DEFAULT '', created_by TEXT NOT NULL,
  trace_id TEXT NOT NULL, created_at TIMESTAMPTZ NOT NULL DEFAULT now(), updated_at TIMESTAMPTZ NOT NULL DEFAULT now(), completed_at TIMESTAMPTZ,
  UNIQUE(tenant_id,idempotency_key)
);
CREATE INDEX IF NOT EXISTS idx_asset_export_jobs_tenant ON asset_export_jobs(tenant_id,created_at DESC,job_id);
CREATE INDEX IF NOT EXISTS idx_asset_export_jobs_ready ON asset_export_jobs(next_attempt_at,created_at,job_id) WHERE status IN ('accepted','running');
CREATE TABLE IF NOT EXISTS asset_export_outbox (
  outbox_id BIGSERIAL PRIMARY KEY, event_id UUID NOT NULL UNIQUE, job_id UUID NOT NULL REFERENCES asset_export_jobs(job_id) ON DELETE RESTRICT,
  tenant_id TEXT NOT NULL REFERENCES tenants(tenant_id) ON DELETE RESTRICT, event_type TEXT NOT NULL,
  aggregate_version BIGINT NOT NULL CHECK(aggregate_version>0), schema_version INTEGER NOT NULL DEFAULT 1 CHECK(schema_version>0),
  partition_key TEXT NOT NULL CHECK(partition_key<>''), payload JSONB NOT NULL,
  status TEXT NOT NULL DEFAULT 'pending' CHECK(status IN ('pending','processing','published','dead','cancelled')),
  attempts INTEGER NOT NULL DEFAULT 0 CHECK(attempts>=0), next_attempt_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  locked_by TEXT NOT NULL DEFAULT '', locked_until TIMESTAMPTZ, last_error TEXT NOT NULL DEFAULT '',
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(), published_at TIMESTAMPTZ, UNIQUE(job_id,aggregate_version)
);
CREATE INDEX IF NOT EXISTS idx_asset_export_outbox_ready ON asset_export_outbox(next_attempt_at,outbox_id) WHERE status='pending';
CREATE INDEX IF NOT EXISTS idx_asset_export_outbox_reclaim ON asset_export_outbox(locked_until,outbox_id) WHERE status='processing';
CREATE TABLE IF NOT EXISTS asset_column_preferences (
  tenant_id TEXT NOT NULL REFERENCES tenants(tenant_id) ON DELETE CASCADE, user_id TEXT NOT NULL, view_id TEXT NOT NULL,
  columns JSONB NOT NULL, revision BIGINT NOT NULL DEFAULT 1 CHECK(revision>0), updated_by TEXT NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(), updated_at TIMESTAMPTZ NOT NULL DEFAULT now(), PRIMARY KEY(tenant_id,user_id,view_id)
);
CREATE INDEX IF NOT EXISTS idx_asset_column_preferences_updated ON asset_column_preferences(tenant_id,updated_at DESC);

CREATE TABLE IF NOT EXISTS asset_governance_work_orders (
  work_order_id UUID PRIMARY KEY, tenant_id TEXT NOT NULL,
  asset_id UUID NOT NULL REFERENCES assets(asset_id) ON DELETE RESTRICT,
  action_id TEXT NOT NULL CHECK(action_id='asset-governance-work-order-create'),
  source_lifecycle_state TEXT NOT NULL,
  target_lifecycle_state TEXT NOT NULL CHECK(target_lifecycle_state IN ('candidate','confirmed','managed','isolated','retired','merged')),
  target_asset_id UUID REFERENCES assets(asset_id) ON DELETE RESTRICT,
  status TEXT NOT NULL DEFAULT 'pending_approval' CHECK(status IN ('pending_approval','approved','rejected','executing','completed','failed','cancelled','compensated')),
  revision BIGINT NOT NULL DEFAULT 1 CHECK(revision>0), expected_asset_revision BIGINT NOT NULL CHECK(expected_asset_revision>0),
  resulting_asset_revision BIGINT, owner TEXT NOT NULL CHECK(owner<>''), requested_by TEXT NOT NULL CHECK(requested_by<>''),
  approved_by TEXT NOT NULL DEFAULT '', due_at TIMESTAMPTZ NOT NULL, evidence_required BOOLEAN NOT NULL DEFAULT true,
  evidence_refs JSONB NOT NULL DEFAULT '[]'::jsonb CHECK(jsonb_typeof(evidence_refs)='array'),
  reason TEXT NOT NULL CHECK(length(reason) BETWEEN 8 AND 2000), external_system TEXT NOT NULL DEFAULT 'internal',
  external_ticket_id TEXT NOT NULL DEFAULT '', external_status TEXT NOT NULL DEFAULT '',
  idempotency_key TEXT NOT NULL CHECK(length(idempotency_key) BETWEEN 16 AND 200), request_hash TEXT NOT NULL CHECK(length(request_hash)=64),
  trace_id TEXT NOT NULL CHECK(trace_id<>''), created_at TIMESTAMPTZ NOT NULL DEFAULT now(), updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  completed_at TIMESTAMPTZ, UNIQUE(tenant_id,idempotency_key),
  CHECK((target_lifecycle_state='merged' AND target_asset_id IS NOT NULL AND target_asset_id<>asset_id) OR (target_lifecycle_state<>'merged' AND target_asset_id IS NULL))
);
CREATE INDEX IF NOT EXISTS idx_asset_governance_orders_asset ON asset_governance_work_orders(tenant_id,asset_id,created_at DESC);
DROP INDEX IF EXISTS uq_asset_governance_active_target;
CREATE UNIQUE INDEX uq_asset_governance_active_target ON asset_governance_work_orders(tenant_id,asset_id) WHERE status IN ('pending_approval','approved','executing');
CREATE TABLE IF NOT EXISTS asset_governance_work_order_history (
  history_id BIGSERIAL PRIMARY KEY, work_order_id UUID NOT NULL REFERENCES asset_governance_work_orders(work_order_id) ON DELETE RESTRICT,
  tenant_id TEXT NOT NULL, revision BIGINT NOT NULL CHECK(revision>0), action_id TEXT NOT NULL, from_status TEXT NOT NULL,
  to_status TEXT NOT NULL, from_lifecycle_state TEXT NOT NULL, to_lifecycle_state TEXT NOT NULL, actor TEXT NOT NULL CHECK(actor<>''),
  reason TEXT NOT NULL, evidence_refs JSONB NOT NULL DEFAULT '[]'::jsonb, trace_id TEXT NOT NULL CHECK(trace_id<>''),
  detail JSONB NOT NULL DEFAULT '{}'::jsonb, created_at TIMESTAMPTZ NOT NULL DEFAULT now(), UNIQUE(work_order_id,revision)
);
CREATE TABLE IF NOT EXISTS asset_governance_control_requests (
  request_id UUID PRIMARY KEY, tenant_id TEXT NOT NULL, work_order_id UUID NOT NULL REFERENCES asset_governance_work_orders(work_order_id) ON DELETE RESTRICT,
  idempotency_key TEXT NOT NULL CHECK(length(idempotency_key) BETWEEN 16 AND 200), request_hash TEXT NOT NULL CHECK(length(request_hash)=64),
  action_id TEXT NOT NULL, actor TEXT NOT NULL CHECK(actor<>''), expected_revision BIGINT NOT NULL CHECK(expected_revision>0),
  resulting_revision BIGINT NOT NULL CHECK(resulting_revision>0), trace_id TEXT NOT NULL CHECK(trace_id<>''), result JSONB NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(), UNIQUE(tenant_id,idempotency_key)
);
CREATE TABLE IF NOT EXISTS asset_governance_outbox (
  outbox_id BIGSERIAL PRIMARY KEY, event_id UUID NOT NULL UNIQUE, tenant_id TEXT NOT NULL,
  work_order_id UUID NOT NULL REFERENCES asset_governance_work_orders(work_order_id) ON DELETE RESTRICT,
  aggregate_version BIGINT NOT NULL CHECK(aggregate_version>0), schema_version INTEGER NOT NULL DEFAULT 1 CHECK(schema_version>0),
  partition_key TEXT NOT NULL CHECK(partition_key<>''), event_type TEXT NOT NULL, delivery_target TEXT NOT NULL DEFAULT 'internal' CHECK(delivery_target IN ('internal','external')),
  payload JSONB NOT NULL, status TEXT NOT NULL DEFAULT 'pending' CHECK(status IN ('pending','processing','delivered','dead','cancelled')),
  attempt_count INTEGER NOT NULL DEFAULT 0, available_at TIMESTAMPTZ NOT NULL DEFAULT now(), locked_by TEXT NOT NULL DEFAULT '',
  locked_until TIMESTAMPTZ, last_error TEXT NOT NULL DEFAULT '', created_at TIMESTAMPTZ NOT NULL DEFAULT now(), delivered_at TIMESTAMPTZ,
  UNIQUE(work_order_id,aggregate_version)
);
CREATE INDEX IF NOT EXISTS idx_asset_governance_outbox_ready ON asset_governance_outbox(available_at,outbox_id) WHERE status='pending';
CREATE INDEX IF NOT EXISTS idx_assets_tenant ON assets(tenant_id);
CREATE UNIQUE INDEX IF NOT EXISTS idx_assets_tenant_display_code_unique ON assets(tenant_id, display_code) WHERE display_code IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_assets_tenant_type_status ON assets(tenant_id, asset_type, status, last_seen DESC);
CREATE INDEX IF NOT EXISTS idx_assets_cursor_v2 ON assets(tenant_id, last_seen DESC, asset_id DESC) INCLUDE(updated_at);
CREATE INDEX IF NOT EXISTS idx_assets_ip ON assets(tenant_id, ip_address);
CREATE UNIQUE INDEX IF NOT EXISTS idx_assets_tenant_ip_unique ON assets(tenant_id, ip) WHERE ip IS NOT NULL;
CREATE UNIQUE INDEX IF NOT EXISTS idx_assets_tenant_mac_unique ON assets(tenant_id, mac_address) WHERE mac_address IS NOT NULL;

COMMIT;
