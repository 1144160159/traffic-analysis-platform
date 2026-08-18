-- Fresh-schema mirror of the M03 file-restoration authority. This file must
-- remain self-contained because ephemeral and Kubernetes runners pipe/copy it
-- without the deployments/postgres directory.
BEGIN;

CREATE TABLE IF NOT EXISTS file_restoration_manifests (
  tenant_id TEXT NOT NULL,
  restoration_id UUID NOT NULL,
  revision BIGINT NOT NULL DEFAULT 1 CHECK (revision > 0),
  idempotency_key TEXT NOT NULL CHECK (length(idempotency_key) BETWEEN 16 AND 200),
  request_sha256 TEXT NOT NULL CHECK (request_sha256 ~ '^[0-9a-f]{64}$'),
  session_id TEXT NOT NULL,
  community_id TEXT NOT NULL,
  session_authority JSONB NOT NULL CHECK (jsonb_typeof(session_authority) = 'object'),
  primary_flow_id TEXT NOT NULL CHECK (primary_flow_id<>''),
  flow_ids JSONB NOT NULL CHECK (jsonb_typeof(flow_ids) = 'array'),
  flow_selections JSONB NOT NULL CHECK (jsonb_typeof(flow_selections) = 'array' AND jsonb_array_length(flow_selections) BETWEEN 1 AND 2),
  pcap_index_ids JSONB NOT NULL CHECK (jsonb_typeof(pcap_index_ids) = 'array'),
  source_object_receipts JSONB NOT NULL CHECK (jsonb_typeof(source_object_receipts) = 'array'),
  five_tuple JSONB NOT NULL CHECK (jsonb_typeof(five_tuple) = 'object'),
  direction TEXT NOT NULL CHECK (direction IN ('client_to_server','server_to_client')),
  capture_time_start TIMESTAMPTZ NOT NULL,
  capture_time_end TIMESTAMPTZ NOT NULL CHECK (capture_time_end >= capture_time_start),
  packet_ranges JSONB NOT NULL CHECK (jsonb_typeof(packet_ranges) = 'array'),
  tcp_sequence_ranges JSONB NOT NULL CHECK (jsonb_typeof(tcp_sequence_ranges) = 'array'),
  protocol_profile_id TEXT NOT NULL,
  parser_name TEXT NOT NULL,
  parser_version TEXT NOT NULL,
  algorithm_version TEXT NOT NULL,
  status TEXT NOT NULL CHECK (status IN ('complete','partial','truncated','corrupt','oversize','unsupported')),
  status_reason TEXT NOT NULL,
  missing_ranges JSONB NOT NULL CHECK (jsonb_typeof(missing_ranges) = 'array'),
  truncation_offset BIGINT,
  wire_filename TEXT NOT NULL DEFAULT '',
  sanitized_filename TEXT NOT NULL DEFAULT '',
  declared_mime_type TEXT NOT NULL DEFAULT '',
  detected_mime_type TEXT NOT NULL DEFAULT '',
  declared_size BIGINT,
  visible_size BIGINT NOT NULL CHECK (visible_size >= 0),
  restored_size BIGINT NOT NULL CHECK (restored_size >= 0),
  wire_sha256 TEXT NOT NULL CHECK (wire_sha256 = '' OR wire_sha256 ~ '^[0-9a-f]{64}$'),
  content_sha256 TEXT NOT NULL CHECK (content_sha256 = '' OR content_sha256 ~ '^[0-9a-f]{64}$'),
  object_bucket TEXT NOT NULL DEFAULT '',
  object_key TEXT NOT NULL DEFAULT '',
  object_version TEXT NOT NULL DEFAULT '',
  object_etag TEXT NOT NULL DEFAULT '',
  object_size_bytes BIGINT NOT NULL DEFAULT 0 CHECK (object_size_bytes >= 0),
  object_sha256 TEXT NOT NULL DEFAULT '' CHECK (object_sha256 = '' OR object_sha256 ~ '^[0-9a-f]{64}$'),
  object_observed_at TIMESTAMPTZ,
  retention_until TIMESTAMPTZ,
  legal_hold BOOLEAN NOT NULL DEFAULT false,
  quarantined BOOLEAN NOT NULL DEFAULT true,
  executable BOOLEAN NOT NULL DEFAULT false CHECK (NOT executable),
  automatic_open BOOLEAN NOT NULL DEFAULT false CHECK (NOT automatic_open),
  automatic_decompress BOOLEAN NOT NULL DEFAULT false CHECK (NOT automatic_decompress),
  malware_scan_status TEXT NOT NULL DEFAULT 'not_scanned' CHECK (malware_scan_status IN ('not_scanned','pending','clean','malicious','error')),
  download_eligible BOOLEAN NOT NULL DEFAULT false,
  trace_id TEXT NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  completed_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  PRIMARY KEY (tenant_id,restoration_id),
  UNIQUE (tenant_id,idempotency_key),
  CHECK (
    session_authority ?& ARRAY['tenant_id','session_id','community_id','event_id','probe_id','flow_ids','ts_start','ts_end','event_schema_version','aggregate_version','identity_version','session_version','completeness','is_partial']
    AND jsonb_typeof(session_authority->'aggregate_version')='number'
    AND jsonb_typeof(session_authority->'session_version')='number'
    AND jsonb_typeof(session_authority->'is_partial')='boolean'
    AND session_authority->>'tenant_id'=tenant_id
    AND session_authority->>'session_id'=session_id
    AND session_authority->>'community_id'=community_id
    AND length(session_authority->>'event_id')>0
    AND length(session_authority->>'probe_id')>0
    AND session_authority->>'event_schema_version'='v1'
    AND (session_authority->>'aggregate_version')::bigint>0
    AND session_authority->>'identity_version'='session-id-sha256-v1'
    AND (session_authority->>'session_version')::bigint>0
    AND jsonb_typeof(session_authority->'flow_ids')='array'
    AND session_authority->'flow_ids' ? primary_flow_id
    AND flow_ids ? primary_flow_id
    AND flow_selections @> jsonb_build_array(jsonb_build_object('role','primary','community_id',community_id,'flow_id',primary_flow_id,'five_tuple',five_tuple,'direction',direction))
    AND (session_authority->>'ts_start')::timestamptz<=capture_time_start
    AND (session_authority->>'ts_end')::timestamptz>=capture_time_end
    AND (
      (session_authority->>'completeness'='SESSION_COMPLETENESS_COMPLETE' AND (session_authority->>'is_partial')::boolean=false)
      OR
      (session_authority->>'completeness' IN ('SESSION_COMPLETENESS_PARTIAL','SESSION_COMPLETENESS_TRUNCATED') AND (session_authority->>'is_partial')::boolean=true)
    )
  ),
  CHECK (object_key = '' OR (object_key LIKE ('tenants/'||tenant_id||'/restorations/'||restoration_id::text||'/%') AND object_key NOT LIKE '%..%')),
  CHECK (status<>'complete' OR (object_key<>'' AND object_version<>'' AND object_sha256<>'' AND restored_size=object_size_bytes AND content_sha256=object_sha256)),
  CHECK (object_key='' OR (object_version<>'' AND object_etag<>'' AND object_sha256<>'' AND object_observed_at IS NOT NULL AND restored_size=object_size_bytes AND content_sha256=object_sha256)),
  CHECK (status<>'unsupported' OR object_key=''),
  CHECK (quarantined OR malware_scan_status='clean'),
  CHECK (download_eligible = (status='complete' AND malware_scan_status='clean' AND NOT quarantined))
);

