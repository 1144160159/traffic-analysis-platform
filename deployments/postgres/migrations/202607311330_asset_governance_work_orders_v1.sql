-- F-ASSET-006 authoritative asset-governance work orders.
-- Expand: additive lifecycle columns, durable work-order/history/request/outbox facts.
-- Cutover: enable ASSET_GOVERNANCE_V1_ENABLED only after API and UI candidates match.
-- Reconcile:
--   SELECT count(*) FROM assets WHERE lifecycle_state NOT IN
--     ('candidate','confirmed','managed','isolated','retired','merged');
--   SELECT count(*) FROM asset_governance_work_orders w LEFT JOIN assets a
--     ON a.tenant_id=w.tenant_id AND a.asset_id=w.asset_id WHERE a.asset_id IS NULL;
-- Rollback: disable the feature flag. Preserve all additive facts for audit/replay.

BEGIN;

ALTER TABLE assets
  ADD COLUMN IF NOT EXISTS lifecycle_state TEXT NOT NULL DEFAULT 'managed';
DO $asset_id_compat$
DECLARE asset_id_type TEXT;
BEGIN
  SELECT format_type(a.atttypid,a.atttypmod) INTO asset_id_type FROM pg_attribute a
   WHERE a.attrelid='assets'::regclass AND a.attname='asset_id' AND NOT a.attisdropped;
  IF asset_id_type NOT IN ('uuid','text') THEN RAISE EXCEPTION 'unsupported assets.asset_id type: %',asset_id_type; END IF;
  EXECUTE format('ALTER TABLE assets ADD COLUMN IF NOT EXISTS merged_into_asset_id %s REFERENCES assets(asset_id) ON DELETE RESTRICT',asset_id_type);
END
$asset_id_compat$;
ALTER TABLE assets DROP CONSTRAINT IF EXISTS chk_assets_lifecycle_state;
ALTER TABLE assets ADD CONSTRAINT chk_assets_lifecycle_state CHECK (
  lifecycle_state IN ('candidate','confirmed','managed','isolated','retired','merged')
);
ALTER TABLE assets DROP CONSTRAINT IF EXISTS chk_assets_merge_target;
ALTER TABLE assets ADD CONSTRAINT chk_assets_merge_target CHECK (
  (lifecycle_state='merged' AND merged_into_asset_id IS NOT NULL AND merged_into_asset_id<>asset_id)
  OR (lifecycle_state<>'merged' AND merged_into_asset_id IS NULL)
);

DO $asset_id_compat$
DECLARE asset_id_type TEXT;
BEGIN
  SELECT format_type(a.atttypid,a.atttypmod) INTO asset_id_type FROM pg_attribute a
   WHERE a.attrelid='assets'::regclass AND a.attname='asset_id' AND NOT a.attisdropped;
  IF asset_id_type NOT IN ('uuid','text') THEN RAISE EXCEPTION 'unsupported assets.asset_id type: %',asset_id_type; END IF;
  EXECUTE format($ddl$CREATE TABLE IF NOT EXISTS asset_governance_work_orders (
  work_order_id            UUID PRIMARY KEY,
  tenant_id                TEXT NOT NULL,
  asset_id                 %s NOT NULL REFERENCES assets(asset_id) ON DELETE RESTRICT,
  action_id                TEXT NOT NULL CHECK (action_id='asset-governance-work-order-create'),
  source_lifecycle_state   TEXT NOT NULL,
  target_lifecycle_state   TEXT NOT NULL CHECK (
    target_lifecycle_state IN ('candidate','confirmed','managed','isolated','retired','merged')
  ),
  target_asset_id          %s REFERENCES assets(asset_id) ON DELETE RESTRICT,
  status                   TEXT NOT NULL DEFAULT 'pending_approval' CHECK (
    status IN ('pending_approval','approved','rejected','executing','completed','failed','cancelled','compensated')
  ),
  revision                 BIGINT NOT NULL DEFAULT 1 CHECK (revision>0),
  expected_asset_revision  BIGINT NOT NULL CHECK (expected_asset_revision>0),
  resulting_asset_revision BIGINT,
  owner                    TEXT NOT NULL CHECK (owner<>''),
  requested_by             TEXT NOT NULL CHECK (requested_by<>''),
  approved_by              TEXT NOT NULL DEFAULT '',
  due_at                   TIMESTAMPTZ NOT NULL,
  evidence_required        BOOLEAN NOT NULL DEFAULT true,
  evidence_refs            JSONB NOT NULL DEFAULT '[]'::jsonb CHECK (jsonb_typeof(evidence_refs)='array'),
  reason                   TEXT NOT NULL CHECK (length(reason) BETWEEN 8 AND 2000),
  external_system          TEXT NOT NULL DEFAULT 'internal',
  external_ticket_id       TEXT NOT NULL DEFAULT '',
  external_status          TEXT NOT NULL DEFAULT '',
  idempotency_key          TEXT NOT NULL CHECK (length(idempotency_key) BETWEEN 16 AND 200),
  request_hash             TEXT NOT NULL CHECK (length(request_hash)=64),
  trace_id                 TEXT NOT NULL CHECK (trace_id<>''),
  created_at               TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at               TIMESTAMPTZ NOT NULL DEFAULT now(),
  completed_at             TIMESTAMPTZ,
  UNIQUE (tenant_id,idempotency_key),
  CHECK ((target_lifecycle_state='merged' AND target_asset_id IS NOT NULL AND target_asset_id<>asset_id)
      OR (target_lifecycle_state<>'merged' AND target_asset_id IS NULL))
)$ddl$,asset_id_type,asset_id_type);
END
$asset_id_compat$;
CREATE INDEX IF NOT EXISTS idx_asset_governance_orders_asset
  ON asset_governance_work_orders(tenant_id,asset_id,created_at DESC);
