-- F-ASSET-002 / T-PG-002: close legacy asset mutation boundaries.
-- Expand: add the durable batch request ledger used by inactive sweeps.
-- Cutover: deploy the service paths that route gRPC, passive bindings and V1
-- discovery through UpsertAtomic, then enable the scheduler implementation.
-- Reconcile: every changed asset revision has one history and one outbox row;
-- asset_inactive_requests.affected_count equals result_payload.count.
-- Rollback: stop the new writers. Keep request/history/outbox rows for replay.

BEGIN;

CREATE TABLE IF NOT EXISTS asset_inactive_requests (
  request_id       UUID PRIMARY KEY,
  tenant_id        TEXT NOT NULL,
  idempotency_key  TEXT NOT NULL CHECK (length(idempotency_key) BETWEEN 16 AND 200),
  request_hash     TEXT NOT NULL CHECK (length(request_hash)=64),
  actor            TEXT NOT NULL CHECK (actor<>''),
  action_id        TEXT NOT NULL CHECK (action_id='asset-inactive-sweep'),
  reason           TEXT NOT NULL CHECK (reason<>''),
  cutoff           TIMESTAMPTZ NOT NULL,
  affected_count   INTEGER NOT NULL CHECK (affected_count>=0),
  result_payload   JSONB NOT NULL,
  trace_id         TEXT NOT NULL CHECK (trace_id<>''),
  created_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE (tenant_id,idempotency_key)
);
CREATE INDEX IF NOT EXISTS idx_asset_inactive_requests_cutoff
  ON asset_inactive_requests(tenant_id,cutoff DESC);

INSERT INTO alignment_schema_migrations(version,description)
VALUES ('202608031510','F-ASSET-002 close legacy asset mutation boundaries')
ON CONFLICT (version) DO NOTHING;

COMMIT;
