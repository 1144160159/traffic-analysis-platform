-- T1-M02-P241-EXP-n011-l02
-- Expand-only PCAP manifest and Kafka source-receipt columns.
--
-- The migration runner applies this file directly to every ClickHouse node.
-- Existing PCAP rows remain readable with zero/empty compatibility defaults.
-- New writers MUST validate the live engine, shard key and the ordered column
-- contract before activating; applying this migration alone is not cutover.
--
-- Verify:
--   SELECT database,table,name,type,default_expression
--   FROM system.columns
--   WHERE database='traffic' AND table IN ('pcap_index_local','pcap_index')
--   ORDER BY table,position;
-- Rollback: stop the carrier writer and retain these columns and all accepted
-- rows. Do not DROP columns or reset Kafka offsets to hide projection facts.

ALTER TABLE traffic.pcap_index_local
  ADD COLUMN IF NOT EXISTS bucket String DEFAULT '' AFTER file_key,
  ADD COLUMN IF NOT EXISTS object_version String DEFAULT '' AFTER bucket,
  ADD COLUMN IF NOT EXISTS etag String DEFAULT '' AFTER object_version,
  ADD COLUMN IF NOT EXISTS original_size UInt64 DEFAULT 0 AFTER etag,
  ADD COLUMN IF NOT EXISTS stored_size UInt64 DEFAULT 0 AFTER original_size,
  ADD COLUMN IF NOT EXISTS compression LowCardinality(String) DEFAULT '' AFTER stored_size,
  ADD COLUMN IF NOT EXISTS manifest_version UInt16 DEFAULT 0 AFTER compression,
  ADD COLUMN IF NOT EXISTS kafka_topic String DEFAULT '' AFTER created_ts,
  ADD COLUMN IF NOT EXISTS kafka_partition Int32 DEFAULT -1 AFTER kafka_topic,
  ADD COLUMN IF NOT EXISTS kafka_offset Int64 DEFAULT -1 AFTER kafka_partition,
  ADD COLUMN IF NOT EXISTS kafka_key_sha256 FixedString(64) DEFAULT repeat('0', 64) AFTER kafka_offset,
  ADD COLUMN IF NOT EXISTS kafka_headers_sha256 FixedString(64) DEFAULT repeat('0', 64) AFTER kafka_key_sha256,
  ADD COLUMN IF NOT EXISTS raw_sha256 FixedString(64) DEFAULT repeat('0', 64) AFTER kafka_headers_sha256,
  ADD COLUMN IF NOT EXISTS projection_identity FixedString(64) DEFAULT repeat('0', 64) AFTER raw_sha256;

ALTER TABLE traffic.pcap_index
  ADD COLUMN IF NOT EXISTS bucket String DEFAULT '' AFTER file_key,
  ADD COLUMN IF NOT EXISTS object_version String DEFAULT '' AFTER bucket,
  ADD COLUMN IF NOT EXISTS etag String DEFAULT '' AFTER object_version,
  ADD COLUMN IF NOT EXISTS original_size UInt64 DEFAULT 0 AFTER etag,
  ADD COLUMN IF NOT EXISTS stored_size UInt64 DEFAULT 0 AFTER original_size,
  ADD COLUMN IF NOT EXISTS compression LowCardinality(String) DEFAULT '' AFTER stored_size,
  ADD COLUMN IF NOT EXISTS manifest_version UInt16 DEFAULT 0 AFTER compression,
  ADD COLUMN IF NOT EXISTS kafka_topic String DEFAULT '' AFTER manifest_version,
  ADD COLUMN IF NOT EXISTS kafka_partition Int32 DEFAULT -1 AFTER kafka_topic,
  ADD COLUMN IF NOT EXISTS kafka_offset Int64 DEFAULT -1 AFTER kafka_partition,
  ADD COLUMN IF NOT EXISTS kafka_key_sha256 FixedString(64) DEFAULT repeat('0', 64) AFTER kafka_offset,
  ADD COLUMN IF NOT EXISTS kafka_headers_sha256 FixedString(64) DEFAULT repeat('0', 64) AFTER kafka_key_sha256,
  ADD COLUMN IF NOT EXISTS raw_sha256 FixedString(64) DEFAULT repeat('0', 64) AFTER kafka_headers_sha256,
  ADD COLUMN IF NOT EXISTS projection_identity FixedString(64) DEFAULT repeat('0', 64) AFTER raw_sha256;