DROP INDEX IF EXISTS uq_asset_governance_active_target;
CREATE UNIQUE INDEX uq_asset_governance_active_target
  ON asset_governance_work_orders(tenant_id,asset_id)
  WHERE status IN ('pending_approval','approved','executing');

CREATE TABLE IF NOT EXISTS asset_governance_work_order_history (
  history_id                BIGSERIAL PRIMARY KEY,
  work_order_id             UUID NOT NULL REFERENCES asset_governance_work_orders(work_order_id) ON DELETE RESTRICT,
  tenant_id                 TEXT NOT NULL,
  revision                  BIGINT NOT NULL CHECK (revision>0),
  action_id                 TEXT NOT NULL,
  from_status               TEXT NOT NULL,
  to_status                 TEXT NOT NULL,
  from_lifecycle_state      TEXT NOT NULL,
  to_lifecycle_state        TEXT NOT NULL,
  actor                     TEXT NOT NULL CHECK (actor<>''),
  reason                    TEXT NOT NULL,
  evidence_refs             JSONB NOT NULL DEFAULT '[]'::jsonb,
  trace_id                  TEXT NOT NULL CHECK (trace_id<>''),
  detail                    JSONB NOT NULL DEFAULT '{}'::jsonb,
  created_at                TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE(work_order_id,revision)
);

CREATE TABLE IF NOT EXISTS asset_governance_control_requests (
  request_id                UUID PRIMARY KEY,
  tenant_id                 TEXT NOT NULL,
  work_order_id             UUID NOT NULL REFERENCES asset_governance_work_orders(work_order_id) ON DELETE RESTRICT,
  idempotency_key           TEXT NOT NULL CHECK (length(idempotency_key) BETWEEN 16 AND 200),
  request_hash              TEXT NOT NULL CHECK (length(request_hash)=64),
  action_id                 TEXT NOT NULL,
  actor                     TEXT NOT NULL CHECK (actor<>''),
  expected_revision         BIGINT NOT NULL CHECK (expected_revision>0),
  resulting_revision        BIGINT NOT NULL CHECK (resulting_revision>0),
  trace_id                  TEXT NOT NULL CHECK (trace_id<>''),
  result                    JSONB NOT NULL,
  created_at                TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE(tenant_id,idempotency_key)
);

CREATE TABLE IF NOT EXISTS asset_governance_outbox (
  outbox_id          BIGSERIAL PRIMARY KEY,
  event_id           UUID NOT NULL UNIQUE,
  tenant_id          TEXT NOT NULL,
  work_order_id      UUID NOT NULL REFERENCES asset_governance_work_orders(work_order_id) ON DELETE RESTRICT,
  aggregate_version  BIGINT NOT NULL CHECK (aggregate_version>0),
  schema_version     INTEGER NOT NULL DEFAULT 1 CHECK (schema_version>0),
  partition_key      TEXT NOT NULL CHECK (partition_key<>''),
  event_type         TEXT NOT NULL,
  delivery_target    TEXT NOT NULL DEFAULT 'internal' CHECK (delivery_target IN ('internal','external')),
  payload            JSONB NOT NULL,
  status             TEXT NOT NULL DEFAULT 'pending' CHECK (
    status IN ('pending','processing','delivered','dead','cancelled')
  ),
  attempt_count      INTEGER NOT NULL DEFAULT 0,
  available_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
  locked_by          TEXT NOT NULL DEFAULT '',
  locked_until       TIMESTAMPTZ,
  last_error         TEXT NOT NULL DEFAULT '',
  created_at         TIMESTAMPTZ NOT NULL DEFAULT now(),
  delivered_at       TIMESTAMPTZ,
  UNIQUE(work_order_id,aggregate_version)
);
CREATE INDEX IF NOT EXISTS idx_asset_governance_outbox_ready
  ON asset_governance_outbox(available_at,outbox_id) WHERE status='pending';

INSERT INTO alignment_schema_migrations(version,description)
VALUES ('202607311330','F-ASSET-006 authoritative asset governance work orders')
ON CONFLICT (version) DO NOTHING;

COMMIT;
