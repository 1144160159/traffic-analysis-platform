-- M02 manifest-v2 PCAP projection authority.
--
-- Expand only: the legacy pcap_index tables remain unchanged for PCAP cutter
-- rollback. The carrier writer and M03 restoration reader use this isolated
-- exact schema. Applying this DDL does not enable either writer.

CREATE TABLE IF NOT EXISTS traffic.pcap_index_v2_local (
  tenant_id            String,
  probe_id             String,
  file_key              String,
  bucket                String,
  object_version        String,
  etag                  String,
  original_size         UInt64,
  stored_size           UInt64,
  compression           LowCardinality(String),
  manifest_version      UInt16,
  kafka_topic           String,
  kafka_partition       Int32,
  kafka_offset          Int64,
  kafka_key_sha256      FixedString(64),
  kafka_headers_sha256  FixedString(64),
  raw_sha256            FixedString(64),
  projection_identity   FixedString(64),
  ts_start              DateTime64(3, 'UTC'),
  ts_end                DateTime64(3, 'UTC'),
  byte_size             UInt64,
  zstd_level            UInt8,
  sha256                String,
  community_id          String,
  flow_id               String,
  offset_start          Nullable(UInt64),
  offset_end            Nullable(UInt64),
  bloom_filter_b64      String,
  community_ids         Array(String),
  created_ts            DateTime64(3, 'UTC')
)
ENGINE = ReplicatedReplacingMergeTree(
  '/clickhouse/tables/{shard}/pcap_index_v2_local',
  '{replica}',
  created_ts
)
PARTITION BY toYYYYMMDD(ts_start)
ORDER BY (tenant_id, probe_id, file_key, projection_identity)
TTL toDateTime(ts_start) + INTERVAL 90 DAY
SETTINGS index_granularity = 8192;

CREATE TABLE IF NOT EXISTS traffic.pcap_index_v2
AS traffic.pcap_index_v2_local
ENGINE = Distributed(
  traffic_cluster,
  traffic,
  pcap_index_v2_local,
  cityHash64(tenant_id, file_key)
);