CREATE TABLE IF NOT EXISTS file_restoration_outbox (
  outbox_id BIGSERIAL PRIMARY KEY,
  event_id UUID NOT NULL UNIQUE,
  tenant_id TEXT NOT NULL,
  restoration_id UUID NOT NULL,
  aggregate_version BIGINT NOT NULL CHECK (aggregate_version > 0),
  event_type TEXT NOT NULL CHECK (event_type='traffic.forensics.file-restoration.v1.committed'),
  schema_version INTEGER NOT NULL DEFAULT 1 CHECK (schema_version=1),
  partition_key TEXT NOT NULL CHECK (partition_key<>''),
  payload JSONB NOT NULL,
  trace_id TEXT NOT NULL,
  status TEXT NOT NULL DEFAULT 'pending' CHECK (status IN ('pending','processing','published','dead')),
  attempt_count INTEGER NOT NULL DEFAULT 0 CHECK (attempt_count>=0),
  available_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  locked_until TIMESTAMPTZ,
  locked_by TEXT NOT NULL DEFAULT '',
  last_error TEXT NOT NULL DEFAULT '',
  occurred_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  published_at TIMESTAMPTZ,
  FOREIGN KEY (tenant_id,restoration_id) REFERENCES file_restoration_manifests(tenant_id,restoration_id) ON DELETE RESTRICT
);
CREATE INDEX IF NOT EXISTS idx_file_restoration_outbox_ready ON file_restoration_outbox(available_at,occurred_at,outbox_id) WHERE status='pending';

