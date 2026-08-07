BEGIN;

ALTER TABLE asset_discovery_credentials
  ADD COLUMN IF NOT EXISTS revision BIGINT NOT NULL DEFAULT 1 CHECK(revision>0);
ALTER TABLE asset_topology_links
  ADD COLUMN IF NOT EXISTS revision BIGINT NOT NULL DEFAULT 1 CHECK(revision>0);

ALTER TABLE asset_discovery_outbox
  ALTER COLUMN run_id DROP NOT NULL,
  ADD COLUMN IF NOT EXISTS resource_type TEXT,
  ADD COLUMN IF NOT EXISTS resource_id TEXT,
  ADD COLUMN IF NOT EXISTS action_id TEXT;
UPDATE asset_discovery_outbox
SET resource_type='run', resource_id=run_id, action_id='asset-active-discovery-run'
WHERE resource_type IS NULL OR resource_id IS NULL OR action_id IS NULL;
ALTER TABLE asset_discovery_outbox
  ALTER COLUMN resource_type SET NOT NULL,
  ALTER COLUMN resource_type SET DEFAULT 'run',
  ALTER COLUMN resource_id SET NOT NULL,
  ALTER COLUMN action_id SET NOT NULL;
ALTER TABLE asset_discovery_outbox
  DROP CONSTRAINT IF EXISTS asset_discovery_outbox_run_id_aggregate_version_event_type_key,
  DROP CONSTRAINT IF EXISTS chk_asset_discovery_outbox_resource_type,
  DROP CONSTRAINT IF EXISTS chk_asset_discovery_outbox_resource_identity;
ALTER TABLE asset_discovery_outbox
  ADD CONSTRAINT chk_asset_discovery_outbox_resource_type
    CHECK(resource_type IN ('run','credential','topology_link')),
  ADD CONSTRAINT chk_asset_discovery_outbox_resource_identity
    CHECK((resource_type='run' AND run_id=resource_id) OR (resource_type<>'run' AND run_id IS NULL));
CREATE UNIQUE INDEX IF NOT EXISTS uq_asset_discovery_outbox_resource_version
  ON asset_discovery_outbox(resource_type,resource_id,aggregate_version,event_type);

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

INSERT INTO alignment_schema_migrations(version,description)
VALUES ('202608031520','F-ASSET-003 make discovery resource mutations atomic')
ON CONFLICT (version) DO NOTHING;

COMMIT;
