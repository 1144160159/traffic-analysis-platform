-- =============================================================================
-- 扩展 ClickHouse 表 — device_logs, user_events, dlq_events (集群模式)
-- 对应 Proto: DeviceLog (10 fields), UserEvent (11 fields), DeadLetter (8 fields)
-- =============================================================================

-- 7. device_logs — DeviceLog
-- NOTE: Use DateTime (not DateTime64) for TTL compatibility in ClickHouse 24.3
CREATE TABLE IF NOT EXISTS traffic.device_logs_local ON CLUSTER traffic_cluster (
    log_id      String,
    tenant_id   String,
    device_ip   String,
    device_type String,
    facility    UInt8,
    severity    UInt8,
    timestamp   DateTime,
    message     String,
    parsed      String,
    source      String
)
ENGINE = ReplicatedMergeTree('/clickhouse/tables/{shard}/device_logs', '{replica}')
PARTITION BY toDate(timestamp)
ORDER BY (tenant_id, timestamp, device_ip, severity)
TTL timestamp + INTERVAL 30 DAY;

CREATE TABLE IF NOT EXISTS traffic.device_logs ON CLUSTER traffic_cluster
AS traffic.device_logs_local
ENGINE = Distributed(traffic_cluster, traffic, device_logs_local, rand());

-- 8. user_events — UserEvent
-- NOTE: Use DateTime (not DateTime64) for TTL compatibility in ClickHouse 24.3
CREATE TABLE IF NOT EXISTS traffic.user_events_local ON CLUSTER traffic_cluster (
    event_id    String,
    tenant_id   String,
    user_id     String,
    username    String,
    event_type  String,
    source_ip   String,
    user_agent  String,
    resource    String,
    action      String,
    result      String,
    timestamp   DateTime
)
ENGINE = ReplicatedMergeTree('/clickhouse/tables/{shard}/user_events', '{replica}')
PARTITION BY toDate(timestamp)
ORDER BY (tenant_id, timestamp, user_id, event_type)
TTL timestamp + INTERVAL 180 DAY;

CREATE TABLE IF NOT EXISTS traffic.user_events ON CLUSTER traffic_cluster
AS traffic.user_events_local
ENGINE = Distributed(traffic_cluster, traffic, user_events_local, rand());

-- 9. dlq_events — DeadLetter
-- NOTE: Use DateTime (not DateTime64) for TTL compatibility in ClickHouse 24.3
CREATE TABLE IF NOT EXISTS traffic.dlq_events_local ON CLUSTER traffic_cluster (
    event_id     String,
    tenant_id    String,
    source_topic String,
    source_key   String,
    error_msg    String,
    raw_payload  String,
    retry_count  UInt32 DEFAULT 0,
    created_at   DateTime DEFAULT now()
)
ENGINE = ReplicatedMergeTree('/clickhouse/tables/{shard}/dlq_events', '{replica}')
PARTITION BY toDate(created_at)
ORDER BY (tenant_id, created_at, source_topic)
TTL created_at + INTERVAL 7 DAY;

CREATE TABLE IF NOT EXISTS traffic.dlq_events ON CLUSTER traffic_cluster
AS traffic.dlq_events_local
ENGINE = Distributed(traffic_cluster, traffic, dlq_events_local, rand());

-- 10. graph_query_log — Graph Service 查询日志
CREATE TABLE IF NOT EXISTS traffic.graph_query_log_local ON CLUSTER traffic_cluster (
    tenant_id            String,
    query_id             String,
    user_id              String,
    query_type           LowCardinality(String),
    center_ip            String,
    center_ips           Array(String),
    depth                UInt8,
    run_id               String,
    query_start_time     DateTime64(3),
    query_end_time       DateTime64(3),
    node_count           UInt32,
    edge_count           UInt32,
    path_count           UInt32,
    result_size_bytes    UInt64,
    duration_ms          UInt32,
    cache_hit            UInt8,
    ch_query_count       UInt16,
    ch_total_duration_ms UInt32,
    ch_rows_read         UInt64,
    ch_bytes_read        UInt64,
    status               LowCardinality(String),
    error_code           String,
    error_message        String,
    trace_id             String,
    client_ip            String,
    user_agent           String,
    created_at           DateTime64(3) DEFAULT now64(3)
)
ENGINE = ReplicatedMergeTree('/clickhouse/tables/{shard}/graph_query_log', '{replica}')
PARTITION BY toDate(created_at)
ORDER BY (tenant_id, created_at, query_type, status)
TTL toDateTime(created_at) + INTERVAL 7 DAY;

CREATE TABLE IF NOT EXISTS traffic.graph_query_log ON CLUSTER traffic_cluster
AS traffic.graph_query_log_local
ENGINE = Distributed(traffic_cluster, traffic, graph_query_log_local, cityHash64(tenant_id, query_id));