CREATE TABLE IF NOT EXISTS file_restoration_requests (
  tenant_id TEXT NOT NULL,
  idempotency_key TEXT NOT NULL CHECK (length(idempotency_key) BETWEEN 16 AND 200),
  request_sha256 TEXT NOT NULL CHECK (request_sha256 ~ '^[0-9a-f]{64}$'),
  state TEXT NOT NULL CHECK (state IN ('processing','committed')),
  trace_id TEXT NOT NULL,
  claim_token UUID NOT NULL,
  lease_until TIMESTAMPTZ NOT NULL,
  restoration_id UUID,
  resulting_revision BIGINT CHECK (resulting_revision>0),
  event_id UUID REFERENCES file_restoration_outbox(event_id) ON DELETE RESTRICT,
  response_payload JSONB NOT NULL DEFAULT '{}'::jsonb,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  PRIMARY KEY (tenant_id,idempotency_key),
  UNIQUE (event_id),
  FOREIGN KEY (tenant_id,restoration_id) REFERENCES file_restoration_manifests(tenant_id,restoration_id) ON DELETE RESTRICT,
  CHECK (
    (state='processing' AND restoration_id IS NULL AND resulting_revision IS NULL AND event_id IS NULL AND response_payload='{}'::jsonb)
    OR
    (state='committed' AND restoration_id IS NOT NULL AND resulting_revision IS NOT NULL AND event_id IS NOT NULL AND response_payload<>'{}'::jsonb)
  )
);
CREATE INDEX IF NOT EXISTS idx_file_restoration_requests_lease ON file_restoration_requests(lease_until) WHERE state='processing';

CREATE TABLE IF NOT EXISTS file_restoration_audit (
  audit_id BIGSERIAL PRIMARY KEY,
  event_id UUID NOT NULL UNIQUE,
  tenant_id TEXT NOT NULL,
  restoration_id UUID NOT NULL,
  action TEXT NOT NULL CHECK (action='forensics.file-restoration.commit'),
  actor_id TEXT NOT NULL,
  reason TEXT NOT NULL,
  trace_id TEXT NOT NULL,
  object_sha256 TEXT NOT NULL DEFAULT '' CHECK (object_sha256='' OR object_sha256 ~ '^[0-9a-f]{64}$'),
  status TEXT NOT NULL CHECK (status IN ('complete','partial','truncated','corrupt','oversize','unsupported')),
  occurred_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  FOREIGN KEY (tenant_id,restoration_id) REFERENCES file_restoration_manifests(tenant_id,restoration_id) ON DELETE RESTRICT
);

CREATE TABLE IF NOT EXISTS file_restoration_orphans (
  tenant_id TEXT NOT NULL,
  restoration_id UUID NOT NULL,
  object_bucket TEXT NOT NULL,
  object_key TEXT NOT NULL,
  object_version TEXT NOT NULL,
  object_etag TEXT NOT NULL,
  object_size_bytes BIGINT NOT NULL CHECK (object_size_bytes>=0),
  object_sha256 TEXT NOT NULL CHECK (object_sha256 ~ '^[0-9a-f]{64}$'),
  observed_at TIMESTAMPTZ NOT NULL,
  retention_until TIMESTAMPTZ,
  reconciliation_status TEXT NOT NULL DEFAULT 'candidate' CHECK (reconciliation_status IN ('candidate','reconciled','quarantined_conflict')),
  reconciliation_attempts INTEGER NOT NULL DEFAULT 0 CHECK (reconciliation_attempts>=0),
  next_reconcile_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  reconciled_at TIMESTAMPTZ,
  PRIMARY KEY (tenant_id,object_bucket,object_key,object_version),
  CHECK (object_key LIKE ('tenants/'||tenant_id||'/restorations/'||restoration_id::text||'/%') AND object_key NOT LIKE '%..%')
);
CREATE INDEX IF NOT EXISTS idx_file_restoration_orphans_ready
  ON file_restoration_orphans(next_reconcile_at,observed_at) WHERE reconciliation_status='candidate';

CREATE TABLE IF NOT EXISTS alignment_schema_migrations (
  version TEXT PRIMARY KEY, description TEXT NOT NULL,
  applied_at TIMESTAMPTZ NOT NULL DEFAULT now(), applied_by TEXT NOT NULL DEFAULT current_user
);
INSERT INTO alignment_schema_migrations(version,description)
VALUES ('202608141300','M03 file restoration immutable manifests object receipts outbox idempotency and orphan reconciliation')
ON CONFLICT (version) DO NOTHING;

COMMIT;
