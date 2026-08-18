-- M07 immutable attack-chain and GNN graph snapshots.
BEGIN;

CREATE TABLE IF NOT EXISTS gnn_graph_snapshots_v1 (
  graph_snapshot_id       TEXT PRIMARY KEY,
  tenant_id               TEXT NOT NULL REFERENCES tenants(tenant_id) ON DELETE RESTRICT,
  chain_id                TEXT NOT NULL,
  attack_chain_version    BIGINT NOT NULL CHECK (attack_chain_version>0),
  schema_version          TEXT NOT NULL CHECK (schema_version='gnn-graph/v1'),
  as_of                   TIMESTAMPTZ NOT NULL,
  node_count              INTEGER NOT NULL CHECK (node_count>=0),
  edge_count              INTEGER NOT NULL CHECK (edge_count>=0),
  node_sha256             TEXT NOT NULL CHECK (node_sha256 ~ '^[0-9a-f]{64}$'),
  edge_sha256             TEXT NOT NULL CHECK (edge_sha256 ~ '^[0-9a-f]{64}$'),
  graph_snapshot_sha256   TEXT NOT NULL CHECK (graph_snapshot_sha256 ~ '^[0-9a-f]{64}$'),
  nodes_json              JSONB NOT NULL,
  edge_ids_json           JSONB NOT NULL,
  label_refs_json         JSONB NOT NULL,
  evidence_refs_json      JSONB NOT NULL,
  source_watermarks_json  JSONB NOT NULL,
  created_at              TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE (tenant_id,chain_id,attack_chain_version),
  UNIQUE (tenant_id,graph_snapshot_sha256),
  CHECK (jsonb_typeof(nodes_json)='array' AND jsonb_array_length(nodes_json)=node_count),
  CHECK (jsonb_typeof(edge_ids_json)='array' AND jsonb_array_length(edge_ids_json)=edge_count),
  CHECK (jsonb_typeof(label_refs_json)='object'),
  CHECK (jsonb_typeof(evidence_refs_json)='array'),
  CHECK (jsonb_typeof(source_watermarks_json)='object')
);

CREATE TABLE IF NOT EXISTS attack_chain_snapshots_v1 (
  snapshot_id              TEXT PRIMARY KEY,
  tenant_id                TEXT NOT NULL REFERENCES tenants(tenant_id) ON DELETE RESTRICT,
  chain_id                 TEXT NOT NULL,
  snapshot_version         BIGINT NOT NULL CHECK (snapshot_version>0),
  as_of                    TIMESTAMPTZ NOT NULL,
  source_vertex_id         TEXT NOT NULL CHECK (source_vertex_id ~ '^[0-9a-f]{32}$'),
  target_vertex_id         TEXT NOT NULL CHECK (target_vertex_id ~ '^[0-9a-f]{32}$'),
  candidate_path_sha256    TEXT NOT NULL CHECK (candidate_path_sha256 ~ '^[0-9a-f]{64}$'),
  alternative_path_count  INTEGER NOT NULL CHECK (alternative_path_count>=0),
  graph_snapshot_id        TEXT NOT NULL REFERENCES gnn_graph_snapshots_v1(graph_snapshot_id) ON DELETE RESTRICT,
  graph_snapshot_sha256    TEXT NOT NULL CHECK (graph_snapshot_sha256 ~ '^[0-9a-f]{64}$'),
  snapshot_sha256          TEXT NOT NULL CHECK (snapshot_sha256 ~ '^[0-9a-f]{64}$'),
  partial                  BOOLEAN NOT NULL,
  partial_reasons          TEXT[] NOT NULL DEFAULT '{}',
  truncated                BOOLEAN NOT NULL,
  truncation_reason        TEXT NOT NULL DEFAULT '',
  continuation_boundary    TEXT NOT NULL DEFAULT '',
  state                    TEXT NOT NULL CHECK (state IN ('active','withdrawn')),
  payload_json             JSONB NOT NULL,
  assembled_at             TIMESTAMPTZ NOT NULL DEFAULT now(),
  withdrawn_at             TIMESTAMPTZ,
  withdrawn_by             TEXT,
  withdrawal_reason        TEXT,
  UNIQUE (tenant_id,chain_id,snapshot_version),
  UNIQUE (tenant_id,snapshot_sha256),
  CHECK (jsonb_typeof(payload_json)='object'),
  CHECK ((truncated AND truncation_reason<>'' AND continuation_boundary<>'')
      OR (NOT truncated AND truncation_reason='' AND continuation_boundary='')),
  CHECK ((state='withdrawn' AND withdrawn_at IS NOT NULL AND withdrawn_by IS NOT NULL AND withdrawal_reason IS NOT NULL)
      OR (state='active' AND withdrawn_at IS NULL AND withdrawn_by IS NULL AND withdrawal_reason IS NULL))
);

CREATE INDEX IF NOT EXISTS idx_attack_chain_snapshots_v1_tenant_as_of
  ON attack_chain_snapshots_v1(tenant_id,as_of DESC,snapshot_id)
  WHERE state='active';

CREATE TABLE IF NOT EXISTS attack_chain_snapshot_current_v1 (
  tenant_id               TEXT NOT NULL REFERENCES tenants(tenant_id) ON DELETE RESTRICT,
  chain_id                TEXT NOT NULL,
  snapshot_id             TEXT NOT NULL UNIQUE REFERENCES attack_chain_snapshots_v1(snapshot_id) ON DELETE RESTRICT,
  snapshot_version        BIGINT NOT NULL CHECK (snapshot_version>0),
  snapshot_sha256         TEXT NOT NULL CHECK (snapshot_sha256 ~ '^[0-9a-f]{64}$'),
  updated_at              TIMESTAMPTZ NOT NULL DEFAULT now(),
  PRIMARY KEY (tenant_id,chain_id)
);

CREATE TABLE IF NOT EXISTS attack_chain_evidence_manifest_v1 (
  snapshot_id             TEXT NOT NULL REFERENCES attack_chain_snapshots_v1(snapshot_id) ON DELETE RESTRICT,
  tenant_id               TEXT NOT NULL REFERENCES tenants(tenant_id) ON DELETE RESTRICT,
  evidence_id             TEXT NOT NULL,
  evidence_kind           TEXT NOT NULL CHECK (evidence_kind IN ('event','rule','model','analyst_conclusion')),
  immutable_uri           TEXT NOT NULL,
  evidence_sha256         TEXT NOT NULL CHECK (evidence_sha256 ~ '^[0-9a-f]{64}$'),
  source_event_id         TEXT NOT NULL,
  occurred_at_ms          BIGINT NOT NULL CHECK (occurred_at_ms>0),
  available               BOOLEAN NOT NULL,
  created_at              TIMESTAMPTZ NOT NULL DEFAULT now(),
  PRIMARY KEY (snapshot_id,evidence_id)
);

INSERT INTO alignment_schema_migrations(version,description)
VALUES ('202608142100','M07 immutable attack-chain and GNN graph snapshots')
ON CONFLICT(version) DO NOTHING;

COMMIT;
